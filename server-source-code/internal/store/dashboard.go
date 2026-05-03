package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/PatchMon/PatchMon/server-source-code/internal/safeconv"
	"github.com/jackc/pgx/v5/pgtype"
)

// DashboardStore provides dashboard stats.
type DashboardStore struct {
	db database.DBProvider
}

// NewDashboardStore creates a new dashboard store.
func NewDashboardStore(db database.DBProvider) *DashboardStore {
	return &DashboardStore{db: db}
}

// withWorkMem is a thin wrapper around the package-level helper so existing
// dashboard call sites (s.withWorkMem(...)) keep compiling.
func (s *DashboardStore) withWorkMem(ctx context.Context, fn func(q *db.Queries) error) error {
	return withWorkMemTx(ctx, s.db, fn)
}

// GetStats returns dashboard statistics matching Node backend structure for frontend compatibility.
func (s *DashboardStore) GetStats(ctx context.Context) (map[string]interface{}, error) {
	now := time.Now()
	updateIntervalMinutes := 60
	if settings, _ := s.getSettings(ctx); settings != nil {
		updateIntervalMinutes = settings.UpdateInterval
		if updateIntervalMinutes <= 0 {
			updateIntervalMinutes = 60
		}
	}
	thresholdMinutes := updateIntervalMinutes * 2
	thresholdTime := now.Add(-time.Duration(thresholdMinutes) * time.Minute)
	offlineThreshold := now.Add(-time.Duration(updateIntervalMinutes*3) * time.Minute)

	// Collect everything that benefits from the work_mem bump in a single
	// transaction. Settings + tableExists stay outside (they're cheap and
	// don't need session memory).
	var (
		stats  db.GetDashboardStatsRow
		osRows []db.GetOSDistributionByTypeAndVersionRow
		trends = []interface{}{}
		hasUH  = s.tableExists(ctx, "update_history")
		runErr error
	)
	runErr = s.withWorkMem(ctx, func(q *db.Queries) error {
		var err error
		stats, err = q.GetDashboardStats(ctx, db.GetDashboardStatsParams{
			LastUpdate:   pgtime.From(thresholdTime),
			LastUpdate_2: pgtime.From(offlineThreshold),
		})
		if err != nil {
			return err
		}
		osRows, _ = q.GetOSDistributionByTypeAndVersion(ctx)
		if hasUH {
			trendsSince := now.AddDate(0, 0, -7)
			trendRows, _ := q.GetUpdateTrends(ctx, pgtime.From(trendsSince))
			for _, r := range trendRows {
				tsStr := ""
				if r.Ts.Valid {
					tsStr = r.Ts.Time.Format(time.RFC3339)
				}
				trends = append(trends, map[string]interface{}{
					"timestamp": tsStr,
					"_count":    map[string]interface{}{"id": r.Cnt},
					"_sum":      map[string]interface{}{"packages_count": r.PkgSum, "security_count": r.SecSum},
				})
			}
		}
		return nil
	})
	if runErr != nil {
		return nil, runErr
	}

	totalHosts := int(stats.TotalHosts)
	hostsNeedingUpdates := int(stats.HostsNeedingUpdates)
	totalOutdatedPackages := int(stats.TotalOutdatedPackages)
	erroredHosts := int(stats.ErroredHosts)
	securityUpdates := int(stats.SecurityUpdates)
	offlineHosts := int(stats.OfflineHosts)
	hostsNeedingReboot := int(stats.HostsNeedingReboot)
	totalHostGroups := int(stats.Column8)
	totalUsers := int(stats.Column9)
	totalRepos := int(stats.Column10)

	upToDateHosts := totalHosts - hostsNeedingUpdates
	if upToDateHosts < 0 {
		upToDateHosts = 0
	}

	osDistribution := make([]map[string]interface{}, len(osRows))
	for i, r := range osRows {
		osDistribution[i] = map[string]interface{}{
			"name": r.Name, "count": r.Count,
			"os_type": r.OsType, "os_version": r.OsVersion,
		}
	}

	updateStatusDistribution := []map[string]interface{}{
		{"name": "Up to date", "count": upToDateHosts},
		{"name": "Needs updates", "count": hostsNeedingUpdates},
		{"name": "Errored", "count": erroredHosts},
	}

	regularUpdates := totalOutdatedPackages - securityUpdates
	if regularUpdates < 0 {
		regularUpdates = 0
	}
	packageUpdateDistribution := []map[string]interface{}{
		{"name": "Security", "count": securityUpdates},
		{"name": "Regular", "count": regularUpdates},
	}

	return map[string]interface{}{
		"cards": map[string]interface{}{
			"totalHosts":            totalHosts,
			"hostsNeedingUpdates":   hostsNeedingUpdates,
			"upToDateHosts":         upToDateHosts,
			"totalOutdatedPackages": totalOutdatedPackages,
			"erroredHosts":          erroredHosts,
			"securityUpdates":       securityUpdates,
			"offlineHosts":          offlineHosts,
			"hostsNeedingReboot":    hostsNeedingReboot,
			"totalHostGroups":       totalHostGroups,
			"totalUsers":            totalUsers,
			"totalRepos":            totalRepos,
		},
		"charts": map[string]interface{}{
			"osDistribution":            osDistribution,
			"updateStatusDistribution":  updateStatusDistribution,
			"packageUpdateDistribution": packageUpdateDistribution,
		},
		"trends":      trends,
		"lastUpdated": now.Format(time.RFC3339),
	}, nil
}

