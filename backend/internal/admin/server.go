package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/chaitanyabankanhal/ai-gateway/config"
	"github.com/chaitanyabankanhal/ai-gateway/internal/auth"
	"github.com/chaitanyabankanhal/ai-gateway/internal/auth/oidc"
	"github.com/chaitanyabankanhal/ai-gateway/internal/clickhouse"
	"github.com/chaitanyabankanhal/ai-gateway/internal/db"
	"github.com/chaitanyabankanhal/ai-gateway/internal/guardrails"
	"github.com/chaitanyabankanhal/ai-gateway/internal/providers"
	llmrouter "github.com/chaitanyabankanhal/ai-gateway/internal/router"
	"github.com/redis/go-redis/v9"
)

// Server holds dependencies for the admin API.
type Server struct {
	db         *gorm.DB
	cfg        *config.Config
	llmRouter  *llmrouter.Router
	redis      *redis.Client
	modelsSvc  providers.ModelProvider
	guardrails *guardrails.Engine
	ch         *clickhouse.Client
}

// NewRouter builds the internal admin API handler.
// This port should never be exposed publicly — bind to 127.0.0.1 in production
// or keep it cluster-internal in Kubernetes.
func NewRouter(cfg *config.Config, database *gorm.DB, llmRouter *llmrouter.Router, redis *redis.Client, modelsSvc providers.ModelProvider, gr *guardrails.Engine, ch *clickhouse.Client) http.Handler {
	srv := &Server{
		db:         database,
		cfg:        cfg,
		llmRouter:  llmRouter,
		redis:      redis,
		modelsSvc:  modelsSvc,
		guardrails: gr,
		ch:         ch,
	}

	// OIDC service — optional. Discovery happens here at startup so the callback
	// stays fast. If discovery fails (e.g. offline dev), log and skip; password
	// login still works.
	oidcReg, err := oidc.NewRegistry(context.Background(), cfg.OIDC)
	var oidcSvc *oidc.Service
	if err != nil {
		slog.Warn("admin: OIDC registry init failed; SSO disabled", "err", err)
	} else if len(oidcReg.Enabled()) > 0 {
		oidcSvc = oidc.NewService(cfg.OIDC, cfg.Admin, oidcReg, oidc.NewRedisStateStore(redis), database)
	}

	mux := chi.NewRouter()

	mux.Use(middleware.RequestID)
	mux.Use(middleware.Recoverer)

	// Health + readiness probes — public, no auth required
	mux.Get("/health", handleHealth)
	mux.Get("/ready", srv.handleReady)

	mux.Route("/api/v1", func(r chi.Router) {
		// Public: login
		r.Post("/auth/login", srv.handleLogin)

		// Public: OIDC SSO (only mounted if at least one provider is enabled)
		if oidcSvc != nil {
			r.Route("/auth/oidc", oidcSvc.Routes)
		}

		// Protected: everything else
		r.Group(func(r chi.Router) {
			r.Use(srv.requireAuth)

			r.Get("/router/endpoints", srv.handleListEndpoints)
			r.Post("/providers/{provider}/models", srv.handleListProviderModels)
			r.Get("/roles", srv.handleListRoles)
			r.Get("/users", srv.handleListUsers)

			// handleListTenants scopes non-superadmins to their own memberships.
			r.Get("/tenants", srv.handleListTenants)
			r.Post("/tenants", srv.handleCreateTenant) // Superadmin only ideally.

			// Self-service: any authenticated user creates a tenant + becomes its admin.
			r.Post("/onboarding/tenant", srv.handleSelfServeTenant)

			r.Route("/tenants/{tenantID}", func(r chi.Router) {
				r.With(srv.RequirePermission("tenant.read")).Get("/", srv.handleGetTenant)
				r.With(srv.RequirePermission("tenant.update")).Patch("/", srv.handleUpdateTenant)
				r.With(srv.RequirePermission("tenant.update")).Delete("/", srv.handleDeleteTenant)

				r.With(srv.RequirePermission("tenant.read")).Get("/rules", srv.handleListRules)
				r.With(srv.RequirePermission("tenant.update")).Post("/rules", srv.handleCreateRule)
				r.With(srv.RequirePermission("tenant.update")).Patch("/rules/{ruleID}", srv.handleUpdateRule)
				r.With(srv.RequirePermission("tenant.update")).Delete("/rules/{ruleID}", srv.handleDeleteRule)
				r.With(srv.RequirePermission("tenant.read")).Get("/guardrail-events", srv.handleGuardrailEvents)
				r.With(srv.RequirePermission("tenant.read")).Get("/traces", srv.handleListTraces)
				r.With(srv.RequirePermission("tenant.read")).Get("/traces/{traceID}/spans", srv.handleGetTraceSpans)

				r.With(srv.RequirePermission("agents.read")).Get("/agents", srv.handleListAgents)
				r.With(srv.RequirePermission("agents.manage")).Post("/agents", srv.handleCreateAgent)

				r.With(srv.RequirePermission("keys.read")).Get("/keys", srv.handleListKeys)
				r.With(srv.RequirePermission("keys.manage")).Post("/keys", srv.handleCreateKey)
				r.With(srv.RequirePermission("keys.manage")).Delete("/keys/{keyID}", srv.handleRevokeKey)

				r.With(srv.RequirePermission("upstreams.read")).Get("/upstreams", srv.handleListUpstreams)
				r.With(srv.RequirePermission("upstreams.manage")).Post("/upstreams", srv.handleCreateUpstream)
				r.With(srv.RequirePermission("upstreams.manage")).Put("/upstreams/{keyID}", srv.handleUpdateUpstream)
				r.With(srv.RequirePermission("upstreams.manage")).Delete("/upstreams/{keyID}", srv.handleDeleteUpstream)

				r.With(srv.RequirePermission("members.read")).Get("/members", srv.handleListMembers)
				r.With(srv.RequirePermission("members.manage")).Post("/members", srv.handleAddMember)
				r.With(srv.RequirePermission("members.manage")).Delete("/members/{userID}", srv.handleRemoveMember)
			})
		})
	})

	return mux
}

