package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/99designs/keyring"
)

// useFileBackend pins the encrypted-file keyring backend into a temp config
// dir so the test never touches the developer's real OS keychain.
func useFileBackend(t *testing.T) {
	t.Helper()
	t.Setenv("AIGW_KEYRING_BACKEND", string(keyring.FileBackend))
	t.Setenv("AIGW_KEYRING_PASSPHRASE", "test-passphrase")
	// os.UserConfigDir honours XDG_CONFIG_HOME on Linux and HOME on macOS.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
}

// TestFileBackendRoundTrip guards the jose2go crypto path used by the
// encrypted-file fallback (see E13.S5 — jose2go 1.5.0 -> 1.7.0).
func TestFileBackendRoundTrip(t *testing.T) {
	useFileBackend(t)

	want := Credentials{
		Token:    "header.payload.signature",
		Endpoint: "https://gateway.example.com",
		Expiry:   time.Now().Add(time.Hour).UTC().Truncate(time.Second),
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Token != want.Token || got.Endpoint != want.Endpoint || !got.Expiry.Equal(want.Expiry) {
		t.Fatalf("round trip mismatch: got %+v want %+v", *got, want)
	}
	if got.Expired() {
		t.Error("credentials with a future expiry reported as expired")
	}

	if err := Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := Load(); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("after Clear, Load err = %v, want ErrNotLoggedIn", err)
	}
	if err := Clear(); err != nil {
		t.Errorf("Clear on empty keyring should be a no-op, got %v", err)
	}
}

// TestProxyKeyRoundTrip covers the per-tenant cached proxy key, the other
// jose2go Encrypt/Decrypt call site.
func TestProxyKeyRoundTrip(t *testing.T) {
	useFileBackend(t)

	want := ProxyKey{ID: 7, Name: "aigw-cli", Key: "sk-gw-abc123"}
	if err := SaveProxyKey("42", want); err != nil {
		t.Fatalf("SaveProxyKey: %v", err)
	}

	got, err := LoadProxyKey("42")
	if err != nil {
		t.Fatalf("LoadProxyKey: %v", err)
	}
	if *got != want {
		t.Fatalf("round trip mismatch: got %+v want %+v", *got, want)
	}

	if _, err := LoadProxyKey("99"); !ErrProxyKeyNotCached(err) {
		t.Fatalf("LoadProxyKey for uncached tenant: got %v, want key-not-found", err)
	}

	if err := ClearProxyKey("42"); err != nil {
		t.Fatalf("ClearProxyKey: %v", err)
	}
	if _, err := LoadProxyKey("42"); !ErrProxyKeyNotCached(err) {
		t.Fatalf("after ClearProxyKey: got %v, want key-not-found", err)
	}
}