// GetHomepageStats returns statistics for the GetHomepage widget (matches Node backend structure).
func (s *DashboardStore) GetHomepageStats(ctx context.Context) (map[string]interface{}, error) {
	d := s.db.DB(ctx)
	now := time.Now()
	oneDayAgo := now.Add(-24 * time.Hour)

	stats, err := d.Queries.GetHomepageStats(ctx, pgtime.From(oneDayAgo))
	if err != nil {
		return nil, err
	}

	totalHosts := int(stats.TotalHosts)
	hostsNeedingUpdates := int(stats.HostsNeedingUpdates)
	upToDateHosts := totalHosts - hostsNeedingUpdates
	if upToDateHosts < 0 {
		upToDateHosts = 0
	}

	osRows, _ := d.Queries.GetOSDistributionByTypeAndVersion(ctx)
	osDistribution := make([]map[string]interface{}, len(osRows))
	for i, r := range osRows {
		osDistribution[i] = map[string]interface{}{
			"name": r.Name, "count": r.Count,
			"os_type": r.OsType, "os_version": r.OsVersion,
		}
	}

	topOS1 := map[string]interface{}{"name": "None", "count": 0}
	topOS2 := map[string]interface{}{"name": "None", "count": 0}
	topOS3 := map[string]interface{}{"name": "None", "count": 0}
	if len(osDistribution) > 0 {
		topOS1 = osDistribution[0]
	}
	if len(osDistribution) > 1 {
		topOS2 = osDistribution[1]
	}
	if len(osDistribution) > 2 {
		topOS3 = osDistribution[2]
	}

	topOS1Name, _ := topOS1["name"].(string)
	topOS1Count, _ := topOS1["count"].(int32)
	topOS2Name, _ := topOS2["name"].(string)
	topOS2Count, _ := topOS2["count"].(int32)
	topOS3Name, _ := topOS3["name"].(string)
	topOS3Count, _ := topOS3["count"].(int32)

	return map[string]interface{}{
		"total_hosts":                 totalHosts,
		"total_outdated_packages":     int(stats.TotalOutdatedPackages),
		"total_repos":                 int(stats.TotalRepos),
		"hosts_needing_updates":       hostsNeedingUpdates,
		"up_to_date_hosts":            upToDateHosts,
		"security_updates":            int(stats.SecurityUpdates),
		"hosts_with_security_updates": int(stats.HostsWithSecurityUpdates),
		"recent_updates_24h":          int(stats.RecentUpdates24h),
		"os_distribution":             osDistribution,
		"top_os_1_name":               topOS1Name,
		"top_os_1_count":              topOS1Count,
		"top_os_2_name":               topOS2Name,
		"top_os_2_count":              topOS2Count,
		"top_os_3_name":               topOS3Name,
		"top_os_3_count":              topOS3Count,
		"last_updated":                now.Format(time.RFC3339),
	}, nil
}

func (s *DashboardStore) getSettings(ctx context.Context) (*models.Settings, error) {
	d := s.db.DB(ctx)
	setting, err := d.Queries.GetFirstSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := dbSettingToModel(setting)
	return &out, nil
}

func (s *DashboardStore) tableExists(ctx context.Context, name string) bool {
	d := s.db.DB(ctx)
	var exists bool
	err := d.RawQueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, name).Scan(&exists)
	return err == nil && exists
}

// HostWithCounts has host info plus package counts.
type HostWithCounts struct {
	ID                   string
	MachineID            *string
	FriendlyName         string
	Hostname             *string
	IP                   *string
	OSType               string
	OSVersion            string
	Status               string
	AgentVersion         *string
	AutoUpdate           bool
	Notes                *string
	ApiID                string
	NeedsReboot          *bool
	SystemUptime         *string
	DockerEnabled        bool
	ComplianceEnabled    bool
	ComplianceOnDemand   bool
	LastUpdate           *time.Time
	UpdatesCount         int
	SecurityUpdatesCount int
	TotalPackagesCount   int
}

type dashboardHostRow struct {
	ID                     string
	MachineID              *string
	FriendlyName           string
	Hostname               *string
	IP                     *string
	OSType                 string
	OSVersion              string
	Status                 string
	AgentVersion           *string
	AutoUpdate             bool
	Notes                  *string
	ApiID                  string
	NeedsReboot            *bool
	RebootReason           *string
	SystemUptime           *string
	DockerEnabled          bool
	ComplianceEnabled      bool
	ComplianceOnDemandOnly bool
	LastUpdate             pgtype.Timestamp
	SsgVersion             interface{}
	UpdatesCount           int32
	SecurityUpdatesCount   int32
	TotalPackagesCount     int32
}

// HostsListParams holds optional filters for GetHostsWithCounts.
//
// Sort/Order/Limit/Offset drive server-side pagination. Defaults:
// Sort = "last_update", Order = "desc", Limit = 100, Offset = 0.
//
// The hard cap (HostsListMaxLimit, 5000) is a sanity bound to prevent
// pathological requests from materialising tens of millions of host_packages
// rows in one transaction; it is not a UX-facing page size limit. The
// HTTP handler enforces a smaller cap (500) for caller-supplied `limit`
// to keep paginated UIs honest, but server-internal callers (legacy
// unwrapped /dashboard/hosts, notification report renderer) may pass up
// to HostsListMaxLimit when they intentionally want a wide page.
type HostsListParams struct {
	Search      string
	Group       string
	Status      string
	OS          string
	OSVersion   string
	Filter      string
	SelectedIDs []string
	RebootOnly  bool
	HideStale   bool
	Sort        string // whitelisted public sort key
	Order       string // "asc" | "desc"
	Limit       int    // page size (1..HostsListMaxLimit; default 100)
	Offset      int    // starting row (>= 0)
}

