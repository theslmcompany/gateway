package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/chaitanyabankanhal/ai-gateway/config"
)

// serveClientIP runs a request through clientIPMiddleware and reports the
// resolved client IP plus the RemoteAddr the handler observed.
func serveClientIP(t *testing.T, net config.NetworkConfig, r *http.Request) (ip, remoteAddr string) {
	t.Helper()
	h := clientIPMiddleware(net)(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		ip = middleware.GetClientIP(req.Context())
		remoteAddr = req.RemoteAddr
	}))
	h.ServeHTTP(httptest.NewRecorder(), r)
	return ip, remoteAddr
}

// spoofed builds a request whose headers all claim a forged source IP.
func spoofed(remoteAddr string, xff string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.RemoteAddr = remoteAddr
	r.Header.Set("X-Forwarded-For", xff)
	r.Header.Set("X-Real-IP", "1.2.3.4")
	r.Header.Set("True-Client-IP", "1.2.3.4")
	return r
}

// TestClientIPDefaultIgnoresHeaders is the regression test for GO-2026-5775 /
// GO-2026-5777: with no trusted proxy configured, a caller must not be able to
// dictate its own IP via headers.
func TestClientIPDefaultIgnoresHeaders(t *testing.T) {
	ip, remoteAddr := serveClientIP(t, config.NetworkConfig{}, spoofed("203.0.113.9:44321", "1.2.3.4"))

	if ip != "203.0.113.9" {
		t.Errorf("client IP = %q, want the TCP peer 203.0.113.9 (headers must be ignored)", ip)
	}
	if remoteAddr != "203.0.113.9:44321" {
		t.Errorf("RemoteAddr = %q, want it left unmodified", remoteAddr)
	}
}

func TestClientIPTrustedHeader(t *testing.T) {
	net := config.NetworkConfig{TrustedClientIPHeader: "CF-Connecting-IP"}

	r := spoofed("10.0.0.7:1234", "1.2.3.4")
	r.Header.Set("CF-Connecting-IP", "198.51.100.5")
	if ip, _ := serveClientIP(t, net, r); ip != "198.51.100.5" {
		t.Errorf("client IP = %q, want 198.51.100.5 from the trusted header", ip)
	}

	// Header absent → fail closed, no client IP rather than an XFF fallback.
	if ip, _ := serveClientIP(t, net, spoofed("10.0.0.7:1234", "1.2.3.4")); ip != "" {
		t.Errorf("client IP = %q, want empty when the trusted header is absent", ip)
	}
}

func TestClientIPTrustedCIDRs(t *testing.T) {
	net := config.NetworkConfig{TrustedProxyCIDRs: []string{"10.0.0.0/8"}}

	// Real client 198.51.100.5, then one trusted proxy hop. The attacker
	// prepended 1.2.3.4 hoping to be read as the leftmost entry.
	r := spoofed("10.0.0.7:1234", "1.2.3.4, 198.51.100.5, 10.0.0.7")
	if ip, _ := serveClientIP(t, net, r); ip != "198.51.100.5" {
		t.Errorf("client IP = %q, want 198.51.100.5 (rightmost entry outside the trusted CIDRs)", ip)
	}
}

func TestClientIPTrustedProxyCount(t *testing.T) {
	net := config.NetworkConfig{TrustedProxyCount: 1}

	r := spoofed("10.0.0.7:1234", "1.2.3.4, 198.51.100.5")
	if ip, _ := serveClientIP(t, net, r); ip != "198.51.100.5" {
		t.Errorf("client IP = %q, want 198.51.100.5 (entry added by the single trusted hop)", ip)
	}

	// Chain shorter than the configured hop count → fail closed.
	short := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	short.RemoteAddr = "10.0.0.7:1234"
	if ip, _ := serveClientIP(t, net, short); ip != "" {
		t.Errorf("client IP = %q, want empty when X-Forwarded-For is missing", ip)
	}
}

// TestClientIPPrecedence pins the documented ordering when several settings
// are supplied at once.
func TestClientIPPrecedence(t *testing.T) {
	net := config.NetworkConfig{
		TrustedClientIPHeader: "X-Real-IP",
		TrustedProxyCIDRs:     []string{"10.0.0.0/8"},
		TrustedProxyCount:     1,
	}
	if ip, _ := serveClientIP(t, net, spoofed("10.0.0.7:1234", "198.51.100.5, 10.0.0.7")); ip != "1.2.3.4" {
		t.Errorf("client IP = %q, want the trusted header to win", ip)
	}

	net.TrustedClientIPHeader = ""
	if ip, _ := serveClientIP(t, net, spoofed("10.0.0.7:1234", "198.51.100.5, 10.0.0.7")); ip != "198.51.100.5" {
		t.Errorf("client IP = %q, want CIDRs to win over the hop count", ip)
	}
}
