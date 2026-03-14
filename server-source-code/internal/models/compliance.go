package models

import "time"

// ComplianceProfile matches compliance_profiles table.
type ComplianceProfile struct {
	ID          string    `db:"id"`
	Name        string    `db:"name"`
	Type        string    `db:"type"`
	OSFamily    *string   `db:"os_family"`
	Version     *string   `db:"version"`
	Description *string   `db:"description"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// ComplianceScan matches compliance_scans table.
type ComplianceScan struct {
	ID            string     `db:"id"`
	HostID        string     `db:"host_id"`
	ProfileID     string     `db:"profile_id"`
	StartedAt     time.Time  `db:"started_at"`
	CompletedAt   *time.Time `db:"completed_at"`
	Status        string     `db:"status"`
	TotalRules    int        `db:"total_rules"`
	Passed        int        `db:"passed"`
	Failed        int        `db:"failed"`
	Warnings      int        `db:"warnings"`
	Skipped       int        `db:"skipped"`
	NotApplicable int        `db:"not_applicable"`
	Score         *float64   `db:"score"`
	ErrorMessage  *string    `db:"error_message"`
	RawOutput     *string    `db:"raw_output"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
}

// ComplianceRule matches compliance_rules table.
type ComplianceRule struct {
	ID          string  `db:"id"`
	ProfileID   string  `db:"profile_id"`
	RuleRef     string  `db:"rule_ref"`
	Title       string  `db:"title"`
	Description *string `db:"description"`
	Rationale   *string `db:"rationale"`
	Severity    *string `db:"severity"`
	Section     *string `db:"section"`
	Remediation *string `db:"remediation"`
}

// ComplianceResult matches compliance_results table.
type ComplianceResult struct {
	ID          string    `db:"id"`
	ScanID      string    `db:"scan_id"`
	RuleID      string    `db:"rule_id"`
	Status      string    `db:"status"`
	Finding     *string   `db:"finding"`
	Actual      *string   `db:"actual"`
	Expected    *string   `db:"expected"`
	Remediation *string   `db:"remediation"`
	CreatedAt   time.Time `db:"created_at"`
}
