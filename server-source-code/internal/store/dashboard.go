package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
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

// GetStats returns dashboard statistics matching Node backend structure for frontend compatibility.
func (s *DashboardStore) GetStats(ctx context.Context) (map[string]interface{}, error) {
	d := s.db.DB(ctx)
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

	stats, err := d.Queries.GetDashboardStats(ctx, db.GetDashboardStatsParams{
		LastUpdate:   pgtype.Timestamp{Time: thresholdTime, Valid: true},
		LastUpdate_2: pgtype.Timestamp{Time: offlineThreshold, Valid: true},
	})
	if err != nil {
		return nil, err
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

	osRows, _ := d.Queries.GetOSDistributionByTypeAndVersion(ctx)
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

	trends := []interface{}{}
	if s.tableExists(ctx, "update_history") {
		trendsSince := now.AddDate(0, 0, -7)
		trendRows, _ := d.Queries.GetUpdateTrends(ctx, pgtype.Timestamp{Time: trendsSince, Valid: true})
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

	stats, err := d.Queries.GetHomepageStats(ctx, pgtype.Timestamp{Time: oneDayAgo, Valid: true})
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

// HostsListParams holds optional filters for GetHostsWithCounts.
type HostsListParams struct {
	Search    string
	Group     string
	Status    string
	OS        string
	OSVersion string
}

// GetHostsWithCounts returns hosts with update counts for dashboard.
func (s *DashboardStore) GetHostsWithCounts(ctx context.Context, params HostsListParams) ([]map[string]interface{}, error) {
	d := s.db.DB(ctx)
	arg := db.GetHostsWithCountsParams{}
	if params.Search != "" {
		arg.Search = &params.Search
	}
	if params.Group != "" {
		arg.Group = &params.Group
	}
	if params.Status != "" {
		arg.Status = &params.Status
	}
	if params.OS != "" {
		arg.Os = &params.OS
	}
	if params.OSVersion != "" {
		arg.OsVersion = &params.OSVersion
	}

	rows, err := d.Queries.GetHostsWithCounts(ctx, arg)
	if err != nil {
		return nil, err
	}

	hostIDs := make([]string, len(rows))
	for i, r := range rows {
		hostIDs[i] = r.ID
	}
	groupsMap := make(map[string][]map[string]interface{})
	if len(hostIDs) > 0 {
		groupRows, _ := d.Queries.GetHostGroupsForHosts(ctx, hostIDs)
		for _, r := range groupRows {
			m := map[string]interface{}{
				"host_groups": map[string]interface{}{"id": r.ID, "name": r.Name, "color": r.Color},
			}
			groupsMap[r.HostID] = append(groupsMap[r.HostID], m)
		}
	}

	updateIntervalMinutes := 60
	if settings, _ := s.getSettings(ctx); settings != nil {
		updateIntervalMinutes = settings.UpdateInterval
		if updateIntervalMinutes <= 0 {
			updateIntervalMinutes = 60
		}
	}
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
			"ip": h.Ip, "os_type": h.OsType, "os_version": h.OsVersion,
			"status": h.Status, "agent_version": h.AgentVersion, "auto_update": h.AutoUpdate,
			"notes": h.Notes, "api_id": h.ApiID, "needs_reboot": h.NeedsReboot, "system_uptime": h.SystemUptime,
			"docker_enabled": h.DockerEnabled, "compliance_enabled": h.ComplianceEnabled,
			"compliance_on_demand_only": h.ComplianceOnDemandOnly,
			"last_update":               lastUpdateStr, "isStale": isStale, "effectiveStatus": effectiveStatus,
			"host_group_memberships": hostGroups,
			"updatesCount":           h.UpdatesCount, "securityUpdatesCount": h.SecurityUpdatesCount,
			"totalPackagesCount": h.TotalPackagesCount,
		}
	}
	return result, nil
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
		Limit:  int32(limit),
		Offset: int32(offset),
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
	rows, err := d.Queries.GetRecentUsers(ctx, int32(limit))
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
		Limit: int32(limit),
	})
}

// GetRecentCollection returns recent hosts by last_update.
func (s *DashboardStore) GetRecentCollection(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetRecentHosts(ctx, int32(limit))
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
	startPg := pgtype.Timestamp{Time: startDate, Valid: true}
	endPg := pgtype.Timestamp{Time: endDate, Valid: true}

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
