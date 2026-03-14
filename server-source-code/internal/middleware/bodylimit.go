package middleware

import (
	"io"
	"net/http"
)

// BodyLimit returns middleware that limits the request body size.
func BodyLimit(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// BodyLimitReader wraps r.Body with io.LimitReader. Use when you need a custom limit
// that differs from the default MaxBytesReader behavior (e.g. for streaming).
func BodyLimitReader(r *http.Request, limit int64) io.Reader {
	if limit <= 0 {
		return r.Body
	}
	return io.LimitReader(r.Body, limit)
}
