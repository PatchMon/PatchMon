package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/go-chi/chi/v5"
)

func TestTfaEnrolmentGate(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	x := newTfaResetHarness(t, d, d)
	cfg := &config.Config{JWTSecret: "gate-test-secret-0123456789abcdef"}
	mint := &AuthHandler{cfg: cfg}

	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.AuthWithSessionCheck(cfg, x.sessions, nil, nil))
		r.Get("/tfa/setup", ok)
		r.Post("/tfa/verify-setup", ok)
		r.Get("/tfa/status", ok)
		r.Get("/auth/profile", ok)
		r.Post("/auth/logout", ok)
		r.Put("/auth/profile", ok)
		r.Post("/tfa/disable", ok)
		r.Put("/auth/change-password", ok)
		r.Get("/hosts", ok)
		r.Get("/auth/admin/users", ok)
		r.Post("/auth/admin/users", ok)
		r.Get("/tfa/setup/", ok)
	})

	do := func(token, method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	tokenFor := func(u *tfaTestUser) string {
		sid := x.mkSession(ctx, u.ID)
		tok, err := mint.createAccessToken(u.ID, u.Role, sid, 600)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}

	admin := x.mkUser(ctx, "admin", false)
	target := x.mkUser(ctx, "user", true)
	if rec := x.post(ctx, admin, target.ID, map[string]string{"password": tfaResetTestPassword}); rec.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	tok := tokenFor(target)

	allowed := [][2]string{
		{http.MethodGet, "/api/v1/tfa/setup"},
		{http.MethodPost, "/api/v1/tfa/verify-setup"},
		{http.MethodGet, "/api/v1/tfa/status"},
		{http.MethodGet, "/api/v1/auth/profile"},
		{http.MethodPost, "/api/v1/auth/logout"},
	}
	refused := [][2]string{
		{http.MethodPut, "/api/v1/auth/profile"},
		{http.MethodPost, "/api/v1/tfa/disable"},
		{http.MethodGet, "/api/v1/hosts"},
		{http.MethodGet, "/api/v1/auth/admin/users"},
		{http.MethodPost, "/api/v1/auth/admin/users"},
		{http.MethodGet, "/api/v1/tfa/setup/"},
	}
	for _, c := range allowed {
		if got := do(tok, c[0], c[1]); got != http.StatusOK {
			t.Errorf("pending %s %s = %d, want 200", c[0], c[1], got)
		}
	}
	for _, c := range refused {
		if got := do(tok, c[0], c[1]); got != http.StatusForbidden {
			t.Errorf("pending %s %s = %d, want 403", c[0], c[1], got)
		}
	}

	t.Run("a non-canonically encoded allowlisted path is refused", func(t *testing.T) {
		if got := do(tok, http.MethodGet, "/api/v1/auth/%70rofile"); got != http.StatusForbidden {
			t.Fatalf("got %d, want 403", got)
		}
	})

	t.Run("an inactive restricted session is revoked, not just refused", func(t *testing.T) {
		u := x.mkUser(ctx, "user", true)
		if rec := x.post(ctx, admin, u.ID, map[string]string{"password": tfaResetTestPassword}); rec.Code != http.StatusOK {
			t.Fatalf("reset: %d", rec.Code)
		}
		sid := x.mkSession(ctx, u.ID)
		if _, err := d.Exec(ctx, `UPDATE user_sessions SET last_activity = NOW() - INTERVAL '2 hours' WHERE id=$1`, sid); err != nil {
			t.Fatal(err)
		}
		itok, err := mint.createAccessToken(u.ID, u.Role, sid, 600)
		if err != nil {
			t.Fatal(err)
		}
		resolver := hostctx.NewConfigResolver(cfg, &config.ResolvedConfig{SessionInactivityTimeoutMin: 30}, nil)
		ir := chi.NewRouter()
		ir.With(middleware.AuthWithSessionCheck(cfg, x.sessions, resolver, nil)).Get("/api/v1/hosts", ok)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
		req.Header.Set("Authorization", "Bearer "+itok)
		rec := httptest.NewRecorder()
		ir.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d %s, want 401", rec.Code, rec.Body.String())
		}
		if n := x.count(ctx, `SELECT count(*) FROM user_sessions WHERE id=$1 AND is_revoked`, sid); n != 1 {
			t.Fatal("inactive session was not revoked")
		}
	})

	t.Run("an unflagged user is not restricted", func(t *testing.T) {
		if got := do(tokenFor(admin), http.MethodGet, "/api/v1/hosts"); got != http.StatusOK {
			t.Fatalf("got %d", got)
		}
	})

	t.Run("an IdP link lifts the restriction", func(t *testing.T) {
		other := x.mkUser(ctx, "user", false)
		if _, err := d.Exec(ctx, `UPDATE users SET tfa_enrolment_required=true WHERE id=$1`, other.ID); err != nil {
			t.Fatal(err)
		}
		otok := tokenFor(other)
		if got := do(otok, http.MethodGet, "/api/v1/hosts"); got != http.StatusForbidden {
			t.Fatalf("flagged local user: got %d", got)
		}
		if _, err := d.Exec(ctx, `UPDATE users SET oidc_sub=$1 WHERE id=$2`, "sub-"+other.ID, other.ID); err != nil {
			t.Fatal(err)
		}
		if got := do(otok, http.MethodGet, "/api/v1/hosts"); got != http.StatusOK {
			t.Fatalf("IdP-linked user: got %d", got)
		}
	})

	t.Run("a legacy provider-only IdP link lifts the restriction", func(t *testing.T) {
		other := x.mkUser(ctx, "user", false)
		if _, err := d.Exec(ctx, `UPDATE users SET tfa_enrolment_required=true, oidc_provider='idp.example.com' WHERE id=$1`, other.ID); err != nil {
			t.Fatal(err)
		}
		if got := do(tokenFor(other), http.MethodGet, "/api/v1/hosts"); got != http.StatusOK {
			t.Fatalf("provider-only IdP-linked user: got %d", got)
		}
	})

	t.Run("a temporary password confines the session to changing it, then to enrolment", func(t *testing.T) {
		u := x.mkUser(ctx, "user", true)
		if rec := x.post(ctx, admin, u.ID, map[string]string{"password": tfaResetTestPassword, "newPassword": "Temp-Passw0rd!x"}); rec.Code != http.StatusOK {
			t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
		}
		tok := tokenFor(u)
		for _, c := range [][3]string{
			{http.MethodPut, "/api/v1/auth/change-password", "200"},
			{http.MethodGet, "/api/v1/auth/profile", "200"},
			{http.MethodPost, "/api/v1/auth/logout", "200"},
			{http.MethodGet, "/api/v1/tfa/setup", "403"},
			{http.MethodPost, "/api/v1/tfa/verify-setup", "403"},
			{http.MethodGet, "/api/v1/hosts", "403"},
		} {
			if got := do(tok, c[0], c[1]); fmt.Sprint(got) != c[2] {
				t.Errorf("password pending %s %s = %d, want %s", c[0], c[1], got, c[2])
			}
		}
		hash := *x.reload(ctx, u.ID).PasswordHash
		if err := x.users.UpdatePassword(ctx, u.ID, hash); err != nil {
			t.Fatal(err)
		}
		for _, c := range [][3]string{
			{http.MethodGet, "/api/v1/tfa/setup", "200"},
			{http.MethodPut, "/api/v1/auth/change-password", "403"},
			{http.MethodGet, "/api/v1/hosts", "403"},
		} {
			if got := do(tok, c[0], c[1]); fmt.Sprint(got) != c[2] {
				t.Errorf("enrolment pending %s %s = %d, want %s", c[0], c[1], got, c[2])
			}
		}
	})

	js := `[]`
	if err := x.users.UpdateTfaEnabled(ctx, target.ID, &js); err != nil {
		t.Fatal(err)
	}
	for _, c := range refused[:4] {
		if got := do(tok, c[0], c[1]); got != http.StatusOK {
			t.Errorf("after enrolment %s %s = %d, want 200 on the same session", c[0], c[1], got)
		}
	}
}

