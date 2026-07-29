package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"

	"github.com/chaitanyabankanhal/ai-gateway/internal/guardrails"
	llmrouter "github.com/chaitanyabankanhal/ai-gateway/internal/router"
)

const maxBodyBytes = 10 << 20 // 10MB

// errorResponse is the OpenAI-compatible error envelope.
type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg, errType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := errorResponse{}
	resp.Error.Message = msg
	resp.Error.Type = errType
	json.NewEncoder(w).Encode(resp)
}

// streamChunk is the minimal shape of an OpenAI SSE chunk used for usage extraction.
type streamChunk struct {
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage,omitempty"`
}

// chatResponse is the minimal shape of a non-streaming OpenAI response.
type chatResponse struct {
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type listModelsResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

type anthropicModel struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

type anthropicListModelsResponse struct {
	Data    []anthropicModel `json:"data"`
	HasMore bool             `json:"has_more"`
}

// modelPrice holds per-million-token USD pricing.
type modelPrice struct {
	inputPerMTok  float64
	outputPerMTok float64
}

var pricingTable = map[string]modelPrice{
	"gpt-4o":                    {2.50, 10.00},
	"gpt-4o-mini":               {0.15, 0.60},
	"gpt-4-turbo":               {10.00, 30.00},
	"gpt-3.5-turbo":             {0.50, 1.50},
	"claude-opus-4-7":           {15.00, 75.00},
	"claude-sonnet-4-6":         {3.00, 15.00},
	"claude-haiku-4-5-20251001": {0.80, 4.00},
}

func calcCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := pricingTable[model]
	if !ok {
		return 0
	}
	return (float64(inputTokens)*p.inputPerMTok + float64(outputTokens)*p.outputPerMTok) / 1_000_000
}

// flattenMessages extracts all text content from the raw JSON messages array
// into a single string for guardrail evaluation.
func flattenMessages(raw json.RawMessage) string {
	var msgs []struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, m := range msgs {
		// Content may be a plain string or an array of content blocks.
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			sb.WriteString(s)
			sb.WriteByte('\n')
			continue
		}
		var blocks []struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(m.Content, &blocks); err == nil {
			for _, b := range blocks {
				sb.WriteString(b.Text)
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}

// extractTextFromResponse pulls assistant message text from a non-streaming
// OpenAI or Anthropic response body for output guardrail evaluation.
func extractTextFromResponse(provider string, body []byte) string {
	switch provider {
	case "anthropic":
		var resp struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &resp); err == nil {
			var sb strings.Builder
			for _, c := range resp.Content {
				sb.WriteString(c.Text)
				sb.WriteByte('\n')
			}
			return sb.String()
		}
	case "gemini":
		var resp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(body, &resp); err == nil {
			var sb strings.Builder
			for _, cand := range resp.Candidates {
				for _, part := range cand.Content.Parts {
					sb.WriteString(part.Text)
					sb.WriteByte('\n')
				}
			}
			return sb.String()
		}
	default: // openai-compatible
		var resp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &resp); err == nil {
			var sb strings.Builder
			for _, c := range resp.Choices {
				sb.WriteString(c.Message.Content)
				sb.WriteByte('\n')
			}
			return sb.String()
		}
	}
	return ""
}

