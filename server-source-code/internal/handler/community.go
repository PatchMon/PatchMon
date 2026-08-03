package handler

import (
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/social"
)

// CommunityLink represents a single community/social link with optional stat.
type CommunityLink struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Label     string `json:"label"`
	Stat      string `json:"stat,omitempty"`      // e.g. "2.1K", "500"
	StatLabel string `json:"statLabel,omitempty"` // e.g. "stars", "members"
}

// CommunityLinksResponse is the response for GET /community/links.
type CommunityLinksResponse struct {
	Links []CommunityLink `json:"links"`
}

// Default community links and stats. The counts below are the last known good
// values and are only used when the build injected nothing; see internal/social.
var defaultCommunityLinks = []CommunityLink{
	{ID: "discord", URL: "https://patchmon.net/discord", Label: "Discord", Stat: "811", StatLabel: "members"},
	{ID: "github", URL: "https://github.com/PatchMon/PatchMon", Label: "GitHub", Stat: "3.2K", StatLabel: "stars"},
	{ID: "email", URL: "mailto:support@patchmon.net", Label: "Email", Stat: "support@patchmon.net"},
	{ID: "linkedin", URL: "https://linkedin.com/company/patchmon", Label: "LinkedIn", Stat: "803"},
	{ID: "youtube", URL: "https://www.youtube.com/@PatchMonTV", Label: "YouTube", Stat: "194"},
	{ID: "buymeacoffee", URL: "https://buymeacoffee.com/iby___", Label: "Buy Me a Coffee"},
	// Feature requests and bugs are tracked in two separate places: the feedback
	// portal owns the feature roadmap, GitHub Issues owns bugs. Bugs never enter
	// the portal, so these must not be collapsed back into one link.
	{ID: "roadmap", URL: "https://feedback.patchmon.net/roadmap", Label: "Feature Roadmap"},
	{ID: "github_issues", URL: "https://github.com/PatchMon/PatchMon/issues", Label: "Report a Bug"},
	{ID: "docs", URL: "https://patchmon.net/docs", Label: "Documentation"},
	{ID: "website", URL: "https://patchmon.net", Label: "Website"},
}

// injectedCounts maps a link ID to the raw count baked in at build time.
var injectedCounts = map[string]*string{
	"github":   &social.GitHubStars,
	"discord":  &social.DiscordMembers,
	"youtube":  &social.YouTubeSubscribers,
	"linkedin": &social.LinkedInFollowers,
}

// applyInjectedStat overwrites a link's stat with the build-time count when one
// was injected. A count of 0 clears the stat and its label so the UI renders the
// link without a number, rather than showing a stale one.
func applyInjectedStat(l *CommunityLink) {
	raw, tracked := injectedCounts[l.ID]
	if !tracked {
		return
	}
	n, ok := social.Count(*raw)
	if !ok {
		return
	}
	l.Stat = social.Format(n)
	if l.Stat == "" {
		l.StatLabel = ""
	}
}

// CommunityHandler handles community/social links (public).
type CommunityHandler struct {
	cfg *config.Config
}

// NewCommunityHandler creates a new community handler.
func NewCommunityHandler(cfg *config.Config) *CommunityHandler {
	return &CommunityHandler{cfg: cfg}
}

// GetLinks returns community links and stats for nav bar, login UI, and wizard.
// Public endpoint - no auth required.
func (h *CommunityHandler) GetLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	links := make([]CommunityLink, 0, len(defaultCommunityLinks))
	for _, l := range defaultCommunityLinks {
		// Hide donate link in managed/multi-context deployments.
		if h.cfg != nil && h.cfg.AdminMode && l.ID == "buymeacoffee" {
			continue
		}
		applyInjectedStat(&l)
		links = append(links, l)
	}
	if h.cfg != nil && h.cfg.AdminMode && h.cfg.BillingPortalURL != "" {
		links = append(links, CommunityLink{
			ID:    "billing",
			URL:   h.cfg.BillingPortalURL,
			Label: "Manage membership",
		})
	}
	JSON(w, http.StatusOK, CommunityLinksResponse{Links: links})
}
