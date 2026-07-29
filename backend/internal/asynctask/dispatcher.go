// Package asynctask runs LLM upstream calls in the background and delivers
// the result to a tenant-supplied webhook URL.
//
// Flow:
//  1. Client POSTs /v1/chat/completions with {"webhook_url": "...", ...}.
//  2. Proxy persists an AsyncTask row (status=pending), returns 202 + task ID.
//  3. Dispatcher.Submit spawns a goroutine that runs the upstream call.
//  4. On completion (success or failure), goroutine persists result and
//     POSTs a signed JSON envelope to the webhook with exponential backoff.
package asynctask

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/chaitanyabankanhal/ai-gateway/internal/db"
)

// Runner executes the actual upstream call for a task and returns the
// raw response body, status code, and token usage. Implementations live in
// the proxy package to keep the asynctask package free of proxy internals.
type Runner func(ctx context.Context, task *db.AsyncTask) (respBody []byte, status int, inputTokens, outputTokens int, costUSD float64, err error)

// Dispatcher owns the background worker pool + webhook delivery loop.
type Dispatcher struct {
	db          *gorm.DB
	httpClient  *http.Client
	runner      Runner
	maxAttempts int
	backoff     []time.Duration
}

// Options configures a Dispatcher.
type Options struct {
	MaxAttempts int             // default 4
	Backoff     []time.Duration // default 1s, 5s, 25s, 2m
	HTTPTimeout time.Duration   // default 30s for webhook POST
}

// New builds a Dispatcher. runner must be supplied by the caller (proxy package).
func New(database *gorm.DB, runner Runner, opts Options) *Dispatcher {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 4
	}
	if len(opts.Backoff) == 0 {
		opts.Backoff = []time.Duration{1 * time.Second, 5 * time.Second, 25 * time.Second, 2 * time.Minute}
	}
	if opts.HTTPTimeout <= 0 {
		opts.HTTPTimeout = 30 * time.Second
	}
	return &Dispatcher{
		db:          database,
		runner:      runner,
		maxAttempts: opts.MaxAttempts,
		backoff:     opts.Backoff,
		httpClient:  &http.Client{Timeout: opts.HTTPTimeout},
	}
}

// NewTaskID returns a fresh opaque task identifier.
func NewTaskID() string { return "task_" + uuid.NewString() }

// GenerateSecret returns a 32-byte hex string suitable as a per-tenant HMAC secret.
func GenerateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Submit starts background execution of an already-persisted task row.
// The caller has already returned 202 to the client; do not block here.
func (d *Dispatcher) Submit(parent context.Context, task *db.AsyncTask, secret string) {
	// #nosec G118 -- deliberate: the request context is cancelled the moment
	// the 202 is written, which would kill the task we just accepted.
	go d.execute(context.Background(), task.ID, secret)
}

// execute runs one task end-to-end: upstream call, persist result, webhook delivery.
// parentCtx intentionally ignored — request context is cancelled when 202 returns.
func (d *Dispatcher) execute(ctx context.Context, rowID uint, secret string) {
	var task db.AsyncTask
	if err := d.db.First(&task, rowID).Error; err != nil {
		slog.Error("asynctask: load task failed", "row_id", rowID, "err", err)
		return
	}

	now := time.Now()
	d.db.Model(&task).Updates(map[string]interface{}{
		"status":     "running",
		"started_at": &now,
	})

	respBody, status, in, out, cost, err := d.runner(ctx, &task)

	completed := time.Now()
	updates := map[string]interface{}{
		"completed_at":    &completed,
		"input_tokens":    in,
		"output_tokens":   out,
		"cost_usd":        cost,
		"response_status": status,
	}
	if err != nil {
		updates["status"] = "failed"
		updates["error_message"] = err.Error()
	} else {
		updates["status"] = "succeeded"
		updates["response_body"] = respBody
	}
	if err := d.db.Model(&task).Updates(updates).Error; err != nil {
		slog.Error("asynctask: persist result failed", "task_id", task.TaskID, "err", err)
	}

	// Reload to deliver fresh state.
	d.db.First(&task, rowID)
	d.deliver(&task, secret)
}

