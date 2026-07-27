package alerts

import (
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
)

// TestHostDownState_LiveWebsocketIsNeverDown is the core regression guard for
// the fleet-wide false-alert storm.
//
// hosts.last_update only moves on the agent's HTTP check-in, which runs on
// update_interval (default 60 MINUTES). The host_down threshold is 30 SECONDS
// and describes a WebSocket disconnect duration; the 30-second WebSocket pings
// never touch the database. Comparing last_update against a 30-second window
// therefore flagged essentially every host on every sweep, alerting the whole
// fleet and then flapping down/recovered forever.
//
// A live WebSocket is proof the agent is reachable and must win regardless of
// how old last_update is.
func TestHostDownState_LiveWebsocketIsNeverDown(t *testing.T) {
	t.Parallel()

	reg := agentregistry.New()
	reg.Register("api-1", true)

	now := time.Now()
	// last_update an hour old: entirely normal one minute before the next
	// hourly check-in, and the exact situation that used to fire an alert.
	lastUpdate := now.Add(-59 * time.Minute)
	// Report-cadence window for the default 60-minute interval.
	cadenceThreshold := now.Add(-180 * time.Minute)

	got := hostDownState(reg, "api-1", now, 30*time.Second, lastUpdate, cadenceThreshold, 60)
	if got.down {
		t.Fatalf("regression: host with a live WebSocket reported down purely because last_update is older than the 30s threshold: %+v", got)
	}
	if got.detection != detectionWebsocket {
		t.Errorf("expected detection=%q, got %q", detectionWebsocket, got.detection)
	}
}

// TestHostDownState_DisconnectedWebsocketUsesThreshold verifies the WebSocket
// path applies the configured threshold to the DISCONNECT duration, which is
// the only thing a sub-minute threshold was ever meaningful for.
func TestHostDownState_DisconnectedWebsocketUsesThreshold(t *testing.T) {
	t.Parallel()

	base := time.Now()
	cases := []struct {
		name       string
		sinceDrop  time.Duration
		wantDown   bool
		wantReason string
	}{
		{"within grace window", 5 * time.Second, false, detectionWebsocket},
		{"beyond threshold", 90 * time.Second, true, detectionWebsocket},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := agentregistry.New()
			reg.Register("api-1", false)
			reg.Unregister("api-1") // stamps DisconnectedAt ~= base

			// Advance "now" instead of sleeping.
			now := base.Add(tc.sinceDrop)
			// last_update is fresh, so a cadence-based rule would say "up" —
			// proving the WebSocket rule is what decided this.
			lastUpdate := base
			cadenceThreshold := base.Add(-180 * time.Minute)

			got := hostDownState(reg, "api-1", now, 30*time.Second, lastUpdate, cadenceThreshold, 60)
			if got.down != tc.wantDown {
				t.Fatalf("down = %v, want %v (%+v)", got.down, tc.wantDown, got)
			}
			if got.detection != tc.wantReason {
				t.Errorf("detection = %q, want %q", got.detection, tc.wantReason)
			}
			if got.thresholdSeconds != 30 {
				t.Errorf("thresholdSeconds = %d, want the configured 30", got.thresholdSeconds)
			}
		})
	}
}

// TestHostDownState_NoWebsocketHistoryFallsBackToReportCadence covers agents
// the registry has never seen: report cadence is the only available signal, so
// the pre-2.0.3 rule (update_interval * 3) applies, NOT the WebSocket threshold.
func TestHostDownState_NoWebsocketHistoryFallsBackToReportCadence(t *testing.T) {
	t.Parallel()

	now := time.Now()
	const updateIntervalMin = 60
	cadenceThreshold := now.Add(-updateIntervalMin * reportCadenceStaleMultiplier * time.Minute)

	cases := []struct {
		name       string
		lastUpdate time.Time
		wantDown   bool
	}{
		// Two hours is two missed cycles: still inside the 3x window.
		{"reporting late but inside the window", now.Add(-2 * time.Hour), false},
		// Four hours is past 3x60 minutes.
		{"past the report cadence window", now.Add(-4 * time.Hour), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := agentregistry.New() // never saw this api_id

			got := hostDownState(reg, "never-seen", now, 30*time.Second, tc.lastUpdate, cadenceThreshold, updateIntervalMin)
			if got.down != tc.wantDown {
				t.Fatalf("down = %v, want %v (%+v)", got.down, tc.wantDown, got)
			}
			if got.detection != detectionReportCadence {
				t.Errorf("detection = %q, want %q", got.detection, detectionReportCadence)
			}
			// The quoted threshold must be the cadence window, not the 30s
			// WebSocket value — operators read this number in the alert body.
			if want := updateIntervalMin * reportCadenceStaleMultiplier * 60; got.thresholdSeconds != want {
				t.Errorf("thresholdSeconds = %d, want %d", got.thresholdSeconds, want)
			}
		})
	}
}

// TestHostDownState_NilRegistryFallsBackToReportCadence keeps the function safe
// for callers with no registry wired (test fixtures, future call sites).
func TestHostDownState_NilRegistryFallsBackToReportCadence(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cadenceThreshold := now.Add(-180 * time.Minute)

	if got := hostDownState(nil, "api-1", now, 30*time.Second, now.Add(-10*time.Minute), cadenceThreshold, 60); got.down {
		t.Errorf("fresh host reported down with a nil registry: %+v", got)
	}
	if got := hostDownState(nil, "api-1", now, 30*time.Second, now.Add(-10*time.Hour), cadenceThreshold, 60); !got.down {
		t.Errorf("long-silent host not reported down with a nil registry: %+v", got)
	}
}

// TestHostDownState_ReconnectClearsDownState verifies that a reconnect flips the
// decision straight back to up, so the sweep resolves the alert on its next run.
func TestHostDownState_ReconnectClearsDownState(t *testing.T) {
	t.Parallel()

	reg := agentregistry.New()
	base := time.Now()
	reg.Register("api-1", false)
	reg.Unregister("api-1")

	now := base.Add(10 * time.Minute)
	cadenceThreshold := base.Add(-180 * time.Minute)

	if got := hostDownState(reg, "api-1", now, 30*time.Second, base, cadenceThreshold, 60); !got.down {
		t.Fatalf("precondition failed: expected down after a 10-minute disconnect, got %+v", got)
	}

	reg.Register("api-1", false)
	if got := hostDownState(reg, "api-1", now, 30*time.Second, base, cadenceThreshold, 60); got.down {
		t.Fatalf("expected up after reconnect, got %+v", got)
	}
}

// TestFormatHostDownThreshold pins the operator-facing rendering, which now has
// to cope with both the sub-minute WebSocket window and the multi-hour report
// cadence window.
func TestFormatHostDownThreshold(t *testing.T) {
	t.Parallel()

	cases := []struct {
		seconds int
		want    string
	}{
		{30, "30 seconds"},
		{59, "59 seconds"},
		{60, "1 minute"},
		{120, "2 minutes"},
		{10800, "180 minutes"},
	}
	for _, tc := range cases {
		if got := formatHostDownThreshold(tc.seconds); got != tc.want {
			t.Errorf("formatHostDownThreshold(%d) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}
