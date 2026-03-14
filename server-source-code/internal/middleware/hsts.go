package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	// HSTSMaxAgeOneYear is the standard max-age for HSTS (31536000 seconds).
	HSTSMaxAgeOneYear = 31536000
)

// HSTS returns middleware that adds Strict-Transport-Security when enabled and request is HTTPS.
func HSTS(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}
			secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
			if secure {
				w.Header().Set("Strict-Transport-Security", "max-age="+strconv.Itoa(HSTSMaxAgeOneYear)+"; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