// HostsListMaxLimit is the absolute upper bound on rows returned by
// GetHostsWithCounts in a single call. Sized to comfortably cover any
// realistic single-screen fleet view; beyond this, callers should
// paginate. At ~1 KB per host JSON the resulting payload is ~5 MB.
const HostsListMaxLimit = 5000

// HostsListResult bundles a paginated page of hosts with the total
// count matching the same filters. Total is what the UI uses to render
// "Showing X-Y of Z" and the page selector.
type HostsListResult struct {
	Items  []map[string]interface{} `json:"items"`
	Total  int                      `json:"total"`
	Limit  int                      `json:"limit"`
	Offset int                      `json:"offset"`
}

// hostsListSortWhitelist maps the public sort keys to the same identifiers
// used in the SQL CASE statements. Keep in sync with the ORDER BY clause
// in queries/dashboard.sql:GetHostsWithCounts.
var hostsListSortWhitelist = map[string]string{
	"agent_version":    "agent_version",
	"friendly_name":    "friendly_name",
	"group":            "group",
	"hostname":         "hostname",
	"integrations":     "integrations",
	"ip":               "ip",
	"last_update":      "last_update",
	"needs_reboot":     "needs_reboot",
	"notes":            "notes",
	"os_type":          "os_type",
	"os_version":       "os_version",
	"security_updates": "security_updates",
	"ssg_version":      "ssg_version",
	"status":           "status",
	"updates":          "updates",
	"uptime":           "uptime",
}

// HostsListSortKey normalises a public sort key to the identifier used by
// the sqlc query. The boolean return lets handlers reject unsupported sorts
// instead of silently serving a different order than requested.
func HostsListSortKey(sort string) (string, bool) {
	if sort == "" {
		return "last_update", true
	}
	key, ok := hostsListSortWhitelist[sort]
	return key, ok
}

