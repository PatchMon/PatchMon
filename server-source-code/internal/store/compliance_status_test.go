package store

import (
	"os"
	"strings"
	"testing"
)

func TestClassifyComplianceScan(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		score  float64
		failed int
		want   string
	}{
		{"high score", 92, 3, ComplianceStatusCompliant},
		{"threshold compliant", 80, 0, ComplianceStatusCompliant},
		{"mid score with failures", 70, 4, ComplianceStatusWarning},
		{"low score with failures", 40, 10, ComplianceStatusCritical},
		{"low score warnings only", 30, 0, ComplianceStatusWarning},
		{"zero score warnings only", 0, 0, ComplianceStatusWarning},
		{"just under warning with one failure", 59.9, 1, ComplianceStatusCritical},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyComplianceScan(tc.score, tc.failed); got != tc.want {
				t.Errorf("classifyComplianceScan(%v, %d) = %q, want %q", tc.score, tc.failed, got, tc.want)
			}
		})
	}
}

func TestHostComplianceStatusWorseThan(t *testing.T) {
	t.Parallel()
	dockerWarnOnly := hostComplianceStatus{status: ComplianceStatusWarning, score: 20, scanType: "docker-bench"}
	openscapCritical := hostComplianceStatus{status: ComplianceStatusCritical, score: 55, scanType: "openscap"}
	if !openscapCritical.worseThan(dockerWarnOnly) {
		t.Error("a critical scan must outrank a warning scan even when the warning scan scores lower")
	}
	if dockerWarnOnly.worseThan(openscapCritical) {
		t.Error("a warning scan must not outrank a critical scan")
	}
	lower := hostComplianceStatus{status: ComplianceStatusWarning, score: 61}
	higher := hostComplianceStatus{status: ComplianceStatusWarning, score: 75}
	if !lower.worseThan(higher) || higher.worseThan(lower) {
		t.Error("within the same bucket the lower score is worse")
	}
}

func complianceScanQuery(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../sqlc/queries/compliance_scans.sql")
	if err != nil {
		t.Fatalf("reading query file: %v", err)
	}
	sql := string(raw)
	start := strings.Index(sql, "-- name: "+name+" ")
	if start < 0 {
		t.Fatalf("%s query not found", name)
	}
	stmt := sql[start:]
	if next := strings.Index(stmt[1:], "\n-- name: "); next >= 0 {
		stmt = stmt[:next+1]
	}
	return stmt
}

func normaliseSQL(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Cancel must hit exactly the rows ListActiveComplianceScans reports, or the
// dashboard keeps showing a cancelled scan as running.
func TestCancelActiveComplianceScansMatchesActivePredicate(t *testing.T) {
	t.Parallel()
	cancel := normaliseSQL(complianceScanQuery(t, "CancelActiveComplianceScansByHost"))
	active := normaliseSQL(complianceScanQuery(t, "ListActiveComplianceScans"))

	const activePredicate = "status = 'running' OR (completed_at IS NULL AND status != 'failed')"
	if !strings.Contains(strings.ReplaceAll(active, "cs.", ""), activePredicate) {
		t.Fatalf("ListActiveComplianceScans predicate changed; update this test and the cancel query together")
	}
	if !strings.Contains(cancel, "host_id = $1 AND ("+activePredicate+")") {
		t.Errorf("cancel query must target the active-scan predicate for one host, got:\n%s", cancel)
	}
	for _, want := range []string{"status = 'cancelled'", "completed_at = NOW()"} {
		if !strings.Contains(cancel, want) {
			t.Errorf("cancel query must set %s so the row leaves the active set", want)
		}
	}
}

// SubmitScan clears only 'running' placeholders, so a late upload from the
// agent cannot erase or revive a cancelled row.
func TestDeleteRunningScansLeavesCancelledRows(t *testing.T) {
	t.Parallel()
	del := normaliseSQL(complianceScanQuery(t, "DeleteRunningComplianceScansByHost"))
	if !strings.HasSuffix(strings.TrimSuffix(del, ";"), "WHERE host_id = $1 AND status = 'running'") {
		t.Errorf("DeleteRunningComplianceScansByHost must only delete status='running' rows, got:\n%s", del)
	}
}
