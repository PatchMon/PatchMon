package handler

import (
	"errors"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/discord"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

// TestApplyDiscordSettingsUpdate_NewFields covers the Discord-specific PUT
// contract: discord_allow_registration coerces to bool and
// discord_required_guild_id stores the value, with an empty string clearing the
// restriction (nil) — mirroring the source's `|| null` coercion — while an
// absent field leaves the existing value untouched.
func TestApplyDiscordSettingsUpdate_NewFields(t *testing.T) {
	t.Parallel()

	str := func(v string) *string { return &v }

	t.Run("sets guild id when provided non-empty", func(t *testing.T) {
		s := &models.Settings{}
		applyDiscordSettingsUpdate(s, map[string]interface{}{
			"discord_required_guild_id": "guild-42",
		}, nil)
		if s.DiscordRequiredGuildID == nil || *s.DiscordRequiredGuildID != "guild-42" {
			t.Fatalf("expected guild id guild-42, got %+v", s.DiscordRequiredGuildID)
		}
	})

	t.Run("empty string clears guild id to nil", func(t *testing.T) {
		s := &models.Settings{DiscordRequiredGuildID: str("old")}
		applyDiscordSettingsUpdate(s, map[string]interface{}{
			"discord_required_guild_id": "",
		}, nil)
		if s.DiscordRequiredGuildID != nil {
			t.Fatalf("expected nil guild id after empty update, got %+v", s.DiscordRequiredGuildID)
		}
	})

	t.Run("absent guild id leaves existing value", func(t *testing.T) {
		s := &models.Settings{DiscordRequiredGuildID: str("kept")}
		applyDiscordSettingsUpdate(s, map[string]interface{}{
			"discord_oauth_enabled": true,
		}, nil)
		if s.DiscordRequiredGuildID == nil || *s.DiscordRequiredGuildID != "kept" {
			t.Fatalf("absent field must not clear existing guild id, got %+v", s.DiscordRequiredGuildID)
		}
	})

	t.Run("allow registration toggles to true", func(t *testing.T) {
		s := &models.Settings{DiscordAllowRegistration: false}
		applyDiscordSettingsUpdate(s, map[string]interface{}{
			"discord_allow_registration": true,
		}, nil)
		if !s.DiscordAllowRegistration {
			t.Fatal("expected DiscordAllowRegistration to be true")
		}
	})

	t.Run("allow registration toggles to false", func(t *testing.T) {
		s := &models.Settings{DiscordAllowRegistration: true}
		applyDiscordSettingsUpdate(s, map[string]interface{}{
			"discord_allow_registration": false,
		}, nil)
		if s.DiscordAllowRegistration {
			t.Fatal("expected DiscordAllowRegistration to be false")
		}
	})

	t.Run("absent allow registration leaves existing value", func(t *testing.T) {
		s := &models.Settings{DiscordAllowRegistration: true}
		applyDiscordSettingsUpdate(s, map[string]interface{}{
			"discord_button_text": "Login with Discord",
		}, nil)
		if !s.DiscordAllowRegistration {
			t.Fatal("absent field must not clear existing DiscordAllowRegistration")
		}
	})

	t.Run("snake_case contract only — camelCase keys are ignored", func(t *testing.T) {
		// The two new fields use snake_case only (source 105c4e3). camelCase
		// must NOT be accepted, unlike the legacy Discord fields, so a
		// consumer that sends only camelCase cannot silently flip these flags.
		s := &models.Settings{DiscordAllowRegistration: true, DiscordRequiredGuildID: str("kept")}
		applyDiscordSettingsUpdate(s, map[string]interface{}{
			"discordAllowRegistration": false,
			"discordRequiredGuildId":   "c42",
		}, nil)
		if !s.DiscordAllowRegistration {
			t.Fatal("camelCase discordAllowRegistration must not mutate the field (snake_case-only contract)")
		}
		if s.DiscordRequiredGuildID == nil || *s.DiscordRequiredGuildID != "kept" {
			t.Fatalf("camelCase discordRequiredGuildId must not mutate the field (snake_case-only contract), got %+v", s.DiscordRequiredGuildID)
		}
	})
}

// TestShouldAutoCreateDiscordUser enforces the existing-vs-new-user gate and
// the dual requirement (global signup AND Discord opt-in). Existing users must
// always be able to log in regardless of the opt-in.
func TestShouldAutoCreateDiscordUser(t *testing.T) {
	t.Parallel()

	existing := &models.User{ID: "u1"}
	signupOn := &models.Settings{SignupEnabled: true, DiscordAllowRegistration: true}
	signupOnNotAllowed := &models.Settings{SignupEnabled: true, DiscordAllowRegistration: false}
	signupOff := &models.Settings{SignupEnabled: false, DiscordAllowRegistration: true}

	cases := []struct {
		name string
		user *models.User
		s    *models.Settings
		want bool
	}{
		{"existing user: never auto-create", existing, signupOn, false},
		{"new user, signup + allow: create", nil, signupOn, true},
		{"new user, signup on but discord not allowed: no create", nil, signupOnNotAllowed, false},
		{"new user, signup off: no create even if discord allowed", nil, signupOff, false},
		{"new user, nil settings: no create", nil, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAutoCreateDiscordUser(tc.user, tc.s); got != tc.want {
				t.Fatalf("shouldAutoCreateDiscordUser = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestGuildRequirementSatisfied enforces fail-closed behaviour: a fetch/decode
// error or empty guild list must never satisfy the requirement. With no
// required guild configured, the check is a no-op pass-through.
func TestGuildRequirementSatisfied(t *testing.T) {
	t.Parallel()

	member := []discord.Guild{{ID: "guild-42"}, {ID: "other"}}
	nonMember := []discord.Guild{{ID: "other"}}

	cases := []struct {
		name     string
		required string
		guilds   []discord.Guild
		err      error
		want     bool
	}{
		{"no restriction configured passes", "", member, nil, true},
		{"member of required guild", "guild-42", member, nil, true},
		{"not a member fails", "guild-42", nonMember, nil, false},
		{"fetch error fails closed", "guild-42", member, errors.New("boom"), false},
		{"fetch error fails closed even if list looks ok", "guild-42", member, errors.New("http 500"), false},
		{"empty guild list fails closed", "guild-42", nil, nil, false},
		{"empty list with nil err still fails for a required guild", "guild-42", []discord.Guild{}, nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := guildRequirementSatisfied(tc.required, tc.guilds, tc.err); got != tc.want {
				t.Fatalf("guildRequirementSatisfied = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestGuildRequirementSatisfied_NoRestrictionIgnoresError pins the invariant
// that when no guild is required, even a fetch error is acceptable — the check
// short-circuits before touching the network result.
func TestGuildRequirementSatisfied_NoRestrictionIgnoresError(t *testing.T) {
	t.Parallel()

	if !guildRequirementSatisfied("", nil, errors.New("would-be-fatal")) {
		t.Fatal("empty required guild must pass even when an error is supplied")
	}
}
