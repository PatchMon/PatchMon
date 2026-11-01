package handler

import "testing"

func TestScanOutcome(t *testing.T) {
	total120 := 120
	tests := []struct {
		name       string
		scan       complianceScanItem
		results    int
		wantStatus string
		wantErr    string
	}{
		{"zero rules openscap becomes failed", complianceScanItem{ProfileType: "openscap", Status: "completed"}, 0, "failed", noRulesAvailableMessage},
		{"zero rules with empty type becomes failed", complianceScanItem{Status: "completed"}, 0, "failed", noRulesAvailableMessage},
		{"rules present stays completed", complianceScanItem{ProfileType: "openscap", Status: "completed", TotalRules: &total120}, 120, "completed", ""},
		{"results without total stays completed", complianceScanItem{ProfileType: "openscap", Status: "completed"}, 3, "completed", ""},
		{"docker bench untouched", complianceScanItem{ProfileType: "docker-bench", Status: "completed"}, 0, "completed", ""},
		{"agent failure keeps its error", complianceScanItem{ProfileType: "openscap", Status: "failed", Error: "oscap crashed"}, 0, "failed", "oscap crashed"},
		{"cancelled untouched", complianceScanItem{ProfileType: "openscap", Status: "cancelled", Error: "x"}, 0, "cancelled", "x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotErr := scanOutcome(tc.scan, tc.results)
			if gotStatus != tc.wantStatus || gotErr != tc.wantErr {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotStatus, gotErr, tc.wantStatus, tc.wantErr)
			}
		})
	}
}
