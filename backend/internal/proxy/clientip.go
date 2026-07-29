package proxy

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/chaitanyabankanhal/ai-gateway/config"
)

// clientIPMiddleware resolves the caller's IP according to the deployment's
// trust boundary and stores it in the request context (read it with
// middleware.GetClientIP).
//
// It replaces chi's deprecated middleware.RealIP, which rewrote r.RemoteAddr
// from an unvalidated X-Forwarded-For / X-Real-IP / True-Client-IP header —
// letting any caller claim any source IP (GO-2026-5775, GO-2026-5777). None
// of the middlewares used here mutate r.RemoteAddr, so the TCP peer address
// stays available as ground truth for telemetry.
//
// Precedence, most to least trustworthy:
//  1. a single-IP header the proxy overwrites on every request
//  2. X-Forwarded-For with the proxy ranges enumerated as CIDRs
//  3. X-Forwarded-For with a fixed number of proxy hops
//  4. nothing configured — the TCP peer address; headers are ignored
func clientIPMiddleware(net config.NetworkConfig) func(http.Handler) http.Handler {
	switch {
	case net.TrustedClientIPHeader != "":
		slog.Info("client IP resolution: trusted header", "header", net.TrustedClientIPHeader)
		return middleware.ClientIPFromHeader(net.TrustedClientIPHeader)

	case len(net.TrustedProxyCIDRs) > 0:
		slog.Info("client IP resolution: X-Forwarded-For with trusted proxy CIDRs",
			"cidrs", net.TrustedProxyCIDRs)
		// CIDRs are validated in config.Load, so the MustParsePrefix inside
		// cannot panic on a running gateway.
		return middleware.ClientIPFromXFF(net.TrustedProxyCIDRs...)

	case net.TrustedProxyCount > 0:
		slog.Info("client IP resolution: X-Forwarded-For with fixed hop count",
			"hops", net.TrustedProxyCount)
		return middleware.ClientIPFromXFFTrustedProxies(net.TrustedProxyCount)

	default:
		slog.Info("client IP resolution: TCP peer address; forwarded-for headers ignored " +
			"(set TRUSTED_PROXY_CIDRS, TRUSTED_PROXY_COUNT or TRUSTED_CLIENT_IP_HEADER when behind a proxy)")
		return middleware.ClientIPFromRemoteAddr
	}
}
