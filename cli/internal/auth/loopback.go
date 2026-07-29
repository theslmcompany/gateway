package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pkg/browser"
)

// loginTimeout bounds how long BrowserLogin waits for the user to finish SSO.
const loginTimeout = 3 * time.Minute

type loginOutcome struct {
	token string
	err   error
}

// BrowserLogin runs the loopback OIDC flow: it starts a local HTTP listener,
// opens the browser at the gateway's OIDC login URL pointed back at that
// listener, and returns the JWT the gateway redirects in once SSO completes.
func BrowserLogin(ctx context.Context, endpoint, provider string) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start loopback listener: %w", err)
	}
	defer ln.Close()

	cliState, err := randHex(16)
	if err != nil {
		return "", err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback?cli_state=%s", port, cliState)

	outcome := make(chan loginOutcome, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("cli_state") != cliState:
			resultPage(w, "Login failed: state mismatch.")
			outcome <- loginOutcome{err: errors.New("cli_state mismatch — possible forged callback")}
		case q.Get("error") != "":
			e := q.Get("error")
			resultPage(w, "Login failed: "+e)
			outcome <- loginOutcome{err: fmt.Errorf("identity provider error: %s", e)}
		case q.Get("token") == "":
			resultPage(w, "Login failed: no token returned.")
			outcome <- loginOutcome{err: errors.New("no token in gateway callback")}
		default:
			resultPage(w, "Login successful. You can close this tab and return to the terminal.")
			outcome <- loginOutcome{token: q.Get("token")}
		}
	})

	srv := &http.Server{
		Handler: mux,
		// Bounded so a stalled connection can't hold the listener open for the
		// full login window (slowloris, gosec G112).
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	loginURL := fmt.Sprintf("%s/api/v1/auth/oidc/%s/login?cli_redirect=%s",
		strings.TrimRight(endpoint, "/"), url.PathEscape(provider), url.QueryEscape(redirect))

	fmt.Fprintln(os.Stderr, "Opening browser to complete login...")
	fmt.Fprintln(os.Stderr, "If it does not open, visit this URL manually:\n  "+loginURL)
	_ = browser.OpenURL(loginURL)

	ctx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("login timed out or cancelled: %w", ctx.Err())
	case res := <-outcome:
		return res.token, res.err
	}
}

// randHex returns n cryptographically random bytes, hex-encoded.
func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func resultPage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// msg can carry the `error` query parameter straight from the callback URL,
	// so it must be escaped — otherwise a crafted login link reflects script
	// into this page, on the loopback origin holding the session token.
	fmt.Fprintf(w, `<!doctype html><html><head><title>aigw login</title></head>`+
		`<body style="font-family:system-ui,sans-serif;text-align:center;padding-top:4rem">`+
		`<h2>%s</h2></body></html>`, template.HTMLEscapeString(msg))
}
