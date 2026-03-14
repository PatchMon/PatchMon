package store

import (
	"context"
	"sync"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

const dashboardCacheTTL = 45 * time.Second

var (
	dashboardCacheMu   sync.Mutex
	dashboardCacheData *ComplianceDashboard
	dashboardCacheExp  time.Time
)

// ComplianceDashboard is the dashboard response structure (matches legacy/frontend).
// JSON tags use snake_case to match Node API and frontend expectations.
type ComplianceDashboard struct {
	Summary               ComplianceDashboardSummary           `json:"summary"`
	RecentScans           []ComplianceDashboardScan            `json:"recent_scans"`
	HostsWithLatestScan   []ComplianceDashboardHost            `json:"hosts_with_latest_scan"`
	WorstHosts            []ComplianceDashboardWorstHost       `json:"worst_hosts"`
	TopFailingRules       []ComplianceDashboardRuleCount       `json:"top_failing_rules"`
	TopWarningRules       []ComplianceDashboardRuleCount       `json:"top_warning_rules"`
	ProfileDistribution   []ComplianceDashboardProfileDist     `json:"profile_distribution"`
	SeverityBreakdown     []ComplianceDashboardSeverityCount   `json:"severity_breakdown"`
	SeverityByProfileType []ComplianceDashboardSeverityByType  `json:"severity_by_profile_type"`
	DockerBenchBySection  []ComplianceDashboardSectionCount    `json:"docker_bench_by_section"`
	ScanAgeDistribution   ComplianceDashboardScanAge           `json:"scan_age_distribution"`
	ProfileTypeStats      []ComplianceDashboardProfileTypeStat `json:"profile_type_stats"`
}

type ComplianceDashboardSummary struct {
	TotalHosts           int                       `json:"total_hosts"`
	AverageScore         float64                   `json:"average_score"`
	HostsCompliant       int                       `json:"hosts_compliant"`
	HostsWarning         int                       `json:"hosts_warning"`
	HostsCritical        int                       `json:"hosts_critical"`
	Unscanned            int                       `json:"unscanned"`
	HostsWithCompliance  int                       `json:"hosts_with_compliance_enabled"`
	HostStatusByScanType map[string]map[string]int `json:"host_status_by_scan_type"`
	Compliant            int                       `json:"compliant"`
	Warning              int                       `json:"warning"`
	Critical             int                       `json:"critical"`
	TotalScans           int                       `json:"total_scans"`
	TotalPassedRules     int                       `json:"total_passed_rules"`
	TotalFailedRules     int                       `json:"total_failed_rules"`
	TotalRules           int                       `json:"total_rules"`
}

type ComplianceDashboardScan struct {
	ID            string                 `json:"id"`
	HostID        string                 `json:"host_id"`
	Status        string                 `json:"status"`
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   *time.Time             `json:"completed_at"`
	TotalRules    int                    `json:"total_rules"`
	Passed        int                    `json:"passed"`
	Failed        int                    `json:"failed"`
	Warnings      int                    `json:"warnings"`
	Skipped       int                    `json:"skipped"`
	NotApplicable int                    `json:"not_applicable"`
	Score         *float64               `json:"score"`
	ErrorMessage  *string                `json:"error_message"`
	Host          map[string]interface{} `json:"host"`
	Profile       map[string]interface{} `json:"profile"`
}

type ComplianceDashboardHost struct {
	HostID            string     `json:"host_id"`
	Hostname          *string    `json:"hostname"`
	FriendlyName      string     `json:"friendly_name"`
	LastScanDate      *time.Time `json:"last_scan_date"`
	LastActivityTitle *string    `json:"last_activity_title"`
	Passed            *int       `json:"passed"`
	Failed            *int       `json:"failed"`
	Skipped           *int       `json:"skipped"`
	Score             *float64   `json:"score"`
	ScannerStatus     string     `json:"scanner_status"`
	ComplianceMode    string     `json:"compliance_mode"`
	ComplianceEnabled bool       `json:"compliance_enabled"`
	DockerEnabled     bool       `json:"docker_enabled"`
}

type ComplianceDashboardWorstHost struct {
	ID                 string                 `json:"id"`
	HostID             string                 `json:"host_id"`
	Score              *float64               `json:"score"`
	CompletedAt        *time.Time             `json:"completed_at"`
	Host               map[string]interface{} `json:"host"`
	Profile            map[string]interface{} `json:"profile"`
	ComplianceProfiles map[string]interface{} `json:"compliance_profiles"`
}

type ComplianceDashboardRuleCount struct {
	RuleID      string  `json:"rule_id"`
	Title       *string `json:"title"`
	Severity    *string `json:"severity"`
	ProfileType *string `json:"profile_type"`
	FailCount   int     `json:"fail_count"`
	WarnCount   int     `json:"warn_count"`
}

type ComplianceDashboardProfileDist struct {
	Name      string  `json:"name"`
	Type      *string `json:"type"`
	HostCount int     `json:"host_count"`
}

type ComplianceDashboardSeverityCount struct {
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

type ComplianceDashboardSeverityByType struct {
	Severity    string `json:"severity"`
	ProfileType string `json:"profile_type"`
	Count       int    `json:"count"`
}

type ComplianceDashboardSectionCount struct {
	Section string `json:"section"`
	Count   int    `json:"count"`
}

type ComplianceDashboardScanAge struct {
	Today     map[string]int `json:"today"`
	ThisWeek  map[string]int `json:"this_week"`
	ThisMonth map[string]int `json:"this_month"`
	Older     map[string]int `json:"older"`
}

type ComplianceDashboardProfileTypeStat struct {
	Type          string   `json:"type"`
	HostsScanned  int      `json:"hosts_scanned"`
	AverageScore  *float64 `json:"average_score"`
	TotalPassed   int      `json:"total_passed"`
	TotalFailed   int      `json:"total_failed"`
	TotalWarnings int      `json:"total_warnings"`
	TotalRules    int      `json:"total_rules"`
}

// GetDashboard returns the compliance dashboard data (matches legacy structure).
func (s *ComplianceStore) GetDashboard(ctx context.Context) (*ComplianceDashboard, error) {
	d := s.db.DB(ctx)
	dashboardCacheMu.Lock()
	if dashboardCacheData != nil && time.Now().Before(dashboardCacheExp) {
		data := dashboardCacheData
		dashboardCacheMu.Unlock()
		return data, nil
	}
	dashboardCacheMu.Unlock()

	latestScans, err := d.Queries.GetLatestCompletedScans(ctx)
	if err != nil {
		return nil, err
	}
	allHosts, err := d.Queries.ListHostsForComplianceDashboard(ctx)
	if err != nil {
		return nil, err
	}
	unscanned, err := d.Queries.CountUnscannedHosts(ctx)
	if err != nil {
		return nil, err
	}
	recentScans, err := d.Queries.GetRecentComplianceScans(ctx)
	if err != nil {
		return nil, err
	}

	// Build profile type map
	profileTypes := make(map[string]string)
	for _, r := range latestScans {
		profileTypes[r.ProfileID] = r.ProfileType
	}

	// Latest per host (single most recent scan across all profiles)
	latestPerHost := make(map[string]db.GetLatestCompletedScansRow)
	for _, sc := range latestScans {
		existing, ok := latestPerHost[sc.HostID]
		if !ok || (sc.CompletedAt.Valid && existing.CompletedAt.Valid && sc.CompletedAt.Time.After(existing.CompletedAt.Time)) {
			latestPerHost[sc.HostID] = sc
		}
	}

	// Host-level worst scores
	hostWorstScores := make(map[string]struct {
		score    float64
		scanType string
	})
	for _, sc := range latestScans {
		score := 0.0
		if sc.Score != nil {
			score = *sc.Score
		}
		scanType := profileTypes[sc.ProfileID]
		if scanType == "" {
			scanType = "unknown"
		}
		current, ok := hostWorstScores[sc.HostID]
		if !ok || score < current.score {
			hostWorstScores[sc.HostID] = struct {
				score    float64
				scanType string
			}{score, scanType}
		}
	}

	hostsCompliant := 0
	hostsWarning := 0
	hostsCritical := 0
	for _, h := range hostWorstScores {
		if h.score >= 80 {
			hostsCompliant++
		} else if h.score >= 60 {
			hostsWarning++
		} else {
			hostsCritical++
		}
	}

	// Host status by scan type
	hostStatusByScanType := map[string]map[string]int{
		"compliant": {"openscap": 0, "docker-bench": 0},
		"warning":   {"openscap": 0, "docker-bench": 0},
		"critical":  {"openscap": 0, "docker-bench": 0},
	}
	for _, h := range hostWorstScores {
		status := "compliant"
		if h.score < 60 {
			status = "critical"
		} else if h.score < 80 {
			status = "warning"
		}
		if m, ok := hostStatusByScanType[status]; ok {
			m[h.scanType]++
		}
	}

	// Scan-level counts
	scansCompliant := 0
	scansWarning := 0
	scansCritical := 0
	for _, sc := range latestScans {
		score := 0.0
		if sc.Score != nil {
			score = *sc.Score
		}
		if score >= 80 {
			scansCompliant++
		} else if score >= 60 {
			scansWarning++
		} else {
			scansCritical++
		}
	}

	// Averages
	uniqueHostIDs := make(map[string]bool)
	for _, sc := range latestScans {
		uniqueHostIDs[sc.HostID] = true
	}
	totalHosts := len(uniqueHostIDs)
	avgScore := 0.0
	if len(latestScans) > 0 {
		sum := 0.0
		for _, sc := range latestScans {
			if sc.Score != nil {
				sum += *sc.Score
			}
		}
		avgScore = sum / float64(len(latestScans))
	}

	hostsWithCompliance := 0
	for _, h := range allHosts {
		if h.ComplianceEnabled {
			hostsWithCompliance++
		}
	}

	// Hosts with latest scan
	hostsWithLatestScan := make([]ComplianceDashboardHost, 0, len(allHosts))
	for _, h := range allHosts {
		scan := latestPerHost[h.ID]
		lastScan := (*time.Time)(nil)
		if scan.CompletedAt.Valid {
			lastScan = &scan.CompletedAt.Time
		}
		profileName := scan.ProfileName
		passed := int(scan.Passed)
		failed := int(scan.Failed)
		skipped := int(scan.Skipped) + int(scan.NotApplicable)
		var scannerStatus string
		if lastScan != nil {
			scannerStatus = "Scanned"
		} else if h.ComplianceEnabled {
			scannerStatus = "Enabled"
		} else {
			scannerStatus = "Never scanned"
		}
		complianceMode := "disabled"
		if h.ComplianceEnabled {
			if h.ComplianceOnDemandOnly {
				complianceMode = "on-demand"
			} else {
				complianceMode = "enabled"
			}
		}
		hostsWithLatestScan = append(hostsWithLatestScan, ComplianceDashboardHost{
			HostID:            h.ID,
			Hostname:          h.Hostname,
			FriendlyName:      h.FriendlyName,
			LastScanDate:      lastScan,
			LastActivityTitle: &profileName,
			Passed:            &passed,
			Failed:            &failed,
			Skipped:           &skipped,
			Score:             scan.Score,
			ScannerStatus:     scannerStatus,
			ComplianceMode:    complianceMode,
			ComplianceEnabled: h.ComplianceEnabled,
			DockerEnabled:     h.DockerEnabled,
		})
	}

	// Sort by last scan date (most recent first)
	for i := 0; i < len(hostsWithLatestScan)-1; i++ {
		for j := i + 1; j < len(hostsWithLatestScan); j++ {
			a, b := hostsWithLatestScan[i].LastScanDate, hostsWithLatestScan[j].LastScanDate
			if a == nil && b != nil {
				hostsWithLatestScan[i], hostsWithLatestScan[j] = hostsWithLatestScan[j], hostsWithLatestScan[i]
			} else if a != nil && b != nil && b.After(*a) {
				hostsWithLatestScan[i], hostsWithLatestScan[j] = hostsWithLatestScan[j], hostsWithLatestScan[i]
			}
		}
	}

	// Recent scans (transform)
	recentScansOut := make([]ComplianceDashboardScan, 0, len(recentScans))
	for _, r := range recentScans {
		var completedAt *time.Time
		if r.CompletedAt.Valid {
			completedAt = &r.CompletedAt.Time
		}
		recentScansOut = append(recentScansOut, ComplianceDashboardScan{
			ID:            r.ID,
			HostID:        r.HostID,
			Status:        r.Status,
			StartedAt:     pgTime(r.StartedAt),
			CompletedAt:   completedAt,
			TotalRules:    int(r.TotalRules),
			Passed:        int(r.Passed),
			Failed:        int(r.Failed),
			Warnings:      int(r.Warnings),
			Skipped:       int(r.Skipped),
			NotApplicable: int(r.NotApplicable),
			Score:         r.Score,
			ErrorMessage:  r.ErrorMessage,
			Host:          map[string]interface{}{"id": r.HostID, "hostname": r.Hostname, "friendly_name": r.FriendlyName},
			Profile:       map[string]interface{}{"name": r.ProfileName, "type": r.ProfileType},
		})
	}

	// Worst hosts (lowest scores)
	worstHosts := make([]ComplianceDashboardWorstHost, 0, 5)
	for _, sc := range latestScans {
		host, _ := findHost(allHosts, sc.HostID)
		hostMap := map[string]interface{}{"id": sc.HostID, "hostname": nil, "friendly_name": nil}
		if host != nil {
			hostMap["hostname"] = host.Hostname
			hostMap["friendly_name"] = host.FriendlyName
		}
		worstHosts = append(worstHosts, ComplianceDashboardWorstHost{
			ID:                 sc.ID,
			HostID:             sc.HostID,
			Score:              sc.Score,
			CompletedAt:        pgTimePtr(sc.CompletedAt),
			Host:               hostMap,
			Profile:            map[string]interface{}{"name": sc.ProfileName},
			ComplianceProfiles: map[string]interface{}{"type": sc.ProfileType, "name": sc.ProfileName},
		})
	}
	// Sort worst hosts by score ascending
	for i := 0; i < len(worstHosts)-1; i++ {
		for j := i + 1; j < len(worstHosts); j++ {
			sa, sb := 0.0, 0.0
			if worstHosts[i].Score != nil {
				sa = *worstHosts[i].Score
			}
			if worstHosts[j].Score != nil {
				sb = *worstHosts[j].Score
			}
			if sa > sb {
				worstHosts[i], worstHosts[j] = worstHosts[j], worstHosts[i]
			}
		}
	}
	if len(worstHosts) > 5 {
		worstHosts = worstHosts[:5]
	}

	// Rule totals
	totalPassed := 0
	totalFailed := 0
	totalRules := 0
	for _, sc := range latestScans {
		totalPassed += int(sc.Passed)
		totalFailed += int(sc.Failed)
		totalRules += int(sc.TotalRules)
	}

	// Profile type stats (openscap, docker-bench) - aggregate from latestScans
	profileTypeStatsMap := make(map[string]struct {
		hostsScanned  int
		totalScore    float64
		totalPassed   int
		totalFailed   int
		totalWarnings int
		totalRules    int
	})
	for _, sc := range latestScans {
		t := sc.ProfileType
		if t != "openscap" && t != "docker-bench" {
			t = "unknown"
		}
		p := profileTypeStatsMap[t]
		p.hostsScanned++
		if sc.Score != nil {
			p.totalScore += *sc.Score
		}
		p.totalPassed += int(sc.Passed)
		p.totalFailed += int(sc.Failed)
		p.totalWarnings += int(sc.Warnings)
		p.totalRules += int(sc.TotalRules)
		profileTypeStatsMap[t] = p
	}
	profileTypeStatsList := make([]ComplianceDashboardProfileTypeStat, 0, len(profileTypeStatsMap))
	for t, p := range profileTypeStatsMap {
		var avgScore *float64
		if p.hostsScanned > 0 {
			avg := p.totalScore / float64(p.hostsScanned)
			avgScore = &avg
		}
		profileTypeStatsList = append(profileTypeStatsList, ComplianceDashboardProfileTypeStat{
			Type:          t,
			HostsScanned:  p.hostsScanned,
			AverageScore:  avgScore,
			TotalPassed:   p.totalPassed,
			TotalFailed:   p.totalFailed,
			TotalWarnings: p.totalWarnings,
			TotalRules:    p.totalRules,
		})
	}

	// Severity breakdown - failures by severity across latest scans
	latestScanIDs := make([]string, 0, len(latestScans))
	for _, sc := range latestScans {
		latestScanIDs = append(latestScanIDs, sc.ID)
	}
	severityBreakdownList := []ComplianceDashboardSeverityCount{}
	if len(latestScanIDs) > 0 {
		sevRows, err := d.Queries.GetComplianceResultSeverityBreakdownForScans(ctx, latestScanIDs)
		if err == nil {
			for _, r := range sevRows {
				severityBreakdownList = append(severityBreakdownList, ComplianceDashboardSeverityCount{
					Severity: r.Severity,
					Count:    int(r.Count),
				})
			}
		}
	}

	// Profile distribution
	profileDist := make(map[string]ComplianceDashboardProfileDist)
	for _, sc := range latestScans {
		key := sc.ProfileID
		if p, ok := profileDist[key]; ok {
			p.HostCount++
			profileDist[key] = p
		} else {
			profileDist[key] = ComplianceDashboardProfileDist{
				Name:      sc.ProfileName,
				Type:      &sc.ProfileType,
				HostCount: 1,
			}
		}
	}
	profileDistList := make([]ComplianceDashboardProfileDist, 0, len(profileDist))
	for _, p := range profileDist {
		profileDistList = append(profileDistList, p)
	}

	// Scan age distribution
	scanNow := time.Now()
	oneDayAgo := scanNow.Add(-24 * time.Hour)
	oneWeekAgo := scanNow.Add(-7 * 24 * time.Hour)
	oneMonthAgo := scanNow.Add(-30 * 24 * time.Hour)
	scanAgeDist := ComplianceDashboardScanAge{
		Today:     map[string]int{"openscap": 0, "docker-bench": 0},
		ThisWeek:  map[string]int{"openscap": 0, "docker-bench": 0},
		ThisMonth: map[string]int{"openscap": 0, "docker-bench": 0},
		Older:     map[string]int{"openscap": 0, "docker-bench": 0},
	}
	hostLastScansByType := make(map[string]db.GetLatestCompletedScansRow)
	for _, sc := range latestScans {
		key := sc.HostID + ":" + sc.ProfileType
		existing, ok := hostLastScansByType[key]
		if !ok || (sc.CompletedAt.Valid && existing.CompletedAt.Valid && sc.CompletedAt.Time.After(existing.CompletedAt.Time)) {
			hostLastScansByType[key] = sc
		}
	}
	for _, sc := range hostLastScansByType {
		if sc.ProfileType != "openscap" && sc.ProfileType != "docker-bench" {
			continue
		}
		scanDate := sc.CompletedAt.Time
		if sc.CompletedAt.Valid {
			if scanDate.After(oneDayAgo) {
				scanAgeDist.Today[sc.ProfileType]++
			} else if scanDate.After(oneWeekAgo) {
				scanAgeDist.ThisWeek[sc.ProfileType]++
			} else if scanDate.After(oneMonthAgo) {
				scanAgeDist.ThisMonth[sc.ProfileType]++
			} else {
				scanAgeDist.Older[sc.ProfileType]++
			}
		}
	}

	out := &ComplianceDashboard{
		Summary: ComplianceDashboardSummary{
			TotalHosts:           totalHosts,
			AverageScore:         avgScore,
			HostsCompliant:       hostsCompliant,
			HostsWarning:         hostsWarning,
			HostsCritical:        hostsCritical,
			Unscanned:            int(unscanned),
			HostsWithCompliance:  hostsWithCompliance,
			HostStatusByScanType: hostStatusByScanType,
			Compliant:            scansCompliant,
			Warning:              scansWarning,
			Critical:             scansCritical,
			TotalScans:           len(latestScans),
			TotalPassedRules:     totalPassed,
			TotalFailedRules:     totalFailed,
			TotalRules:           totalRules,
		},
		RecentScans:           recentScansOut,
		HostsWithLatestScan:   hostsWithLatestScan,
		WorstHosts:            worstHosts,
		TopFailingRules:       []ComplianceDashboardRuleCount{},
		TopWarningRules:       []ComplianceDashboardRuleCount{},
		ProfileDistribution:   profileDistList,
		SeverityBreakdown:     severityBreakdownList,
		SeverityByProfileType: []ComplianceDashboardSeverityByType{},
		DockerBenchBySection:  []ComplianceDashboardSectionCount{},
		ScanAgeDistribution:   scanAgeDist,
		ProfileTypeStats:      profileTypeStatsList,
	}

	dashboardCacheMu.Lock()
	dashboardCacheData = out
	dashboardCacheExp = time.Now().Add(dashboardCacheTTL)
	dashboardCacheMu.Unlock()

	return out, nil
}

func findHost(hosts []db.ListHostsForComplianceDashboardRow, id string) (*db.ListHostsForComplianceDashboardRow, bool) {
	for i := range hosts {
		if hosts[i].ID == id {
			return &hosts[i], true
		}
	}
	return nil, false
}