// webhookPayload is the JSON envelope POSTed to the tenant webhook.
type webhookPayload struct {
	ID             string          `json:"id"`
	Object         string          `json:"object"`
	Endpoint       string          `json:"endpoint"`
	Model          string          `json:"model"`
	Status         string          `json:"status"`
	TenantID       uint            `json:"tenant_id"`
	AgentID        string          `json:"agent_id,omitempty"`
	ThreadID       string          `json:"thread_id,omitempty"`
	CreatedAt      int64           `json:"created_at"`
	CompletedAt    int64           `json:"completed_at,omitempty"`
	InputTokens    int             `json:"input_tokens"`
	OutputTokens   int             `json:"output_tokens"`
	CostUSD        float64         `json:"cost_usd"`
	ResponseStatus int             `json:"response_status"`
	Response       json.RawMessage `json:"response,omitempty"`
	Error          string          `json:"error,omitempty"`
}

func buildPayload(t *db.AsyncTask) webhookPayload {
	p := webhookPayload{
		ID:             t.TaskID,
		Object:         "async_task",
		Endpoint:       t.Endpoint,
		Model:          t.ModelName,
		Status:         t.Status,
		TenantID:       t.TenantID,
		AgentID:        t.AgentID,
		ThreadID:       t.ThreadID,
		CreatedAt:      t.CreatedAt.Unix(),
		InputTokens:    t.InputTokens,
		OutputTokens:   t.OutputTokens,
		CostUSD:        t.CostUSD,
		ResponseStatus: t.ResponseStatus,
		Error:          t.ErrorMessage,
	}
	if t.CompletedAt != nil {
		p.CompletedAt = t.CompletedAt.Unix()
	}
	if len(t.ResponseBody) > 0 && json.Valid(t.ResponseBody) {
		p.Response = t.ResponseBody
	}
	return p
}

// Sign returns the hex HMAC-SHA256 of body using the tenant secret.
// Receivers verify: hmac.Equal(expected, recv).
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// deliver POSTs the payload to the webhook URL with retries.
func (d *Dispatcher) deliver(t *db.AsyncTask, secret string) {
	payload := buildPayload(t)
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("asynctask: marshal payload failed", "task_id", t.TaskID, "err", err)
		d.markWebhook(t, "dead_letter", t.WebhookAttempts, "marshal: "+err.Error())
		return
	}

	sig := Sign(secret, body)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	// Signature input format: "t={ts}.v1={hex}". Receivers may also recompute
	// over `ts + "." + body` if they want timestamp protection — we expose ts header.

	var lastErr string
	for attempt := 0; attempt < d.maxAttempts; attempt++ {
		if attempt > 0 {
			delay := d.backoff[min(attempt-1, len(d.backoff)-1)]
			time.Sleep(delay)
		}
		t.WebhookAttempts = attempt + 1

		req, err := http.NewRequest(http.MethodPost, t.WebhookURL, bytes.NewReader(body))
		if err != nil {
			lastErr = "build_request: " + err.Error()
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "ai-gateway-webhook/1.0")
		req.Header.Set("X-AIGW-Task-Id", t.TaskID)
		req.Header.Set("X-AIGW-Timestamp", timestamp)
		req.Header.Set("X-AIGW-Signature", "t="+timestamp+",v1="+sig)
		req.Header.Set("X-AIGW-Delivery-Attempt", fmt.Sprintf("%d", attempt+1))

		resp, err := d.httpClient.Do(req)
		if err != nil {
			lastErr = "transport: " + err.Error()
			continue
		}
		respPreview, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			d.markWebhook(t, "delivered", attempt+1, "")
			return
		}
		lastErr = fmt.Sprintf("http_%d: %s", resp.StatusCode, string(respPreview))
		// 4xx (except 408/429) is non-retryable — webhook receiver rejected.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 408 && resp.StatusCode != 429 {
			d.markWebhook(t, "dead_letter", attempt+1, lastErr)
			return
		}
	}
	d.markWebhook(t, "dead_letter", t.WebhookAttempts, lastErr)
}

func (d *Dispatcher) markWebhook(t *db.AsyncTask, status string, attempts int, errMsg string) {
	updates := map[string]interface{}{
		"webhook_status":   status,
		"webhook_attempts": attempts,
		"webhook_last_err": errMsg,
	}
	if status == "delivered" {
		now := time.Now()
		updates["webhook_delivered_at"] = &now
	}
	if err := d.db.Model(&db.AsyncTask{}).Where("id = ?", t.ID).Updates(updates).Error; err != nil {
		slog.Error("asynctask: persist webhook status failed", "task_id", t.TaskID, "err", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
