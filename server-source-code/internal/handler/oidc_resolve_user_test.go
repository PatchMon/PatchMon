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

const testIssuer = "https://idp.example.com"

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
	createBound := func(email, sub, provider string) *models.User {
		t.Helper()
		u := &models.User{ID: uuid.New().String(), Username: "u" + uuid.New().String()[:12], Email: email, Role: "superadmin", IsActive: true}
		if err := users.CreateOidcUser(ctx, u, sub, provider, nil); err != nil {
			t.Fatal(err)
		}
		return u
	}
	providerOf := func(id string) string {
		t.Helper()
		u, err := users.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return strVal(u.OidcProvider, "")
	}

	t.Run("auto-created user signs in on first login", func(t *testing.T) {
		h := newHandler(true)
		for i := 0; i < 2; i++ {
			sub, email := unique("new")
			got, failURL := h.resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email, EmailVerified: true})
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
		got, failURL := newHandler(true).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email})
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
		got, failURL := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email})
		if got == nil || got.ID != existing.ID {
			t.Fatalf("got user=%v url=%q, want %s", got, failURL, existing.ID)
		}
	})

	t.Run("email match never takes over an account bound to another subject", func(t *testing.T) {
		for _, autoCreate := range []bool{true, false} {
			victimSub, email := unique("victim")
			createLocal(email, &victimSub)
			attackerSub, _ := unique("attacker")
			got, failURL := newHandler(autoCreate).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: attackerSub, Email: email, EmailVerified: true})
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
		got, failURL := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email, EmailVerified: true})
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
		got, failURL := newHandler(true).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email})
		if got != nil || failURL != rejected {
			t.Fatalf("got user=%v url=%q, want rejection", got, failURL)
		}
		stored, err := users.GetByID(ctx, local.ID)
		if err != nil || stored.OidcSub != nil {
			t.Fatalf("local account was linked: %v", err)
		}
	})

	t.Run("new links store the full issuer", func(t *testing.T) {
		sub, email := unique("fullissuer")
		got, failURL := newHandler(true).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email, EmailVerified: true})
		if got == nil {
			t.Fatalf("rejected: %q", failURL)
		}
		if p := providerOf(got.ID); p != testIssuer {
			t.Fatalf("stored provider %q, want %q", p, testIssuer)
		}
	})

	t.Run("same subject from another issuer never signs in", func(t *testing.T) {
		for _, autoCreate := range []bool{true, false} {
			sub, email := unique("victimiss")
			victim := createBound(email, sub, testIssuer)
			for _, attackerEmail := range []string{"attacker-" + uuid.New().String()[:8] + "@evil.test", email} {
				got, failURL := newHandler(autoCreate).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: "https://evil.example.net", Sub: sub, Email: attackerEmail, EmailVerified: true})
				if got != nil || failURL != rejected {
					t.Fatalf("autoCreate=%v email=%s: got user=%v url=%q, want rejection", autoCreate, attackerEmail, got, failURL)
				}
			}
			if p := providerOf(victim.ID); p != testIssuer {
				t.Fatalf("victim binding changed to %q", p)
			}
		}
	})

	t.Run("a different realm on the same host is a different issuer", func(t *testing.T) {
		sub, email := unique("realm")
		createBound(email, sub, "https://idp.example.com/realms/a")
		got, _ := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: "https://idp.example.com/realms/b", Sub: sub, Email: email, EmailVerified: true})
		if got != nil {
			t.Fatal("subject from another realm signed in")
		}
	})

	t.Run("issuer comparison ignores a trailing slash", func(t *testing.T) {
		sub, email := unique("slash")
		u := createBound(email, sub, testIssuer+"/")
		got, failURL := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email})
		if got == nil || got.ID != u.ID {
			t.Fatalf("got %v, %q", got, failURL)
		}
	})

	t.Run("legacy hostname-only link matches its host and is upgraded", func(t *testing.T) {
		sub, email := unique("legacy")
		u := createBound(email, sub, "idp.example.com")
		got, failURL := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email})
		if got == nil || got.ID != u.ID {
			t.Fatalf("got %v, %q", got, failURL)
		}
		if p := providerOf(u.ID); p != testIssuer {
			t.Fatalf("legacy provider not upgraded: %q", p)
		}
	})

	t.Run("legacy hostname-only link refuses another host", func(t *testing.T) {
		sub, email := unique("legacyother")
		u := createBound(email, sub, "idp.example.com")
		if got, _ := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: "https://evil.example.net", Sub: sub, Email: email, EmailVerified: true}); got != nil {
			t.Fatal("legacy link accepted a subject from another host")
		}
		if p := providerOf(u.ID); p != "idp.example.com" {
			t.Fatalf("refused login changed the provider to %q", p)
		}
	})

	t.Run("empty provider matches only the configured issuer's host and is upgraded", func(t *testing.T) {
		sub, email := unique("emptyprov")
		u := createBound(email, sub, "")
		if got, _ := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: "https://evil.example.net", Sub: sub, Email: email, EmailVerified: true}); got != nil {
			t.Fatal("empty provider accepted another host")
		}
		if p := providerOf(u.ID); p != "" {
			t.Fatalf("refused login changed the provider to %q", p)
		}
		got, failURL := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: testIssuer, Sub: sub, Email: email})
		if got == nil || got.ID != u.ID {
			t.Fatalf("got %v, %q", got, failURL)
		}
		if p := providerOf(u.ID); p != testIssuer {
			t.Fatalf("empty provider not upgraded: %q", p)
		}
	})

	t.Run("Entra v1 and v2 issuers of one tenant match, other tenants do not", func(t *testing.T) {
		const tid = "11111111-2222-3333-4444-555555555555"
		sub, email := unique("entra")
		u := createBound(email, sub, "https://login.microsoftonline.com/"+tid+"/v2.0")
		got, failURL := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: "https://sts.windows.net/" + tid, Sub: sub, Email: email})
		if got == nil || got.ID != u.ID {
			t.Fatalf("same tenant: got %v, %q", got, failURL)
		}
		other := "https://login.microsoftonline.com/99999999-2222-3333-4444-555555555555/v2.0"
		if got, _ := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Issuer: other, Sub: sub, Email: email, EmailVerified: true}); got != nil {
			t.Fatal("subject from another Entra tenant signed in")
		}
	})

	t.Run("missing issuer is refused", func(t *testing.T) {
		sub, email := unique("noiss")
		createBound(email, sub, testIssuer)
		if got, _ := newHandler(false).resolveOidcUser(ctx, &oidc.UserInfo{Sub: sub, Email: email}); got != nil {
			t.Fatal("login without an issuer signed in")
		}
	})
}
