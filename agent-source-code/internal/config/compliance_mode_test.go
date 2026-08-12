package config

import "testing"

func TestParseComplianceMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantMode ComplianceMode
		wantOK   bool
	}{
		// The three values the server's compliance_mode field can carry.
		{name: "server on-demand", input: "on-demand", wantMode: ComplianceOnDemand, wantOK: true},
		{name: "server enabled", input: "enabled", wantMode: ComplianceEnabled, wantOK: true},
		{name: "server disabled", input: "disabled", wantMode: ComplianceDisabled, wantOK: true},

		// Spellings config.yml itself is allowed to hold, so a value read back
		// out of the file parses the same way it went in.
		{name: "underscore spelling", input: "on_demand", wantMode: ComplianceOnDemand, wantOK: true},
		{name: "stringified true", input: "true", wantMode: ComplianceEnabled, wantOK: true},
		{name: "stringified false", input: "false", wantMode: ComplianceDisabled, wantOK: true},

		// Anything else must be reported as unrecognised rather than guessed
		// at, so the caller can decide what to do with a value neither side
		// agrees on.
		{name: "empty means the server did not send the field", input: "", wantOK: false},
		{name: "unknown mode", input: "ondemand", wantOK: false},
		{name: "wrong case", input: "On-Demand", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotMode, gotOK := ParseComplianceMode(tt.input)
			if gotOK != tt.wantOK {
				t.Fatalf("ParseComplianceMode(%q) ok = %v, want %v", tt.input, gotOK, tt.wantOK)
			}
			if gotMode != tt.wantMode {
				t.Fatalf("ParseComplianceMode(%q) = %q, want %q", tt.input, gotMode, tt.wantMode)
			}
		})
	}
}

// TestSetIntegrationEnabledCannotExpressOnDemand pins the reason the startup
// sync cannot go through the boolean setter. Both on-demand and scheduled read
// back as "enabled" through IsIntegrationEnabled, so a caller holding only the
// boolean has no way to tell them apart, and writing it back collapses
// on-demand into scheduled scans.
func TestSetIntegrationEnabledCannotExpressOnDemand(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)

	if err := m.SetComplianceMode(ComplianceOnDemand); err != nil {
		t.Fatalf("SetComplianceMode(on-demand): %v", err)
	}
	if !m.IsIntegrationEnabled("compliance") {
		t.Fatal("on-demand compliance reads as disabled through IsIntegrationEnabled")
	}

	if err := m.SetIntegrationEnabled("compliance", true); err != nil {
		t.Fatalf("SetIntegrationEnabled(compliance, true): %v", err)
	}
	if got := m.GetComplianceMode(); got != ComplianceEnabled {
		t.Fatalf("compliance mode = %q, want %q", got, ComplianceEnabled)
	}
}
