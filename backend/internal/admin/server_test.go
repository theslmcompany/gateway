package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chaitanyabankanhal/ai-gateway/config"
	"github.com/chaitanyabankanhal/ai-gateway/internal/auth"
	"github.com/chaitanyabankanhal/ai-gateway/internal/db"
	llmrouter "github.com/chaitanyabankanhal/ai-gateway/internal/router"
)

const testJWTSecret = "test-secret-for-unit-tests"

func testCfg() *config.Config {
	return &config.Config{
		Admin: config.AdminConfig{
			JWTSecret:   testJWTSecret,
			JWTTTLHours: 1,
		},
	}
}

func setupTestDB(t *testing.T) *gorm.DB {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(database))
	return database
}

// authRequest adds a valid JWT Bearer header to req.
func authRequest(t *testing.T, req *http.Request) *http.Request {
	token, err := auth.GenerateToken(1, "Test User", "test-admin@example.com", true, testJWTSecret, time.Hour)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func ptrUint(v uint) *uint { return &v }

func TestHandleLogin(t *testing.T) {
	database := setupTestDB(t)
	cfg := testCfg()
	cfg.Admin.Email = "admin@example.com"
	cfg.Admin.Password = "secret"
	router := NewRouter(cfg, database, nil, nil, nil, nil, nil)

	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))

	body := `{"email":"admin@example.com","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["token"])
	assert.NotEmpty(t, resp["expires_at"])
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	body := `{"email":"admin@example.com","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestRequireAuth_Missing(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestTenantHandlers(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	// Create tenant
	body := `{"name":"Test Tenant"}`
	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(body)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)

	var created db.Tenant
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))
	assert.Equal(t, "Test Tenant", created.Name)
	assert.NotZero(t, created.ID)

	// List tenants
	req = authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var tenants []db.Tenant
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &tenants))
	assert.Len(t, tenants, 1)
	assert.Equal(t, created.Name, tenants[0].Name)
}

func TestGetTenantHandler(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	// Create via DB directly
	tenant := db.Tenant{Name: "direct", APIKey: "k"}
	database.Create(&tenant)

	req := authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/1/", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "direct", resp["Name"])
}

func TestDeleteTenantHandler(t *testing.T) {
	database := setupTestDB(t)
	r := llmrouter.New([]config.UpstreamConfig{})
	router := NewRouter(testCfg(), database, r, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "to-delete", APIKey: "k"}
	database.Create(&tenant)

	req := authRequest(t, httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/1/", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)

	var count int64
	database.Unscoped().Model(&db.Tenant{}).Where("id = ?", 1).Count(&count)
	assert.Equal(t, int64(1), count) // soft-deleted, still in DB
}

func TestHandleReady(t *testing.T) {
	database := setupTestDB(t)
	srv := &Server{db: database}

	t.Run("ready", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rr := httptest.NewRecorder()
		srv.handleReady(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp healthResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Equal(t, "ready", resp.Status)
	})

	t.Run("db failure", func(t *testing.T) {
		// Create a server with a closed DB to simulate failure
		sqlDB, _ := database.DB()
		sqlDB.Close()
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rr := httptest.NewRecorder()
		srv.handleReady(rr, req)
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	})
}

func TestHandleListEndpoints_NilRouter(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/router/endpoints", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var endpoints []llmrouter.EndpointStatus
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &endpoints))
	assert.Empty(t, endpoints)
}

func TestHandleListEndpoints_WithRouter(t *testing.T) {
	database := setupTestDB(t)
	r := llmrouter.New([]config.UpstreamConfig{
		{KeyID: "openai-primary", Model: "gpt-4o", BaseURL: "https://api.openai.com", APIKey: "sk-test"},
	})
	handler := NewRouter(testCfg(), database, r, nil, nil, nil, nil)

	req := authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/router/endpoints", nil))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var endpoints []llmrouter.EndpointStatus
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &endpoints))
	require.Len(t, endpoints, 1)
	assert.Equal(t, "openai-primary", endpoints[0].ID)
	assert.Equal(t, "gpt-4o", endpoints[0].Model)
	assert.Equal(t, "CLOSED", endpoints[0].CircuitState)
}

