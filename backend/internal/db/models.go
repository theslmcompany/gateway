package db

import (
	"time"

	"gorm.io/gorm"
)

// Tenant represents an LLM consumer or project.
type Tenant struct {
	gorm.Model
	Name          string `gorm:"uniqueIndex;not null"`
	Description   string
	// APIKey is the plaintext of the tenant's default gateway key — a live
	// proxy credential (its hash is the one row in APIKeys that authenticates
	// requests). json:"-" because every admin handler that encodes a Tenant
	// would otherwise hand it to any member of the tenant, regardless of role,
	// bypassing the keys.read / keys.manage permissions entirely.
	// TODO: drop this column — APIKeys already stores the hash, and keeping a
	// plaintext credential in the DB has no remaining consumer.
	APIKey        string `gorm:"uniqueIndex;not null" json:"-"`
	WebhookSecret string `gorm:"default:''" json:"-"` // HMAC-SHA256 signing secret for async task callbacks
	Agents        []Agent
	APIKeys       []APIKey
	Upstreams     []Upstream
}

// Agent represents a specific AI agent within a tenant.
type Agent struct {
	gorm.Model
	Name        string `gorm:"not null"`
	Description string
	TenantID    uint `gorm:"not null;index"`
	Tenant      Tenant
	APIKeys     []APIKey
}

// APIKey represents a key used to authenticate requests to the gateway.
type APIKey struct {
	gorm.Model
	KeyHash    string     `gorm:"uniqueIndex;not null" json:"-"`
	Prefix     string     `gorm:"not null" json:"prefix"` // e.g. "sk_"
	Suffix     string     `gorm:"not null" json:"suffix"` // Last 4 characters of the key
	Name       string     `gorm:"not null" json:"name"`
	TenantID   uint       `gorm:"not null;index" json:"tenant_id"`
	Tenant     Tenant     `gorm:"-" json:"-"`
	AgentID    *uint      `gorm:"index" json:"agent_id"` // Optional: if set, key is scoped to this agent
	Agent      Agent      `gorm:"foreignKey:AgentID" json:"-"`
	UserID     *uint      `gorm:"index" json:"user_id"` // Optional: if set, key is scoped to this user
	User       User       `gorm:"foreignKey:UserID" json:"-"`
	LastUsedAt *time.Time `json:"last_used_at"`
	IsActive   bool       `gorm:"default:true" json:"is_active"`
}

// Upstream represents an LLM provider endpoint.
type Upstream struct {
	gorm.Model
	KeyID    string `gorm:"not null;index" json:"key_id"`
	Provider string `gorm:"not null;default:'openai'" json:"provider"`
	Models   string `gorm:"not null" json:"models"` // Comma-separated list of models
	BaseURL  string `gorm:"not null" json:"base_url"`
	APIKey   string `gorm:"not null" json:"-"` // Never export API Key
	TenantID uint   `gorm:"not null;index" json:"tenant_id"`
	Tenant   Tenant `gorm:"-" json:"-"`
}

