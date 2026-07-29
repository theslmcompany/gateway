// Package auth handles the aigw CLI's browser SSO login flow and the
// secure storage of the resulting session credentials.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/99designs/keyring"
	cliconfig "github.com/chaitanyabankanhal/ai-gateway/cli/internal/config"
)

const (
	keyringService = "aigw"
	credItemKey    = "credentials"
)

// ErrNotLoggedIn is returned when no stored session exists.
var ErrNotLoggedIn = errors.New("not logged in — run `aigw login`")

// Credentials is a persisted login session.
type Credentials struct {
	Token    string    `json:"token"`    // gateway JWT
	Endpoint string    `json:"endpoint"` // admin API base URL the token was minted for
	Expiry   time.Time `json:"expiry"`   // JWT expiry, for proactive warnings
}

// Expired reports whether the session's JWT has passed its expiry.
func (c Credentials) Expired() bool {
	return !c.Expiry.IsZero() && time.Now().After(c.Expiry)
}

// openKeyring opens the OS keyring, falling back to an encrypted file backend
// (~/.config/aigw/keyring) on systems without a native secret service.
func openKeyring() (keyring.Keyring, error) {
	dir, err := cliconfig.Dir()
	if err != nil {
		return nil, err
	}
	cfg := keyring.Config{
		ServiceName:              keyringService,
		KeychainName:             keyringService,
		KeychainTrustApplication: true,
		FileDir:                  filepath.Join(dir, "keyring"),
		FilePasswordFunc:         filePassword,
	}
	// AIGW_KEYRING_BACKEND pins the backend — needed on headless machines
	// where the auto-detected secret service is unusable (e.g. `file`).
	if b := os.Getenv("AIGW_KEYRING_BACKEND"); b != "" {
		cfg.AllowedBackends = []keyring.BackendType{keyring.BackendType(b)}
	}
	return keyring.Open(cfg)
}

// filePassword supplies the passphrase for the encrypted file backend. It
// prefers AIGW_KEYRING_PASSPHRASE for non-interactive use, else prompts.
func filePassword(prompt string) (string, error) {
	if p := os.Getenv("AIGW_KEYRING_PASSPHRASE"); p != "" {
		return p, nil
	}
	return keyring.TerminalPrompt(prompt)
}

// Save persists the session credentials.
func Save(c Credentials) error {
	kr, err := openKeyring()
	if err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return kr.Set(keyring.Item{
		Key:   credItemKey,
		Data:  data,
		Label: "aigw login session",
	})
}

// Load returns the stored session, or ErrNotLoggedIn if none exists.
func Load() (*Credentials, error) {
	kr, err := openKeyring()
	if err != nil {
		return nil, err
	}
	item, err := kr.Get(credItemKey)
	if errors.Is(err, keyring.ErrKeyNotFound) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var c Credentials
	if err := json.Unmarshal(item.Data, &c); err != nil {
		return nil, fmt.Errorf("corrupt credentials (run `aigw login` again): %w", err)
	}
	return &c, nil
}

// Clear removes the stored session. It is a no-op when nothing is stored.
func Clear() error {
	kr, err := openKeyring()
	if err != nil {
		return err
	}
	if err := kr.Remove(credItemKey); err != nil && !isNotFound(err) {
		return fmt.Errorf("clear credentials: %w", err)
	}
	return nil
}

// ProxyKey is a cached gateway API key whose plaintext is reused by
// `aigw env` / `aigw run` to point SDKs at the proxy without minting a
// fresh key on every invocation.
type ProxyKey struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"` // plaintext, only obtainable at creation time
}

// proxyKeyItem is the keyring item key for a tenant's cached proxy key.
func proxyKeyItem(tenant string) string {
	return "proxykey/" + tenant
}

// SaveProxyKey caches the plaintext gateway key for a tenant.
func SaveProxyKey(tenant string, k ProxyKey) error {
	kr, err := openKeyring()
	if err != nil {
		return err
	}
	data, err := json.Marshal(k)
	if err != nil {
		return err
	}
	return kr.Set(keyring.Item{
		Key:   proxyKeyItem(tenant),
		Data:  data,
		Label: "aigw proxy key (tenant " + tenant + ")",
	})
}

// LoadProxyKey returns the cached proxy key for a tenant, or ErrKeyNotFound
// (wrapped) when none is cached.
func LoadProxyKey(tenant string) (*ProxyKey, error) {
	kr, err := openKeyring()
	if err != nil {
		return nil, err
	}
	item, err := kr.Get(proxyKeyItem(tenant))
	if err != nil {
		return nil, err // includes keyring.ErrKeyNotFound for callers to test
	}
	var k ProxyKey
	if err := json.Unmarshal(item.Data, &k); err != nil {
		return nil, fmt.Errorf("corrupt cached proxy key: %w", err)
	}
	return &k, nil
}

// ClearProxyKey removes the cached proxy key for a tenant. No-op when absent.
func ClearProxyKey(tenant string) error {
	kr, err := openKeyring()
	if err != nil {
		return err
	}
	if err := kr.Remove(proxyKeyItem(tenant)); err != nil && !isNotFound(err) {
		return fmt.Errorf("clear cached proxy key: %w", err)
	}
	return nil
}

// ErrProxyKeyNotCached reports whether err means no proxy key is cached.
func ErrProxyKeyNotCached(err error) bool {
	return isNotFound(err)
}

// isNotFound normalises "no such item" across keyring backends. The encrypted
// file backend surfaces a raw fs.ErrNotExist from Remove instead of
// keyring.ErrKeyNotFound, which would otherwise make `aigw logout` fail when
// there is nothing to remove.
func isNotFound(err error) bool {
	return errors.Is(err, keyring.ErrKeyNotFound) || errors.Is(err, fs.ErrNotExist)
}
