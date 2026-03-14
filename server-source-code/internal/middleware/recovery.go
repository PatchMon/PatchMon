package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
)

// Recovery returns a middleware that recovers from panics.
// For /api routes, returns JSON so piped install scripts don't execute error text as shell commands.
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error("panic recovered",
						"error", err,
						"path", r.URL.Path,
						"method", r.Method,
						"stack", string(debug.Stack()))
					if strings.HasPrefix(r.URL.Path, "/api") {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusInternalServerError)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal Server Error"})
					} else {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
