package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ComplianceStore provides compliance data access.
type ComplianceStore struct {
	db database.DBProvider
}

// NewComplianceStore creates a new compliance store.
func NewComplianceStore(db database.DBProvider) *ComplianceStore {
	return &ComplianceStore{db: db}
}

// Valid result statuses and severities (match legacy)
var (
	ValidResultStatuses = []string{"pass", "fail", "warn", "skip", "notapplicable", "skipped", "error"}
	ValidSeverities     = []string{"low", "medium", "high", "critical", "unknown"}
	ValidProfileTypes   = []string{"openscap", "docker-bench", "oscap-docker", "all"}
)

// Status priority for deduplication: fail > warn > pass > skip > notapplicable > error
var statusPriority = map[string]int{
	"fail": 6, "failed": 6, "failure": 6,
	"warn": 5, "warning": 5, "warned": 5,
	"pass": 4, "passed": 4,
	"skip": 3, "skipped": 3,
	"notapplicable": 2, "not_applicable": 2, "na": 2,
	"error": 1,
}

func normalizeResultStatus(s string) string {
	if s == "" {
		return s
	}
	switch s {
	case "fail", "failed", "failure":
		return "fail"
	case "pass", "passed":
		return "pass"
	case "warn", "warning", "warned":
		return "warn"
	case "skip", "skipped":
		return "skip"
	case "notapplicable", "not_applicable", "na":
		return "notapplicable"
	case "error":
		return "error"
	}
	return s
}

// ListProfiles returns all compliance profiles.
func (s *ComplianceStore) ListProfiles(ctx context.Context) ([]models.ComplianceProfile, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListComplianceProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.ComplianceProfile, len(rows))
	for i, r := range rows {
		out[i] = models.ComplianceProfile{
			ID:          r.ID,
			Name:        r.Name,
			Type:        r.Type,
			OSFamily:    r.OsFamily,
			Version:     r.Version,
			Description: r.Description,
			CreatedAt:   pgTime(r.CreatedAt),
			UpdatedAt:   pgTime(r.UpdatedAt),
		}
	}
	return out, nil
}

// GetOrCreateProfile returns a profile by name, creating it if it doesn't exist.
func (s *ComplianceStore) GetOrCreateProfile(ctx context.Context, name, profileType string) (*models.ComplianceProfile, error) {
	d := s.db.DB(ctx)
	prof, err := d.Queries.GetComplianceProfileByName(ctx, name)
	if err == nil {
		return &models.ComplianceProfile{
			ID:          prof.ID,
			Name:        prof.Name,
			Type:        prof.Type,
			OSFamily:    prof.OsFamily,
			Version:     prof.Version,
			Description: prof.Description,
			CreatedAt:   pgTime(prof.CreatedAt),
			UpdatedAt:   pgTime(prof.UpdatedAt),
		}, nil
	}
	// Create new profile
	if profileType == "" {
		profileType = "openscap"
	}
	created, err := d.Queries.CreateComplianceProfile(ctx, db.CreateComplianceProfileParams{
		ID:          uuid.New().String(),
		Name:        name,
		Type:        profileType,
		OsFamily:    nil,
		Version:     nil,
		Description: nil,
	})
	if err != nil {
		return nil, err
	}
	return &models.ComplianceProfile{
		ID:          created.ID,
		Name:        created.Name,
		Type:        created.Type,
		OSFamily:    created.OsFamily,
		Version:     created.Version,
		Description: created.Description,
		CreatedAt:   pgTime(created.CreatedAt),
		UpdatedAt:   pgTime(created.UpdatedAt),
	}, nil
}

// SubmittedScanResult represents a single result from the agent.
type SubmittedScanResult struct {
	RuleRef     string
	Title       string
	Description string
	Severity    string
	Section     string
	Remediation string
	Status      string
	Finding     string
	Actual      string
	Expected    string
}

// SubmittedScan represents a scan from the agent.
type SubmittedScan struct {
	ProfileName   string
	ProfileType   string
	Results       []SubmittedScanResult
	StartedAt     *time.Time
	CompletedAt   *time.Time
	Status        string
	Score         *float64
	TotalRules    *int
	Passed        *int
	Failed        *int
	Warnings      *int
	Skipped       *int
	NotApplicable *int
	Error         string
}

// ProcessedScan represents a successfully processed scan.
type ProcessedScan struct {
	ScanID        string
	ProfileName   string
	Score         *float64
	Stats         map[string]int
	ResultsStored int
}

