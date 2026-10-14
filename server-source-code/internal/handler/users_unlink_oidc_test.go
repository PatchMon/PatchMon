package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/oidc"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestUnlinkOidcAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	users := store.NewUsersStore(d)
	trust := false
	h := &UsersHandler{users: users, permissions: store.NewPermissionsStore(d), audit: store.NewAuditLogsStore(d), trustUnverifiedEmail: func(context.Context) bool { return trust }}
	oidcHandler := &OidcHandler{
		cfg:   &config.Config{},
		users: users,
		entries: map[string]*oidcContextEntry{"": {
			resolved:        &config.ResolvedOidcConfig{IssuerURL: "https://idp.example.com", DefaultRole: "user"},
			configuredValid: true,
			expiresAt:       time.Now().Add(time.Hour),
		}},
	}

	createLinked := func(role string) (*models.User, string) {
		t.Helper()
		id := uuid.New().String()
		sub := "sub-" + id[:8]
		u := &models.User{ID: id, Username: "u" + id[:12], Email: id[:8] + "@example.com", Role: role, IsActive: true}
		if err := users.CreateOidcUser(ctx, u, sub, "idp.example.com", nil); err != nil {
			t.Fatal(err)
		}
		return u, sub
	}
	unlinkAs := func(userID, callerRole, callerID string) int {
		t.Helper()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("userId", userID)
		reqCtx := context.WithValue(ctx, chi.RouteCtxKey, rctx)
		reqCtx = context.WithValue(reqCtx, middleware.UserRoleKey, callerRole)
		reqCtx = context.WithValue(reqCtx, middleware.UserIDKey, callerID)
		rec := httptest.NewRecorder()
		h.UnlinkOidc(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(reqCtx))
		return rec.Code
	}
	unlink := func(userID, callerRole string) int {
		t.Helper()
		return unlinkAs(userID, callerRole, "caller")
	}
	auditRows := func(targetID string) string {
		t.Helper()
		var rows string
		if err := d.RawQueryRow(ctx, `SELECT coalesce(string_agg(success::text || ' ' || coalesce(details, ''), '; ' ORDER BY created_at), '') FROM audit_logs WHERE event = 'user_oidc_unlink' AND target_user_id = $1`, targetID).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	storedSub := func(id string) *string {
		t.Helper()
		u, err := users.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return u.OidcSub
	}

	t.Run("unlink lets the next verified login re-link a new subject", func(t *testing.T) {
		u, _ := createLinked("user")
		newSub := "recreated-" + uuid.New().String()[:8]
		info := &oidc.UserInfo{Issuer: testIssuer, Sub: newSub, Email: u.Email, EmailVerified: true}

		if got, _ := oidcHandler.resolveOidcUser(ctx, info); got != nil {
			t.Fatal("login with a new subject was accepted before unlink")
		}
		if code := unlink(u.ID, "admin"); code != http.StatusOK {
			t.Fatalf("unlink: status %d", code)
		}
		if s := storedSub(u.ID); s != nil {
			t.Fatalf("subject still stored after unlink: %q", *s)
		}
		got, failURL := oidcHandler.resolveOidcUser(ctx, info)
		if got == nil || got.ID != u.ID {
			t.Fatalf("re-link after unlink: got %v, %q", got, failURL)
		}
		if s := storedSub(u.ID); s == nil || *s != newSub {
			t.Fatal("new subject was not stored")
		}
	})

	t.Run("unlinked account still refuses an unverified email", func(t *testing.T) {
		u, _ := createLinked("user")
		if code := unlink(u.ID, "admin"); code != http.StatusOK {
			t.Fatalf("unlink: status %d", code)
		}
		if got, _ := oidcHandler.resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: "x-" + u.ID[:8], Email: u.Email}); got != nil {
			t.Fatal("unverified email re-linked an unlinked account")
		}
	})

	for _, tc := range []struct {
		target, caller string
		want           int
	}{
		{"admin", "host_manager", http.StatusForbidden},
		{"admin", "admin", http.StatusForbidden},
		{"host_manager", "host_manager", http.StatusForbidden},
		{"superadmin", "admin", http.StatusForbidden},
		{"user", "admin", http.StatusOK},
		{"admin", "superadmin", http.StatusOK},
		{"superadmin", "superadmin", http.StatusOK},
	} {
		t.Run(tc.caller+" unlinking "+tc.target, func(t *testing.T) {
			u, sub := createLinked(tc.target)
			if code := unlink(u.ID, tc.caller); code != tc.want {
				t.Fatalf("status %d, want %d", code, tc.want)
			}
			want := `true {"previous_provider":"idp.example.com","previous_sub":"` + sub + `"}`
			if tc.want != http.StatusOK {
				if s := storedSub(u.ID); s == nil || *s != sub {
					t.Fatal("refused unlink changed the stored subject")
				}
				want = `false {"reason":"insufficient_role"}`
			}
			if got := auditRows(u.ID); got != want {
				t.Fatalf("audit: got %q, want %q", got, want)
			}
		})
	}

	t.Run("own identity cannot be unlinked", func(t *testing.T) {
		u, sub := createLinked("superadmin")
		if code := unlinkAs(u.ID, "superadmin", u.ID); code != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", code)
		}
		if got := auditRows(u.ID); got != `false {"reason":"self"}` {
			t.Fatalf("audit: got %q", got)
		}
		if s := storedSub(u.ID); s == nil || *s != sub {
			t.Fatal("refused unlink changed the stored subject")
		}
	})

	t.Run("trusting unverified email blocks unlinking a superadmin only", func(t *testing.T) {
		trust = true
		t.Cleanup(func() { trust = false })
		super, sub := createLinked("superadmin")
		if code := unlink(super.ID, "superadmin"); code != http.StatusBadRequest {
			t.Fatalf("superadmin: status %d, want 400", code)
		}
		if got := auditRows(super.ID); got != `false {"reason":"trust_unverified_email"}` {
			t.Fatalf("audit: got %q", got)
		}
		if s := storedSub(super.ID); s == nil || *s != sub {
			t.Fatal("refused unlink changed the stored subject")
		}
		u, _ := createLinked("user")
		if code := unlink(u.ID, "superadmin"); code != http.StatusOK {
			t.Fatalf("user: status %d, want 200", code)
		}
	})

	t.Run("trust setting is read from the context's settings", func(t *testing.T) {
		for _, k := range []string{"OIDC_ISSUER_URL", "OIDC_CLIENT_ID", "OIDC_CLIENT_SECRET", "OIDC_REDIRECT_URI", "OIDC_SCOPES", "OIDC_ENABLED"} {
			t.Setenv(k, "")
		}
		settings := store.NewSettingsStore(d)
		row, err := settings.GetFirst(ctx)
		if err != nil {
			t.Fatal(err)
		}
		previous := row.OidcTrustUnverifiedEmail
		t.Cleanup(func() {
			row.OidcTrustUnverifiedEmail = previous
			_ = settings.Update(ctx, row)
		})
		resolving := &UsersHandler{users: users, cfg: &config.Config{}, settings: settings}
		for _, on := range []bool{false, true} {
			row.OidcTrustUnverifiedEmail = on
			if err := settings.Update(ctx, row); err != nil {
				t.Fatal(err)
			}
			if got := resolving.oidcTrustUnverifiedEmail(ctx); got != on {
				t.Fatalf("setting %v resolved as %v", on, got)
			}
		}
	})

	t.Run("unlinking an unlinked user is a bad request", func(t *testing.T) {
		u, _ := createLinked("user")
		if code := unlink(u.ID, "admin"); code != http.StatusOK {
			t.Fatalf("first unlink: status %d", code)
		}
		if code := unlink(u.ID, "admin"); code != http.StatusBadRequest {
			t.Fatalf("second unlink: status %d, want 400", code)
		}
		if got := auditRows(u.ID); !strings.HasPrefix(got, "true ") || !strings.HasSuffix(got, `; false {"reason":"not_linked"}`) {
			t.Fatalf("audit: got %q", got)
		}
	})

	t.Run("unknown user is not found", func(t *testing.T) {
		missing := uuid.New().String()
		if code := unlink(missing, "admin"); code != http.StatusNotFound {
			t.Fatalf("status %d, want 404", code)
		}
		if got := auditRows(missing); got != `false {"reason":"target_not_found"}` {
			t.Fatalf("audit: got %q", got)
		}
	})
}