// User represents a user who can log in to the gateway.
// PasswordHash is empty for SSO-only users (provisioned via OIDC/SAML).
type User struct {
	gorm.Model
	Name         string `gorm:"not null;default:''"`
	Email        string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"default:''"`
	IsSuperAdmin bool   `gorm:"default:false"`
}

// OIDCIdentity links an external IdP subject to a local User.
// Unique on (Provider, Subject) so the same Google/MS account always maps to one user.
type OIDCIdentity struct {
	gorm.Model
	Provider string `gorm:"uniqueIndex:idx_provider_subject;not null"` // "google" | "microsoft"
	Subject  string `gorm:"uniqueIndex:idx_provider_subject;not null"` // IdP-issued stable user ID (`sub` claim)
	UserID   uint   `gorm:"not null;index"`
	User     User
	Email    string // snapshot at link time, for audit
}

// Role represents a collection of permissions.
type Role struct {
	gorm.Model
	Name        string `gorm:"uniqueIndex;not null"`
	Description string
	Permissions []Permission `gorm:"many2many:role_permissions;"`
}

// Permission represents an atomic action that can be performed.
type Permission struct {
	gorm.Model
	Name string `gorm:"uniqueIndex;not null"` // e.g., "tenant.read"
}

// TenantMember maps a user to a tenant with a specific role.
type TenantMember struct {
	gorm.Model
	UserID   uint `gorm:"uniqueIndex:idx_user_tenant;not null"`
	User     User
	TenantID uint `gorm:"uniqueIndex:idx_user_tenant;not null"`
	Tenant   Tenant
	RoleID   uint `gorm:"not null"`
	Role     Role
}

// GuardrailRule defines a policy evaluated against inbound/outbound LLM content.
type GuardrailRule struct {
	gorm.Model
	TenantID  uint   `gorm:"not null;index" json:"tenant_id"`
	AgentID   *uint  `gorm:"index" json:"agent_id"` // nil = applies to all agents
	Name      string `gorm:"not null" json:"name"`
	Priority  int    `gorm:"not null;default:100" json:"priority"`
	Scope     string `gorm:"not null" json:"scope"`                    // CSV: "input", "output", "tool_call"
	Direction string `gorm:"not null;default:'both'" json:"direction"` // "inbound"|"outbound"|"both"
	Condition string `gorm:"type:text" json:"condition"`               // JSON-encoded Condition struct
	Action    string `gorm:"not null" json:"action"`                   // "block"|"log"|"tag"|"rewrite"|"shadow"|"substitute"
	Mode      string `gorm:"not null;default:'parallel'" json:"mode"`
	Managed   bool   `gorm:"default:false" json:"managed"`
	ManagedID string `json:"managed_id"` // e.g., "prompt-injection"
	Version   string `json:"version"`
	Enabled   bool   `gorm:"default:true" json:"enabled"`
}

// AsyncTask tracks a long-running LLM call whose result is delivered via webhook.
// Client receives 202 + task ID immediately; gateway runs upstream call in
// background, persists result, then POSTs signed payload to WebhookURL.
type AsyncTask struct {
	gorm.Model
	TaskID         string     `gorm:"uniqueIndex;not null" json:"id"`        // external opaque ID (UUID)
	TenantID       uint       `gorm:"not null;index" json:"tenant_id"`
	AgentID        string     `json:"agent_id,omitempty"`
	ThreadID       string     `json:"thread_id,omitempty"`
	Endpoint       string     `gorm:"not null" json:"endpoint"`              // "chat.completions" | "messages" | "responses"
	ModelName      string     `gorm:"not null;column:model" json:"model"`
	Status         string     `gorm:"not null;default:'pending';index" json:"status"` // pending|running|succeeded|failed
	WebhookURL     string     `gorm:"not null" json:"webhook_url"`
	RequestBody    []byte     `gorm:"type:bytea" json:"-"`                   // original client request JSON
	ResponseBody   []byte     `gorm:"type:bytea" json:"-"`                   // upstream response JSON (when succeeded)
	ResponseStatus int        `json:"response_status,omitempty"`
	ErrorMessage   string     `json:"error,omitempty"`
	InputTokens    int        `json:"input_tokens,omitempty"`
	OutputTokens   int        `json:"output_tokens,omitempty"`
	CostUSD        float64    `json:"cost_usd,omitempty"`
	WebhookAttempts int       `gorm:"default:0" json:"webhook_attempts"`
	WebhookStatus  string     `gorm:"default:'pending'" json:"webhook_status"` // pending|delivered|dead_letter
	WebhookLastErr string     `json:"webhook_last_error,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	WebhookDeliveredAt *time.Time `json:"webhook_delivered_at,omitempty"`
}

// AutoMigrate runs schema migrations for all models.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Tenant{},
		&Agent{},
		&APIKey{},
		&Upstream{},
		&User{},
		&OIDCIdentity{},
		&Role{},
		&Permission{},
		&TenantMember{},
		&GuardrailRule{},
		&AsyncTask{},
	)
}
