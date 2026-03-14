package handler

import (
	"net/http"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// GetHomepageHandler handles GetHomepage widget API endpoints.
type GetHomepageHandler struct {
	dashboard *store.DashboardStore
}

// NewGetHomepageHandler creates a new GetHomepage handler.
func NewGetHomepageHandler(dashboard *store.DashboardStore) *GetHomepageHandler {
	return &GetHomepageHandler{dashboard: dashboard}
}

// Stats handles GET /gethomepage/stats. Returns widget statistics for GetHomepage dashboard.
func (h *GetHomepageHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dashboard.GetHomepageStats(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch statistics")
		return
	}
	JSON(w, http.StatusOK, stats)
}

// Health handles GET /gethomepage/health. Returns a simple health check.
func (h *GetHomepageHandler) Health(w http.ResponseWriter, r *http.Request) {
	token := middleware.GetApiToken(r.Context())
	tokenName := ""
	if token != nil {
		tokenName = token.TokenName
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"api_key":   tokenName,
	})
}