// requireAuth validates the Bearer JWT on protected routes.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := auth.ValidateToken(token, s.cfg.Admin.JWTSecret)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r.WithContext(withClaims(r.Context(), claims)))
	})
}

// claimsCtxKey is an unexported type so no other package can write (or
// overwrite) the authenticated claims under the same context key. A bare
// string key would let any package in the process shadow them — staticcheck
// SA1029.
type claimsCtxKey struct{}

// withClaims stores the authenticated caller's claims on the context.
func withClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, claimsCtxKey{}, claims)
}

// claimsFrom returns the authenticated caller's claims, and whether the
// request passed through requireAuth at all.
func claimsFrom(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(claimsCtxKey{}).(*auth.Claims)
	return claims, ok && claims != nil
}

// RequirePermission enforces that the user has a specific permission for the tenant in the URL.
func (s *Server) RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := claimsFrom(r.Context())
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// SuperAdmins bypass all permission checks
			if claims.IsSuperAdmin {
				next.ServeHTTP(w, r)
				return
			}

			tenantIDStr := chi.URLParam(r, "tenantID")
			tenantID, err := strconv.Atoi(tenantIDStr)
			if err != nil {
				http.Error(w, "invalid tenant id", http.StatusBadRequest)
				return
			}

			// Query TenantMember to get role and verify permission
			var tm db.TenantMember
			if err := s.db.Preload("Role.Permissions").Where("user_id = ? AND tenant_id = ?", claims.UserID, tenantID).First(&tm).Error; err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			hasPermission := false
			for _, p := range tm.Role.Permissions {
				if p.Name == permission {
					hasPermission = true
					break
				}
			}

			if !hasPermission {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// handleLogin authenticates a user and returns a JWT.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var user db.User
	if err := s.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	ttl := time.Duration(s.cfg.Admin.JWTTTLHours) * time.Hour
	token, err := auth.GenerateToken(user.ID, user.Name, user.Email, user.IsSuperAdmin, s.cfg.Admin.JWTSecret, ttl)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      token,
		"expires_at": time.Now().Add(ttl).UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	if s.llmRouter == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.llmRouter.List())
}

