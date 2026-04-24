package store

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

// RuleWithCounts represents a rule with aggregated pass/fail/warn counts.
type RuleWithCounts struct {
	ID          string
	RuleRef     string
	Title       string
	Severity    *string
	Section     *string
	ProfileID   string
	ProfileType *string
	ProfileName *string
	HostsPassed int
	HostsFailed int
	HostsWarned int
	TotalHosts  int
}

// ListRules returns rules with aggregated counts from latest scans.
func (s *ComplianceStore) ListRules(ctx context.Context, severity, statusFilter, search, profileType, hostID *string, limit, offset int32, sortBy, sortDir string) ([]RuleWithCounts, int, error) {
	d := s.db.DB(ctx)
	latestScans, err := d.Queries.GetLatestCompletedScans(ctx)
	if err != nil {
		return nil, 0, err
	}
	latestScanIDs := make([]string, 0, len(latestScans))
	scanIDsSet := make(map[string]bool)
	for _, sc := range latestScans {
		if hostID != nil && *hostID != "" && sc.HostID != *hostID {
			continue
		}
		if profileType != nil && *profileType != "" && *profileType != "all" && sc.ProfileType != *profileType {
			continue
		}
		if !scanIDsSet[sc.ID] {
			scanIDsSet[sc.ID] = true
			latestScanIDs = append(latestScanIDs, sc.ID)
		}
	}
	if len(latestScanIDs) == 0 {
		return []RuleWithCounts{}, 0, nil
	}

	// Single bulk query to get all results across latest scans, replacing N+1 per-scan queries
	allResults, err := d.Queries.GetRuleAggregationsFromScans(ctx, db.GetRuleAggregationsFromScansParams{
		Column1:        latestScanIDs,
		SeverityFilter: severity,
	})
	if err != nil {
		return nil, 0, err
	}

	ruleCounts := make(map[string]*RuleWithCounts)
	for _, r := range allResults {
		key := r.RuleID + ":" + r.ProfileType
		if ruleCounts[key] == nil {
			profType := r.ProfileType
			profName := r.ProfileName
			ruleCounts[key] = &RuleWithCounts{
				ID:          r.RuleID,
				RuleRef:     r.RuleRef,
				Title:       r.Title,
				Severity:    r.Severity,
				Section:     r.Section,
				ProfileID:   r.ProfileID,
				ProfileType: &profType,
				ProfileName: &profName,
			}
		}
		rc := ruleCounts[key]
		rc.TotalHosts++
		switch r.Status {
		case "pass", "passed":
			rc.HostsPassed++
		case "fail", "failed", "failure":
			rc.HostsFailed++
		case "warn", "warning", "warned":
			rc.HostsWarned++
		}
	}

	// Filter by status
	if statusFilter != nil && *statusFilter != "" {
		filtered := make(map[string]*RuleWithCounts)
		for k, rc := range ruleCounts {
			keep := false
			switch *statusFilter {
			case "fail":
				keep = rc.HostsFailed > 0
			case "warn":
				keep = rc.HostsWarned > 0
			case "pass":
				keep = rc.HostsPassed > 0
			}
			if keep {
				filtered[k] = rc
			}
		}
		ruleCounts = filtered
	}

	// Filter by search
	if search != nil && *search != "" {
		searchLower := strings.ToLower(*search)
		filtered := make(map[string]*RuleWithCounts)
		for k, rc := range ruleCounts {
			if strings.Contains(strings.ToLower(rc.Title), searchLower) ||
				strings.Contains(strings.ToLower(rc.RuleRef), searchLower) ||
				(rc.Section != nil && strings.Contains(strings.ToLower(*rc.Section), searchLower)) {
				filtered[k] = rc
			}
		}
		ruleCounts = filtered
	}

	// To slice and sort
	rules := make([]RuleWithCounts, 0, len(ruleCounts))
	for _, rc := range ruleCounts {
		rules = append(rules, *rc)
	}

	// Sort
	severityRank := map[string]int{"critical": 4, "high": 3, "medium": 2, "low": 1, "unknown": 0}
	getStatusRank := func(r RuleWithCounts) int {
		if r.HostsFailed > 0 {
			return 3
		}
		if r.HostsWarned > 0 {
			return 2
		}
		if r.HostsPassed > 0 {
			return 1
		}
		return 0
	}
	asc := sortDir == "asc"
	sort.Slice(rules, func(i, j int) bool {
		switch sortBy {
		case "status":
			va, vb := getStatusRank(rules[i]), getStatusRank(rules[j])
			if va != vb {
				return (asc && va < vb) || (!asc && va > vb)
			}
			sa := severityRank[ptrStr(rules[i].Severity)]
			sb := severityRank[ptrStr(rules[j].Severity)]
			if sa != sb {
				return (asc && sa < sb) || (!asc && sa > sb)
			}
			return rules[i].Title < rules[j].Title
		case "severity":
			sa, sb := severityRank[ptrStr(rules[i].Severity)], severityRank[ptrStr(rules[j].Severity)]
			if sa != sb {
				return (asc && sa < sb) || (!asc && sa > sb)
			}
			return getStatusRank(rules[i]) > getStatusRank(rules[j])
		case "title":
			return (asc && rules[i].Title < rules[j].Title) || (!asc && rules[i].Title > rules[j].Title)
		case "hosts_failed":
			return (asc && rules[i].HostsFailed < rules[j].HostsFailed) || (!asc && rules[i].HostsFailed > rules[j].HostsFailed)
		default:
			return false
		}
	})

	total := len(rules)
	start := int(offset)
	if start > total {
		start = total
	}
	end := start + int(limit)
	if end > total {
		end = total
	}
	return rules[start:end], total, nil
}

