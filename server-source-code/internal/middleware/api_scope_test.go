package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

func TestRequireApiScope(t *testing.T) {
	tests := []struct {
		name     string
		scopes   string
		resource string
		action   string
		want     int
	}{
		{"existing host get", `{"host":["get"]}`, "host", "get", http.StatusNoContent},
		{"existing host delete", `{"host":["delete"]}`, "host", "delete", http.StatusNoContent},
		{"host get cannot delete", `{"host":["get"]}`, "host", "delete", http.StatusForbidden},
		{"host permission cannot read groups", `{"host":["get"]}`, "host_group", "get", http.StatusForbidden},
		{"group get cannot create", `{"host_group":["get"]}`, "host_group", "create", http.StatusForbidden},
		{"group create cannot update", `{"host_group":["create"]}`, "host_group", "update", http.StatusForbidden},
		{"group update cannot delete", `{"host_group":["update"]}`, "host_group", "delete", http.StatusForbidden},
		{"token get cannot create", `{"enrollment_token":["get"]}`, "enrollment_token", "create", http.StatusForbidden},
		{"token create cannot update", `{"enrollment_token":["create"]}`, "enrollment_token", "update", http.StatusForbidden},
		{"token update cannot delete", `{"enrollment_token":["update"]}`, "enrollment_token", "delete", http.StatusForbidden},
		{"group does not grant token access", `{"host_group":["get"]}`, "enrollment_token", "get", http.StatusForbidden},
		{"no scopes", ``, "host", "get", http.StatusForbidden},
		{"empty scopes", `{}`, "host", "get", http.StatusForbidden},
		{"null scopes", `null`, "host", "get", http.StatusForbidden},
		{"malformed scopes", `{"host":`, "host", "get", http.StatusForbidden},
		{"wrong action type", `{"host":[true]}`, "host", "get", http.StatusForbidden},
		{"wildcard resource", `{"*":["get"]}`, "host", "get", http.StatusForbidden},
		{"wildcard action", `{"host":["*"]}`, "host", "get", http.StatusForbidden},
		{"admin action", `{"host":["admin"]}`, "host", "get", http.StatusForbidden},
	}
	for _, resource := range []string{"host_group", "enrollment_token"} {
		for _, action := range []string{"get", "create", "update", "delete"} {
			tests = append(tests, struct {
				name     string
				scopes   string
				resource string
				action   string
				want     int
			}{resource + " " + action, `{"` + resource + `":["` + action + `"]}`, resource, action, http.StatusNoContent})
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &db.AutoEnrollmentToken{Metadata: []byte(`{"integration_type":"api"}`), Scopes: []byte(tt.scopes)}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r = r.WithContext(context.WithValue(r.Context(), ApiTokenKey, token))
			w := httptest.NewRecorder()
			RequireApiScope(tt.resource, tt.action)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestRequireApiScopeRequiresAPICredential(t *testing.T) {
	for _, integration := range []string{"", "proxmox-lxc", "direct-host", "gethomepage"} {
		t.Run(integration, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			token := &db.AutoEnrollmentToken{Metadata: []byte(`{"integration_type":"` + integration + `"}`), Scopes: []byte(`{"host_group":["delete"]}`)}
			r = r.WithContext(context.WithValue(r.Context(), ApiTokenKey, token))
			w := httptest.NewRecorder()
			RequireApiScope("host_group", "delete")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("handler called") })).ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", w.Code)
			}
		})
	}
	t.Run("no authenticated token", func(t *testing.T) {
		w := httptest.NewRecorder()
		RequireApiScope("host", "get")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("handler called") })).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}
