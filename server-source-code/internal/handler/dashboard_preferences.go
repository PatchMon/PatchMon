package handler

import (
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// DefaultDashboardPreferencesForNewUser returns default dashboard preferences for a new user.
// Used when creating users via OIDC or Discord OAuth.
func DefaultDashboardPreferencesForNewUser(userID string) []models.DashboardPreference {
	prefs := make([]models.DashboardPreference, len(defaultCardLayout))
	for i, card := range defaultCardLayout {
		prefs[i] = models.DashboardPreference{
			UserID:  userID,
			CardID:  card.CardID,
			Enabled: card.Enabled,
			Order:   card.Order,
			ColSpan: card.ColSpan,
		}
	}
	return prefs
}

// DashboardPreferencesHandler handles dashboard-preferences routes.
type DashboardPreferencesHandler struct {
	store *store.DashboardPreferencesStore
}

// NewDashboardPreferencesHandler creates a new dashboard preferences handler.
func NewDashboardPreferencesHandler(store *store.DashboardPreferencesStore) *DashboardPreferencesHandler {
	return &DashboardPreferencesHandler{store: store}
}

// Default grid layout (matches Node DEFAULT_GRID_LAYOUT).
var defaultGridLayout = struct {
	StatsColumns  int `json:"stats_columns"`
	ChartsColumns int `json:"charts_columns"`
}{
	StatsColumns:  6,
	ChartsColumns: 4,
}

// Default card layout (matches Node DEFAULT_CARD_LAYOUT exactly).
var defaultCardLayout = []struct {
	CardID             string
	RequiredPermission string
	Order              int
	Enabled            bool
	ColSpan            int
}{
	{CardID: "totalHosts", RequiredPermission: "can_view_hosts", Order: 0, Enabled: true, ColSpan: 1},
	{CardID: "upToDateHosts", RequiredPermission: "can_view_hosts", Order: 1, Enabled: true, ColSpan: 1},
	{CardID: "quickStats", RequiredPermission: "can_view_dashboard", Order: 2, Enabled: true, ColSpan: 2},
	{CardID: "hostsNeedingUpdates", RequiredPermission: "can_view_hosts", Order: 3, Enabled: true, ColSpan: 1},
	{CardID: "hostsNeedingReboot", RequiredPermission: "can_view_hosts", Order: 4, Enabled: true, ColSpan: 1},
	{CardID: "totalOutdatedPackages", RequiredPermission: "can_view_packages", Order: 5, Enabled: true, ColSpan: 1},
	{CardID: "securityUpdates", RequiredPermission: "can_view_packages", Order: 6, Enabled: true, ColSpan: 1},
	{CardID: "totalUsers", RequiredPermission: "can_view_users", Order: 7, Enabled: true, ColSpan: 1},
	{CardID: "totalHostGroups", RequiredPermission: "can_view_hosts", Order: 8, Enabled: true, ColSpan: 1},
	{CardID: "complianceStats", RequiredPermission: "can_view_hosts", Order: 9, Enabled: true, ColSpan: 1},
	{CardID: "totalRepos", RequiredPermission: "can_view_hosts", Order: 10, Enabled: true, ColSpan: 1},
	{CardID: "updateStatus", RequiredPermission: "can_view_reports", Order: 11, Enabled: true, ColSpan: 1},
	{CardID: "osDistributionBar", RequiredPermission: "can_view_reports", Order: 12, Enabled: true, ColSpan: 1},
	{CardID: "packageTrends", RequiredPermission: "can_view_packages", Order: 13, Enabled: true, ColSpan: 2},
	{CardID: "complianceHostStatus", RequiredPermission: "can_view_hosts", Order: 14, Enabled: true, ColSpan: 1},
	{CardID: "complianceActiveBenchmarkScans", RequiredPermission: "can_view_hosts", Order: 15, Enabled: true, ColSpan: 1},
	{CardID: "recentCollection", RequiredPermission: "can_view_hosts", Order: 16, Enabled: true, ColSpan: 1},
	{CardID: "recentUsers", RequiredPermission: "can_view_users", Order: 17, Enabled: true, ColSpan: 1},
	{CardID: "complianceFailuresBySeverity", RequiredPermission: "can_view_hosts", Order: 18, Enabled: true, ColSpan: 1},
	{CardID: "osDistributionDoughnut", RequiredPermission: "can_view_reports", Order: 19, Enabled: true, ColSpan: 1},
	{CardID: "complianceOpenSCAPDistribution", RequiredPermission: "can_view_hosts", Order: 20, Enabled: true, ColSpan: 2},
	{CardID: "packagePriority", RequiredPermission: "can_view_packages", Order: 21, Enabled: true, ColSpan: 1},
	{CardID: "complianceProfilesInUse", RequiredPermission: "can_view_hosts", Order: 22, Enabled: true, ColSpan: 1},
	{CardID: "complianceLastScanAge", RequiredPermission: "can_view_hosts", Order: 23, Enabled: true, ColSpan: 1},
	{CardID: "osDistribution", RequiredPermission: "can_view_reports", Order: 24, Enabled: true, ColSpan: 1},
	{CardID: "complianceTrendLine", RequiredPermission: "can_view_hosts", Order: 25, Enabled: false, ColSpan: 1},
	{CardID: "patchingRunStatus", RequiredPermission: "can_view_hosts", Order: 26, Enabled: true, ColSpan: 1},
	{CardID: "patchingRunOutcomesDoughnut", RequiredPermission: "can_view_hosts", Order: 27, Enabled: true, ColSpan: 1},
	{CardID: "patchingPendingApproval", RequiredPermission: "can_view_hosts", Order: 28, Enabled: true, ColSpan: 1},
	{CardID: "patchingRunsByType", RequiredPermission: "can_view_hosts", Order: 29, Enabled: true, ColSpan: 1},
	{CardID: "patchingActivePolicies", RequiredPermission: "can_view_hosts", Order: 30, Enabled: true, ColSpan: 1},
	{CardID: "patchingRecentRuns", RequiredPermission: "can_view_hosts", Order: 31, Enabled: true, ColSpan: 1},
}

// Card metadata (matches Node CARD_METADATA).
var cardMetadata = map[string]struct {
	Title string
	Icon  string
}{
	"totalHosts":                     {Title: "Total Hosts", Icon: "Server"},
	"hostsNeedingUpdates":            {Title: "Needs Updating", Icon: "AlertTriangle"},
	"totalOutdatedPackages":          {Title: "Outdated Packages", Icon: "Package"},
	"securityUpdates":                {Title: "Security Updates", Icon: "Shield"},
	"upToDateHosts":                  {Title: "Up to date", Icon: "CheckCircle"},
	"hostsNeedingReboot":             {Title: "Needs Reboots", Icon: "RotateCcw"},
	"totalHostGroups":                {Title: "Host Groups", Icon: "Folder"},
	"totalRepos":                     {Title: "Repositories", Icon: "GitBranch"},
	"totalUsers":                     {Title: "Users", Icon: "Users"},
	"complianceStats":                {Title: "Compliance", Icon: "Shield"},
	"quickStats":                     {Title: "Quick Stats", Icon: "TrendingUp"},
	"osDistribution":                 {Title: "OS Distribution", Icon: "BarChart3"},
	"osDistributionBar":              {Title: "OS Distribution (Bar)", Icon: "BarChart3"},
	"osDistributionDoughnut":         {Title: "OS Distribution (Doughnut)", Icon: "PieChart"},
	"recentCollection":               {Title: "Recent Collection", Icon: "Server"},
	"updateStatus":                   {Title: "Update Status", Icon: "BarChart3"},
	"packagePriority":                {Title: "Package Priority", Icon: "BarChart3"},
	"packageTrends":                  {Title: "Package Trends", Icon: "TrendingUp"},
	"recentUsers":                    {Title: "Recent Users Logged in", Icon: "Users"},
	"complianceHostStatus":           {Title: "Host Compliance Status", Icon: "BarChart3"},
	"complianceOpenSCAPDistribution": {Title: "OpenSCAP Distribution", Icon: "PieChart"},
	"complianceFailuresBySeverity":   {Title: "Failures by Severity", Icon: "PieChart"},
	"complianceProfilesInUse":        {Title: "Compliance Profiles in Use", Icon: "PieChart"},
	"complianceLastScanAge":          {Title: "Last Scan Age", Icon: "BarChart3"},
	"complianceTrendLine":            {Title: "Compliance Trend", Icon: "TrendingUp"},
	"complianceActiveBenchmarkScans": {Title: "Active Benchmark Scans", Icon: "Shield"},
	"patchingRunStatus":              {Title: "Patch Run Status", Icon: "ListChecks"},
	"patchingRunOutcomesDoughnut":    {Title: "Run Outcomes", Icon: "PieChart"},
	"patchingPendingApproval":        {Title: "Pending Approval", Icon: "AlertTriangle"},
	"patchingRunsByType":             {Title: "Runs by Type", Icon: "PieChart"},
	"patchingActivePolicies":         {Title: "Active Runs", Icon: "PlayCircle"},
	"patchingRecentRuns":             {Title: "Recent Runs", Icon: "History"},
}

// Get returns the user's dashboard preferences (GET /dashboard-preferences).
func (h *DashboardPreferencesHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	prefs, err := h.store.ListByUserID(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch dashboard preferences")
		return
	}
	// Return in Node format (snake_case for DB columns)
	out := make([]map[string]interface{}, len(prefs))
	for i, p := range prefs {
		out[i] = map[string]interface{}{
			"id":         p.ID,
			"user_id":    p.UserID,
			"card_id":    p.CardID,
			"enabled":    p.Enabled,
			"order":      p.Order,
			"col_span":   p.ColSpan,
			"created_at": p.CreatedAt,
			"updated_at": p.UpdatedAt,
		}
	}
	JSON(w, http.StatusOK, out)
}

// UpdatePreferencesRequest is the body for PUT /dashboard-preferences.
type UpdatePreferencesRequest struct {
	Preferences []PreferenceItem `json:"preferences"`
}

// PreferenceItem is a single preference in the update request.
type PreferenceItem struct {
	CardID     string `json:"cardId"`
	Enabled    bool   `json:"enabled"`
	Order      int    `json:"order"`
	ColSpan    *int   `json:"col_span,omitempty"`
	ColSpanAlt *int   `json:"colSpan,omitempty"`
}

// Update handles PUT /dashboard-preferences.
func (h *DashboardPreferencesHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req UpdatePreferencesRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.Preferences) == 0 {
		Error(w, http.StatusBadRequest, "Preferences must be a non-empty array")
		return
	}
	prefs := make([]models.DashboardPreference, len(req.Preferences))
	for i, p := range req.Preferences {
		colSpan := 1
		if p.ColSpan != nil && *p.ColSpan >= 1 && *p.ColSpan <= 3 {
			colSpan = *p.ColSpan
		} else if p.ColSpanAlt != nil && *p.ColSpanAlt >= 1 && *p.ColSpanAlt <= 3 {
			colSpan = *p.ColSpanAlt
		}
		order := p.Order
		if order < 0 {
			order = i
		}
		prefs[i] = models.DashboardPreference{
			UserID:  userID,
			CardID:  p.CardID,
			Enabled: p.Enabled,
			Order:   order,
			ColSpan: colSpan,
		}
	}
	if err := h.store.ReplaceAll(r.Context(), userID, prefs); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update dashboard preferences")
		return
	}
	// Return new preferences in Node format
	out := make([]map[string]interface{}, len(prefs))
	for i, p := range prefs {
		out[i] = map[string]interface{}{
			"id": p.ID, "user_id": p.UserID, "card_id": p.CardID,
			"enabled": p.Enabled, "order": p.Order, "col_span": p.ColSpan,
			"updated_at": p.UpdatedAt,
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":     "Dashboard preferences updated successfully",
		"preferences": out,
	})
}