// GetHostsWithCounts returns a page of hosts with package counts plus the
// total matching the same filters. Backwards-compat: callers that pass
// a zero Limit get the default (100), matching the previous "all rows"
// shape only insofar as small fleets fit in one page.
func (s *DashboardStore) GetHostsWithCounts(ctx context.Context, params HostsListParams) (*HostsListResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > HostsListMaxLimit {
		limit = HostsListMaxLimit
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	sortKey, ok := HostsListSortKey(params.Sort)
	if !ok {
		sortKey = "last_update"
	}
	order := strings.ToLower(params.Order)
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	arg := db.GetHostsWithCountsParams{
		SelectedIds:    params.SelectedIDs,
		StaleThreshold: pgtime.From(time.Now().Add(-time.Duration(UpdateIntervalMinutes(ctx, s)*2) * time.Minute)),
		RebootOnly:     params.RebootOnly,
		HideStale:      params.HideStale,
		SortKey:        sortKey,
		SortDir:        order,
		RowLimit:       safeconv.ClampToInt32(limit),
		RowOffset:      safeconv.ClampToInt32(offset),
	}
	countArg := db.CountHostsForListParams{
		SelectedIds:    params.SelectedIDs,
		StaleThreshold: arg.StaleThreshold,
		RebootOnly:     params.RebootOnly,
		HideStale:      params.HideStale,
	}
	if params.Search != "" {
		arg.Search = &params.Search
		countArg.Search = &params.Search
	}
	if params.Group != "" {
		arg.Group = &params.Group
		countArg.Group = &params.Group
	}
	if params.Status != "" {
		arg.Status = &params.Status
		countArg.Status = &params.Status
	}
	if params.OS != "" {
		arg.Os = &params.OS
		countArg.Os = &params.OS
	}
	if params.OSVersion != "" {
		arg.OsVersion = &params.OSVersion
		countArg.OsVersion = &params.OSVersion
	}
	if params.Filter != "" {
		arg.Filter = &params.Filter
		countArg.Filter = &params.Filter
	}
	pageArg := db.GetHostsWithPageCountsParams{
		SelectedIds:    arg.SelectedIds,
		StaleThreshold: arg.StaleThreshold,
		RebootOnly:     arg.RebootOnly,
		HideStale:      arg.HideStale,
		SortKey:        arg.SortKey,
		SortDir:        arg.SortDir,
		RowLimit:       arg.RowLimit,
		RowOffset:      arg.RowOffset,
		Search:         arg.Search,
		Group:          arg.Group,
		Status:         arg.Status,
		Os:             arg.Os,
		OsVersion:      arg.OsVersion,
		Filter:         arg.Filter,
	}

	var (
		rows      []dashboardHostRow
		total     int32
		groupsMap = make(map[string][]map[string]interface{})
	)
	if err := s.withWorkMem(ctx, func(q *db.Queries) error {
		var err error
		if sortKey == "updates" || sortKey == "security_updates" {
			fullRows, fullErr := q.GetHostsWithCounts(ctx, arg)
			if fullErr != nil {
				return fullErr
			}
			rows = make([]dashboardHostRow, len(fullRows))
			for i, r := range fullRows {
				rows[i] = dashboardHostRow{
					ID: r.ID, MachineID: r.MachineID, FriendlyName: r.FriendlyName, Hostname: r.Hostname,
					IP: r.Ip, OSType: r.OsType, OSVersion: r.OsVersion, Status: r.Status,
					AgentVersion: r.AgentVersion, AutoUpdate: r.AutoUpdate, Notes: r.Notes, ApiID: r.ApiID,
					NeedsReboot: r.NeedsReboot, RebootReason: r.RebootReason, SystemUptime: r.SystemUptime,
					DockerEnabled: r.DockerEnabled, ComplianceEnabled: r.ComplianceEnabled,
					ComplianceOnDemandOnly: r.ComplianceOnDemandOnly, LastUpdate: r.LastUpdate,
					SsgVersion: r.SsgVersion, UpdatesCount: r.UpdatesCount,
					SecurityUpdatesCount: r.SecurityUpdatesCount, TotalPackagesCount: r.TotalPackagesCount,
				}
			}
		} else {
			pageRows, pageErr := q.GetHostsWithPageCounts(ctx, pageArg)
			if pageErr != nil {
				return pageErr
			}
			rows = make([]dashboardHostRow, len(pageRows))
			for i, r := range pageRows {
				rows[i] = dashboardHostRow{
					ID: r.ID, MachineID: r.MachineID, FriendlyName: r.FriendlyName, Hostname: r.Hostname,
					IP: r.Ip, OSType: r.OsType, OSVersion: r.OsVersion, Status: r.Status,
					AgentVersion: r.AgentVersion, AutoUpdate: r.AutoUpdate, Notes: r.Notes, ApiID: r.ApiID,
					NeedsReboot: r.NeedsReboot, RebootReason: r.RebootReason, SystemUptime: r.SystemUptime,
					DockerEnabled: r.DockerEnabled, ComplianceEnabled: r.ComplianceEnabled,
					ComplianceOnDemandOnly: r.ComplianceOnDemandOnly, LastUpdate: r.LastUpdate,
					SsgVersion: r.SsgVersion, UpdatesCount: r.UpdatesCount,
					SecurityUpdatesCount: r.SecurityUpdatesCount, TotalPackagesCount: r.TotalPackagesCount,
				}
			}
		}
		total, err = q.CountHostsForList(ctx, countArg)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		hostIDs := make([]string, len(rows))
		for i, r := range rows {
			hostIDs[i] = r.ID
		}
		groupRows, _ := q.GetHostGroupsForHosts(ctx, hostIDs)
		for _, r := range groupRows {
			m := map[string]interface{}{
				"host_groups": map[string]interface{}{"id": r.ID, "name": r.Name, "color": r.Color},
			}
			groupsMap[r.HostID] = append(groupsMap[r.HostID], m)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	updateIntervalMinutes := UpdateIntervalMinutes(ctx, s)
	thresholdMinutes := updateIntervalMinutes * 2
	thresholdTime := time.Now().Add(-time.Duration(thresholdMinutes) * time.Minute)

	result := make([]map[string]interface{}, len(rows))
	for i, h := range rows {
		var lastUpdate *time.Time
		if h.LastUpdate.Valid {
			lastUpdate = &h.LastUpdate.Time
		}
		isStale := false
		effectiveStatus := h.Status
		if lastUpdate != nil && h.Status == "active" {
			if lastUpdate.Before(thresholdTime) {
				isStale = true
				effectiveStatus = "inactive"
			}
		}
		lastUpdateStr := ""
		if lastUpdate != nil {
			lastUpdateStr = lastUpdate.Format(time.RFC3339)
		}
		hostGroups := groupsMap[h.ID]
		if hostGroups == nil {
			hostGroups = []map[string]interface{}{}
		}
		result[i] = map[string]interface{}{
			"id": h.ID, "machine_id": h.MachineID, "friendly_name": h.FriendlyName, "hostname": h.Hostname,
			"ip": h.IP, "os_type": h.OSType, "os_version": h.OSVersion,
			"status": h.Status, "agent_version": h.AgentVersion, "auto_update": h.AutoUpdate,
			"notes": h.Notes, "api_id": h.ApiID, "needs_reboot": h.NeedsReboot, "reboot_reason": h.RebootReason,
			"system_uptime":  h.SystemUptime,
			"docker_enabled": h.DockerEnabled, "compliance_enabled": h.ComplianceEnabled,
			"compliance_on_demand_only": h.ComplianceOnDemandOnly,
			"last_update":               lastUpdateStr, "isStale": isStale, "effectiveStatus": effectiveStatus,
			"host_group_memberships": hostGroups,
			"ssg_version":            h.SsgVersion,
			"updatesCount":           h.UpdatesCount, "securityUpdatesCount": h.SecurityUpdatesCount,
			"totalPackagesCount": h.TotalPackagesCount,
		}
	}
	return &HostsListResult{
		Items:  result,
		Total:  int(total),
		Limit:  limit,
		Offset: offset,
	}, nil
}

// HostCountsResult is the wire shape for the cheap host-counts endpoint.
// All fields are non-nullable counts derived in a single SQL pass.
type HostCountsResult struct {
	Total        int `json:"total"`
	Up           int `json:"up"`
	Stale        int `json:"stale"`
	Down         int `json:"down"`
	Inactive     int `json:"inactive"`
	NeedsReboot  int `json:"needsReboot"`
	NeedsUpdates int `json:"needsUpdates"`
}

// GetHostCounts returns sidebar-shaped host counters in one round-trip.
// stale = active + last_update older than the stale threshold but newer
// than the down threshold. down = active + last_update older than the
// down threshold. The two thresholds are computed by the caller from
// the configured update interval.
func (s *DashboardStore) GetHostCounts(ctx context.Context, staleThreshold, downThreshold time.Time) (*HostCountsResult, error) {
	d := s.db.DB(ctx)
	row, err := d.Queries.GetHostCounts(ctx, db.GetHostCountsParams{
		LastUpdate:   pgtime.From(staleThreshold),
		LastUpdate_2: pgtime.From(downThreshold),
	})
	if err != nil {
		return nil, err
	}
	return &HostCountsResult{
		Total:        int(row.Total),
		Up:           int(row.Up),
		Stale:        int(row.Stale),
		Down:         int(row.Down),
		Inactive:     int(row.Inactive),
		NeedsReboot:  int(row.NeedsReboot),
		NeedsUpdates: int(row.NeedsUpdates),
	}, nil
}

// GetHostFilterOptions returns cheap host-only filter metadata for the Hosts UI.
func (s *DashboardStore) GetHostFilterOptions(ctx context.Context) (map[string]interface{}, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetOSDistributionByTypeAndVersion(ctx)
	if err != nil {
		return nil, err
	}
	osDistribution := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		osDistribution[i] = map[string]interface{}{
			"name":       r.Name,
			"count":      r.Count,
			"os_type":    r.OsType,
			"os_version": r.OsVersion,
		}
	}
	return map[string]interface{}{"osDistribution": osDistribution}, nil
}

// GetNavigationStats returns cheap sidebar/header counters without running
// the full dashboard aggregate query.
func (s *DashboardStore) GetNavigationStats(ctx context.Context) (map[string]interface{}, error) {
	d := s.db.DB(ctx)
	row, err := d.Queries.GetNavigationStats(ctx)
	if err != nil {
		return nil, err
	}
	lastUpdated := time.Now().Format(time.RFC3339)
	if row.LastUpdated.Valid {
		lastUpdated = row.LastUpdated.Time.Format(time.RFC3339)
	}
	return map[string]interface{}{
		"cards": map[string]interface{}{
			"totalHosts":            int(row.TotalHosts),
			"totalOutdatedPackages": int(row.TotalOutdatedPackages),
			"totalRepos":            int(row.TotalRepos),
		},
		"lastUpdated": lastUpdated,
	}, nil
}

// UpdateIntervalMinutesOrDefault reads the configured agent update
// interval (Settings.UpdateInterval) with a 60-minute fallback. Used by
// the host-counts handler to derive stale/down thresholds.
func (s *DashboardStore) UpdateIntervalMinutesOrDefault(ctx context.Context) int {
	return UpdateIntervalMinutes(ctx, s)
}

// UpdateIntervalMinutes reads the configured agent update interval with a
// conservative fallback. It is a helper so the list and count endpoints use
// exactly the same stale/down thresholds.
func UpdateIntervalMinutes(ctx context.Context, s *DashboardStore) int {
	if settings, _ := s.getSettings(ctx); settings != nil {
		if settings.UpdateInterval > 0 {
			return settings.UpdateInterval
		}
	}
	return 60
}

// GetHostDetail returns host detail with packages and history for dashboard (matches Node structure).
func (s *DashboardStore) GetHostDetail(ctx context.Context, hostID string, historyLimit, historyOffset int) (map[string]interface{}, error) {
	d := s.db.DB(ctx)
	hostsStore := NewHostsStore(s.db)
	host, err := hostsStore.GetByID(ctx, hostID)
	if err != nil || host == nil {
		return nil, err
	}

	groups, _ := hostsStore.GetHostGroups(ctx, hostID)
	hg := make([]map[string]interface{}, len(groups))
	for i, g := range groups {
		hg[i] = map[string]interface{}{
			"host_groups": map[string]interface{}{"id": g.ID, "name": g.Name, "color": g.Color},
		}
	}

	var dnsServers []string
	if len(host.DNSServers) > 0 {
		_ = json.Unmarshal(host.DNSServers, &dnsServers)
	}
	var networkInterfaces []interface{}
	if len(host.NetworkInterfaces) > 0 {
		_ = json.Unmarshal(host.NetworkInterfaces, &networkInterfaces)
	}
	var diskDetails interface{}
	if len(host.DiskDetails) > 0 {
		_ = json.Unmarshal(host.DiskDetails, &diskDetails)
	}
	var loadAverage interface{}
	if len(host.LoadAverage) > 0 {
		_ = json.Unmarshal(host.LoadAverage, &loadAverage)
	}

	res := map[string]interface{}{
		"id": host.ID, "machine_id": host.MachineID, "friendly_name": host.FriendlyName,
		"hostname": host.Hostname, "ip": host.IP, "gateway_ip": host.GatewayIP,
		"os_type": host.OSType, "os_version": host.OSVersion, "architecture": host.Architecture,
		"last_update": host.LastUpdate, "status": host.Status, "api_id": host.ApiID,
		"agent_version": host.AgentVersion, "auto_update": host.AutoUpdate, "notes": host.Notes,
		"system_uptime": host.SystemUptime, "needs_reboot": host.NeedsReboot,
		"reboot_reason":  host.RebootReason,
		"docker_enabled": host.DockerEnabled, "compliance_enabled": host.ComplianceEnabled,
		"compliance_on_demand_only": host.ComplianceOnDemandOnly,
		"host_down_alerts_enabled":  host.HostDownAlertsEnabled,
		"kernel_version":            host.KernelVersion, "installed_kernel_version": host.InstalledKernelVersion,
		"cpu_cores": host.CPUCores, "cpu_model": host.CPUModel,
		"ram_installed": host.RamInstalled, "swap_size": host.SwapSize,
		"selinux_status": host.SelinuxStatus, "created_at": host.CreatedAt, "updated_at": host.UpdatedAt,
		"dns_servers": dnsServers, "network_interfaces": networkInterfaces,
		"disk_details": diskDetails, "load_average": loadAverage,
		"host_group_memberships": hg, "primary_interface": host.PrimaryInterface,
	}

	stats, _ := d.Queries.GetHostPackageStats(ctx, hostID)
	res["stats"] = map[string]interface{}{
		"total_packages":    stats.Column1,
		"outdated_packages": stats.Column2,
		"security_updates":  stats.Column3,
	}

	hostPackages, _ := s.getHostPackagesWithPackages(ctx, hostID)
	res["host_packages"] = hostPackages

	updateHistory, totalHistory := s.getUpdateHistory(ctx, hostID, historyLimit, historyOffset)
	res["update_history"] = updateHistory
	res["pagination"] = map[string]interface{}{
		"total":   totalHistory,
		"limit":   historyLimit,
		"offset":  historyOffset,
		"hasMore": historyOffset+historyLimit < totalHistory,
	}

	return res, nil
}

func (s *DashboardStore) getHostPackagesWithPackages(ctx context.Context, hostID string) ([]map[string]interface{}, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetHostPackagesWithPackages(ctx, hostID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		lastChecked := ""
		if r.LastChecked.Valid {
			lastChecked = r.LastChecked.Time.Format("2006-01-02T15:04:05Z07:00")
		}
		out[i] = map[string]interface{}{
			"id": r.ID, "host_id": r.HostID, "package_id": r.PackageID,
			"current_version": r.CurrentVersion, "available_version": r.AvailableVersion,
			"needs_update": r.NeedsUpdate, "is_security_update": r.IsSecurityUpdate,
			"last_checked": lastChecked,
			"packages":     map[string]interface{}{"name": r.PkgName},
		}
	}
	return out, nil
}

func (s *DashboardStore) getUpdateHistory(ctx context.Context, hostID string, limit, offset int) ([]map[string]interface{}, int) {
	if !s.tableExists(ctx, "update_history") {
		return []map[string]interface{}{}, 0
	}
	d := s.db.DB(ctx)
	total, _ := d.Queries.CountUpdateHistory(ctx, hostID)
	rows, err := d.Queries.GetUpdateHistory(ctx, db.GetUpdateHistoryParams{
		HostID: hostID,
		Limit:  safeconv.ClampToInt32(limit),
		Offset: safeconv.ClampToInt32(offset),
	})
	if err != nil {
		return []map[string]interface{}{}, int(total)
	}
	out := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		totalPkg := 0
		if r.TotalPackages != nil {
			totalPkg = int(*r.TotalPackages)
		}
		out[i] = map[string]interface{}{
			"id": r.ID, "host_id": r.HostID, "packages_count": r.PackagesCount,
			"security_count": r.SecurityCount, "total_packages": totalPkg,
			"payload_size_kb": r.PayloadSizeKb, "execution_time": r.ExecutionTime,
			"timestamp": pgTime(r.Timestamp).Format(time.RFC3339), "status": r.Status, "error_message": r.ErrorMessage,
		}
	}
	return out, int(total)
}

// GetPackagesWithHosts returns packages needing updates with affected hosts.
func (s *DashboardStore) GetPackagesWithHosts(ctx context.Context) ([]map[string]interface{}, error) {
	pkgStore := NewPackagesStore(s.db)
	return pkgStore.ListNeedingUpdates(ctx)
}

// GetRecentUsers returns recent users by last_login.
func (s *DashboardStore) GetRecentUsers(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetRecentUsers(ctx, safeconv.ClampToInt32(limit))
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, len(rows))
	for i, u := range rows {
		var lastLogin, createdAt interface{}
		if u.LastLogin.Valid {
			lastLogin = u.LastLogin.Time
		}
		if u.CreatedAt.Valid {
			createdAt = u.CreatedAt.Time
		}
		result[i] = map[string]interface{}{
			"id": u.ID, "username": u.Username, "email": u.Email, "first_name": u.FirstName,
			"last_name": u.LastName, "role": u.Role, "last_login": lastLogin,
			"created_at": createdAt, "avatar_url": u.AvatarUrl,
		}
	}
	return result, nil
}

// GetJobHistoryByApiID returns job history rows for a host by api_id.
func (s *DashboardStore) GetJobHistoryByApiID(ctx context.Context, apiID string, limit int) ([]db.JobHistory, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListJobHistoryByApiID(ctx, db.ListJobHistoryByApiIDParams{
		ApiID: &apiID,
		Limit: safeconv.ClampToInt32(limit),
	})
}

// GetRecentCollection returns recent hosts by last_update.
func (s *DashboardStore) GetRecentCollection(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetRecentHosts(ctx, safeconv.ClampToInt32(limit))
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, len(rows))
	for i, h := range rows {
		var lastUpdate interface{}
		if h.LastUpdate.Valid {
			lastUpdate = h.LastUpdate.Time
		}
		result[i] = map[string]interface{}{
			"id": h.ID, "friendly_name": h.FriendlyName, "hostname": h.Hostname,
			"last_update": lastUpdate, "status": h.Status,
		}
	}
	return result, nil
}