func (s *Server) handleListProviderModels(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")

	var req struct {
		APIKey        string `json:"apiKey"`
		TenantID      string `json:"tenantID"`
		UpstreamKeyID string `json:"upstreamKeyID"`
		BaseURL       string `json:"baseURL"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		// Ignore error if body is empty for backward compatibility
	}

	apiKey := req.APIKey
	tenantIDStr := req.TenantID
	upstreamKeyID := req.UpstreamKeyID
	baseURL := req.BaseURL

	// Fallback to query params if not in body
	if apiKey == "" {
		apiKey = r.URL.Query().Get("apiKey")
	}
	if tenantIDStr == "" {
		tenantIDStr = r.URL.Query().Get("tenantID")
	}
	if upstreamKeyID == "" {
		upstreamKeyID = r.URL.Query().Get("upstreamKeyID")
	}
	if baseURL == "" {
		baseURL = r.URL.Query().Get("baseURL")
	}

	// Fallback to database key if tenant and key ID are provided
	if apiKey == "" && tenantIDStr != "" && upstreamKeyID != "" {
		tid, _ := strconv.Atoi(tenantIDStr)
		var up db.Upstream
		if err := s.db.Where("tenant_id = ? AND key_id = ?", tid, upstreamKeyID).First(&up).Error; err == nil {
			apiKey = up.APIKey
			if baseURL == "" {
				baseURL = up.BaseURL
			}
		}
	}

	// Fallback to default API key from config if still not found
	if apiKey == "" {
		for _, u := range s.cfg.Upstreams {
			if u.Provider == provider && u.APIKey != "" {
				apiKey = u.APIKey
				if baseURL == "" {
					baseURL = u.BaseURL
				}
				break
			}
		}
	}

	if apiKey == "" {
		http.Error(w, "API key required and no default found", http.StatusBadRequest)
		return
	}

	models, err := s.modelsSvc.GetModels(r.Context(), provider, apiKey, baseURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch models: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

type healthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healthResponse{
		Status: "ok",
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}

// handleReady is the readiness probe — checks that backing services are reachable.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := s.db.DB()
	if err != nil {
		http.Error(w, "failed to get underlying DB", http.StatusInternalServerError)
		return
	}

	if err := sqlDB.Ping(); err != nil {
		http.Error(w, "database unreachable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healthResponse{
		Status: "ready",
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFrom(r.Context())

	var tenants []db.Tenant
	query := s.db.Model(&db.Tenant{})

	if !claims.IsSuperAdmin {
		// Join with TenantMembers to only show tenants the user belongs to
		query = query.Joins("JOIN tenant_members ON tenant_members.tenant_id = tenants.id").
			Where("tenant_members.user_id = ?", claims.UserID)
	}

	if err := query.Find(&tenants).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenants)
}

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFrom(r.Context())
	if !claims.IsSuperAdmin {
		http.Error(w, "forbidden: superadmin only", http.StatusForbidden)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	tenant, err := s.createTenantWithDefaults(req.Name, req.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tenant)
}

// createTenantWithDefaults provisions a tenant together with its default API
// key and the managed guardrail rule set. Shared by the superadmin create-tenant
// handler and the self-service onboarding handler.
func (s *Server) createTenantWithDefaults(name, description string) (*db.Tenant, error) {
	key, hash, suffix, err := auth.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}

	tenant := db.Tenant{
		Name:        name,
		Description: description,
		APIKey:      key,
	}
	if err := s.db.Create(&tenant).Error; err != nil {
		return nil, err
	}

	apiKey := db.APIKey{
		Name:     "Default Key",
		KeyHash:  hash,
		Prefix:   auth.KeyPrefix,
		Suffix:   suffix,
		TenantID: tenant.ID,
		IsActive: true,
	}
	if err := s.db.Create(&apiKey).Error; err != nil {
		return nil, err
	}

	s.seedManagedRules(tenant.ID)
	return &tenant, nil
}

// handleSelfServeTenant lets any authenticated user create a tenant and become
// its admin. Used for new-user onboarding (when a user belongs to no tenant)
// and for creating additional organizations later.
func (s *Server) handleSelfServeTenant(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFrom(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	var adminRole db.Role
	if err := s.db.Where("name = ?", "tenantAdmin").First(&adminRole).Error; err != nil {
		http.Error(w, "tenantAdmin role not found — RBAC not seeded", http.StatusInternalServerError)
		return
	}

	tenant, err := s.createTenantWithDefaults(strings.TrimSpace(req.Name), strings.TrimSpace(req.Description))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	member := db.TenantMember{
		UserID:   claims.UserID,
		TenantID: tenant.ID,
		RoleID:   adminRole.ID,
	}
	if err := s.db.Create(&member).Error; err != nil {
		http.Error(w, "failed to grant tenant membership", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tenant)
}

func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantID")
	tenantID, _ := strconv.Atoi(tenantIDStr)

	var tenant db.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	var agentCount, keyCount int64
	s.db.Model(&db.Agent{}).Where("tenant_id = ?", tenantID).Count(&agentCount)
	s.db.Model(&db.APIKey{}).Where("tenant_id = ? AND is_active = ?", tenantID, true).Count(&keyCount)

	resp := struct {
		db.Tenant
		AgentCount int64 `json:"agent_count"`
		KeyCount   int64 `json:"key_count"`
	}{
		Tenant:     tenant,
		AgentCount: agentCount,
		KeyCount:   keyCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantID")
	tenantID, _ := strconv.Atoi(tenantIDStr)

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var tenant db.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}

	if err := s.db.Model(&tenant).Updates(updates).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenant)
}

func (s *Server) handleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantID")
	tenantID, _ := strconv.Atoi(tenantIDStr)

	var tenant db.Tenant
	if err := s.db.First(&tenant, tenantID).Error; err != nil {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	// Cascade: remove upstreams from live router
	if s.llmRouter != nil {
		var upstreams []db.Upstream
		s.db.Where("tenant_id = ?", tenantID).Find(&upstreams)
		for _, u := range upstreams {
			s.llmRouter.Remove(uint(tenantID), u.KeyID)
		}
	}

	// Soft-delete related records then the tenant
	s.db.Where("tenant_id = ?", tenantID).Delete(&db.Upstream{})
	s.db.Where("tenant_id = ?", tenantID).Delete(&db.APIKey{})
	s.db.Where("tenant_id = ?", tenantID).Delete(&db.Agent{})
	if err := s.db.Delete(&tenant).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	var agents []db.Agent
	if err := s.db.Where("tenant_id = ?", tenantID).Find(&agents).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantID")
	tenantID, _ := strconv.ParseUint(tenantIDStr, 10, 32)

	var agent db.Agent
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if agent.Name == "" {
		http.Error(w, "agent name is required", http.StatusBadRequest)
		return
	}

	agent.TenantID = uint(tenantID)
	if err := s.db.Create(&agent).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(agent)
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	var keys []db.APIKey
	if err := s.db.Where("tenant_id = ?", tenantID).Find(&keys).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Don't return hashes in list
	for i := range keys {
		keys[i].KeyHash = ""
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantID")
	tenantID, _ := strconv.ParseUint(tenantIDStr, 10, 32)

	var req struct {
		Name    string `json:"name"`
		AgentID *uint  `json:"agent_id"`
		UserID  *uint  `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	key, hash, suffix, err := auth.GenerateAPIKey()
	if err != nil {
		http.Error(w, "failed to generate key", http.StatusInternalServerError)
		return
	}

	apiKey := db.APIKey{
		Name:     req.Name,
		KeyHash:  hash,
		Prefix:   auth.KeyPrefix,
		Suffix:   suffix,
		TenantID: uint(tenantID),
		AgentID:  req.AgentID,
		UserID:   req.UserID,
		IsActive: true,
	}

	if err := s.db.Create(&apiKey).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the plaintext key ONLY ONCE
	resp := struct {
		db.APIKey
		Key string `json:"key"`
	}{
		APIKey: apiKey,
		Key:    key,
	}
	resp.KeyHash = "" // Hide hash

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	keyID := chi.URLParam(r, "keyID")
	if err := s.db.Model(&db.APIKey{}).Where("id = ?", keyID).Update("is_active", false).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var validActions = map[string]bool{
	"block": true, "log": true, "tag": true,
	"rewrite": true, "shadow": true, "substitute": true,
}

// defaultManagedRules are the built-in guardrail rules seeded for every tenant.
var defaultManagedRules = []struct {
	name      string
	priority  int
	scope     string
	action    string
	managedID string
	enabled   bool
}{
	{"Prompt Injection Detection", 1, "input", "block", "prompt-injection", true},
	{"Secret Detection", 2, "input,output", "block", "secret-detection", true},
	{"PII Leakage — Output", 3, "output", "log", "pii-leakage-output", true},
	{"Toxicity / Hate Speech", 4, "input,output", "block", "toxicity-basic", true},
	{"ML Injection Detection", 5, "input", "block", "ml-injection-detection", false},
}

// seedManagedRules inserts the default managed guardrail rules for a tenant if
// none exist yet. Safe to call on every list request — skips if already seeded.
func (s *Server) seedManagedRules(tenantID uint) {
	// Fetch existing managed_ids so we can skip already-seeded rules without
	// blocking new entries added to defaultManagedRules after first seed.
	var existing []string
	s.db.Model(&db.GuardrailRule{}).
		Where("tenant_id = ? AND managed = ?", tenantID, true).
		Pluck("managed_id", &existing)
	seeded := make(map[string]bool, len(existing))
	for _, id := range existing {
		seeded[id] = true
	}

	for _, r := range defaultManagedRules {
		if seeded[r.managedID] {
			continue
		}
		var cond string
		if r.managedID == "ml-injection-detection" {
			cond = `{"type":"mlrunner"}`
		} else {
			cond = fmt.Sprintf(`{"type":"managed","rule_id":%q}`, r.managedID)
		}
		rule := db.GuardrailRule{
			TenantID:  tenantID,
			Name:      r.name,
			Priority:  r.priority,
			Scope:     r.scope,
			Direction: "both",
			Condition: cond,
			Action:    r.action,
			Mode:      "parallel",
			Managed:   true,
			ManagedID: r.managedID,
			Version:   "1.0",
			Enabled:   r.enabled,
		}
		s.db.Create(&rule) //nolint:errcheck — best-effort seed
	}
	if s.guardrails != nil {
		s.guardrails.InvalidateCache(context.Background(), tenantID) //nolint:errcheck
	}
}

type ruleWithStats struct {
	db.GuardrailRule
	Fires24h int `json:"fires24h"`
}

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "tenantID"))
	s.seedManagedRules(uint(tenantID))
	query := s.db.Where("tenant_id = ?", tenantID).Order("priority asc")
	if agentIDStr := r.URL.Query().Get("agent_id"); agentIDStr != "" {
		agentID, err := strconv.Atoi(agentIDStr)
		if err != nil {
			http.Error(w, "invalid agent_id", http.StatusBadRequest)
			return
		}
		query = query.Where("agent_id = ? OR agent_id IS NULL", agentID)
	}
	var rules []db.GuardrailRule
	if err := query.Find(&rules).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch fires/24h counts from ClickHouse; silently zero-fill if unavailable.
	countByRule := map[string]int{}
	if s.ch != nil {
		if counts, err := s.ch.GuardrailFireCounts(r.Context(), strconv.Itoa(tenantID)); err == nil {
			for _, c := range counts {
				countByRule[c.RuleID] = int(c.Fires24h)
			}
		}
	}

	out := make([]ruleWithStats, len(rules))
	for i, rule := range rules {
		out[i] = ruleWithStats{
			GuardrailRule: rule,
			Fires24h:      countByRule[strconv.Itoa(int(rule.ID))],
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out) //nolint:errcheck
}

