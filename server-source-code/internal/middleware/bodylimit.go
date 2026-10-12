package middleware

import (
	"context"
	"io"
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
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

type unlimitedBodyKey struct{}

// BodyLimitFor limits the request body using the calling context's setting.
// MaxBytesReader composes rather than replaces, so rewrapping the stashed
// original is what lets a route limit raise, not just tighten, a group limit.
func BodyLimitFor(cfgResolver *hostctx.ConfigResolver, pick func(*config.ResolvedConfig) int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resolved := cfgResolver.Resolve(r.Context())
			if resolved == nil {
				next.ServeHTTP(w, r)
				return
			}
			limit := pick(resolved)
			if limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			body := r.Body
			if orig, ok := r.Context().Value(unlimitedBodyKey{}).(io.ReadCloser); ok {
				body = orig
			} else {
				r = r.WithContext(context.WithValue(r.Context(), unlimitedBodyKey{}, body))
			}
			r.Body = http.MaxBytesReader(w, body, limit)
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