// packageTrendPoint holds one data point for the package trends chart.
type packageTrendPoint struct {
	timeKey       string
	totalPackages int
	packagesCount int
	securityCount int
}

// GetPackageTrends returns package trends data for the chart (matches Node backend structure).
// Uses MAX aggregation per day for days > 1 to ensure spikes are visible.
func (s *DashboardStore) GetPackageTrends(ctx context.Context, days int, hostID string) (map[string]interface{}, error) {
	d := s.db.DB(ctx)
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	needsAggregation := hostID == "" || hostID == "all" || hostID == "undefined"

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)
	startPg := pgtime.From(startDate)
	endPg := pgtime.From(endDate)

	var aggregated []packageTrendPoint

	if needsAggregation {
		if !s.tableExists(ctx, "system_statistics") {
			return s.buildPackageTrendsResponse(ctx, []packageTrendPoint{}, days, "all")
		}
		if days <= 1 {
			rows, err := d.Queries.ListSystemStatisticsByDateRange(ctx, db.ListSystemStatisticsByDateRangeParams{
				Timestamp:   startPg,
				Timestamp_2: endPg,
			})
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				if r.TotalPackages < 0 || r.UniquePackagesCount < 0 || r.UniqueSecurityCount < 0 || r.UniqueSecurityCount > r.UniquePackagesCount {
					continue
				}
				aggregated = append(aggregated, packageTrendPoint{
					timeKey:       pgTime(r.Timestamp).Format(time.RFC3339),
					totalPackages: int(r.TotalPackages),
					packagesCount: int(r.UniquePackagesCount),
					securityCount: int(r.UniqueSecurityCount),
				})
			}
		} else {
			rows, err := d.Queries.GetSystemStatisticsDaily(ctx, db.GetSystemStatisticsDailyParams{
				Timestamp:   startPg,
				Timestamp_2: endPg,
			})
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				aggregated = append(aggregated, packageTrendPoint{
					timeKey:       r.Ts,
					totalPackages: int(r.TotalPackages),
					packagesCount: int(r.PackagesCount),
					securityCount: int(r.SecurityCount),
				})
			}
		}
		// Fallback: when no system_statistics rows exist in range, use the latest/current aggregate state.
		if len(aggregated) == 0 {
			latest, err := d.Queries.GetLatestSystemStatistics(ctx)
			if err == nil {
				aggregated = append(aggregated, packageTrendPoint{
					timeKey:       endDate.Format("2006-01-02"),
					totalPackages: int(latest.TotalPackages),
					packagesCount: int(latest.UniquePackagesCount),
					securityCount: int(latest.UniqueSecurityCount),
				})
			} else {
				fallback, err := d.Queries.GetSystemStatsForInsert(ctx)
				if err == nil {
					aggregated = append(aggregated, packageTrendPoint{
						timeKey:       endDate.Format("2006-01-02"),
						totalPackages: int(fallback.Column3),
						packagesCount: int(fallback.Column1),
						securityCount: int(fallback.Column2),
					})
				}
			}
		}
	} else {
		if !s.tableExists(ctx, "update_history") {
			return s.buildPackageTrendsResponse(ctx, []packageTrendPoint{}, days, hostID)
		}
		if days <= 1 {
			rows, err := d.Queries.ListUpdateHistoryByDateRange(ctx, db.ListUpdateHistoryByDateRangeParams{
				HostID:      hostID,
				Timestamp:   startPg,
				Timestamp_2: endPg,
			})
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				totalPkg := 0
				if r.TotalPackages != nil {
					totalPkg = int(*r.TotalPackages)
				}
				if totalPkg < 0 || r.PackagesCount < 0 || r.SecurityCount < 0 || r.SecurityCount > r.PackagesCount {
					continue
				}
				aggregated = append(aggregated, packageTrendPoint{
					timeKey:       pgTime(r.Timestamp).Format(time.RFC3339),
					totalPackages: totalPkg,
					packagesCount: int(r.PackagesCount),
					securityCount: int(r.SecurityCount),
				})
			}
			// Fallback for 24h view: when no update_history, use current state
			if len(aggregated) == 0 {
				stats, err := d.Queries.GetHostPackageStats(ctx, hostID)
				if err == nil {
					aggregated = append(aggregated, packageTrendPoint{
						timeKey:       endDate.Format(time.RFC3339),
						totalPackages: int(stats.Column1),
						packagesCount: int(stats.Column2),
						securityCount: int(stats.Column3),
					})
				}
			}
		} else {
			rows, err := d.Queries.GetUpdateHistoryDaily(ctx, db.GetUpdateHistoryDailyParams{
				HostID:      hostID,
				Timestamp:   startPg,
				Timestamp_2: endPg,
			})
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				aggregated = append(aggregated, packageTrendPoint{
					timeKey:       r.Ts,
					totalPackages: int(r.TotalPackages),
					packagesCount: int(r.PackagesCount),
					securityCount: int(r.SecurityCount),
				})
			}
		}
		// Fallback: when no update_history for this host, use current state from host_packages
		if len(aggregated) == 0 {
			stats, err := d.Queries.GetHostPackageStats(ctx, hostID)
			if err == nil {
				aggregated = append(aggregated, packageTrendPoint{
					timeKey:       endDate.Format("2006-01-02"),
					totalPackages: int(stats.Column1),
					packagesCount: int(stats.Column2),
					securityCount: int(stats.Column3),
				})
			}
		}
	}

	filled := s.fillMissingPeriods(aggregated, days)
	return s.buildPackageTrendsResponse(ctx, filled, days, hostID)
}

