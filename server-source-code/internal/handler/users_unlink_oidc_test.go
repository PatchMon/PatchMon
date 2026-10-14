package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	h := &UsersHandler{users: users, permissions: store.NewPermissionsStore(d)}
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
	unlink := func(userID, callerRole string) int {
		t.Helper()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("userId", userID)
		reqCtx := context.WithValue(ctx, chi.RouteCtxKey, rctx)
		reqCtx = context.WithValue(reqCtx, middleware.UserRoleKey, callerRole)
		reqCtx = context.WithValue(reqCtx, middleware.UserIDKey, "caller")
		rec := httptest.NewRecorder()
		h.UnlinkOidc(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(reqCtx))
		return rec.Code
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
		info := &oidc.UserInfo{Sub: newSub, Email: u.Email, EmailVerified: true}

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
		if got, _ := oidcHandler.resolveOidcUser(ctx, &oidc.UserInfo{Sub: "x-" + u.ID[:8], Email: u.Email}); got != nil {
			t.Fatal("unverified email re-linked an unlinked account")
		}
	})

	for _, tc := range []struct {
		name, target, caller string
	}{
		{"lower rank cannot unlink a higher rank", "admin", "host_manager"},
		{"admin without can_manage_superusers cannot unlink a superadmin", "superadmin", "admin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, sub := createLinked(tc.target)
			if code := unlink(u.ID, tc.caller); code != http.StatusForbidden {
				t.Fatalf("status %d, want 403", code)
			}
			if s := storedSub(u.ID); s == nil || *s != sub {
				t.Fatal("refused unlink changed the stored subject")
			}
		})
	}

	t.Run("superadmin can unlink a superadmin", func(t *testing.T) {
		u, _ := createLinked("superadmin")
		if code := unlink(u.ID, "superadmin"); code != http.StatusOK {
			t.Fatalf("status %d", code)
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
	})

	t.Run("unknown user is not found", func(t *testing.T) {
		if code := unlink(uuid.New().String(), "admin"); code != http.StatusNotFound {
			t.Fatalf("status %d, want 404", code)
		}
	})
}
