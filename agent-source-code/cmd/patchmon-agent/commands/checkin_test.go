package commands

import (
	"encoding/json"
	"testing"

	"patchmon-agent/pkg/models"
)

// TestServerSupportsHashGate decodes the two ping response shapes the agent
// can meet in the wild and asserts the capability decision for each.
//
// The JSON here is deliberately literal rather than built from the struct:
// "hashGate" is a client/server contract string, and a rename on either side
// must fail this test rather than silently downgrade every agent to full
// reports (or, worse, leave a new agent thinking a pre-2.0.3 server said
// "nothing changed").
func TestServerSupportsHashGate(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "v2_0_3_server_advertises_hash_gate",
			body: `{"message":"pong","timestamp":"2026-07-27T10:00:00Z","friendlyName":"web-01","hashGate":true,"requestFull":["packages"]}`,
			want: true,
		},
		{
			name: "v2_0_3_server_steady_state_no_sections_requested",
			body: `{"message":"pong","timestamp":"2026-07-27T10:00:00Z","friendlyName":"web-01","hashGate":true}`,
			want: true,
		},
		{
			name: "pre_2_0_3_server_omits_marker",
			body: `{"message":"pong","timestamp":"2026-07-27T10:00:00Z","friendlyName":"web-01"}`,
			want: false,
		},
		{
			name: "explicit_false_marker_is_treated_as_legacy",
			body: `{"message":"pong","timestamp":"2026-07-27T10:00:00Z","hashGate":false}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp models.PingResponse
			if err := json.Unmarshal([]byte(tt.body), &resp); err != nil {
				t.Fatalf("failed to decode ping response: %v", err)
			}
			if got := serverSupportsHashGate(&resp); got != tt.want {
				t.Fatalf("serverSupportsHashGate() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("nil_response_is_not_capable", func(t *testing.T) {
		if serverSupportsHashGate(nil) {
			t.Fatal("expected nil ping response to be treated as legacy")
		}
	})
}

// TestShouldForceFullReport pins the defensive full-report cadence. A host
// must never go more than forcedFullReportInterval ticks without a full
// report, however well the hashes appear to match.
func TestShouldForceFullReport(t *testing.T) {
	for tick := uint64(1); tick < forcedFullReportInterval; tick++ {
		if shouldForceFullReport(tick) {
			t.Fatalf("shouldForceFullReport(%d) = true, want false", tick)
		}
	}

	for _, tick := range []uint64{forcedFullReportInterval, forcedFullReportInterval * 2, forcedFullReportInterval * 100} {
		if !shouldForceFullReport(tick) {
			t.Fatalf("shouldForceFullReport(%d) = false, want true", tick)
		}
	}
}
