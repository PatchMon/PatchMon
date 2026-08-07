package middleware

import (
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Timeout returns a request timeout middleware that skips WebSocket upgrades.
// WebSocket connections are long-lived and must not be interrupted by a
// request-level timeout. Attempting to write a timeout response to a hijacked
// connection causes "response.WriteHeader on hijacked connection" errors.
func Timeout(dt time.Duration) func(http.Handler) http.Handler {
	timeoutMw := chimw.Timeout(dt)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isWebSocketUpgrade(r) {
				next.ServeHTTP(w, r)
				return
			}
			timeoutMw(next).ServeHTTP(w, r)
		})
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}
