package handler

import (
	"sync"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
)

// TestOidcHandler_ResolvedAccessorsAreRaceFree guards the accessors that read
// h.resolved on the request path.
//
// reinitOidcClient replaces h.resolved under clientMu when an admin saves OIDC
// settings, and sets it to nil when OIDC is disabled. The accessors read the
// field directly with no lock, so a login concurrent with a settings save was a
// data race. A read that saw nil silently fell back to the h.cfg env values,
// giving different auto-create and role-mapping behaviour for that login.
//
// Run under -race this fails without the snapshot helper.
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
		resolved: &config.ResolvedOidcConfig{
			IssuerURL:       "https://db.example.com",
			ClientID:        "db-client",
			AutoCreateUsers: true,
			SyncRoles:       true,
			SuperadminGroup: "db-superadmins",
		},
	}

	const iterations = 200
	var wg sync.WaitGroup

	// Writer: the settings-save path, flipping between configured and disabled.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			h.clientMu.Lock()
			if i%2 == 0 {
				h.resolved = nil // OIDC disabled
			} else {
				h.resolved = &config.ResolvedOidcConfig{
					IssuerURL:       "https://db.example.com",
					ClientID:        "db-client",
					AutoCreateUsers: true,
					SyncRoles:       true,
					SuperadminGroup: "db-superadmins",
				}
			}
			h.clientMu.Unlock()
		}
	}()

	// Readers: every accessor a callback touches.
	readers := []func(){
		func() { _ = h.oidcIssuerURL() },
		func() { _ = h.oidcClientID() },
		func() { _ = h.oidcAutoCreateUsers() },
		func() { _ = h.oidcSyncRoles() },
		func() { _ = h.oidcSuperadminGroup() },
		func() { _ = h.mapGroupsToRole([]string{"admins", "users"}) },
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