func (s *Server) handleGuardrailEvents(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	ruleID := r.URL.Query().Get("rule_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if s.ch == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]")) //nolint:errcheck
		return
	}

	events, err := s.ch.GuardrailEvents(r.Context(), tenantID, ruleID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []clickhouse.GuardrailEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events) //nolint:errcheck
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "tenantID"))

	var req struct {
		Name      string          `json:"name"`
		Priority  int             `json:"priority"`
		Scope     []string        `json:"scope"`
		Direction string          `json:"direction"`
		Condition json.RawMessage `json:"condition"`
		Action    string          `json:"action"`
		Mode      string          `json:"mode"`
		Managed   bool            `json:"managed"`
		ManagedID string          `json:"managed_id"`
		Version   string          `json:"version"`
		Enabled   *bool           `json:"enabled"`
		AgentID   *uint           `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if !validActions[req.Action] {
		http.Error(w, "invalid action", http.StatusBadRequest)
		return
	}

	direction := req.Direction
	if direction == "" {
		direction = "both"
	}
	mode := req.Mode
	if mode == "" {
		mode = "parallel"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := req.Priority
	if priority == 0 {
		priority = 100
	}

	condJSON := ""
	if len(req.Condition) > 0 {
		condJSON = string(req.Condition)
	}

	rule := db.GuardrailRule{
		TenantID:  uint(tenantID),
		AgentID:   req.AgentID,
		Name:      req.Name,
		Priority:  priority,
		Scope:     strings.Join(req.Scope, ","),
		Direction: direction,
		Condition: condJSON,
		Action:    req.Action,
		Mode:      mode,
		Managed:   req.Managed,
		ManagedID: req.ManagedID,
		Version:   req.Version,
		Enabled:   enabled,
	}
	if err := s.db.Create(&rule).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.guardrails != nil {
		s.guardrails.InvalidateCache(r.Context(), uint(tenantID)) //nolint:errcheck
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule) //nolint:errcheck
}

func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "tenantID"))
	ruleID, _ := strconv.Atoi(chi.URLParam(r, "ruleID"))

	var rule db.GuardrailRule
	if err := s.db.First(&rule, ruleID).Error; err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}
	if rule.TenantID != uint(tenantID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var req struct {
		Name      *string         `json:"name"`
		Priority  *int            `json:"priority"`
		Scope     []string        `json:"scope"`
		Direction *string         `json:"direction"`
		Condition json.RawMessage `json:"condition"`
		Action    *string         `json:"action"`
		Mode      *string         `json:"mode"`
		Enabled   *bool           `json:"enabled"`
		AgentID   *uint           `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if len(req.Scope) > 0 {
		updates["scope"] = strings.Join(req.Scope, ",")
	}
	if req.Direction != nil {
		updates["direction"] = *req.Direction
	}
	if len(req.Condition) > 0 {
		updates["condition"] = string(req.Condition)
	}
	if req.Action != nil {
		if !validActions[*req.Action] {
			http.Error(w, "invalid action", http.StatusBadRequest)
			return
		}
		updates["action"] = *req.Action
	}
	if req.Mode != nil {
		updates["mode"] = *req.Mode
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.AgentID != nil {
		updates["agent_id"] = req.AgentID
	}

	if len(updates) > 0 {
		if err := s.db.Model(&rule).Updates(updates).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if s.guardrails != nil {
		s.guardrails.InvalidateCache(r.Context(), uint(tenantID)) //nolint:errcheck
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule) //nolint:errcheck
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "tenantID"))
	ruleID, _ := strconv.Atoi(chi.URLParam(r, "ruleID"))

	var rule db.GuardrailRule
	if err := s.db.First(&rule, ruleID).Error; err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}
	if rule.TenantID != uint(tenantID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.db.Delete(&rule).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.guardrails != nil {
		s.guardrails.InvalidateCache(r.Context(), uint(tenantID)) //nolint:errcheck
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListUpstreams(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "tenantID"))
	var upstreams []db.Upstream
	if err := s.db.Where("tenant_id = ?", tenantID).Find(&upstreams).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(upstreams)
}

