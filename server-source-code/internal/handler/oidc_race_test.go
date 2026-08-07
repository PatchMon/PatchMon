package handler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

func configuredEntry() *oidcContextEntry {
	return &oidcContextEntry{
		resolved: &config.ResolvedOidcConfig{
			IssuerURL:       "https://db.example.com",
			ClientID:        "db-client",
			AutoCreateUsers: true,
			SyncRoles:       true,
			SuperadminGroup: "db-superadmins",
		},
		configuredValid: true,
		expiresAt:       time.Now().Add(oidcClientTTL),
	}
}

// TestOidcHandler_ResolvedAccessorsAreRaceFree guards the accessors that read a
// context's resolved OIDC config on the request path.
//
// A settings save evicts that context's cached entry while logins are reading
// it. The accessors must go through entryFor rather than touching the cache, or
// a login concurrent with a save is a data race. A read that saw no entry would
// silently fall back to the h.cfg env values, giving different auto-create and
// role-mapping behaviour for that login.
//
// Run under -race this fails if the accessors read the map directly.
func TestOidcHandler_ResolvedAccessorsAreRaceFree(t *testing.T) {
	t.Parallel()

	h := &OidcHandler{
		cfg: &config.Config{
			OidcIssuerURL:       "https://env.example.com",
			OidcClientID:        "env-client",
			OidcAutoCreateUsers: false,
			OidcSyncRoles:       false,
			OidcSuperadminGroup: "env-superadmins",
		},
		entries: map[string]*oidcContextEntry{"": configuredEntry()},
	}

	ctx := context.Background()
	const iterations = 200
	var wg sync.WaitGroup

	// Writer: the settings-save path, alternating between evicting the entry
	// (OIDC disabled) and re-seeding a configured one.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				h.evictOidcClient(ctx)
			} else {
				h.clientMu.Lock()
				h.entries[""] = configuredEntry()
				h.clientMu.Unlock()
			}
		}
	}()

	// Readers: every accessor a callback touches.
	readers := []func(){
		func() { _ = h.oidcIssuerURL(ctx) },
		func() { _ = h.oidcClientID(ctx) },
		func() { _ = h.oidcAutoCreateUsers(ctx) },
		func() { _ = h.oidcSyncRoles(ctx) },
		func() { _ = h.oidcSuperadminGroup(ctx) },
		func() { _ = h.mapGroupsToRole(ctx, []string{"admins", "users"}) },
	}
	for _, read := range readers {
		wg.Add(1)
		go func(fn func()) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				fn()
			}
		}(read)
	}

	wg.Wait()
}

// TestOidcHandler_EntriesAreIsolatedPerContext is the isolation invariant: one
// context's OIDC configuration must never be served to another.
//
// One process serves every context, and each context's OIDC settings live in its
// own database. Before per-context entries existed, a settings save by one
// context replaced the single shared client for everyone, so a customer could
// repoint every other customer's SSO at an issuer of their choosing.
func TestOidcHandler_EntriesAreIsolatedPerContext(t *testing.T) {
	t.Parallel()

	entryFor := func(issuer, clientID string) *oidcContextEntry {
		return &oidcContextEntry{
			resolved: &config.ResolvedOidcConfig{
				IssuerURL: issuer,
				ClientID:  clientID,
			},
			configuredValid: true,
			expiresAt:       time.Now().Add(oidcClientTTL),
		}
	}

	h := &OidcHandler{
		cfg: &config.Config{},
		entries: map[string]*oidcContextEntry{
			"a.example.com": entryFor("https://idp-a.example.com", "client-a"),
			"b.example.com": entryFor("https://idp-b.example.com", "client-b"),
		},
	}

	ctxA := hostctx.WithEntry(context.Background(), &hostctx.Entry{Host: "a.example.com"})
	ctxB := hostctx.WithEntry(context.Background(), &hostctx.Entry{Host: "b.example.com"})

	if got, want := h.oidcIssuerURL(ctxA), "https://idp-a.example.com"; got != want {
		t.Errorf("context A issuer = %q, want %q", got, want)
	}
	if got, want := h.oidcIssuerURL(ctxB), "https://idp-b.example.com"; got != want {
		t.Errorf("context B issuer = %q, want %q", got, want)
	}
	if got, want := h.oidcClientID(ctxA), "client-a"; got != want {
		t.Errorf("context A client id = %q, want %q", got, want)
	}

	// Evicting one context must not disturb the other.
	h.evictOidcClient(ctxA)
	if got, want := h.oidcIssuerURL(ctxB), "https://idp-b.example.com"; got != want {
		t.Errorf("context B issuer after evicting A = %q, want %q", got, want)
	}
}
