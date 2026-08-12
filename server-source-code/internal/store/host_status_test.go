package store

import (
	"testing"
	"time"
)

func TestStaleCutoff(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		interval int
		want     time.Time
	}{
		{"default interval", 60, now.Add(-120 * time.Minute)},
		{"short interval", 5, now.Add(-10 * time.Minute)},
		{"zero falls back to default", 0, now.Add(-120 * time.Minute)},
		{"negative falls back to default", -30, now.Add(-120 * time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StaleCutoff(now, tt.interval); !got.Equal(tt.want) {
				t.Errorf("StaleCutoff(%d) = %v, want %v", tt.interval, got, tt.want)
			}
		})
	}
}

// The boundaries here must stay identical to the effective_status CASE in
// GetHostsWithCounts, otherwise the scoped API and the UI disagree again.
func TestEffectiveStatus(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	staleCutoff := StaleCutoff(now, 60)

	tests := []struct {
		name       string
		status     string
		lastUpdate time.Time
		want       string
	}{
		{"active and reporting", "active", now.Add(-5 * time.Minute), "active"},
		{"active and overdue but not stale", "active", now.Add(-90 * time.Minute), "active"},
		{"active exactly on the cutoff", "active", staleCutoff, "active"},
		{"active past the cutoff", "active", now.Add(-3 * time.Hour), "inactive"},
		{"pending never degrades", "pending", now.Add(-30 * time.Hour), "pending"},
		{"unknown status passes through", "error", now.Add(-30 * time.Hour), "error"},
		{"zero last update on an active host", "active", time.Time{}, "inactive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveStatus(tt.status, tt.lastUpdate, staleCutoff)
			if got != tt.want {
				t.Errorf("EffectiveStatus(%q, %v) = %q, want %q", tt.status, tt.lastUpdate, got, tt.want)
			}
		})
	}
}