func (s *Server) handleCreateUpstream(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "tenantID"))
	var req struct {
		KeyID    string   `json:"key_id"`
		Provider string   `json:"provider"`
		Models   []string `json:"models"`
		Model    string   `json:"model"` // backward compat
		BaseURL  string   `json:"base_url"`
		APIKey   string   `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Provider == "" {
		req.Provider = "openai"
	}

	if len(req.Models) == 0 && req.Model != "" {
		req.Models = []string{req.Model}
	}

	if len(req.Models) == 0 {
		http.Error(w, "at least one model is required", http.StatusBadRequest)
		return
	}

	up := db.Upstream{
		KeyID:    req.KeyID,
		Provider: req.Provider,
		Models:   strings.Join(req.Models, ","),
		BaseURL:  req.BaseURL,
		APIKey:   req.APIKey,
		TenantID: uint(tenantID),
	}

	if err := s.db.Create(&up).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.llmRouter != nil {
		for _, model := range req.Models {
			s.llmRouter.Add(config.UpstreamConfig{
				KeyID:    up.KeyID,
				Provider: up.Provider,
				Model:    model,
				BaseURL:  up.BaseURL,
				APIKey:   up.APIKey,
				TenantID: up.TenantID,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(up)
}

func (s *Server) handleUpdateUpstream(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "tenantID"))
	keyID := chi.URLParam(r, "keyID")

	var req struct {
		Provider string   `json:"provider"`
		Models   []string `json:"models"`
		BaseURL  string   `json:"base_url"`
		APIKey   string   `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var up db.Upstream
	if err := s.db.Where("tenant_id = ? AND key_id = ?", tenantID, keyID).First(&up).Error; err != nil {
		http.Error(w, "upstream not found", http.StatusNotFound)
		return
	}

	if req.Provider != "" {
		up.Provider = req.Provider
	}
	if len(req.Models) > 0 {
		up.Models = strings.Join(req.Models, ",")
	}
	if req.BaseURL != "" {
		up.BaseURL = req.BaseURL
	}
	if req.APIKey != "" {
		up.APIKey = req.APIKey
	}

	if err := s.db.Save(&up).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.llmRouter != nil {
		s.llmRouter.Remove(uint(tenantID), keyID)
		for _, model := range req.Models {
			s.llmRouter.Add(config.UpstreamConfig{
				KeyID:    up.KeyID,
				Provider: up.Provider,
				Model:    model,
				BaseURL:  up.BaseURL,
				APIKey:   up.APIKey,
				TenantID: up.TenantID,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(up)
}

func (s *Server) handleDeleteUpstream(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.Atoi(chi.URLParam(r, "tenantID"))
	keyID := chi.URLParam(r, "keyID")

	var up db.Upstream
	if err := s.db.Where("tenant_id = ? AND key_id = ?", tenantID, keyID).First(&up).Error; err != nil {
		http.Error(w, "upstream not found", http.StatusNotFound)
		return
	}

	if err := s.db.Delete(&up).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.llmRouter != nil {
		s.llmRouter.Remove(uint(tenantID), keyID)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantID")
	tenantID, _ := strconv.Atoi(tenantIDStr)

	var members []db.TenantMember
	if err := s.db.Preload("User").Preload("Role").Where("tenant_id = ?", tenantID).Find(&members).Error; err != nil {
		http.Error(w, "failed to list members", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(members)
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantID")
	tenantID, _ := strconv.Atoi(tenantIDStr)

	var req struct {
		UserID   *uint  `json:"user_id"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		RoleID   uint   `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var user db.User
	if req.UserID != nil {
		if err := s.db.First(&user, *req.UserID).Error; err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
	} else if req.Email != "" && req.Name != "" {
		if err := s.db.Where("email = ?", req.Email).First(&user).Error; err == nil {
			// User exists, just use them
		} else {
			// Auto-create user
			var hash []byte
			if req.Password != "" {
				var err error
				hash, err = bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
				if err != nil {
					http.Error(w, "failed to hash password", http.StatusInternalServerError)
					return
				}
			}

			user = db.User{
				Email:        req.Email,
				Name:         req.Name,
				PasswordHash: string(hash),
			}
			if err := s.db.Create(&user).Error; err != nil {
				http.Error(w, "failed to create user", http.StatusInternalServerError)
				return
			}
		}
	} else {
		http.Error(w, "user_id or email and name required", http.StatusBadRequest)
		return
	}

	member := db.TenantMember{
		UserID:   user.ID,
		TenantID: uint(tenantID),
		RoleID:   req.RoleID,
	}

	if err := s.db.Create(&member).Error; err != nil {
		http.Error(w, "failed to add member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(member)
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantID")
	tenantID, _ := strconv.Atoi(tenantIDStr)
	userIDStr := chi.URLParam(r, "userID")
	userID, _ := strconv.Atoi(userIDStr)

	if err := s.db.Where("tenant_id = ? AND user_id = ?", tenantID, userID).Delete(&db.TenantMember{}).Error; err != nil {
		http.Error(w, "failed to remove member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	var roles []db.Role
	if err := s.db.Find(&roles).Error; err != nil {
		http.Error(w, "failed to list roles", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roles)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	var users []db.User
	if err := s.db.Find(&users).Error; err != nil {
		http.Error(w, "failed to list users", http.StatusInternalServerError)
		return
	}
	// Hide password hashes
	for i := range users {
		users[i].PasswordHash = ""
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}
