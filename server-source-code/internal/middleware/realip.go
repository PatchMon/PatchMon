package middleware

import (
	"log/slog"
	"net/http"
	"net/netip"
	"sync"

	"github.com/PatchMon/PatchMon/server-source-code/internal/clientip"
)

const maxUntrustedPeerWarnings = 10

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
func RealIP(trustedProxies []netip.Prefix, log *slog.Logger) func(http.Handler) http.Handler {
	var mu sync.Mutex
	warned := map[string]bool{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if log != nil && len(r.Header.Values("X-Forwarded-For")) > 0 {
				if peer := clientip.FromRequest(r); !clientip.PeerMayForward(peer, trustedProxies) {
					mu.Lock()
					first := !warned[peer] && len(warned) < maxUntrustedPeerWarnings
					if first {
						warned[peer] = true
					}
					mu.Unlock()
					if first {
						log.Warn("ignoring X-Forwarded-For from an untrusted peer; if this is your reverse proxy, add its address to TRUSTED_PROXY_RANGES",
							"peer", peer)
					}
				}
			}
			if ip := clientip.Resolve(r, trustedProxies); ip != "" {
				r.RemoteAddr = ip
			}
			next.ServeHTTP(w, r)
		})
	}
}