func TestAgentHandlers(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "tenant1"}
	database.Create(&tenant)

	// Create Agent
	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/tenants/1/agents", bytes.NewBufferString(`{"Name":"agent1"}`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)

	// List Agents
	req = authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/1/agents", nil))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var agents []db.Agent
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &agents))
	assert.Len(t, agents, 1)
	assert.Equal(t, "agent1", agents[0].Name)
}

func TestKeyHandlers(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "tenant1"}
	database.Create(&tenant)

	// Create Key
	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/tenants/1/keys", bytes.NewBufferString(`{"name":"test-key"}`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)

	var created struct {
		ID  uint   `json:"ID"`
		Key string `json:"key"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))
	assert.NotEmpty(t, created.Key)

	// List Keys
	req = authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/1/keys", nil))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var keys []db.APIKey
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &keys))
	assert.Len(t, keys, 1)
	assert.Equal(t, "test-key", keys[0].Name)
	assert.NotEmpty(t, keys[0].Suffix)
	assert.Equal(t, 4, len(keys[0].Suffix))
	assert.Empty(t, keys[0].KeyHash, "KeyHash should be hidden")

	// Revoke Key
	req = authRequest(t, httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/1/keys/1", nil))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)

	var revokedKey db.APIKey
	database.First(&revokedKey, 1)
	assert.False(t, revokedKey.IsActive)
}

func TestUpstreamHandlers(t *testing.T) {
	database := setupTestDB(t)
	r := llmrouter.New([]config.UpstreamConfig{})
	router := NewRouter(testCfg(), database, r, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "tenant1"}
	database.Create(&tenant)

	// Create Upstream
	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/tenants/1/upstreams",
		bytes.NewBufferString(`{"key_id":"test-ups","model":"test-model","base_url":"http://test","api_key":"sk-123"}`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)

	// List Upstreams
	req = authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/1/upstreams", nil))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var upstreams []db.Upstream
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &upstreams))
	assert.Len(t, upstreams, 1)
	assert.Equal(t, "test-ups", upstreams[0].KeyID)

	// Update Upstream
	req = authRequest(t, httptest.NewRequest(http.MethodPut, "/api/v1/tenants/1/upstreams/test-ups",
		bytes.NewBufferString(`{"provider":"openai","models":["gpt-4o","gpt-4o-mini"],"base_url":"https://api.openai.com"}`)))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var updated db.Upstream
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &updated))
	assert.Equal(t, "gpt-4o,gpt-4o-mini", updated.Models)

	// Delete Upstream
	req = authRequest(t, httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/1/upstreams/test-ups", nil))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestGuardrailRulesCRUD(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	tenantPath := fmt.Sprintf("/api/v1/tenants/%d/rules", tenant.ID)

	// List — empty
	req := authRequest(t, httptest.NewRequest(http.MethodGet, tenantPath, nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Create — valid rule
	body := `{"name":"block-injection","action":"block","scope":["input"],"condition":{"type":"keyword","patterns":["ignore"]}}`
	req = authRequest(t, httptest.NewRequest(http.MethodPost, tenantPath, bytes.NewBufferString(body)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)

	var created db.GuardrailRule
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&created))
	assert.Equal(t, "block-injection", created.Name)

	// Update — disable
	patchBody := `{"enabled":false}`
	rulePath := fmt.Sprintf("%s/%d", tenantPath, created.ID)
	req = authRequest(t, httptest.NewRequest(http.MethodPatch, rulePath, bytes.NewBufferString(patchBody)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Delete
	req = authRequest(t, httptest.NewRequest(http.MethodDelete, rulePath, nil))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)

	// Create — missing action → 400
	bad := `{"name":"x"}`
	req = authRequest(t, httptest.NewRequest(http.MethodPost, tenantPath, bytes.NewBufferString(bad)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestRBACAndMembers(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))

	cfg := testCfg()
	handler := NewRouter(cfg, database, nil, nil, nil, nil, nil)

	// 1. Create a tenant and a regular user
	tenant := db.Tenant{Name: "Tenant A", APIKey: "key-a"}
	database.Create(&tenant)

	user := db.User{Name: "Normal User", Email: "user@example.com", PasswordHash: "xxx"}
	database.Create(&user)

	// 2. Add user to tenant as 'user' (read-only)
	var userRole db.Role
	database.Where("name = ?", "user").First(&userRole)

	member := db.TenantMember{UserID: user.ID, TenantID: tenant.ID, RoleID: userRole.ID}
	database.Create(&member)

	// 3. Generate token for regular user
	userToken, _ := auth.GenerateToken(user.ID, user.Name, user.Email, false, testJWTSecret, time.Hour)

	t.Run("MemberCanReadAgents", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/tenants/%d/agents", tenant.ID), nil)
		req.Header.Set("Authorization", "Bearer "+userToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("MemberCannotCreateAgent", func(t *testing.T) {
		body := `{"name":"Illegal Agent"}`
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tenants/%d/agents", tenant.ID), bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+userToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("SuperAdminBypassesAll", func(t *testing.T) {
		adminToken, _ := auth.GenerateToken(999, "Admin", "admin@e.com", true, testJWTSecret, time.Hour)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tenants/%d/agents", tenant.ID), bytes.NewBufferString(`{"name":"Admin Agent"}`))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)
	})

	t.Run("ListMembers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/tenants/%d/members", tenant.ID), nil)
		req = authRequest(t, req)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var members []db.TenantMember
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &members))
		assert.Len(t, members, 1)
		assert.Equal(t, "user@example.com", members[0].User.Email)
	})

	t.Run("AddMemberNewUser", func(t *testing.T) {
		body := fmt.Sprintf(`{"name":"New Guy","email":"new@guy.com","password":"pass","role_id":%d}`, userRole.ID)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/tenants/%d/members", tenant.ID), bytes.NewBufferString(body))
		req = authRequest(t, req)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)

		var newUser db.User
		assert.NoError(t, database.Where("email = ?", "new@guy.com").First(&newUser).Error)
		assert.Equal(t, "New Guy", newUser.Name)
	})

	t.Run("RemoveMember", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/tenants/%d/members/%d", tenant.ID, user.ID), nil)
		req = authRequest(t, req)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNoContent, rr.Code)

		var count int64
		database.Model(&db.TenantMember{}).Where("tenant_id = ? AND user_id = ?", tenant.ID, user.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("ListRoles", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil)
		req = authRequest(t, req)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var roles []db.Role
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &roles))
		assert.GreaterOrEqual(t, len(roles), 2)
	})
}

func TestListUsers(t *testing.T) {
	database := setupTestDB(t)
	handler := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	database.Create(&db.User{Name: "User 1", Email: "u1@e.com"})
	database.Create(&db.User{Name: "User 2", Email: "u2@e.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req = authRequest(t, req)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var users []db.User
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &users))
	assert.GreaterOrEqual(t, len(users), 2)
	assert.Empty(t, users[0].PasswordHash)
}

func TestUpdateTenant(t *testing.T) {
	database := setupTestDB(t)
	handler := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "Old Name", Description: "Old Desc", APIKey: "key"}
	database.Create(&tenant)

	body := `{"name":"New Name","description":"New Desc"}`
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/tenants/%d", tenant.ID), bytes.NewBufferString(body))
	req = authRequest(t, req)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var updated db.Tenant
	database.First(&updated, tenant.ID)
	assert.Equal(t, "New Name", updated.Name)
	assert.Equal(t, "New Desc", updated.Description)
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handleHealth(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

type mockModelProvider struct {
	models []string
	err    error
}

func (m *mockModelProvider) GetModels(ctx context.Context, provider, apiKey, baseURL string) ([]string, error) {
	return m.models, m.err
}

func TestListProviderModels(t *testing.T) {
	database := setupTestDB(t)
	mock := &mockModelProvider{models: []string{"gpt-4", "gpt-3.5-turbo"}}
	handler := NewRouter(testCfg(), database, nil, nil, mock, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/openai/models", bytes.NewBufferString(`{"apiKey":"test"}`))
	req = authRequest(t, req)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp []string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.ElementsMatch(t, []string{"gpt-4", "gpt-3.5-turbo"}, resp)
}

// TestHandleGuardrailEvents_NilClickHouse tests that nil ClickHouse returns empty array
func TestHandleGuardrailEvents_NilClickHouse(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	req := authRequest(t, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/tenants/%d/guardrail-events", tenant.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Body.String(), "[]")
}

// TestHandleGuardrailEvents_WithParams tests guardrail events with query parameters
func TestHandleGuardrailEvents_WithParams(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	req := authRequest(t, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/tenants/%d/guardrail-events?rule_id=rule1&limit=100", tenant.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestHandleCreateTenant_InvalidJSON tests error handling for invalid JSON
func TestHandleCreateTenant_InvalidJSON(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	body := `{invalid json`
	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(body)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleCreateRule_ErrorCases tests various error cases for creating rules
func TestHandleCreateRule_ErrorCases(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	tenantPath := fmt.Sprintf("/api/v1/tenants/%d/rules", tenant.ID)

	// Test invalid JSON
	req := authRequest(t, httptest.NewRequest(http.MethodPost, tenantPath, bytes.NewBufferString(`{invalid`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// Test missing required field
	req = authRequest(t, httptest.NewRequest(http.MethodPost, tenantPath, bytes.NewBufferString(`{"name":"test"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleUpdateRule_NotFound tests updating a non-existent rule
func TestHandleUpdateRule_NotFound(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	req := authRequest(t, httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/tenants/%d/rules/999", tenant.ID), bytes.NewBufferString(`{"enabled":false}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestHandleDeleteRule_NotFound tests deleting a non-existent rule
func TestHandleDeleteRule_NotFound(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	req := authRequest(t, httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/tenants/%d/rules/999", tenant.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestHandleRevokeKey_InvalidKeyID tests revoking a non-existent key
func TestHandleRevokeKey_InvalidKeyID(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "tenant1", APIKey: "test"}
	database.Create(&tenant)

	req := authRequest(t, httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/1/keys/999", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Should handle gracefully even if key not found
	assert.True(t, rr.Code == http.StatusNoContent || rr.Code == http.StatusNotFound)
}

// TestHandleAddMember_InvalidEmail tests adding member with invalid data
func TestHandleAddMember_InvalidEmail(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	// Invalid JSON
	req := authRequest(t, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/tenants/%d/members", tenant.ID), bytes.NewBufferString(`{invalid`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleListRules_TenantNotFound tests listing rules for non-existent tenant
func TestHandleListRules_TenantNotFound(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/999/rules", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Should return OK with empty array even if tenant doesn't exist
	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestHandleListRoles_ResponseShape tests the response structure of list roles
func TestHandleListRoles_ResponseShape(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/roles", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var roles []map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &roles))
	assert.NotEmpty(t, roles)
}

// TestHandleLogin_InvalidJSON tests login with invalid body
func TestHandleLogin_InvalidJSON(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{invalid`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleLogin_UserNotFound tests login with non-existent user
func TestHandleLogin_UserNotFound(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	body := `{"email":"noone@example.com","password":"pass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestHandleUpdateRule_Success tests successfully updating a rule
func TestHandleUpdateRule_Success(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	database.Create(&tenant)

	rule := db.GuardrailRule{
		TenantID: tenant.ID,
		Name:     "old-name",
		Action:   "block",
		Scope:    "input",
	}
	database.Create(&rule)

	body := `{"name":"new-name","priority":50,"action":"log"}`
	req := authRequest(t, httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/tenants/%d/rules/%d", tenant.ID, rule.ID), bytes.NewBufferString(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var updated db.GuardrailRule
	database.First(&updated, rule.ID)
	assert.Equal(t, "new-name", updated.Name)
	assert.Equal(t, 50, updated.Priority)
	assert.Equal(t, "log", updated.Action)
}

// TestHandleUpdateRule_InvalidAction tests validation
func TestHandleUpdateRule_InvalidAction(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	database.Create(&tenant)

	rule := db.GuardrailRule{TenantID: tenant.ID, Name: "r", Action: "block", Scope: "input"}
	database.Create(&rule)

	body := `{"action":"invalid"}`
	req := authRequest(t, httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/tenants/%d/rules/%d", tenant.ID, rule.ID), bytes.NewBufferString(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleDeleteRule_Success tests successfully deleting a rule
func TestHandleDeleteRule_Success(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	database.Create(&tenant)

	rule := db.GuardrailRule{TenantID: tenant.ID, Name: "r", Action: "block", Scope: "input"}
	database.Create(&rule)

	req := authRequest(t, httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/tenants/%d/rules/%d", tenant.ID, rule.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)

	var count int64
	database.Model(&db.GuardrailRule{}).Where("id = ?", rule.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

// TestHandleCreateTenant_DuplicateName tests error handling for unique constraint
func TestHandleCreateTenant_DuplicateName(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	database.Create(&db.Tenant{Name: "existing", APIKey: "k1"})

	body := `{"name":"existing"}`
	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(body)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code) // GORM error
}

// TestHandleListProviderModels_WithTenantKeyID tests fetching models using tenant upstream
func TestHandleListProviderModels_WithTenantKeyID(t *testing.T) {
	database := setupTestDB(t)
	mock := &mockModelProvider{models: []string{"gpt-4", "gpt-4-turbo"}}
	handler := NewRouter(testCfg(), database, nil, nil, mock, nil, nil)

	// Create tenant with upstream
	tenant := db.Tenant{Name: "tenant1", APIKey: "key"}
	database.Create(&tenant)

	upstream := db.Upstream{
		TenantID: tenant.ID,
		KeyID:    "test-key",
		Provider: "openai",
		Models:   "gpt-4o",
		BaseURL:  "https://api.openai.com",
		APIKey:   "sk-tenant-key",
	}
	database.Create(&upstream)

	// Request with tenant ID and upstream key ID
	body := fmt.Sprintf(`{"tenantID":"%d","upstreamKeyID":"test-key"}`, tenant.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/openai/models", bytes.NewBufferString(body))
	req = authRequest(t, req)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp []string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.ElementsMatch(t, []string{"gpt-4", "gpt-4-turbo"}, resp)
}

// TestHandleListProviderModels_QueryParams tests using query parameters
func TestHandleListProviderModels_QueryParams(t *testing.T) {
	database := setupTestDB(t)
	mock := &mockModelProvider{models: []string{"claude-3"}}
	handler := NewRouter(testCfg(), database, nil, nil, mock, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/anthropic/models?apiKey=sk-test", nil)
	req = authRequest(t, req)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestHandleListProviderModels_MissingAPIKey tests error when API key is required
func TestHandleListProviderModels_MissingAPIKey(t *testing.T) {
	database := setupTestDB(t)
	handler := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/openai/models", bytes.NewBufferString(`{}`))
	req = authRequest(t, req)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "API key")
}

// TestHandleListProviderModels_ModelsFetchError tests error handling
func TestHandleListProviderModels_ModelsFetchError(t *testing.T) {
	database := setupTestDB(t)
	mock := &mockModelProvider{err: fmt.Errorf("network error")}
	handler := NewRouter(testCfg(), database, nil, nil, mock, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/openai/models", bytes.NewBufferString(`{"apiKey":"test"}`))
	req = authRequest(t, req)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "network error")
}

// TestHandleListRules_WithFilters tests listing rules with filters
func TestHandleListRules_WithFilters(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	// Create a rule
	rule := db.GuardrailRule{
		TenantID: tenant.ID,
		Name:     "test-rule",
		Action:   "block",
		Enabled:  true,
	}
	database.Create(&rule)

	req := authRequest(t, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/tenants/%d/rules", tenant.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var rules []db.GuardrailRule
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &rules))
	assert.GreaterOrEqual(t, len(rules), 1)
}

// TestHandleDeleteUpstream_NotFound tests deleting non-existent upstream
func TestHandleDeleteUpstream_NotFound(t *testing.T) {
	database := setupTestDB(t)
	r := llmrouter.New([]config.UpstreamConfig{})
	router := NewRouter(testCfg(), database, r, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "tenant1", APIKey: "test"}
	database.Create(&tenant)

	req := authRequest(t, httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/1/upstreams/nonexistent", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Should handle gracefully
	assert.True(t, rr.Code == http.StatusNoContent || rr.Code == http.StatusNotFound)
}

// TestHandleUpdateUpstream_InvalidJSON tests error handling
func TestHandleUpdateUpstream_InvalidJSON(t *testing.T) {
	database := setupTestDB(t)
	r := llmrouter.New([]config.UpstreamConfig{})
	router := NewRouter(testCfg(), database, r, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "tenant1", APIKey: "test"}
	database.Create(&tenant)

	req := authRequest(t, httptest.NewRequest(http.MethodPut, "/api/v1/tenants/1/upstreams/test",
		bytes.NewBufferString(`{invalid json`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleRemoveMember_InvalidUserID tests removing non-existent member
func TestHandleRemoveMember_InvalidUserID(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "test-tenant", APIKey: "test-key"}
	require.NoError(t, database.Create(&tenant).Error)

	req := authRequest(t, httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/tenants/%d/members/999", tenant.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Should handle gracefully
	assert.True(t, rr.Code == http.StatusNoContent || rr.Code == http.StatusNotFound)
}

// TestHandleCreateAgent_MissingName tests validation
func TestHandleCreateAgent_MissingName(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "tenant1"}
	database.Create(&tenant)

	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/tenants/1/agents",
		bytes.NewBufferString(`{}`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Empty name should be rejected or result in validation error
	assert.True(t, rr.Code >= 400)
}

// TestHandleCreateUpstream_InvalidModel tests validation
func TestHandleCreateUpstream_InvalidModel(t *testing.T) {
	database := setupTestDB(t)
	r := llmrouter.New([]config.UpstreamConfig{})
	router := NewRouter(testCfg(), database, r, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "tenant1"}
	database.Create(&tenant)

	// Missing required fields
	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/tenants/1/upstreams",
		bytes.NewBufferString(`{"key_id":"test"}`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Should reject incomplete upstream
	assert.True(t, rr.Code >= 400)
}

// TestHandleGetTenant_NotFound tests getting non-existent tenant
func TestHandleGetTenant_NotFound(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/tenants/999", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestHandleUpdateTenant_NotFound tests updating non-existent tenant
func TestHandleUpdateTenant_NotFound(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := authRequest(t, httptest.NewRequest(http.MethodPatch, "/api/v1/tenants/999",
		bytes.NewBufferString(`{"name":"New Name"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestHandleAddMember_ExistingUser tests adding a member that already exists as a user
func TestHandleAddMember_ExistingUser(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "t1", APIKey: "k1"}
	database.Create(&tenant)

	user := db.User{Name: "Existing", Email: "existing@e.com", PasswordHash: "xxx"}
	database.Create(&user)

	var userRole db.Role
	database.Where("name = ?", "user").First(&userRole)

	body := fmt.Sprintf(`{"email":"existing@e.com","name":"Existing","role_id":%d}`, userRole.ID)
	req := authRequest(t, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/tenants/%d/members", tenant.ID), bytes.NewBufferString(body)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
}

// TestHandleUpdateUpstream_Success tests successfully updating an upstream
func TestHandleUpdateUpstream_Success(t *testing.T) {
	database := setupTestDB(t)
	r := llmrouter.New([]config.UpstreamConfig{})
	router := NewRouter(testCfg(), database, r, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "t1", APIKey: "k1"}
	database.Create(&tenant)

	up := db.Upstream{
		TenantID: tenant.ID,
		KeyID:    "u1",
		Provider: "openai",
		Models:   "gpt-4",
		BaseURL:  "http://orig",
		APIKey:   "k1",
	}
	database.Create(&up)

	body := `{"provider":"anthropic","models":["claude-3"],"base_url":"http://new"}`
	req := authRequest(t, httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/v1/tenants/%d/upstreams/u1", tenant.ID), bytes.NewBufferString(body)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var updated db.Upstream
	database.Where("key_id = ?", "u1").First(&updated)
	assert.Equal(t, "anthropic", updated.Provider)
	assert.Equal(t, "claude-3", updated.Models)
	assert.Equal(t, "http://new", updated.BaseURL)
}

// TestHandleListRules_WithAgentID tests listing rules with agent_id filter
func TestHandleListRules_WithAgentID(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "t1", APIKey: "k1"}
	database.Create(&tenant)

	// Rule for agent 1
	database.Create(&db.GuardrailRule{TenantID: tenant.ID, AgentID: ptrUint(1), Name: "r1", Action: "block", Scope: "input"})
	// Global rule (AgentID nil)
	database.Create(&db.GuardrailRule{TenantID: tenant.ID, AgentID: nil, Name: "r2", Action: "log", Scope: "input"})

	req := authRequest(t, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/tenants/%d/rules?agent_id=1", tenant.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var rules []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &rules)
	// Seeded managed rules (4) + 2 custom rules = 6 total expected for agent 1
	// Wait, managed rules are global.
	assert.GreaterOrEqual(t, len(rules), 2)
}

// TestHandleCreateKey_WithAgentID tests creating a key scoped to an agent
func TestHandleCreateKey_WithAgentID(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "t1", APIKey: "k1"}
	database.Create(&tenant)

	body := `{"name":"scoped-key","agent_id":1}`
	req := authRequest(t, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/tenants/%d/keys", tenant.ID), bytes.NewBufferString(body)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	assert.Equal(t, float64(1), resp["agent_id"])
}

// TestHandleListTenants_Success tests listing tenants
func TestHandleListTenants_Success(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	database.Create(&db.Tenant{Name: "t1", APIKey: "k1"})
	database.Create(&db.Tenant{Name: "t2", APIKey: "k2"})

	req := authRequest(t, httptest.NewRequest(http.MethodGet, "/api/v1/tenants", nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var tenants []db.Tenant
	json.Unmarshal(rr.Body.Bytes(), &tenants)
	assert.Len(t, tenants, 2)
}

// TestHandleDeleteTenant_Success tests successfully deleting a tenant
func TestHandleDeleteTenant_Success(t *testing.T) {
	database := setupTestDB(t)
	r := llmrouter.New([]config.UpstreamConfig{})
	router := NewRouter(testCfg(), database, r, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "to-delete", APIKey: "k"}
	database.Create(&tenant)

	req := authRequest(t, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/tenants/%d", tenant.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}

// TestHandleListUpstreams_Success tests listing upstreams
func TestHandleListUpstreams_Success(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "t1", APIKey: "k1"}
	database.Create(&tenant)

	database.Create(&db.Upstream{TenantID: tenant.ID, KeyID: "u1", Models: "m1", BaseURL: "b1", APIKey: "k1"})

	req := authRequest(t, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/tenants/%d/upstreams", tenant.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var ups []db.Upstream
	json.Unmarshal(rr.Body.Bytes(), &ups)
	assert.Len(t, ups, 1)
}

// TestHandleDeleteRule_Forbidden tests deleting a rule from another tenant
func TestHandleDeleteRule_Forbidden(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	database.Create(&db.Tenant{Model: gorm.Model{ID: 1}, Name: "t1", APIKey: "k1"})
	database.Create(&db.Tenant{Model: gorm.Model{ID: 2}, Name: "t2", APIKey: "k2"})

	rule := db.GuardrailRule{TenantID: 2, Name: "r", Action: "block", Scope: "input"}
	database.Create(&rule)

	req := authRequest(t, httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/1/rules/"+fmt.Sprint(rule.ID), nil))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// TestHandleUpdateRule_Forbidden tests updating a rule from another tenant
func TestHandleUpdateRule_Forbidden(t *testing.T) {
	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	database.Create(&db.Tenant{Model: gorm.Model{ID: 1}, Name: "t1", APIKey: "k1"})
	database.Create(&db.Tenant{Model: gorm.Model{ID: 2}, Name: "t2", APIKey: "k2"})

	rule := db.GuardrailRule{TenantID: 2, Name: "r", Action: "block", Scope: "input"}
	database.Create(&rule)

	body := `{"name":"new"}`
	req := authRequest(t, httptest.NewRequest(http.MethodPatch, "/api/v1/tenants/1/rules/"+fmt.Sprint(rule.ID), bytes.NewBufferString(body)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// TestHandleCreateRule_WithAgentID tests creating a rule scoped to an agent
func TestHandleCreateRule_WithAgentID(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, db.SeedSuperAdmin(database, "admin@example.com", "secret"))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	tenant := db.Tenant{Name: "t1", APIKey: "k1"}
	database.Create(&tenant)

	body := `{"name":"agent-rule","action":"block","scope":["input"],"agent_id":1,"condition":{"type":"keyword","patterns":["bad"]}}`
	req := authRequest(t, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/tenants/%d/rules", tenant.ID), bytes.NewBufferString(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var created db.GuardrailRule
	json.Unmarshal(rr.Body.Bytes(), &created)
	assert.NotNil(t, created.AgentID)
	assert.Equal(t, uint(1), *created.AgentID)
}

// TestHandleSelfServeTenant verifies a user with no tenant can create one and
// becomes its admin, with a default API key and managed rules seeded.
func TestHandleSelfServeTenant(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	require.NoError(t, database.Create(&db.User{Model: gorm.Model{ID: 1}, Name: "Test User", Email: "test-admin@example.com"}).Error)
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	body := `{"name":"Acme Inc.","description":"test org"}`
	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/onboarding/tenant", bytes.NewBufferString(body)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)

	var tenant db.Tenant
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &tenant))
	assert.Equal(t, "Acme Inc.", tenant.Name)
	assert.NotZero(t, tenant.ID)

	// Creator must be a tenantAdmin member of the new tenant.
	var member db.TenantMember
	require.NoError(t, database.Where("tenant_id = ? AND user_id = ?", tenant.ID, 1).First(&member).Error)
	var role db.Role
	require.NoError(t, database.First(&role, member.RoleID).Error)
	assert.Equal(t, "tenantAdmin", role.Name)

	// Default API key and managed guardrail rules must be seeded.
	var keyCount, ruleCount int64
	database.Model(&db.APIKey{}).Where("tenant_id = ?", tenant.ID).Count(&keyCount)
	database.Model(&db.GuardrailRule{}).Where("tenant_id = ?", tenant.ID).Count(&ruleCount)
	assert.Equal(t, int64(1), keyCount)
	assert.Positive(t, ruleCount)
}

// TestHandleSelfServeTenant_MissingName rejects an org with no name.
func TestHandleSelfServeTenant_MissingName(t *testing.T) {
	database := setupTestDB(t)
	require.NoError(t, db.SeedRBAC(database))
	router := NewRouter(testCfg(), database, nil, nil, nil, nil, nil)

	req := authRequest(t, httptest.NewRequest(http.MethodPost, "/api/v1/onboarding/tenant", bytes.NewBufferString(`{"description":"x"}`)))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestTenantAPIKeyNeverSerialized guards the tenant credential-disclosure fix:
// Tenant.APIKey is the plaintext of a live gateway key, so no admin handler may
// return it. Without json:"-" every member of a tenant received a working proxy
// credential from these endpoints, regardless of their keys.read permission.
func TestTenantAPIKeyNeverSerialized(t *testing.T) {
	const secret = "sk-gw-plaintext-must-not-leak"

	database := setupTestDB(t)
	router := NewRouter(testCfg(), database, llmrouter.New([]config.UpstreamConfig{}), nil, nil, nil, nil)

	tenant := db.Tenant{Name: "leaky", APIKey: secret}
	require.NoError(t, database.Create(&tenant).Error)

	for _, path := range []string{"/api/v1/tenants", fmt.Sprintf("/api/v1/tenants/%d/", tenant.ID)} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, authRequest(t, httptest.NewRequest(http.MethodGet, path, nil)))
		require.Equal(t, http.StatusOK, rr.Code, path)
		assert.NotContains(t, rr.Body.String(), secret, "%s leaked the tenant API key", path)
	}

	// The create path mints its own key — assert it is withheld there too.
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(t, httptest.NewRequest(
		http.MethodPost, "/api/v1/tenants", bytes.NewBufferString(`{"name":"fresh"}`))))
	require.Equal(t, http.StatusCreated, rr.Code)

	var created db.Tenant
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))

	var stored db.Tenant
	require.NoError(t, database.First(&stored, created.ID).Error)
	require.NotEmpty(t, stored.APIKey, "tenant should still be provisioned with a key")
	assert.NotContains(t, rr.Body.String(), stored.APIKey, "create response leaked the tenant API key")
}