// extractToolMessages concatenates content from tool/function role messages for
// data poisoning detection — RAG context and tool outputs arrive via these roles.
func extractToolMessages(raw json.RawMessage) string {
	var msgs []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, m := range msgs {
		if m.Role != "tool" && m.Role != "function" {
			continue
		}
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			sb.WriteString(s)
			sb.WriteByte('\n')
			continue
		}
		var blocks []struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(m.Content, &blocks); err == nil {
			for _, b := range blocks {
				sb.WriteString(b.Text)
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}

// emitGuardrailEvents records fired guardrail rules as events on the given span.
func emitGuardrailEvents(span trace.Span, fired []guardrails.FireEvent) {
	for _, ev := range fired {
		span.AddEvent("guardrail.fired", trace.WithAttributes(
			attribute.Int("guardrail.rule_id", int(ev.RuleID)),
			attribute.String("guardrail.rule_name", ev.RuleName),
			attribute.String("guardrail.action", ev.Action),
			attribute.String("guardrail.reason", ev.Reason),
		))
	}
}

// emitGuardrailLogs sends one OTel log record per fired rule.
// These flow through otelcol into ClickHouse for audit and analytics queries.
func emitGuardrailLogs(ctx context.Context, fired []guardrails.FireEvent, tenantID, agentID, scope string) {
	logger := global.Logger("guardrails")
	for _, ev := range fired {
		var rec log.Record
		rec.SetTimestamp(time.Now())
		rec.SetSeverity(log.SeverityWarn)
		rec.SetBody(log.StringValue("guardrail.fired"))
		rec.AddAttributes(
			log.Int("guardrail.rule_id", int(ev.RuleID)),
			log.String("guardrail.rule_name", ev.RuleName),
			log.String("guardrail.action", ev.Action),
			log.String("guardrail.reason", ev.Reason),
			log.String("scope", scope),
			log.String("tenant_id", tenantID),
			log.String("agent_id", agentID),
		)
		logger.Emit(ctx, rec)
	}
}

// handleChatCompletions handles POST /v1/chat/completions.
// Span hierarchy per request (all nested under the agentTraceMiddleware's agent→thread spans):
//
//	guardrail.execution (scope=input)
//	llm.generation
//	  └── upstream.call
//	guardrail.execution (scope=output, non-streaming only)
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	baseCtx := r.Context()

	// 1. Read and parse raw body.
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error")
		return
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &fields); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json", "invalid_request_error")
		return
	}

	var model string
	if raw, ok := fields["model"]; ok {
		if err := json.Unmarshal(raw, &model); err != nil || model == "" {
			writeError(w, http.StatusBadRequest, "model is required", "invalid_request_error")
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "model is required", "invalid_request_error")
		return
	}

	if _, ok := fields["messages"]; !ok {
		writeError(w, http.StatusBadRequest, "messages is required", "invalid_request_error")
		return
	}

	var streaming bool
	if raw, ok := fields["stream"]; ok {
		json.Unmarshal(raw, &streaming) //nolint:errcheck
	}

	tc := TenantCtx(baseCtx)
	inputContent := flattenMessages(fields["messages"])

	// 2. Input guardrail span.
	if s.guardrails != nil {
		grCtx, grSpan := tracer.Start(baseCtx, "guardrail.execution",
			trace.WithAttributes(attribute.String("guardrail.scope", guardrails.ScopeInput)))
		decision, evalErr := s.guardrails.Evaluate(grCtx, guardrails.EvalContext{
			TenantID: tc.TenantID,
			AgentID:  tc.AgentID,
			Scope:    guardrails.ScopeInput,
			Content:  inputContent,
		})
		if evalErr == nil {
			emitGuardrailEvents(grSpan, decision.FiredRules)
			emitGuardrailLogs(grCtx, decision.FiredRules, tc.TenantID, tc.AgentID, guardrails.ScopeInput)
		}
		grSpan.End()
		if evalErr == nil && !decision.Allowed {
			writeError(w, http.StatusForbidden,
				"request blocked by guardrail: "+decision.FiredRules[0].RuleName,
				"policy_error")
			return
		}
	}

	// 3a. Semantic injection check on full input (secllm).
	if s.secllm != nil && inputContent != "" {
		secCtx, secSpan := tracer.Start(baseCtx, "secllm.classify",
			trace.WithAttributes(attribute.String("secllm.scope", "input")))
		secStart := time.Now()
		injection, score, secErr := s.secllm.IsInjection(secCtx, inputContent)
		attrs := []attribute.KeyValue{
			attribute.Float64("secllm.score", score),
			attribute.Int("secllm.latency_ms", int(time.Since(secStart).Milliseconds())),
			attribute.Bool("secllm.error", secErr != nil),
		}
		if secErr != nil {
			attrs = append(attrs, attribute.String("secllm.error_msg", secErr.Error()))
		}
		secSpan.SetAttributes(attrs...)
		secSpan.End()
		if secErr == nil && injection {
			writeError(w, http.StatusForbidden, "request blocked by semantic threat analysis", "policy_error")
			return
		}
	}

	// 3b. Data poisoning check on tool/function message content.
	if s.secllm != nil {
		if toolContent := extractToolMessages(fields["messages"]); toolContent != "" {
			secCtx, secSpan := tracer.Start(baseCtx, "secllm.classify",
				trace.WithAttributes(attribute.String("secllm.scope", "tool")))
			secStart := time.Now()
			injection, score, secErr := s.secllm.IsInjection(secCtx, toolContent)
			attrs := []attribute.KeyValue{
				attribute.Float64("secllm.score", score),
				attribute.Int("secllm.latency_ms", int(time.Since(secStart).Milliseconds())),
				attribute.Bool("secllm.error", secErr != nil),
			}
			if secErr != nil {
				attrs = append(attrs, attribute.String("secllm.error_msg", secErr.Error()))
			}
			secSpan.SetAttributes(attrs...)
			secSpan.End()
			if secErr == nil && injection {
				writeError(w, http.StatusForbidden, "request blocked: data poisoning detected in tool output", "policy_error")
				return
			}
		}
	}

	// 4. Pick upstream endpoint.
	var tenantID uint
	fmt.Sscanf(tc.TenantID, "%d", &tenantID)

	endpoint, err := s.router.Pick(tenantID, model)
	if err != nil {
		if errors.Is(err, llmrouter.ErrNoEndpoint) {
			writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("no available upstream for model %q", model), "api_error")
		} else {
			writeError(w, http.StatusInternalServerError, "router error", "api_error")
		}
		return
	}

	forwardBody := rawBody
	if streaming {
		fields["stream_options"] = json.RawMessage(`{"include_usage":true}`)
		if forwardBody, err = json.Marshal(fields); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to marshal request", "api_error")
			return
		}
	}

	// 4. llm.generation span wraps the full upstream round-trip.
	llmCtx, llmSpan := tracer.Start(baseCtx, "llm.generation",
		trace.WithAttributes(
			attribute.String("gen_ai.model", model),
			attribute.String("gen_ai.provider", endpoint.Provider()),
			attribute.String("gen_ai.prompt", truncate(inputContent)),
			attribute.String("agent.id", tc.AgentID),
			attribute.String("tenant.id", tc.TenantID),
			attribute.String("thread.id", tc.ThreadID),
		))

	// 5. upstream.call sub-span — just the HTTP dial + response headers.
	upCtx, upSpan := tracer.Start(llmCtx, "upstream.call",
		trace.WithAttributes(attribute.String("gen_ai.provider", endpoint.Provider())))

	upstreamReq, err := s.buildUpstreamRequest(upCtx, endpoint, model, forwardBody)
	if err != nil {
		upSpan.End()
		llmSpan.End()
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error")
		return
	}

	start := time.Now()
	resp, err := s.httpClient.Do(upstreamReq)
	if err != nil {
		endpoint.RecordError()
		upSpan.RecordError(err)
		upSpan.End()
		llmSpan.End()
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error")
		return
	}
	upSpan.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	upSpan.End()
	defer resp.Body.Close()

	var (
		ttftMS       float64
		inputTokens  int
		outputTokens int
	)

	if streaming {
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		ttftMS, inputTokens, outputTokens = s.proxySSE(w, endpoint.Provider(), resp.Body, start)
	} else {
		body, _ := io.ReadAll(resp.Body)
		ttftMS = float64(time.Since(start).Milliseconds())
		inputTokens, outputTokens = s.extractUsage(endpoint.Provider(), body)
		outputContent := extractTextFromResponse(endpoint.Provider(), body)

		// 6. Output guardrail span — blocks before writing to client.
		if s.guardrails != nil && resp.StatusCode < 400 {
			grCtx, grSpan := tracer.Start(baseCtx, "guardrail.execution",
				trace.WithAttributes(attribute.String("guardrail.scope", guardrails.ScopeOutput)))
			decision, evalErr := s.guardrails.Evaluate(grCtx, guardrails.EvalContext{
				TenantID: tc.TenantID, AgentID: tc.AgentID,
				Scope: guardrails.ScopeOutput, Content: outputContent,
			})
			if evalErr == nil {
				emitGuardrailEvents(grSpan, decision.FiredRules)
				emitGuardrailLogs(grCtx, decision.FiredRules, tc.TenantID, tc.AgentID, guardrails.ScopeOutput)
			}
			grSpan.End()
			if evalErr == nil && !decision.Allowed {
				llmSpan.End()
				writeError(w, http.StatusInternalServerError,
					"response blocked by guardrail: "+decision.FiredRules[0].RuleName,
					"policy_error")
				return
			}
		}

		llmSpan.SetAttributes(attribute.String("gen_ai.completion", truncate(outputContent)))

		// No guardrail fire or 4xx/5xx upstream — pass through
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(body) //nolint:errcheck — client disconnect is non-fatal
	}

	totalSec := time.Since(start).Seconds()
	var tps float64
	if totalSec > 0 && outputTokens > 0 {
		tps = float64(outputTokens) / totalSec
	}

	if resp.StatusCode >= 500 {
		endpoint.RecordError()
	} else {
		endpoint.RecordSuccess(ttftMS, tps)
	}

	llmSpan.SetAttributes(
		attribute.Int("gen_ai.input.tokens", inputTokens),
		attribute.Int("gen_ai.output.tokens", outputTokens),
		attribute.Float64("gen_ai.ttft_ms", ttftMS),
		attribute.Float64("gen_ai.tps", tps),
		attribute.Float64("gen_ai.cost.usd", calcCost(model, inputTokens, outputTokens)),
	)
	llmSpan.End()
}

