package models

import "time"

// ComplianceRule represents a compliance rule definition
type ComplianceRule struct {
	RuleID      string `json:"rule_id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Section     string `json:"section,omitempty"`
	Severity    string `json:"severity,omitempty"` // low, medium, high, critical
	Remediation string `json:"remediation,omitempty"`
}

// ComplianceResult represents a single rule evaluation result
type ComplianceResult struct {
	RuleID      string `json:"rule_ref"` // Backend expects rule_ref, not rule_id
	Title       string `json:"title"`
	Status      string `json:"status"` // pass, fail, warn, skip, notapplicable, error
	Finding     string `json:"finding,omitempty"`
	Actual      string `json:"actual,omitempty"`   // Actual value found on the system
	Expected    string `json:"expected,omitempty"` // Expected/required value
	Section     string `json:"section,omitempty"`
	Description string `json:"description,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// ComplianceScan represents results of a compliance scan
type ComplianceScan struct {
	ProfileName        string             `json:"profile_name"`
	ProfileType        string             `json:"profile_type"` // openscap, docker-bench
	Status             string             `json:"status"`       // completed, failed, in_progress
	Score              float64            `json:"score"`
	TotalRules         int                `json:"total_rules"`
	Passed             int                `json:"passed"`
	Failed             int                `json:"failed"`
	Warnings           int                `json:"warnings"`
	Skipped            int                `json:"skipped"`
	NotApplicable      int                `json:"not_applicable"`
	StartedAt          time.Time          `json:"started_at"`
	CompletedAt        *time.Time         `json:"completed_at,omitempty"`
	Results            []ComplianceResult `json:"results"`
	Error              string             `json:"error,omitempty"`
	RemediationApplied bool               `json:"remediation_applied,omitempty"`
	RemediationCount   int                `json:"remediation_count,omitempty"` // Number of rules remediated
}

// ComplianceData represents all compliance-related data
type ComplianceData struct {
	Scans       []ComplianceScan      `json:"scans"`
	OSInfo      ComplianceOSInfo      `json:"os_info"`
	ScannerInfo ComplianceScannerInfo `json:"scanner_info"`
}

// ComplianceOSInfo represents OS information for compliance context
type ComplianceOSInfo struct {
	Family  string `json:"family"`  // debian, rhel, suse
	Name    string `json:"name"`    // ubuntu, rocky, debian
	Version string `json:"version"` // 22.04, 9, 12
}

// ComplianceScannerInfo represents scanner availability information
type ComplianceScannerInfo struct {
	OpenSCAPAvailable    bool     `json:"openscap_available"`
	OpenSCAPVersion      string   `json:"openscap_version,omitempty"`
	DockerBenchAvailable bool     `json:"docker_bench_available"`
	OscapDockerAvailable bool     `json:"oscap_docker_available"`
	AvailableProfiles    []string `json:"available_profiles,omitempty"`
}

// CompliancePayload represents the payload sent to the compliance endpoint
type CompliancePayload struct {
	ComplianceData
	Hostname     string `json:"hostname"`
	MachineID    string `json:"machine_id"`
	AgentVersion string `json:"agent_version"`
}

// ComplianceResponse represents the response from the compliance endpoint
type ComplianceResponse struct {
	Message       string `json:"message"`
	ScanID        string `json:"scan_id,omitempty"`
	ScansReceived int    `json:"scans_received"`
}
