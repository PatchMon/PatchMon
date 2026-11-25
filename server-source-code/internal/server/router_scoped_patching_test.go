package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestRouterScopedPatchingAuthorization(t *testing.T) {
	f := newScopedAPIFixture(t)
	path := scopedAPIPrefix + "/patching/trigger"
	readOnly := f.credential("api", map[string][]string{"host": {"get"}})
	patcher := f.credential("api", map[string][]string{"patching": {"trigger"}})
	enrollment := f.credential("direct-host", map[string][]string{"patching": {"trigger"}})
	invalid := patcher
	invalid.secret = "wrong-secret"

	// Invalid request bodies are intentional: no real patch jobs are enqueued.
	f.wantStatus(f.request(http.MethodPost, path, `{}`, nil, false), http.StatusUnauthorized)
	f.wantStatus(f.request(http.MethodPost, path, `{}`, &invalid, false), http.StatusUnauthorized)
	f.wantStatus(f.request(http.MethodPost, path, `{}`, &enrollment, false), http.StatusUnauthorized)
	f.wantStatus(f.request(http.MethodPost, path, `{}`, &readOnly, false), http.StatusForbidden)
	w := f.request(http.MethodPost, path, `{}`, &patcher, false)
	f.wantStatus(w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "Valid host_id is required") {
		t.Fatalf("scoped request did not reach the native trigger: %s", w.Body.String())
	}
	// The native JWT route stays protected and is not replaced by Basic Auth.
	f.wantStatus(f.request(http.MethodPost, "/api/v1/patching/trigger", `{}`, &patcher, false), http.StatusUnauthorized)
	f.wantStatus(f.request(http.MethodPost, "/api/v1/patching/trigger", `{}`, nil, true), http.StatusBadRequest)
	// No other patch-run management operations are granted by this scope.
	f.wantStatus(f.request(http.MethodPost, scopedAPIPrefix+"/patching/runs/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/approve", `{}`, &patcher, false), http.StatusNotFound)
}
