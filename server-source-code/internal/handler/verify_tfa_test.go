package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// newTicketlessAuthHandler builds an AuthHandler whose pending-login store can
// never resolve a ticket (its Redis resolver has no client). Every ticket is
// therefore invalid, which is exactly the state we want to assert on: no
// combination of username and TOTP code may produce a session.
func newTicketlessAuthHandler() *AuthHandler {
	return &AuthHandler{
		pendingLogin: store.NewPendingLoginStore(&hostctx.RedisResolver{Default: nil}),
	}
}

func postVerifyTfa(t *testing.T, h *AuthHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-tfa", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.VerifyTfa(rec, req)
	return rec
}

// TestVerifyTfa_RequiresFirstFactorTicket is the regression guard for the
// authentication bypass.
//
// VerifyTfa used to accept {username, token} and call completeLogin directly.
// There was no password check, no pending-login state carried over from Login,
// and no server-side marker behind Login's {"requiresTfa": true} response, so
// the endpoint was callable without ever touching /auth/login. For any
// TFA-enabled account, possession of one 6-digit TOTP code (or one backup code)
// was sufficient to obtain a full session plus a 30-day device-trust cookie.
//
// A session must now be unreachable without a valid single-use ticket, which is
// only issued after the password has been verified.
func TestVerifyTfa_RequiresFirstFactorTicket(t *testing.T) {
	t.Parallel()

	h := newTicketlessAuthHandler()

	cases := []struct {
		name string
		body string
	}{
		{
			name: "the original bypass shape: username plus code, no ticket",
			body: `{"username":"admin","token":"123456"}`,
		},
		{
			name: "explicitly empty ticket",
			body: `{"username":"admin","token":"123456","tfa_ticket":""}`,
		},
		{
			name: "forged ticket",
			body: `{"username":"admin","token":"123456","tfa_ticket":"deadbeefdeadbeefdeadbeefdeadbeef"}`,
		},
		{
			name: "remember_me does not bypass the requirement",
			body: `{"username":"admin","token":"123456","remember_me":true}`,
		},
		{
			name: "backup-code shaped token still needs a ticket",
			body: `{"username":"admin","token":"ABCD12"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := postVerifyTfa(t, h, tc.body)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 without a valid first-factor ticket, got %d: %s", rec.Code, rec.Body.String())
			}
			// Nothing resembling a session may be handed back.
			if body := rec.Body.String(); strings.Contains(body, `"token"`) || strings.Contains(body, `"user"`) {
				t.Fatalf("response must not carry a session: %s", body)
			}
			for _, c := range rec.Result().Cookies() {
				t.Fatalf("no cookie may be set on a rejected TFA verification, got %q", c.Name)
			}
		})
	}
}

// TestVerifyTfa_RejectsMalformedTokenBeforeTicketLookup keeps the existing
// input validation intact.
func TestVerifyTfa_RejectsMalformedToken(t *testing.T) {
	t.Parallel()

	h := newTicketlessAuthHandler()

	for _, body := range []string{
		`{"username":"admin","token":"12345"}`,   // too short
		`{"username":"admin","token":"1234567"}`, // too long
		`{"username":"admin","token":"12-456"}`,  // non-alphanumeric
		`{"username":"admin","token":""}`,        // empty
	} {
		rec := postVerifyTfa(t, h, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for %s, got %d", body, rec.Code)
		}
	}
}

// TestVerifyTfa_FailsClosedWithoutStore ensures a misconfigured deployment
// cannot silently fall back to the old behaviour.
func TestVerifyTfa_FailsClosedWithoutStore(t *testing.T) {
	t.Parallel()

	h := &AuthHandler{} // no pending-login store wired
	rec := postVerifyTfa(t, h, `{"username":"admin","token":"123456"}`)

	if rec.Code == http.StatusOK {
		t.Fatalf("VerifyTfa must not succeed when the pending-login store is absent")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when the store is unconfigured, got %d", rec.Code)
	}
}

// TestPendingLoginStore_ConsumeRejectsEmptyTicket documents the store contract
// the handler relies on.
func TestPendingLoginStore_ConsumeRejectsEmptyTicket(t *testing.T) {
	t.Parallel()

	s := store.NewPendingLoginStore(&hostctx.RedisResolver{Default: nil})
	if _, err := s.Consume(context.Background(), ""); err == nil {
		t.Fatal("an empty ticket must never resolve to a user")
	}
	if _, err := s.Consume(context.Background(), "nonexistent"); err == nil {
		t.Fatal("an unknown ticket must never resolve to a user")
	}
}

// ensure the JSON shape Login promises stays in sync with what VerifyTfa reads.
func TestVerifyTfaRequest_TicketFieldName(t *testing.T) {
	t.Parallel()

	var req VerifyTfaRequest
	if err := json.Unmarshal([]byte(`{"tfa_ticket":"abc"}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.TfaTicket != "abc" {
		t.Fatalf("tfa_ticket must bind to TfaTicket, got %q", req.TfaTicket)
	}
}