func ptrStr(s *string) string {
	if s == nil {
		return "unknown"
	}
	return *s
}

// RuleDetailWithAffectedHosts is the response for GetRuleDetail.
type RuleDetailWithAffectedHosts struct {
	Rule          db.GetComplianceRuleByIDRow
	AffectedHosts []AffectedHostResult
}

// AffectedHostResult is a single host's result for a rule.
type AffectedHostResult struct {
	HostID       string
	Hostname     *string
	FriendlyName string
	IP           *string
	Status       string
	Finding      *string
	Actual       *string
	Expected     *string
	ScanDate     *time.Time
}

// GetRuleDetail returns rule info and affected hosts from latest scans.
func (s *ComplianceStore) GetRuleDetail(ctx context.Context, ruleID string) (*RuleDetailWithAffectedHosts, error) {
	d := s.db.DB(ctx)
	rule, err := d.Queries.GetComplianceRuleByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	latestScans, err := d.Queries.GetLatestCompletedScansByProfile(ctx, rule.ProfileID)
	if err != nil {
		return nil, err
	}
	scanIDs := make([]string, 0, len(latestScans))
	for _, sc := range latestScans {
		scanIDs = append(scanIDs, sc.ID)
	}
	if len(scanIDs) == 0 {
		return &RuleDetailWithAffectedHosts{Rule: rule, AffectedHosts: []AffectedHostResult{}}, nil
	}
	results, err := d.Queries.GetComplianceResultsForRuleFromScans(ctx, db.GetComplianceResultsForRuleFromScansParams{
		RuleID:  ruleID,
		Column2: scanIDs,
	})
	if err != nil {
		return nil, err
	}
	hosts := make([]AffectedHostResult, 0, len(results))
	for _, r := range results {
		var scanDate *time.Time
		if r.CompletedAt.Valid {
			scanDate = &r.CompletedAt.Time
		}
		hosts = append(hosts, AffectedHostResult{
			HostID:       r.HostID,
			Hostname:     r.Hostname,
			FriendlyName: r.FriendlyName,
			IP:           r.Ip,
			Status:       r.Status,
			Finding:      r.Finding,
			Actual:       r.Actual,
			Expected:     r.Expected,
			ScanDate:     scanDate,
		})
	}
	// Sort: fail first, then warn, then pass
	statusOrder := map[string]int{"fail": 0, "failed": 0, "failure": 0, "warn": 1, "warning": 1, "warned": 1, "error": 2, "skip": 3, "notapplicable": 4, "pass": 5, "passed": 5}
	sort.Slice(hosts, func(i, j int) bool {
		return statusOrder[hosts[i].Status] < statusOrder[hosts[j].Status]
	})
	return &RuleDetailWithAffectedHosts{Rule: rule, AffectedHosts: hosts}, nil
}