// proxySSE streams an SSE response to w, returning TTFT ms and token counts from
// the final usage chunk.
func (s *Server) proxySSE(w http.ResponseWriter, provider string, body io.Reader, start time.Time) (ttftMS float64, inputTokens, outputTokens int) {
	const maxSSELine = 1 << 20 // 1MB per line max
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, maxSSELine), maxSSELine)

	flusher, canFlush := w.(http.Flusher)
	var firstToken bool

	for scanner.Scan() {
		line := scanner.Text()

		// Record TTFT on the first data line.
		if !firstToken && (strings.HasPrefix(line, "data: ") || strings.HasPrefix(line, "event: ")) {
			ttftMS = float64(time.Since(start).Milliseconds())
			firstToken = true
		}

		fmt.Fprintf(w, "%s\n", line)

		// Flush on event boundary (empty line separates SSE events).
		if line == "" && canFlush {
			flusher.Flush()
		}

		// Parse usage from every chunk — OpenAI populates it only on the final one.
		if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
			data := line[len("data: "):]
			var chunk streamChunk
			if json.Unmarshal([]byte(data), &chunk) == nil && chunk.Usage != nil {
				inputTokens = chunk.Usage.PromptTokens
				outputTokens = chunk.Usage.CompletionTokens
			}
		}
	}

	return ttftMS, inputTokens, outputTokens
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