// SubmitScan processes and stores scan results from an agent.
func (s *ComplianceStore) SubmitScan(ctx context.Context, hostID string, openscapEnabled, dockerBenchEnabled bool, scans []SubmittedScan) ([]ProcessedScan, error) {
	d := s.db.DB(ctx)
	var processed []ProcessedScan
	for _, scanData := range scans {
		if scanData.ProfileName == "" {
			continue
		}
		profile, err := s.GetOrCreateProfile(ctx, scanData.ProfileName, scanData.ProfileType)
		if err != nil {
			return nil, err
		}
		profileType := profile.Type
		if profileType == "" {
			profileType = scanData.ProfileType
		}
		if profileType == "" {
			profileType = "openscap"
		}
		// Filter by scanner toggles
		if (profileType == "openscap" && !openscapEnabled) || (profileType == "docker-bench" && !dockerBenchEnabled) {
			continue
		}
		// Compute stats
		stats := map[string]int{
			"total_rules": 0, "passed": 0, "failed": 0, "warnings": 0,
			"skipped": 0, "not_applicable": 0,
		}
		if scanData.Results != nil {
			stats["total_rules"] = len(scanData.Results)
			for _, r := range scanData.Results {
				switch normalizeResultStatus(r.Status) {
				case "pass":
					stats["passed"]++
				case "fail":
					stats["failed"]++
				case "warn":
					stats["warnings"]++
				case "skip":
					stats["skipped"]++
				case "notapplicable":
					stats["not_applicable"]++
				}
			}
		}
		if scanData.TotalRules != nil {
			stats["total_rules"] = *scanData.TotalRules
		}
		if scanData.Passed != nil {
			stats["passed"] = *scanData.Passed
		}
		if scanData.Failed != nil {
			stats["failed"] = *scanData.Failed
		}
		if scanData.Warnings != nil {
			stats["warnings"] = *scanData.Warnings
		}
		if scanData.Skipped != nil {
			stats["skipped"] = *scanData.Skipped
		}
		if scanData.NotApplicable != nil {
			stats["not_applicable"] = *scanData.NotApplicable
		}
		score := scanData.Score
		if score == nil && stats["total_rules"] > 0 {
			applicable := stats["total_rules"] - stats["not_applicable"] - stats["skipped"]
			if applicable > 0 {
				sc := float64(stats["passed"]) / float64(applicable) * 100
				score = &sc
			}
		}
		// Delete running placeholders
		if err := d.Queries.DeleteRunningComplianceScansByHost(ctx, hostID); err != nil {
			return nil, err
		}
		// Create scan
		startedAt := time.Now()
		if scanData.StartedAt != nil {
			startedAt = *scanData.StartedAt
		}
		completedAt := time.Now()
		if scanData.CompletedAt != nil {
			completedAt = *scanData.CompletedAt
		}
		status := "completed"
		if scanData.Status == "failed" {
			status = "failed"
		}
		scan, err := d.Queries.CreateComplianceScan(ctx, db.CreateComplianceScanParams{
			ID:            uuid.New().String(),
			HostID:        hostID,
			ProfileID:     profile.ID,
			StartedAt:     pgtype.Timestamp{Time: startedAt, Valid: true},
			CompletedAt:   pgtype.Timestamp{Time: completedAt, Valid: true},
			Status:        status,
			TotalRules:    int32(stats["total_rules"]),
			Passed:        int32(stats["passed"]),
			Failed:        int32(stats["failed"]),
			Warnings:      int32(stats["warnings"]),
			Skipped:       int32(stats["skipped"]),
			NotApplicable: int32(stats["not_applicable"]),
			Score:         score,
			ErrorMessage:  complianceStrPtr(scanData.Error),
			RawOutput:     nil,
		})
		if err != nil {
			return nil, err
		}
		// Deduplicate and create rules + results
		resultsStored := 0
		if len(scanData.Results) > 0 {
			deduped := make(map[string]SubmittedScanResult)
			for _, r := range scanData.Results {
				ruleRef := r.RuleRef
				if ruleRef == "" {
					continue
				}
				existing, ok := deduped[ruleRef]
				if !ok {
					deduped[ruleRef] = r
					continue
				}
				ep := statusPriority[normalizeResultStatus(existing.Status)]
				np := statusPriority[normalizeResultStatus(r.Status)]
				if np > ep {
					deduped[ruleRef] = r
				}
			}
			// Delete existing results for this scan
			if err := d.Queries.DeleteComplianceResultsByScan(ctx, scan.ID); err != nil {
				return nil, err
			}
			for _, r := range deduped {
				ruleRef := r.RuleRef
				// Get or create rule
				rule, err := d.Queries.GetComplianceRuleByProfileAndRef(ctx, db.GetComplianceRuleByProfileAndRefParams{
					ProfileID: profile.ID,
					RuleRef:   ruleRef,
				})
				var ruleID string
				if err != nil {
					ruleID = uuid.New().String()
					_, err = d.Queries.CreateComplianceRule(ctx, db.CreateComplianceRuleParams{
						ID:          ruleID,
						ProfileID:   profile.ID,
						RuleRef:     ruleRef,
						Title:       orEmpty(r.Title, ruleRef, "Unknown"),
						Description: complianceStrPtr(r.Description),
						Rationale:   nil,
						Severity:    complianceStrPtr(r.Severity),
						Section:     complianceStrPtr(r.Section),
						Remediation: complianceStrPtr(r.Remediation),
					})
					if err != nil {
						return nil, err
					}
				} else {
					ruleID = rule.ID
					// Update rule if we have better metadata
					_ = d.Queries.UpdateComplianceRule(ctx, db.UpdateComplianceRuleParams{
						ID:          ruleID,
						Title:       complianceStrPtr(r.Title),
						Description: complianceStrPtr(r.Description),
						Severity:    complianceStrPtr(r.Severity),
						Section:     complianceStrPtr(r.Section),
						Remediation: complianceStrPtr(r.Remediation),
					})
				}
				statusVal := normalizeResultStatus(r.Status)
				if statusVal == "" {
					statusVal = r.Status
				}
				_, err = d.Queries.CreateComplianceResult(ctx, db.CreateComplianceResultParams{
					ID:          uuid.New().String(),
					ScanID:      scan.ID,
					RuleID:      ruleID,
					Status:      statusVal,
					Finding:     complianceStrPtr(r.Finding),
					Actual:      complianceStrPtr(r.Actual),
					Expected:    complianceStrPtr(r.Expected),
					Remediation: complianceStrPtr(r.Remediation),
				})
				if err != nil {
					return nil, err
				}
				resultsStored++
			}
		}
		processed = append(processed, ProcessedScan{
			ScanID:        scan.ID,
			ProfileName:   scanData.ProfileName,
			Score:         score,
			Stats:         stats,
			ResultsStored: resultsStored,
		})
	}
	return processed, nil
}

func complianceStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func orEmpty(a, b, c string) string {
	if a != "" {
		return a
	}
	if b != "" {
		return b
	}
	return c
}

// ListScansByHost returns paginated scans for a host.
func (s *ComplianceStore) ListScansByHost(ctx context.Context, hostID string, limit, offset int32) ([]db.ListComplianceScansByHostRow, int64, error) {
	d := s.db.DB(ctx)
	scans, err := d.Queries.ListComplianceScansByHost(ctx, db.ListComplianceScansByHostParams{
		HostID: hostID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := d.Queries.CountComplianceScansByHost(ctx, hostID)
	if err != nil {
		return nil, 0, err
	}
	return scans, total, nil
}

// GetLatestScan returns the latest completed scan for a host, optionally filtered by profile type.
func (s *ComplianceStore) GetLatestScan(ctx context.Context, hostID string, profileType *string) (*db.GetLatestComplianceScanByHostRow, error) {
	d := s.db.DB(ctx)
	if profileType != nil && *profileType != "" {
		// Get latest by profile type - need to filter
		scans, err := d.Queries.ListComplianceScansByHost(ctx, db.ListComplianceScansByHostParams{
			HostID: hostID,
			Limit:  100,
			Offset: 0,
		})
		if err != nil {
			return nil, err
		}
		for _, sc := range scans {
			if sc.Status == "completed" && sc.ProfileType == *profileType {
				// Fetch full scan
				full, err := d.Queries.GetComplianceScanByID(ctx, sc.ID)
				if err != nil {
					return nil, err
				}
				return &db.GetLatestComplianceScanByHostRow{
					ID: full.ID, HostID: full.HostID, ProfileID: full.ProfileID,
					StartedAt: full.StartedAt, CompletedAt: full.CompletedAt, Status: full.Status,
					TotalRules: full.TotalRules, Passed: full.Passed, Failed: full.Failed,
					Warnings: full.Warnings, Skipped: full.Skipped, NotApplicable: full.NotApplicable,
					Score: full.Score, ErrorMessage: full.ErrorMessage, RawOutput: full.RawOutput,
					CreatedAt: full.CreatedAt, UpdatedAt: full.UpdatedAt,
					ProfileName: full.ProfileName, ProfileType: full.ProfileType,
				}, nil
			}
		}
		return nil, nil
	}
	row, err := d.Queries.GetLatestComplianceScanByHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// GetFirstProfileByType returns the first profile of a given type (for creating placeholder scans).
func (s *ComplianceStore) GetFirstProfileByType(ctx context.Context, profileType string) (*models.ComplianceProfile, error) {
	d := s.db.DB(ctx)
	prof, err := d.Queries.GetFirstComplianceProfileByType(ctx, profileType)
	if err != nil {
		return nil, err
	}
	return &models.ComplianceProfile{
		ID: prof.ID, Name: prof.Name, Type: prof.Type,
		OSFamily: prof.OsFamily, Version: prof.Version, Description: prof.Description,
		CreatedAt: pgTime(prof.CreatedAt), UpdatedAt: pgTime(prof.UpdatedAt),
	}, nil
}

// ListScansHistory returns paginated scan history with optional filters.
func (s *ComplianceStore) ListScansHistory(ctx context.Context, limit, offset int32, status, hostID, profileType *string) ([]db.ListComplianceScansHistoryRow, int64, error) {
	d := s.db.DB(ctx)
	params := db.ListComplianceScansHistoryParams{
		Limit:       limit,
		Offset:      offset,
		Status:      status,
		HostID:      hostID,
		ProfileType: profileType,
	}
	rows, err := d.Queries.ListComplianceScansHistory(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := d.Queries.CountComplianceScansHistory(ctx, db.CountComplianceScansHistoryParams{
		Status: status, HostID: hostID, ProfileType: profileType,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListActiveScans returns currently running scans.
func (s *ComplianceStore) ListActiveScans(ctx context.Context) ([]db.ListActiveComplianceScansRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListActiveComplianceScans(ctx)
}

// ListStalledScans returns scans running over the threshold.
func (s *ComplianceStore) ListStalledScans(ctx context.Context, threshold time.Time) ([]db.ListStalledComplianceScansWithDetailsRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListStalledComplianceScansWithDetails(ctx, pgtype.Timestamp{Time: threshold, Valid: true})
}

// ListResultsByScan returns paginated results for a scan with optional filters.
func (s *ComplianceStore) ListResultsByScan(ctx context.Context, scanID string, statusFilter, severityFilter *string, limit, offset int32) ([]db.ListComplianceResultsByScanRow, int64, map[string]interface{}, error) {
	d := s.db.DB(ctx)
	params := db.ListComplianceResultsByScanParams{
		ScanID:         scanID,
		StatusFilter:   statusFilter,
		SeverityFilter: severityFilter,
	}
	allRows, err := d.Queries.ListComplianceResultsByScan(ctx, params)
	if err != nil {
		return nil, 0, nil, err
	}
	total := int64(len(allRows))
	// Paginate in memory (query doesn't support limit/offset)
	start := int(offset)
	if start > len(allRows) {
		start = len(allRows)
	}
	end := start + int(limit)
	if end > len(allRows) {
		end = len(allRows)
	}
	rows := allRows[start:end]

	// Severity breakdown (when offset=0)
	var severityBreakdown map[string]interface{}
	if offset == 0 {
		sevRows, _ := d.Queries.GetComplianceResultSeverityBreakdown(ctx, scanID)
		statusRows, _ := d.Queries.GetComplianceResultStatusBreakdown(ctx, scanID)
		bySeverity := make(map[string]int)
		for _, r := range sevRows {
			sev := "unknown"
			if r.Severity != nil {
				sev = *r.Severity
			}
			bySeverity[sev] = int(r.Count)
		}
		byStatus := make(map[string]int)
		for _, r := range statusRows {
			byStatus[r.Status] = int(r.Count)
		}
		severityBreakdown = map[string]interface{}{
			"by_status":   byStatus,
			"by_severity": bySeverity,
		}
	}
	return rows, total, severityBreakdown, nil
}

// LatestScansByType is the response for GetLatestScansByType (map of profile type to scan summary).
type LatestScansByType map[string]LatestScanByTypeSummary

// LatestScanByTypeSummary is the summary for one profile type.
type LatestScanByTypeSummary struct {
	ID                string
	ProfileName       *string
	ProfileType       string
	Score             *float64
	TotalRules        int
	Passed            int
	Failed            int
	Warnings          int
	Skipped           int
	CompletedAt       *time.Time
	SeverityBreakdown []SeverityCount
	SectionBreakdown  []SectionCount
}

type SeverityCount struct {
	Severity string
	Count    int
}

type SectionCount struct {
	Section string
	Count   int
}

// GetLatestScansByType returns the latest scan per profile type for a host.
func (s *ComplianceStore) GetLatestScansByType(ctx context.Context, hostID string) (LatestScansByType, error) {
	d := s.db.DB(ctx)
	scans, err := d.Queries.ListComplianceScansByHost(ctx, db.ListComplianceScansByHostParams{
		HostID: hostID,
		Limit:  100,
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}
	result := make(LatestScansByType)
	for _, sc := range scans {
		if sc.Status != "completed" {
			continue
		}
		if _, ok := result[sc.ProfileType]; ok {
			continue // Already have latest for this type
		}
		var completedAt *time.Time
		if sc.CompletedAt.Valid {
			completedAt = &sc.CompletedAt.Time
		}
		summary := LatestScanByTypeSummary{
			ID:          sc.ID,
			ProfileName: &sc.ProfileName,
			ProfileType: sc.ProfileType,
			Score:       sc.Score,
			TotalRules:  int(sc.TotalRules),
			Passed:      int(sc.Passed),
			Failed:      int(sc.Failed),
			Warnings:    int(sc.Warnings),
			Skipped:     int(sc.Skipped),
			CompletedAt: completedAt,
		}
		if sc.ProfileType == "openscap" {
			sevRows, _ := d.Queries.GetComplianceResultSeverityBreakdown(ctx, sc.ID)
			for _, r := range sevRows {
				sev := "unknown"
				if r.Severity != nil {
					sev = *r.Severity
				}
				summary.SeverityBreakdown = append(summary.SeverityBreakdown, SeverityCount{Severity: sev, Count: int(r.Count)})
			}
		}
		if sc.ProfileType == "docker-bench" {
			// Get section breakdown for warnings
			warnFilter := "warn"
			rows, _ := d.Queries.ListComplianceResultsByScan(ctx, db.ListComplianceResultsByScanParams{
				ScanID: sc.ID, StatusFilter: &warnFilter, SeverityFilter: nil,
			})
			sectionCounts := make(map[string]int)
			for _, r := range rows {
				sec := "Unknown"
				if r.Section != nil {
					sec = *r.Section
				}
				sectionCounts[sec]++
			}
			for sec, cnt := range sectionCounts {
				summary.SectionBreakdown = append(summary.SectionBreakdown, SectionCount{Section: sec, Count: cnt})
			}
		}
		result[sc.ProfileType] = summary
	}
	return result, nil
}

// GetTrends returns compliance score trends for a host over time.
func (s *ComplianceStore) GetTrends(ctx context.Context, hostID string, days int) ([]db.GetComplianceScansForTrendsRow, error) {
	d := s.db.DB(ctx)
	since := time.Now().AddDate(0, 0, -days)
	return d.Queries.GetComplianceScansForTrends(ctx, db.GetComplianceScansForTrendsParams{
		HostID:      hostID,
		CompletedAt: pgtype.Timestamp{Time: since, Valid: true},
	})
}

// CreateRunningScan creates a placeholder "running" scan record.
func (s *ComplianceStore) CreateRunningScan(ctx context.Context, hostID, profileID string) error {
	d := s.db.DB(ctx)
	_, err := d.Queries.CreateComplianceScan(ctx, db.CreateComplianceScanParams{
		ID:            uuid.New().String(),
		HostID:        hostID,
		ProfileID:     profileID,
		StartedAt:     pgtype.Timestamp{Time: time.Now(), Valid: true},
		CompletedAt:   pgtype.Timestamp{},
		Status:        "running",
		TotalRules:    0,
		Passed:        0,
		Failed:        0,
		Warnings:      0,
		Skipped:       0,
		NotApplicable: 0,
		Score:         nil,
		ErrorMessage:  nil,
		RawOutput:     nil,
	})
	return err
}