func (s *DashboardStore) fillMissingPeriods(data []packageTrendPoint, daysInt int) []packageTrendPoint {
	if len(data) == 0 || daysInt <= 1 {
		return data
	}
	startDate := time.Now().AddDate(0, 0, -daysInt)
	dataMap := make(map[string]packageTrendPoint)
	for _, p := range data {
		dataMap[p.timeKey] = p
	}
	currentDate := startDate
	endDate := time.Now()
	var filled []packageTrendPoint
	var lastKnown *packageTrendPoint
	firstTimeKey := data[0].timeKey
	for !currentDate.After(endDate) {
		timeKey := currentDate.Format("2006-01-02")
		currentDate = currentDate.AddDate(0, 0, 1)
		if timeKey < firstTimeKey {
			continue
		}
		if p, ok := dataMap[timeKey]; ok {
			filled = append(filled, p)
			lastKnown = &p
		} else if lastKnown != nil {
			filled = append(filled, packageTrendPoint{timeKey, lastKnown.totalPackages, lastKnown.packagesCount, lastKnown.securityCount})
		}
	}
	return filled
}

func (s *DashboardStore) buildPackageTrendsResponse(ctx context.Context, filled []packageTrendPoint, days int, hostID string) (map[string]interface{}, error) {
	d := s.db.DB(ctx)
	hostIDOut := "all"
	if hostID != "" && hostID != "all" && hostID != "undefined" {
		hostIDOut = hostID
	}
	labels := make([]string, 0, len(filled))
	dsTotal := make([]int, 0, len(filled))
	dsOutdated := make([]int, 0, len(filled))
	dsSecurity := make([]int, 0, len(filled))
	for _, p := range filled {
		labels = append(labels, p.timeKey)
		dsTotal = append(dsTotal, p.totalPackages)
		dsOutdated = append(dsOutdated, p.packagesCount)
		dsSecurity = append(dsSecurity, p.securityCount)
	}
	if len(labels) > 0 {
		labels[len(labels)-1] = "Now"
	}
	needsAggregation := hostIDOut == "all"
	chartData := map[string]interface{}{
		"labels": labels,
		"datasets": []map[string]interface{}{
			{
				"label":            map[bool]string{true: "Total Packages (All Hosts)", false: "Total Packages"}[needsAggregation],
				"data":             dsTotal,
				"borderColor":      "#3B82F6",
				"backgroundColor":  "rgba(59, 130, 246, 0.1)",
				"tension":          0.4,
				"hidden":           true, // Hidden by default; user can toggle in chart legend
				"spanGaps":         true,
				"pointRadius":      3,
				"pointHoverRadius": 5,
				"order":            1,
				"borderWidth":      2,
			},
			{
				"label":            map[bool]string{true: "Total Outdated Packages", false: "Outdated Packages"}[needsAggregation],
				"data":             dsOutdated,
				"borderColor":      "#F59E0B",
				"backgroundColor":  "rgba(245, 158, 11, 0.1)",
				"tension":          0.4,
				"spanGaps":         true,
				"pointRadius":      3,
				"pointHoverRadius": 5,
				"order":            2,
				"borderWidth":      2,
			},
			{
				"label":            map[bool]string{true: "Total Security Packages", false: "Security Packages"}[needsAggregation],
				"data":             dsSecurity,
				"borderColor":      "#EF4444",
				"backgroundColor":  "rgba(239, 68, 68, 0.1)",
				"tension":          0.4,
				"spanGaps":         true,
				"pointRadius":      4,
				"pointHoverRadius": 6,
				"order":            3,
				"borderWidth":      3,
			},
		},
	}
	hostsRows, _ := d.Queries.GetHostsForPackageTrends(ctx)
	hosts := make([]map[string]interface{}, len(hostsRows))
	for i, h := range hostsRows {
		hostname := ""
		if h.Hostname != nil {
			hostname = *h.Hostname
		}
		hosts[i] = map[string]interface{}{
			"id": h.ID, "friendly_name": h.FriendlyName, "hostname": hostname,
		}
	}
	var currentPackageState map[string]interface{}
	if hostIDOut != "all" {
		stats, err := d.Queries.GetHostPackageStats(ctx, hostIDOut)
		if err == nil {
			currentPackageState = map[string]interface{}{
				"total_packages": stats.Column1,
				"packages_count": stats.Column2,
				"security_count": stats.Column3,
			}
		}
	} else {
		latest, err := d.Queries.GetLatestSystemStatistics(ctx)
		if err == nil {
			currentPackageState = map[string]interface{}{
				"total_packages": latest.TotalPackages,
				"packages_count": latest.UniquePackagesCount,
				"security_count": latest.UniqueSecurityCount,
			}
		} else {
			fallback, err := d.Queries.GetSystemStatsForInsert(ctx)
			if err == nil {
				currentPackageState = map[string]interface{}{
					"total_packages": fallback.Column3,
					"packages_count": fallback.Column1,
					"security_count": fallback.Column2,
				}
			}
		}
	}
	return map[string]interface{}{
		"chartData":           chartData,
		"hosts":               hosts,
		"period":              days,
		"hostId":              hostIDOut,
		"currentPackageState": currentPackageState,
	}, nil
}
