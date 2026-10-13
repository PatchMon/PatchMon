package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

func testDatabase(t *testing.T) *database.DB {
	t.Helper()
	return pgtest.Open(t)
}

func patchEnvKey(h *SettingsHandler, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/settings/environment/"+key, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", key)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.UpdateEnvironmentConfig(rec, req)
	return rec
}

func TestUpdateEnvironmentConfig_RejectsInvalidBodyLimits(t *testing.T) {
	t.Parallel()
	h := &SettingsHandler{}
	cases := []struct {
		key, body string
	}{
		{"JSON_BODY_LIMIT", `{"value":"abc"}`},
		{"JSON_BODY_LIMIT", `{"value":"0"}`},
		{"JSON_BODY_LIMIT", `{"value":""}`},
		{"AGENT_UPDATE_BODY_LIMIT", `{"value":"-5mb"}`},
		{"AGENT_UPDATE_BODY_LIMIT", `{"value":5}`},
		{"COMPLIANCE_BODY_LIMIT", `{"value":"20 megabytes"}`},
		{"COMPLIANCE_BODY_LIMIT", `{"value":"0kb"}`},
		{"AGENT_PING_BODY_LIMIT", `{"value":"64kib"}`},
		{"AGENT_PING_BODY_LIMIT", `{"value":"0"}`},
	}
	for _, tc := range cases {
		t.Run(tc.key+" "+tc.body, func(t *testing.T) {
			t.Parallel()
			rec := patchEnvKey(h, tc.key, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tc.key) {
				t.Errorf("error %q does not name %s", rec.Body.String(), tc.key)
			}
		})
	}
}

func TestUpdateEnvironmentConfig_AcceptsValidBodyLimits(t *testing.T) {
	d := testDatabase(t)
	h := &SettingsHandler{settings: store.NewSettingsStore(d)}
	for _, tc := range []struct{ key, value string }{
		{"JSON_BODY_LIMIT", "5mb"},
		{"JSON_BODY_LIMIT", "10"},
		{"AGENT_UPDATE_BODY_LIMIT", "1536kb"},
		{"COMPLIANCE_BODY_LIMIT", "50mb"},
		{"AGENT_PING_BODY_LIMIT", "128"},
		{"AGENT_PING_BODY_LIMIT", "64kb"},
	} {
		t.Setenv(tc.key, "")
		rec := patchEnvKey(h, tc.key, `{"value":"`+tc.value+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s=%s: status = %d body %s, want 200", tc.key, tc.value, rec.Code, rec.Body.String())
		}
	}
	s, err := h.settings.GetFirst(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.settings.UpdateConfigKey(context.Background(), s.ID, "COMPLIANCE_BODY_LIMIT", "abc", s); err == nil {
		t.Error("store accepted COMPLIANCE_BODY_LIMIT=abc, want a validation error")
	}
	if s.AgentPingBodyLimit == nil || *s.AgentPingBodyLimit != "64kb" {
		t.Errorf("AGENT_PING_BODY_LIMIT stored = %v, want 64kb", s.AgentPingBodyLimit)
	}
}
