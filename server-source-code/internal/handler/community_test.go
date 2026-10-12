package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/social"
)

// setInjected swaps the build-time counts for the duration of a test.
func setInjected(t *testing.T, github, discord, youtube, linkedin string) {
	t.Helper()
	prev := [4]string{social.GitHubStars, social.DiscordMembers, social.YouTubeSubscribers, social.LinkedInFollowers}
	t.Cleanup(func() {
		social.GitHubStars, social.DiscordMembers, social.YouTubeSubscribers, social.LinkedInFollowers =
			prev[0], prev[1], prev[2], prev[3]
	})
	social.GitHubStars, social.DiscordMembers, social.YouTubeSubscribers, social.LinkedInFollowers =
		github, discord, youtube, linkedin
}

func TestApplyInjectedStat_NoInjectionKeepsDefault(t *testing.T) {
	setInjected(t, "", "", "", "")

	link := CommunityLink{ID: "github", Stat: "2.7K", StatLabel: "stars"}
	applyInjectedStat(&link)

	if link.Stat != "2.7K" || link.StatLabel != "stars" {
		t.Errorf("got stat=%q label=%q, want the compiled-in default to survive", link.Stat, link.StatLabel)
	}
}

func TestApplyInjectedStat_OverridesDefault(t *testing.T) {
	setInjected(t, "3412", "755", "1200", "418")

	tests := []struct {
		id    string
		want  string
		label string
	}{
		{"github", "3.4K", "stars"},
		{"discord", "755", "members"},
		{"youtube", "1.2K", ""},
		{"linkedin", "418", ""},
	}

	for _, tt := range tests {
		link := CommunityLink{ID: tt.id, Stat: "stale", StatLabel: tt.label}
		applyInjectedStat(&link)
		if link.Stat != tt.want {
			t.Errorf("%s: got stat %q, want %q", tt.id, link.Stat, tt.want)
		}
		if link.StatLabel != tt.label {
			t.Errorf("%s: label changed to %q, want %q", tt.id, link.StatLabel, tt.label)
		}
	}
}

// A 0 means the endpoint could not determine the count. The label has to be
// cleared with the number: LoginCommunityLinks renders the star icon off
// statLabel alone, so a label with no stat leaves an orphan icon in the UI.
func TestApplyInjectedStat_ZeroClearsStatAndLabel(t *testing.T) {
	setInjected(t, "0", "0", "0", "0")

	link := CommunityLink{ID: "github", Stat: "2.7K", StatLabel: "stars"}
	applyInjectedStat(&link)

	if link.Stat != "" {
		t.Errorf("got stat %q, want it cleared", link.Stat)
	}
	if link.StatLabel != "" {
		t.Errorf("got label %q, want it cleared so no orphan star icon renders", link.StatLabel)
	}
}

func TestApplyInjectedStat_LeavesUntrackedLinksAlone(t *testing.T) {
	setInjected(t, "3412", "755", "1200", "418")

	link := CommunityLink{ID: "email", Stat: "support@patchmon.net"}
	applyInjectedStat(&link)

	if link.Stat != "support@patchmon.net" {
		t.Errorf("got stat %q, want the email address untouched", link.Stat)
	}
}

// Garbage from a proxy or an HTML error page must not blank the UI.
func TestApplyInjectedStat_MalformedValueKeepsDefault(t *testing.T) {
	setInjected(t, "<!DOCTYPE html>", "", "", "")

	link := CommunityLink{ID: "github", Stat: "2.7K", StatLabel: "stars"}
	applyInjectedStat(&link)

	if link.Stat != "2.7K" {
		t.Errorf("got stat %q, want the compiled-in default to survive", link.Stat)
	}
}

func linkIDs(t *testing.T, cfg *config.Config) map[string]bool {
	t.Helper()
	rec := httptest.NewRecorder()
	NewCommunityHandler(cfg).GetLinks(rec, httptest.NewRequest(http.MethodGet, "/community/links", nil))
	var resp CommunityLinksResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	ids := make(map[string]bool, len(resp.Links))
	for _, l := range resp.Links {
		ids[l.ID] = true
	}
	return ids
}

func TestGetLinks_HideSocialLinks(t *testing.T) {
	ids := linkIDs(t, &config.Config{HideSocialLinks: true})
	for id := range socialLinkIDs {
		if ids[id] {
			t.Errorf("%s: social link returned while HideSocialLinks is on", id)
		}
	}
	for _, id := range []string{"docs", "roadmap", "github_issues", "email", "website"} {
		if !ids[id] {
			t.Errorf("%s: support link missing while HideSocialLinks is on", id)
		}
	}
}

func TestGetLinks_ShowSocialLinks(t *testing.T) {
	ids := linkIDs(t, &config.Config{HideSocialLinks: false})
	for id := range socialLinkIDs {
		if !ids[id] {
			t.Errorf("%s: social link missing while HideSocialLinks is off", id)
		}
	}
}

func TestGetLinks_DonateLink(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{"hidden by HideDonateButtons", &config.Config{HideDonateButtons: true}, false},
		{"hidden by AdminMode", &config.Config{AdminMode: true}, false},
		{"shown when both off", &config.Config{}, true},
		{"independent of HideSocialLinks", &config.Config{HideSocialLinks: true}, true},
	}
	for _, tt := range tests {
		if got := linkIDs(t, tt.cfg)["buymeacoffee"]; got != tt.want {
			t.Errorf("%s: buymeacoffee present = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestDiscordAuthEnabled(t *testing.T) {
	if discordAuthEnabled(&config.Config{DisableDiscordAuth: true}) {
		t.Error("discordAuthEnabled = true with DisableDiscordAuth set")
	}
	if !discordAuthEnabled(&config.Config{}) {
		t.Error("discordAuthEnabled = false with DisableDiscordAuth unset")
	}
}
