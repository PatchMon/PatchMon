package handler

import (
	"context"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/oidc"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/google/uuid"
)

func TestResolveOidcUserAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	users := store.NewUsersStore(d)

	newHandler := func(autoCreate bool) *OidcHandler {
		return &OidcHandler{
			cfg:   &config.Config{},
			users: users,
			entries: map[string]*oidcContextEntry{"": {
				resolved: &config.ResolvedOidcConfig{
					IssuerURL:       "https://idp.example.com",
					AutoCreateUsers: autoCreate,
					DefaultRole:     "user",
				},
				configuredValid: true,
				expiresAt:       time.Now().Add(time.Hour),
			}},
		}
	}
	unique := func(prefix string) (sub, email string) {
		id := uuid.New().String()[:8]
		return prefix + "-sub-" + id, prefix + "-" + id + "@example.com"
	}
	createLocal := func(email string, sub *string) *models.User {
		t.Helper()
		u := &models.User{
			ID:       uuid.New().String(),
			Username: "u" + uuid.New().String()[:12],
			Email:    email,
			Role:     "user",
			IsActive: true,
			OidcSub:  sub,
		}
		if sub != nil {
			if err := users.CreateOidcUser(ctx, u, *sub, "idp.example.com", nil); err != nil {
				t.Fatal(err)
			}
			return u
		}
		if err := users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	const rejected = "/login?error=Unable+to+sign+in+with+this+account"

	t.Run("auto-created user signs in on first login", func(t *testing.T) {
		h := newHandler(true)
		for i := 0; i < 2; i++ {
			sub, email := unique("new")
			got, failURL := h.resolveOidcUser(ctx, &oidc.UserInfo{Sub: sub, Email: email, EmailVerified: true})
			if got == nil {
				t.Fatalf("user %d: first login rejected with %q", i, failURL)
			}
			stored, err := users.GetByOidcSub(ctx, sub)
			if err != nil || stored.ID != got.ID {
				t.Fatalf("user %d: stored row not bound to subject: %v", i, err)
			}
		}
	})

	t.Run("auto-create refuses an unverified email", func(t *testing.T) {
		sub, email := unique("unverified")
		got, failURL := newHandler(true).resolveOidcUser(ctx, &oidc.UserInfo{Sub: sub, Email: email})
		if got != nil || failURL != rejected {
			t.Fatalf("got user=%v url=%q, want rejection", got, failURL)
		}
		if u, _ := users.GetByOidcSub(ctx, sub); u != nil {
			t.Fatal("unverified login created an account")
		}
	})

	t.Run("returning user matched by subject signs in", func(t *testing.T) {
		sub, email := unique("returning")
		existing := createLocal(email, &sub)
		got, failURL := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Sub: sub, Email: email})
		if got == nil || got.ID != existing.ID {
			t.Fatalf("got user=%v url=%q, want %s", got, failURL, existing.ID)
		}
	})

	t.Run("email match never takes over an account bound to another subject", func(t *testing.T) {
		for _, autoCreate := range []bool{true, false} {
			victimSub, email := unique("victim")
			createLocal(email, &victimSub)
			attackerSub, _ := unique("attacker")
			got, failURL := newHandler(autoCreate).resolveOidcUser(ctx, &oidc.UserInfo{Sub: attackerSub, Email: email, EmailVerified: true})
			if got != nil || failURL != rejected {
				t.Fatalf("autoCreate=%v: got user=%v url=%q, want rejection", autoCreate, got, failURL)
			}
			stored, err := users.GetByEmail(ctx, email)
			if err != nil || stored.OidcSub == nil || *stored.OidcSub != victimSub {
				t.Fatalf("autoCreate=%v: victim binding changed: %v", autoCreate, err)
			}
		}
	})

	t.Run("verified email match links an unbound local account", func(t *testing.T) {
		_, email := unique("local")
		local := createLocal(email, nil)
		sub, _ := unique("link")
		got, failURL := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Sub: sub, Email: email, EmailVerified: true})
		if got == nil || got.ID != local.ID {
			t.Fatalf("got user=%v url=%q, want %s", got, failURL, local.ID)
		}
		stored, err := users.GetByID(ctx, local.ID)
		if err != nil || stored.OidcSub == nil || *stored.OidcSub != sub {
			t.Fatalf("local account not linked: %v", err)
		}
	})

	t.Run("unverified email match does not link an unbound local account", func(t *testing.T) {
		_, email := unique("localunv")
		local := createLocal(email, nil)
		sub, _ := unique("linkunv")
		got, failURL := newHandler(true).resolveOidcUser(ctx, &oidc.UserInfo{Sub: sub, Email: email})
		if got != nil || failURL != rejected {
			t.Fatalf("got user=%v url=%q, want rejection", got, failURL)
		}
		stored, err := users.GetByID(ctx, local.ID)
		if err != nil || stored.OidcSub != nil {
			t.Fatalf("local account was linked: %v", err)
		}
	})
}
