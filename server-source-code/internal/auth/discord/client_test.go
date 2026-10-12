package discord

import (
	"net/url"
	"strings"
	"testing"
)

// TestGenerateAuthURL_ScopeConditional guards the source feature's intent:
// the `guilds` scope is requested only when a required guild is configured,
// so the ordinary login/link flow never asks for server-list access.
func TestGenerateAuthURL_ScopeConditional(t *testing.T) {
	t.Parallel()

	c := &Config{ClientID: "cid", ClientSecret: "secret", RedirectURI: "https://app/cb"}

	authURL, err := c.GenerateAuthURL("state123", "verifier")
	if err != nil {
		t.Fatalf("GenerateAuthURL: %v", err)
	}
	scope := scopeFromAuthURL(t, authURL)
	if scope != discordScopes {
		t.Fatalf("expected scopes %q without required guild, got %q", discordScopes, scope)
	}
	if containsGuilds(scope) {
		t.Fatalf("guilds scope must not be requested when no required guild is set, got %q", scope)
	}

	c.RequiredGuildID = "guild-42"
	authURL, err = c.GenerateAuthURL("state123", "verifier")
	if err != nil {
		t.Fatalf("GenerateAuthURL with required guild: %v", err)
	}
	scope = scopeFromAuthURL(t, authURL)
	if !containsGuilds(scope) {
		t.Fatalf("guilds scope must be requested when required guild is set, got %q", scope)
	}
	// identify+email remain the baseline in both cases.
	for _, want := range []string{"identify", "email"} {
		if !containsToken(scope, want) {
			t.Fatalf("expected scope to include %q, got %q", want, scope)
		}
	}
}

// TestIsGuildMember covers required-guild success and failure, plus the
// "no restriction configured" pass-through.
func TestIsGuildMember(t *testing.T) {
	t.Parallel()

	guilds := []Guild{{ID: "111"}, {ID: "guild-42"}, {ID: "999"}}

	cases := []struct {
		name     string
		guilds   []Guild
		required string
		want     bool
	}{
		{"empty required always passes", guilds, "", true},
		{"empty required passes with no guilds", nil, "", true},
		{"member of required guild", guilds, "guild-42", true},
		{"not a member of required guild", guilds, "guild-404", false},
		{"member check with empty guild list fails", nil, "guild-42", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsGuildMember(tc.guilds, tc.required); got != tc.want {
				t.Fatalf("IsGuildMember = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestParseGuilds_Success_Failure_Decode covers the decode-must-fail path the
// callback relies on for fail-closed enforcement.
func TestParseGuilds(t *testing.T) {
	t.Parallel()

	t.Run("valid payload", func(t *testing.T) {
		body := []byte(`[{"id":"111","name":"A","icon":null,"owner":true},{"id":"222","name":"B","icon":"abc","owner":false}]`)
		guilds, err := parseGuilds(body)
		if err != nil {
			t.Fatalf("parseGuilds: %v", err)
		}
		if len(guilds) != 2 || guilds[0].ID != "111" || guilds[1].ID != "222" {
			t.Fatalf("unexpected guilds: %+v", guilds)
		}
	})

	t.Run("malformed JSON must error", func(t *testing.T) {
		if _, err := parseGuilds([]byte(`<html>not json</html>`)); err == nil {
			t.Fatal("expected error parsing non-JSON guilds response")
		}
	})

	t.Run("empty array is valid and empty", func(t *testing.T) {
		guilds, err := parseGuilds([]byte(`[]`))
		if err != nil {
			t.Fatalf("parseGuilds: %v", err)
		}
		if len(guilds) != 0 {
			t.Fatalf("expected empty guilds, got %+v", guilds)
		}
	})
}

func scopeFromAuthURL(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	return u.Query().Get("scope")
}

func containsGuilds(scope string) bool { return containsToken(scope, "guilds") }

func containsToken(scope, token string) bool {
	for _, value := range strings.Fields(scope) {
		if value == token {
			return true
		}
	}
	return false
}