// buildUpstreamRequest constructs the final request to the provider.
// It handles translation from OpenAI format to provider-native formats.
func (s *Server) buildUpstreamRequest(ctx context.Context, endpoint *llmrouter.Endpoint, model string, body []byte) (*http.Request, error) {
	var (
		url         string
		headers     = make(http.Header)
		forwardBody = body
	)

	baseURL := strings.TrimRight(endpoint.BaseURL(), "/")

	switch endpoint.Provider() {
	case "anthropic":
		url = baseURL + "/v1/messages"
		headers.Set("x-api-key", endpoint.APIKey())
		headers.Set("anthropic-version", "2023-06-01")

		// Translate OpenAI -> Anthropic
		var oai openAIRequest
		if err := json.Unmarshal(body, &oai); err == nil {
			type anthropicMsg struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			type anthropicReq struct {
				Model     string         `json:"model"`
				Messages  []anthropicMsg `json:"messages"`
				MaxTokens int            `json:"max_tokens,omitempty"`
				System    string         `json:"system,omitempty"`
				Stream    bool           `json:"stream"`
			}
			req := anthropicReq{
				Model:     endpoint.Model(),
				MaxTokens: 4096, // required by Anthropic
				Stream:    oai.Stream,
			}
			for _, m := range oai.Messages {
				if m.Role == "system" {
					req.System = m.Content
				} else {
					req.Messages = append(req.Messages, anthropicMsg(m))
				}
			}
			forwardBody, _ = json.Marshal(req)
		}

	case "gemini":
		method := "generateContent"
		var oai openAIRequest
		if err := json.Unmarshal(body, &oai); err == nil && oai.Stream {
			method = "streamGenerateContent"
		}
		url = fmt.Sprintf("%s/v1beta/models/%s:%s?key=%s", baseURL, endpoint.Model(), method, endpoint.APIKey())

		// Translate OpenAI -> Gemini
		if err := json.Unmarshal(body, &oai); err == nil {
			type geminiPart struct {
				Text string `json:"text"`
			}
			type geminiContent struct {
				Role  string       `json:"role"`
				Parts []geminiPart `json:"parts"`
			}
			type geminiReq struct {
				Contents []geminiContent `json:"contents"`
			}
			req := geminiReq{}
			for _, m := range oai.Messages {
				role := m.Role
				if role == "assistant" {
					role = "model"
				}
				if role == "system" {
					role = "user" // Simple fallback
				}
				req.Contents = append(req.Contents, geminiContent{
					Role:  role,
					Parts: []geminiPart{{Text: m.Content}},
				})
			}
			forwardBody, _ = json.Marshal(req)
		}

	default:
		// OpenAI or OpenAI-compatible
		url = baseURL + "/v1/chat/completions"
		headers.Set("Authorization", "Bearer "+endpoint.APIKey())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(forwardBody))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	for k, v := range headers {
		req.Header[k] = v
	}
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func (s *Server) extractUsage(provider string, body []byte) (int, int) {
	switch provider {
	case "anthropic":
		var res struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &res) == nil {
			return res.Usage.InputTokens, res.Usage.OutputTokens
		}
	case "gemini":
		var res struct {
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}
		if json.Unmarshal(body, &res) == nil {
			return res.UsageMetadata.PromptTokenCount, res.UsageMetadata.CandidatesTokenCount
		}
	default:
		var result chatResponse
		if json.Unmarshal(body, &result) == nil {
			return result.Usage.PromptTokens, result.Usage.CompletionTokens
		}
	}
	return 0, 0
}