func TestChangePasswordClearsForcedChange(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	x := newTfaResetHarness(t, d, d)
	admin := x.mkUser(ctx, "admin", false)
	u := x.mkUser(ctx, "user", true)
	if rec := x.post(ctx, admin, u.ID, map[string]string{"password": tfaResetTestPassword, "newPassword": "Temp-Passw0rd!x"}); rec.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	h := &AuthHandler{cfg: &config.Config{}, users: x.users}
	change := func(cur, next string) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/change-password", strings.NewReader(`{"currentPassword":"`+cur+`","newPassword":"`+next+`"}`))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, u.ID))
		rec := httptest.NewRecorder()
		h.ChangePassword(rec, req)
		return rec.Code
	}
	if got := change("Temp-Passw0rd!x", "Temp-Passw0rd!x"); got != http.StatusBadRequest {
		t.Fatalf("reusing the temporary password: got %d", got)
	}
	if x.reload(ctx, u.ID).PreviousPasswordHash == nil {
		t.Fatal("pre-reset password hash not kept")
	}
	if got := change("Temp-Passw0rd!x", tfaResetTestPassword); got != http.StatusBadRequest {
		t.Fatalf("returning to the pre-reset password: got %d", got)
	}
	if !x.reload(ctx, u.ID).PasswordChangeRequired {
		t.Fatal("flag cleared by a refused change")
	}
	if got := change("Temp-Passw0rd!x", "Brand-New-Passw0rd!"); got != http.StatusOK {
		t.Fatalf("change: got %d", got)
	}
	if after := x.reload(ctx, u.ID); after.PasswordChangeRequired || after.PreviousPasswordHash != nil {
		t.Fatal("flag or previous hash still set after the change")
	}
}
