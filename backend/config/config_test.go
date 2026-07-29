package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadUpstreams(t *testing.T) {
	keys := []string{
		"UPSTREAM_0_KEY_ID", "UPSTREAM_0_MODEL", "UPSTREAM_0_BASE_URL", "UPSTREAM_0_API_KEY",
		"UPSTREAM_1_KEY_ID",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}

	t.Run("no upstreams configured", func(t *testing.T) {
		ups := loadUpstreams()
		assert.Empty(t, ups)
	})

	t.Run("single upstream", func(t *testing.T) {
		t.Setenv("UPSTREAM_0_KEY_ID", "openai-primary")
		t.Setenv("UPSTREAM_0_MODEL", "gpt-4o")
		t.Setenv("UPSTREAM_0_BASE_URL", "https://api.openai.com")
		t.Setenv("UPSTREAM_0_API_KEY", "sk-test")

		ups := loadUpstreams()
		assert.Len(t, ups, 1)
		assert.Equal(t, "openai-primary", ups[0].KeyID)
		assert.Equal(t, "gpt-4o", ups[0].Model)
		assert.Equal(t, "https://api.openai.com", ups[0].BaseURL)
		assert.Equal(t, "sk-test", ups[0].APIKey)
	})

	t.Run("stops at gap", func(t *testing.T) {
		t.Setenv("UPSTREAM_0_KEY_ID", "first")
		t.Setenv("UPSTREAM_0_MODEL", "gpt-4o")
		// UPSTREAM_1_KEY_ID not set → stops at 1

		ups := loadUpstreams()
		assert.Len(t, ups, 1)
	})

	t.Run("base_url defaults to openai", func(t *testing.T) {
		t.Setenv("UPSTREAM_0_KEY_ID", "k")
		t.Setenv("UPSTREAM_0_MODEL", "gpt-4o")
		// BASE_URL not set

		ups := loadUpstreams()
		assert.Equal(t, "https://api.openai.com", ups[0].BaseURL)
	})
}

func TestLoad(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		// Clear env vars to ensure defaults are used
		vars := []string{"ENV", "PROXY_PORT", "ADMIN_PORT", "OTEL_COLLECTOR_ADDR", "OTEL_SERVICE_NAME", "API_KEY_CACHE_TTL_SEC", "REDIS_ADDR", "POSTGRES_DSN"}
		for _, v := range vars {
			t.Setenv(v, "")
		}

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, "prod", cfg.Env)
		assert.Equal(t, 8080, cfg.ProxyPort)
		assert.Equal(t, 8081, cfg.AdminPort)
		assert.Equal(t, "localhost:4317", cfg.Telemetry.CollectorAddr)
		assert.Equal(t, "ai-gateway", cfg.Telemetry.ServiceName)
		assert.Equal(t, 300*time.Second, cfg.Auth.CacheTTL)
		assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
		assert.Equal(t, "postgres://gateway:gateway@localhost:6432/gateway?sslmode=disable", cfg.Postgres.DSN)
	})

	t.Run("override via env", func(t *testing.T) {
		t.Setenv("ENV", "test")
		t.Setenv("PROXY_PORT", "9090")
		t.Setenv("ADMIN_PORT", "9091")
		t.Setenv("OTEL_COLLECTOR_ADDR", "otel:4317")
		t.Setenv("OTEL_SERVICE_NAME", "test-gateway")
		t.Setenv("API_KEY_CACHE_TTL_SEC", "60")
		t.Setenv("REDIS_ADDR", "redis:6379")
		t.Setenv("POSTGRES_DSN", "postgres://user:pass@host:5432/db")

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, "test", cfg.Env)
		assert.Equal(t, 9090, cfg.ProxyPort)
		assert.Equal(t, 9091, cfg.AdminPort)
		assert.Equal(t, "otel:4317", cfg.Telemetry.CollectorAddr)
		assert.Equal(t, "test-gateway", cfg.Telemetry.ServiceName)
		assert.Equal(t, 60*time.Second, cfg.Auth.CacheTTL)
		assert.Equal(t, "redis:6379", cfg.Redis.Addr)
		assert.Equal(t, "postgres://user:pass@host:5432/db", cfg.Postgres.DSN)
	})
}

func TestIntEnv(t *testing.T) {
	t.Run("valid integer", func(t *testing.T) {
		t.Setenv("TEST_INT_VALID", "42")
		val := intEnv("TEST_INT_VALID", 10)
		assert.Equal(t, 42, val)
	})

	t.Run("invalid integer falls back to default", func(t *testing.T) {
		t.Setenv("TEST_INT_INVALID", "abc")
		val := intEnv("TEST_INT_INVALID", 10)
		assert.Equal(t, 10, val)
	})

	t.Run("unset falls back to default", func(t *testing.T) {
		t.Setenv("TEST_INT_UNSET", "")
		val := intEnv("TEST_INT_UNSET", 10)
		assert.Equal(t, 10, val)
	})
}

func TestLoadJWTSecret(t *testing.T) {
	t.Run("from env", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "explicit-secret")
		assert.Equal(t, "explicit-secret", loadJWTSecret())
	})

	t.Run("randomly generated", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "")
		s1 := loadJWTSecret()
		s2 := loadJWTSecret()
		assert.NotEmpty(t, s1)
		assert.NotEmpty(t, s2)
		assert.NotEqual(t, s1, s2)
		assert.Len(t, s1, 64) // 32 bytes hex encoded
	})
}

func TestLoadNetwork(t *testing.T) {
	for _, k := range []string{"TRUSTED_PROXY_CIDRS", "TRUSTED_PROXY_COUNT", "TRUSTED_CLIENT_IP_HEADER"} {
		t.Setenv(k, "")
	}

	t.Run("defaults to trusting nothing", func(t *testing.T) {
		n, err := loadNetwork()
		assert.NoError(t, err)
		assert.False(t, n.Configured())
		assert.Empty(t, n.TrustedProxyCIDRs)
		assert.Zero(t, n.TrustedProxyCount)
	})

	t.Run("parses CIDR list", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 2400:cb00::/32 ,")
		n, err := loadNetwork()
		assert.NoError(t, err)
		assert.Equal(t, []string{"10.0.0.0/8", "2400:cb00::/32"}, n.TrustedProxyCIDRs)
		assert.True(t, n.Configured())
	})

	t.Run("rejects malformed CIDR", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8,not-a-cidr")
		_, err := loadNetwork()
		assert.ErrorContains(t, err, "not-a-cidr")
	})

	t.Run("bare IP is not a CIDR", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.7")
		_, err := loadNetwork()
		assert.Error(t, err)
	})

	t.Run("parses hop count", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXY_CIDRS", "")
		t.Setenv("TRUSTED_PROXY_COUNT", "2")
		n, err := loadNetwork()
		assert.NoError(t, err)
		assert.Equal(t, 2, n.TrustedProxyCount)
	})

	t.Run("rejects non-positive hop count", func(t *testing.T) {
		for _, v := range []string{"0", "-1", "many"} {
			t.Setenv("TRUSTED_PROXY_COUNT", v)
			_, err := loadNetwork()
			assert.Error(t, err, "TRUSTED_PROXY_COUNT=%s should be rejected", v)
		}
	})
}