// handleAnthropicMessages handles POST /v1/messages for native Anthropic SDKs/CLIs.
// Span hierarchy: llm.generation → upstream.call (same model as handleChatCompletions,
// minus guardrails which operate on the OpenAI-compat surface only).
func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	baseCtx := r.Context()

	rawBody, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body", "invalid_request_error")
		return
	}

	var req struct {
		Model    string          `json:"model"`
		Stream   bool            `json:"stream"`
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json", "invalid_request_error")
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required", "invalid_request_error")
		return
	}

	tc := TenantCtx(baseCtx)
	var tenantID uint
	fmt.Sscanf(tc.TenantID, "%d", &tenantID)

	endpoint, err := s.router.Pick(tenantID, req.Model)
	if err != nil {
		if errors.Is(err, llmrouter.ErrNoEndpoint) {
			writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("no available upstream for model %q", req.Model), "api_error")
		} else {
			writeError(w, http.StatusInternalServerError, "router error", "api_error")
		}
		return
	}

	baseURL := strings.TrimRight(endpoint.BaseURL(), "/")
	url := baseURL + "/v1/messages"

	// llm.generation span wraps the full upstream round-trip.
	llmCtx, llmSpan := tracer.Start(baseCtx, "llm.generation",
		trace.WithAttributes(
			attribute.String("gen_ai.model", req.Model),
			attribute.String("gen_ai.provider", endpoint.Provider()),
			attribute.String("gen_ai.prompt", truncate(flattenMessages(req.Messages))),
			attribute.String("agent.id", tc.AgentID),
			attribute.String("tenant.id", tc.TenantID),
			attribute.String("thread.id", tc.ThreadID),
		))

	// upstream.call sub-span — just the HTTP dial + response headers.
	upCtx, upSpan := tracer.Start(llmCtx, "upstream.call",
		trace.WithAttributes(attribute.String("gen_ai.provider", endpoint.Provider())))

	upstreamReq, err := http.NewRequestWithContext(upCtx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		upSpan.End()
		llmSpan.End()
		writeError(w, http.StatusInternalServerError, "failed to build upstream request", "api_error")
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("x-api-key", endpoint.APIKey())
	upstreamReq.Header.Set("anthropic-version", "2023-06-01")

	start := time.Now()
	resp, err := s.httpClient.Do(upstreamReq)
	if err != nil {
		endpoint.RecordError()
		upSpan.RecordError(err)
		upSpan.End()
		llmSpan.End()
		writeError(w, http.StatusBadGateway, "upstream request failed", "api_error")
		return
	}
	upSpan.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	upSpan.End()
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	var ttftMS float64
	var inputTokens, outputTokens int

	if req.Stream {
		ttftMS, inputTokens, outputTokens = s.proxySSE(w, endpoint.Provider(), resp.Body, start)
	} else {
		body, _ := io.ReadAll(resp.Body)
		ttftMS = float64(time.Since(start).Milliseconds())
		inputTokens, outputTokens = s.extractUsage(endpoint.Provider(), body)
		llmSpan.SetAttributes(attribute.String("gen_ai.completion",
			truncate(extractTextFromResponse(endpoint.Provider(), body))))
		w.Write(body) //nolint:errcheck
	}

	totalSec := time.Since(start).Seconds()
	var tps float64
	if totalSec > 0 && outputTokens > 0 {
		tps = float64(outputTokens) / totalSec
	}

	if resp.StatusCode >= 500 {
		endpoint.RecordError()
	} else {
		endpoint.RecordSuccess(ttftMS, tps)
	}

	llmSpan.SetAttributes(
		attribute.Int("gen_ai.input.tokens", inputTokens),
		attribute.Int("gen_ai.output.tokens", outputTokens),
		attribute.Float64("gen_ai.ttft_ms", ttftMS),
		attribute.Float64("gen_ai.tps", tps),
		attribute.Float64("gen_ai.cost.usd", calcCost(req.Model, inputTokens, outputTokens)),
	)
	llmSpan.End()
}

