package middleware

import (
	"net/http"
	"net/netip"

	"github.com/PatchMon/PatchMon/server-source-code/internal/clientip"
)

// RealIP rewrites r.RemoteAddr to the resolved client IP so every downstream
// consumer keys on the same trustworthy value.
//
// It replaces chi's middleware.RealIP, which was deprecated because it took the
// leftmost X-Forwarded-For entry, letting a client choose the address PatchMon
// used for rate limiting, login lockout, and audit logging. See the clientip
// package for how resolution works.
//
// Apply this before any middleware that reads the client IP (rate limiting, API
// auth) and only when the deployment actually sits behind a reverse proxy, which
// is what TRUST_PROXY signals.
func RealIP(trustedProxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip := clientip.Resolve(r, trustedProxies); ip != "" {
				r.RemoteAddr = ip
			}
			next.ServeHTTP(w, r)
		})
	}
}
