package handler

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// decodedURLParam returns a chi URL param decoded once. chi matches on RawPath
// when the request has one, so the param is still percent-encoded in that case.
func decodedURLParam(r *http.Request, key string) string {
	v := chi.URLParam(r, key)
	if r.URL.RawPath == "" {
		return v
	}
	if decoded, err := url.PathUnescape(v); err == nil {
		return decoded
	}
	return v
}
