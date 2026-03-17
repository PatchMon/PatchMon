package handler

import "net/http"

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
	{ID: "discord", URL: "https://patchmon.net/discord", Label: "Discord", Stat: "500", StatLabel: "members"},
	{ID: "github", URL: "https://github.com/PatchMon/PatchMon", Label: "GitHub", Stat: "2.1K", StatLabel: "stars"},
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
type CommunityHandler struct{}

// NewCommunityHandler creates a new community handler.
func NewCommunityHandler() *CommunityHandler {
	return &CommunityHandler{}
}

// GetLinks returns community links and stats for nav bar, login UI, and wizard.
// Public endpoint - no auth required.
func (h *CommunityHandler) GetLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		Error(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	JSON(w, http.StatusOK, CommunityLinksResponse{Links: defaultCommunityLinks})
}
