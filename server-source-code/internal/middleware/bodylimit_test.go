package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/go-chi/chi/v5"
)

func resolverWith(rc *config.ResolvedConfig) *hostctx.ConfigResolver {
	return hostctx.NewConfigResolver(&config.Config{}, rc, nil)
}

func TestBodyLimitFor_OversizedBodySurfacesMaxBytesError(t *testing.T) {
	t.Parallel()

	resolver := resolverWith(&config.ResolvedConfig{ComplianceBodyLimitBytes: 64})

	var status int
	var limit int64
	h := BodyLimitFor(resolver, func(rc *config.ResolvedConfig) int64 {
		return rc.ComplianceBodyLimitBytes
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		err := json.NewDecoder(r.Body).Decode(&payload)
		if err == nil {
			status = http.StatusOK
			return
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
			limit = tooLarge.Limit
			return
		}
		status = http.StatusBadRequest
	}))

	body := `{"scans":[{"results":"` + strings.Repeat("x", 512) + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/compliance/scans", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", status, http.StatusRequestEntityTooLarge)
	}
	if limit != 64 {
		t.Errorf("reported limit = %d, want 64", limit)
	}
}

func TestBodyLimitFor_BodyUnderLimitPassesThrough(t *testing.T) {
	t.Parallel()

	resolver := resolverWith(&config.ResolvedConfig{ComplianceBodyLimitBytes: 20 * 1024 * 1024})

	var decoded bool
	h := BodyLimitFor(resolver, func(rc *config.ResolvedConfig) int64 {
		return rc.ComplianceBodyLimitBytes
	})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			decoded = true
		}
	}))

	req := httptest.NewRequest(http.MethodPost, "/compliance/scans", strings.NewReader(`{"scans":[]}`))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !decoded {
		t.Error("body under the limit did not decode")
	}
}

func TestBodyLimitFor_RouteLimitOverridesGroupLimit(t *testing.T) {
	t.Parallel()

	res := resolverWith(&config.ResolvedConfig{
		JSONBodyLimitBytes:       5 * 1024 * 1024,
		ComplianceBodyLimitBytes: 20 * 1024 * 1024,
	})

	var status int
	var reported int64
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var p map[string]any
		err := json.NewDecoder(r.Body).Decode(&p)
		if err == nil {
			status = http.StatusOK
			return
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status, reported = http.StatusRequestEntityTooLarge, tooLarge.Limit
			return
		}
		status = http.StatusBadRequest
	})

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(BodyLimitFor(res, func(c *config.ResolvedConfig) int64 { return c.JSONBodyLimitBytes }))
		r.With(BodyLimitFor(res, func(c *config.ResolvedConfig) int64 { return c.ComplianceBodyLimitBytes })).
			Post("/compliance/scans", handler)
	})

	body := `{"x":"` + strings.Repeat("a", 12*1024*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/scans", strings.NewReader(body))
	r.ServeHTTP(httptest.NewRecorder(), req)

	if status != http.StatusOK {
		t.Fatalf("12MB body: status = %d (limit reported %d), want 200. The route limit is not overriding the group limit.", status, reported)
	}
}

func TestBodyLimitFor_RouteLimitStillRejectsAboveItsOwnCeiling(t *testing.T) {
	t.Parallel()

	res := resolverWith(&config.ResolvedConfig{
		JSONBodyLimitBytes:       5 * 1024 * 1024,
		ComplianceBodyLimitBytes: 20 * 1024 * 1024,
	})

	var status int
	var reported int64
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var p map[string]any
		err := json.NewDecoder(r.Body).Decode(&p)
		if err == nil {
			status = http.StatusOK
			return
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status, reported = http.StatusRequestEntityTooLarge, tooLarge.Limit
			return
		}
		status = http.StatusBadRequest
	})

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(BodyLimitFor(res, func(c *config.ResolvedConfig) int64 { return c.JSONBodyLimitBytes }))
		r.With(BodyLimitFor(res, func(c *config.ResolvedConfig) int64 { return c.ComplianceBodyLimitBytes })).
			Post("/compliance/scans", handler)
	})

	body := `{"x":"` + strings.Repeat("a", 21*1024*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/scans", strings.NewReader(body))
	r.ServeHTTP(httptest.NewRecorder(), req)

	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("21MB body: status = %d, want 413", status)
	}
	if reported != 20*1024*1024 {
		t.Errorf("413 named limit %d, want the configured 20mb (%d)", reported, 20*1024*1024)
	}
}

// A route without its own limit must still inherit the group's.
func TestBodyLimitFor_GroupLimitStillAppliesWithoutRouteOverride(t *testing.T) {
	t.Parallel()

	res := resolverWith(&config.ResolvedConfig{JSONBodyLimitBytes: 1024})

	var status int
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var p map[string]any
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				status = http.StatusRequestEntityTooLarge
				return
			}
			status = http.StatusBadRequest
			return
		}
		status = http.StatusOK
	})

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(BodyLimitFor(res, func(c *config.ResolvedConfig) int64 { return c.JSONBodyLimitBytes }))
		r.Post("/settings", handler)
	})

	body := `{"x":"` + strings.Repeat("a", 4096) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings", strings.NewReader(body))
	r.ServeHTTP(httptest.NewRecorder(), req)

	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 from the inherited group limit", status)
	}
}