// handleCompletions handles POST /v1/completions (legacy).
func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented", "api_error")
}

// handleEmbeddings handles POST /v1/embeddings.
func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented", "api_error")
}

// handleListModels handles GET /v1/models.
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	tc := TenantCtx(r.Context())
	var tenantID uint
	fmt.Sscanf(tc.TenantID, "%d", &tenantID)

	endpoints := s.router.List()
	isAnthropic := r.Header.Get("anthropic-version") != ""

	// Deduplicate models by ID for this tenant.
	if isAnthropic {
		data := make([]anthropicModel, 0)
		seen := make(map[string]bool)
		for _, e := range endpoints {
			if e.TenantID == tenantID && !seen[e.Model] {
				seen[e.Model] = true
				data = append(data, anthropicModel{
					ID:          e.Model,
					Type:        "model",
					DisplayName: e.Model,
					CreatedAt:   time.Now().Format(time.RFC3339),
				})
			}
		}
		sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicListModelsResponse{
			Data:    data,
			HasMore: false,
		})
		return
	}

	uniqueModels := make(map[string]openAIModel)
	for _, e := range endpoints {
		if e.TenantID == tenantID {
			if _, ok := uniqueModels[e.Model]; !ok {
				uniqueModels[e.Model] = openAIModel{
					ID:      e.Model,
					Object:  "model",
					Created: time.Now().Unix(), // Simple fallback
					OwnedBy: "ai-gateway",
				}
			}
		}
	}

	data := make([]openAIModel, 0, len(uniqueModels))
	for _, m := range uniqueModels {
		data = append(data, m)
	}

	// Sort by ID for stability
	sort.Slice(data, func(i, j int) bool {
		return data[i].ID < data[j].ID
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(listModelsResponse{
		Object: "list",
		Data:   data,
	})
}
