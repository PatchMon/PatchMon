package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestEnrollmentTokenMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, metadata string
		allowed        bool
	}{
		{"direct host", `{"integration_type":"direct-host"}`, true},
		{"proxmox", `{"integration_type":"proxmox-lxc"}`, true},
		{"API", `{"integration_type":"api"}`, false},
		{"dashboard", `{"integration_type":"gethomepage"}`, false},
		{"unknown", `{"integration_type":"other"}`, false},
		{"missing", `{}`, false},
		{"empty", `{"integration_type":""}`, false},
		{"null type", `{"integration_type":null}`, false},
		{"numeric type", `{"integration_type":1}`, false},
		{"array", `[]`, false},
		{"null", `null`, false},
		{"unset", ``, false},
		{"invalid", `{`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEnrollmentToken(json.RawMessage(tc.metadata)); got != tc.allowed {
				t.Fatalf("allowed=%v, want %v", got, tc.allowed)
			}
		})
	}
}

func TestEnrollmentTokenPermissions(t *testing.T) {
	for _, tc := range []struct {
		name, metadata string
		allowed        bool
	}{
		{"direct host", `{"integration_type":"direct-host"}`, true},
		{"proxmox", `{"integration_type":"proxmox-lxc"}`, true},
		{"API", `{"integration_type":"api"}`, false},
		{"dashboard", `{"integration_type":"gethomepage"}`, false},
		{"unknown", `{"integration_type":"other"}`, false},
		{"missing", `{}`, false},
		{"null", `null`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEnrolFixture(t)
			ctx := context.Background()
			tokenID, key, secret := f.token(100, 0)
			code, seed := f.enrol(key, secret, map[string]interface{}{"friendly_name": "permission-fixture-" + tokenID, "machine_id": "permission-machine-" + tokenID})
			if code != http.StatusCreated {
				t.Fatalf("fixture creation: %d", code)
			}
			hostID := hostField(seed, "id")
			before, err := f.hosts.GetByID(ctx, hostID)
			if err != nil {
				t.Fatal(err)
			}
			countBefore, err := f.hosts.Count(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.d.Exec(ctx, `UPDATE auto_enrollment_tokens SET metadata=$2::jsonb WHERE id=$1`, tokenID, tc.metadata); err != nil {
				t.Fatal(err)
			}

			want := http.StatusForbidden
			if tc.allowed {
				want = http.StatusCreated
			}
			code, _ = f.enrol(key, secret, map[string]interface{}{"friendly_name": "permission-new-" + tokenID, "metadata": map[string]string{"integration_type": "direct-host"}})
			if code != want {
				t.Fatalf("new host status=%d, want %d", code, want)
			}
			if tc.allowed {
				want = http.StatusOK
			}
			code, _ = f.enrol(key, secret, map[string]interface{}{"friendly_name": "permission-fixture-" + tokenID, "machine_id": "permission-machine-" + tokenID, "host_id": hostID})
			if code != want {
				t.Fatalf("existing host status=%d, want %d", code, want)
			}

			for _, kind := range []string{"direct-host", "proxmox-lxc"} {
				query := url.Values{"type": {kind}, "token_key": {key}, "token_secret": {secret}}
				rec := httptest.NewRecorder()
				f.h.ServeScript(rec, httptest.NewRequest(http.MethodGet, "/auto-enrollment/script?"+query.Encode(), nil))
				if rec.Code != want {
					t.Fatalf("script %s status=%d, want %d", kind, rec.Code, want)
				}
				if !tc.allowed && (strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), key)) {
					t.Fatal("denied response contains credentials")
				}
			}
			if !tc.allowed {
				countAfter, err := f.hosts.Count(ctx)
				if err != nil {
					t.Fatal(err)
				}
				after, err := f.hosts.GetByID(ctx, hostID)
				if err != nil {
					t.Fatal(err)
				}
				if countAfter != countBefore || after.ApiKey != before.ApiKey || after.CredentialsReissuedAt != nil || f.createdToday(tokenID) != 1 {
					t.Fatal("denied operation changed stored state")
				}
			}
		})
	}
}

func TestCreatedTokenPermissions(t *testing.T) {
	for _, kind := range []string{"", "api"} {
		t.Run("type="+kind, func(t *testing.T) {
			f := newEnrolFixture(t)
			body := map[string]interface{}{"token_name": "created-permission-fixture"}
			if kind != "" {
				body["metadata"] = map[string]string{"integration_type": kind}
				body["scopes"] = map[string][]string{"host": {"get"}}
			}
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			f.h.Create(rec, httptest.NewRequest(http.MethodPost, "/auto-enrollment/tokens", strings.NewReader(string(b))))
			if rec.Code != http.StatusCreated {
				t.Fatalf("create status=%d", rec.Code)
			}
			var response struct {
				Token struct {
					Key    string `json:"token_key"`
					Secret string `json:"token_secret"`
				} `json:"token"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			code, _ := f.enrol(response.Token.Key, response.Token.Secret, map[string]interface{}{"friendly_name": "created-token-host"})
			want := http.StatusCreated
			if kind == "api" {
				want = http.StatusForbidden
			}
			if code != want {
				t.Fatalf("operation status=%d, want %d", code, want)
			}
			if kind != "api" {
				return
			}
			tokens := store.NewAutoEnrollmentStore(f.d)
			for _, action := range []string{"get", "delete"} {
				called := false
				endpoint := middleware.ApiAuth(tokens, nil)(middleware.RequireApiScope("host", action)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) })))
				req := httptest.NewRequest(http.MethodGet, "/api/hosts", nil)
				req.SetBasicAuth(response.Token.Key, response.Token.Secret)
				rec := httptest.NewRecorder()
				endpoint.ServeHTTP(rec, req)
				want := http.StatusForbidden
				if action == "get" {
					want = http.StatusNoContent
				}
				if rec.Code != want || called != (action == "get") {
					t.Fatalf("scope %s status=%d called=%v", action, rec.Code, called)
				}
			}
		})
	}
}
