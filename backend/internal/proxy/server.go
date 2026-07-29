package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gorm.io/gorm"

	"github.com/chaitanyabankanhal/ai-gateway/config"
	"github.com/chaitanyabankanhal/ai-gateway/internal/auth"
	"github.com/chaitanyabankanhal/ai-gateway/internal/db"
	"github.com/chaitanyabankanhal/ai-gateway/internal/guardrails"
	llmrouter "github.com/chaitanyabankanhal/ai-gateway/internal/router"
	"github.com/chaitanyabankanhal/ai-gateway/internal/secllm"
)

// Server holds shared dependencies for the proxy hot path.
type Server struct {
	router     *llmrouter.Router
	httpClient *http.Client
	db         *gorm.DB
	redis      *redis.Client
	authTTL    time.Duration
	guardrails *guardrails.Engine
	secllm     *secllm.Detector
}

// NewRouter builds the OpenAI-compatible proxy HTTP handler.
func NewRouter(cfg *config.Config, database *gorm.DB, redisCache *redis.Client, r *llmrouter.Router, grEngine *guardrails.Engine, secDet *secllm.Detector) http.Handler {
	srv := &Server{
		router:     r,
		db:         database,
		redis:      redisCache,
		authTTL:    cfg.Auth.CacheTTL,
		guardrails: grEngine,
		secllm:     secDet,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 20,
			},
			// No hard timeout — streaming responses can be long.
			// Callers control deadline via request context.
		},
	}

	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(clientIPMiddleware(cfg.Network))
	mux.Use(middleware.Recoverer)
	mux.Use(srv.Authenticate)
	mux.Use(agentTraceMiddleware)

	// OpenAI-compatible surface
	mux.Post("/v1/chat/completions", srv.handleChatCompletions)
	mux.Post("/v1/completions", srv.handleCompletions)
	mux.Post("/v1/embeddings", srv.handleEmbeddings)
	mux.Get("/v1/models", srv.handleListModels)

	// Anthropic-compatible surface
	mux.Post("/v1/messages", srv.handleAnthropicMessages)

	return otelhttp.NewHandler(mux, "proxy",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)
}

// Authenticate validates the API key and attaches tenant context.
func (s *Server) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var key string

		// 1. Check OpenAI/Standard Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				key = parts[1]
			}
		}

		// 2. Fallback to Anthropic-specific header (for native passthrough)
		if key == "" {
			key = r.Header.Get("x-api-key")
		}

		if key == "" {
			writeError(w, http.StatusUnauthorized, "missing authorization header or x-api-key", "invalid_request_error")
			return
		}

		hash := auth.HashKey(key)

		// 1. Try cache first
		cacheKey := fmt.Sprintf("auth:key:%s", hash)
		var apiKey db.APIKey

		if s.redis != nil {
			val, err := s.redis.Get(r.Context(), cacheKey).Result()
			if err == nil {
				if err := json.Unmarshal([]byte(val), &apiKey); err == nil {
					goto authorized
				}
			}
		}

		// 2. Fallback to DB
		if err := s.db.Where("key_hash = ? AND is_active = ?", hash, true).First(&apiKey).Error; err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or inactive api key", "invalid_request_error")
			return
		}

		// 3. Populate cache
		if s.redis != nil {
			if apiKeyJSON, err := json.Marshal(apiKey); err == nil {
				s.redis.Set(r.Context(), cacheKey, apiKeyJSON, s.authTTL)
			}
		}

	authorized:
		// Update last used at
		now := time.Now()
		s.db.Model(&apiKey).Update("last_used_at", &now)

		// Attach tenant and agent info to context.
		// We preserve the ability to pass these in headers for project-level keys.
		tc := TenantContext{
			TenantID:  r.Header.Get("X-Tenant-Id"),
			AgentID:   r.Header.Get("X-Agent-Id"),
			ThreadID:  r.Header.Get("X-Thread-Id"),
			SpanID:    r.Header.Get("X-Span-Id"),
			SessionID: r.Header.Get("X-Session-Id"),
		}

		// Security: TenantID must match the API key's tenant.
		apiKeyTenantID := fmt.Sprintf("%d", apiKey.TenantID)
		if tc.TenantID == "" {
			tc.TenantID = apiKeyTenantID
		} else if tc.TenantID != apiKeyTenantID {
			writeError(w, http.StatusForbidden, "api key is not authorized for this tenant", "invalid_request_error")
			return
		}

		// Security: If key is scoped to an agent, AgentID must match or be empty.
		if apiKey.AgentID != nil {
			scopedAgentID := fmt.Sprintf("%d", *apiKey.AgentID)
			if tc.AgentID != "" && tc.AgentID != scopedAgentID {
				writeError(w, http.StatusForbidden, "api key is not authorized for this agent", "invalid_request_error")
				return
			}
			tc.AgentID = scopedAgentID
		}

		next.ServeHTTP(w, r.WithContext(withTenantContext(r.Context(), tc)))
	})
}