// GetLayout returns GET /dashboard-preferences/layout.
func (h *DashboardPreferencesHandler) GetLayout(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	layout, err := h.store.GetLayout(r.Context(), userID)
	if err != nil || layout == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"stats_columns":  defaultGridLayout.StatsColumns,
			"charts_columns": defaultGridLayout.ChartsColumns,
		})
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"stats_columns":  layout.StatsColumns,
		"charts_columns": layout.ChartsColumns,
	})
}

// UpdateLayoutRequest is the body for PUT /dashboard-preferences/layout.
type UpdateLayoutRequest struct {
	StatsColumns  *int `json:"stats_columns,omitempty"`
	ChartsColumns *int `json:"charts_columns,omitempty"`
}

// UpdateLayout handles PUT /dashboard-preferences/layout.
func (h *DashboardPreferencesHandler) UpdateLayout(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req UpdateLayoutRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	statsCols := defaultGridLayout.StatsColumns
	chartsCols := defaultGridLayout.ChartsColumns
	if req.StatsColumns != nil && *req.StatsColumns >= 2 && *req.StatsColumns <= 6 {
		statsCols = *req.StatsColumns
	}
	if req.ChartsColumns != nil && *req.ChartsColumns >= 2 && *req.ChartsColumns <= 4 {
		chartsCols = *req.ChartsColumns
	}
	layout := &models.DashboardLayout{
		UserID:        userID,
		StatsColumns:  statsCols,
		ChartsColumns: chartsCols,
	}
	if err := h.store.UpsertLayout(r.Context(), layout); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update dashboard layout")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":        "Dashboard layout updated successfully",
		"stats_columns":  statsCols,
		"charts_columns": chartsCols,
	})
}

// GetDefaults returns GET /dashboard-preferences/defaults.
func (h *DashboardPreferencesHandler) GetDefaults(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	cards := make([]map[string]interface{}, len(defaultCardLayout))
	for i, card := range defaultCardLayout {
		meta := cardMetadata[card.CardID]
		if meta.Title == "" {
			meta = struct{ Title, Icon string }{Title: card.CardID, Icon: "BarChart3"}
		}
		cards[i] = map[string]interface{}{
			"cardId":   card.CardID,
			"title":    meta.Title,
			"icon":     meta.Icon,
			"enabled":  card.Enabled,
			"order":    card.Order,
			"col_span": card.ColSpan,
		}
	}
	JSON(w, http.StatusOK, cards)
}
