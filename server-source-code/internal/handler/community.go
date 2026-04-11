package handler

import (
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
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

// Default community links and stats. Override via env or config if needed.
var defaultCommunityLinks = []CommunityLink{
	{ID: "discord", URL: "https://patchmon.net/discord", Label: "Discord", Stat: "600", StatLabel: "members"},
	{ID: "github", URL: "https://github.com/PatchMon/PatchMon", Label: "GitHub", Stat: "2.5K", StatLabel: "stars"},
	{ID: "github_issues", URL: "https://github.com/PatchMon/PatchMon/issues", Label: "GitHub Issues"},
	{ID: "email", URL: "mailto:support@patchmon.net", Label: "Email", Stat: "support@patchmon.net"},
	{ID: "linkedin", URL: "https://linkedin.com/company/patchmon", Label: "LinkedIn", Stat: "400"},
	{ID: "youtube", URL: "https://www.youtube.com/@PatchMonTV", Label: "YouTube", Stat: "130"},
	{ID: "buymeacoffee", URL: "https://buymeacoffee.com/iby___", Label: "Buy Me a Coffee"},
	{ID: "roadmap", URL: "https://github.com/orgs/PatchMon/projects/2/views/1", Label: "Roadmap"},
	{ID: "docs", URL: "https://docs.patchmon.net", Label: "Documentation"},
	{ID: "website", URL: "https://patchmon.net", Label: "Website"},
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
