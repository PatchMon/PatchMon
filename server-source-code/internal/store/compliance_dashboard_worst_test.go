package store

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

func TestWorstScanPerHost(t *testing.T) {
	score := func(v float64) *float64 { return &v }
	scans := []db.GetLatestCompletedScansRow{
		{ID: "bench", HostID: "h1", ProfileID: "p-bench", Score: score(97), Passed: 95, Failed: 2, ProfileName: "Docker Bench"},
		{ID: "cis", HostID: "h1", ProfileID: "p-cis", Score: score(67), Passed: 231, Failed: 112, ProfileName: "CIS Level 1"},
		{ID: "only", HostID: "h2", ProfileID: "p-cis", Score: score(85), Passed: 300, Failed: 10, ProfileName: "CIS Level 1"},
	}
	types := map[string]string{"p-bench": "docker-bench", "p-cis": "openscap"}

	got := worstScanPerHost(scans, types)

	if h1 := got["h1"]; h1.scan.ID != "cis" || h1.status != ComplianceStatusWarning || h1.scanType != "openscap" {
		t.Fatalf("h1 should be shown by its worse OpenSCAP scan, got scan=%s status=%s type=%s", h1.scan.ID, h1.status, h1.scanType)
	}
	if h2 := got["h2"]; h2.scan.ID != "only" || h2.status != ComplianceStatusCompliant {
		t.Fatalf("h2 has one scan, got scan=%s status=%s", h2.scan.ID, h2.status)
	}
}
