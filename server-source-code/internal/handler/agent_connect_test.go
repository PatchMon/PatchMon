package handler

import (
	"fmt"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// The boundary here must stay identical to store.OverdueCutoff, which also
// drives the reporting pill, so the UI and the reconnect catch-up agree on
// when a host has missed a cycle.
func TestIsOverdue(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	cutoff := store.OverdueCutoff(now, 360)
	ts := func(d time.Duration) pgtype.Timestamp {
		return pgtype.Timestamp{Time: now.Add(d), Valid: true}
	}

	tests := []struct {
		name       string
		status     string
		lastUpdate pgtype.Timestamp
		want       bool
	}{
		{"reported moments ago", store.StatusActive, ts(-time.Minute), false},
		{"inside the interval", store.StatusActive, ts(-5 * time.Hour), false},
		{"exactly on the cutoff", store.StatusActive, pgtype.Timestamp{Time: cutoff, Valid: true}, false},
		{"one interval and a minute", store.StatusActive, ts(-361 * time.Minute), true},
		{"suspended for days", store.StatusActive, ts(-72 * time.Hour), true},
		{"enrolled but never reported", "pending", ts(-72 * time.Hour), false},
		{"decommissioned host", "inactive", ts(-72 * time.Hour), false},
		{"invalid timestamp", store.StatusActive, pgtype.Timestamp{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOverdue(tt.status, tt.lastUpdate, cutoff); got != tt.want {
				t.Errorf("isOverdue(%q, %v) = %v, want %v", tt.status, tt.lastUpdate.Time, got, tt.want)
			}
		})
	}
}

func TestCatchUpReportDelayIsBoundedAndStable(t *testing.T) {
	for _, apiID := range []string{"", "patchmon_0000000000000000", "patchmon_9f3c1b7e2a4d6c81"} {
		got := catchUpReportDelay(apiID)
		if got < 0 || got >= catchUpJitterWindow {
			t.Errorf("catchUpReportDelay(%q) = %v, want within [0, %v)", apiID, got, catchUpJitterWindow)
		}
		if again := catchUpReportDelay(apiID); again != got {
			t.Errorf("catchUpReportDelay(%q) not deterministic: %v then %v", apiID, got, again)
		}
	}
}

func TestCatchUpReportDelaySpreadsAcrossHosts(t *testing.T) {
	const hosts = 600
	slots := int(catchUpJitterWindow / time.Second)

	seen := make(map[time.Duration]int, slots)
	for i := range hosts {
		seen[catchUpReportDelay(fmt.Sprintf("patchmon_%016x", i))]++
	}

	if len(seen) != slots {
		t.Errorf("%d hosts filled %d of %d slots, want all of them", hosts, len(seen), slots)
	}
	for delay, n := range seen {
		if n > hosts/slots*4 {
			t.Errorf("slot %v holds %d of %d hosts, want a flatter spread", delay, n, hosts)
		}
	}
}
