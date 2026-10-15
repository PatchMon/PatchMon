package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func newDiscordLocalAuthHandler(t *testing.T, policy LocalAuthPolicy) *DiscordHandler {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &DiscordHandler{
		cfg:          &config.Config{},
		resolved:     &config.ResolvedConfig{CORSOrigin: "http://127.0.0.1"},
		settings:     store.NewSettingsStore(pgtest.Open(t)),
		discordStore: store.NewDiscordSessionStore(&hostctx.RedisResolver{Default: rdb}),
		auth:         (&AuthHandler{cfg: &config.Config{}}).WithLocalAuthPolicy(policy),
	}
}

func discordCallback(t *testing.T, h *DiscordHandler, mode string) *httptest.ResponseRecorder {
	t.Helper()
	const state = "state-0123456789"
	if err := h.discordStore.Store(context.Background(), state, &store.DiscordSessionData{CodeVerifier: "v", State: state, Mode: mode, UserID: "u1"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/callback?code=c&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "discord_state", Value: state})
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	return rec
}

func TestDiscordRefusedWhileLocalAuthDisabled(t *testing.T) {
	for name, policy := range map[string]fixedLocalAuthPolicy{
		"local auth disabled":         {disabled: true},
		"SSO outage fallback in play": {fallback: "discovery returned HTTP 503"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newDiscordLocalAuthHandler(t, policy)

			rec := httptest.NewRecorder()
			h.Login(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/login", nil))
			if rec.Code != http.StatusForbidden || rec.Header().Get("Location") != "" {
				t.Fatalf("login: got %d (Location %q), want 403 with no redirect to Discord", rec.Code, rec.Header().Get("Location"))
			}

			rec = httptest.NewRecorder()
			h.Config(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/config", nil))
			var cfg struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil || cfg.Enabled {
				t.Fatalf("config must report Discord as unavailable: %s", rec.Body.String())
			}

			// Refused before the code exchange, so neither sign-in nor auto-create can run.
			rec = discordCallback(t, h, "login")
			if loc := rec.Header().Get("Location"); rec.Code != http.StatusFound || !strings.Contains(loc, "Discord+sign-in+is+disabled") {
				t.Fatalf("callback: got %d Location %q, want a redirect explaining Discord is disabled", rec.Code, loc)
			}
		})
	}
}

func TestDiscordAllowedWhenLocalAuthOn(t *testing.T) {
	h := newDiscordLocalAuthHandler(t, fixedLocalAuthPolicy{})

	rec := httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/login", nil))
	if rec.Code == http.StatusForbidden {
		t.Fatalf("login refused although local auth is on: %s", rec.Body.String())
	}

	rec = discordCallback(t, h, "login")
	if strings.Contains(rec.Header().Get("Location"), "Discord+sign-in+is+disabled") {
		t.Fatal("callback refused although local auth is on")
	}
}

// Linking a Discord identity to a signed-in account is not a sign-in.
func TestDiscordLinkCallbackNotBlocked(t *testing.T) {
	h := newDiscordLocalAuthHandler(t, fixedLocalAuthPolicy{disabled: true})
	rec := discordCallback(t, h, "link")
	if strings.Contains(rec.Header().Get("Location"), "Discord+sign-in+is+disabled") {
		t.Fatal("link callback was refused by the local-auth gate")
	}
}
