package alerts

import (
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestThresholdInterval(t *testing.T) {
	t.Parallel()
	n := func(v int) *store.AlertConfigWithUser {
		cfg := &store.AlertConfigWithUser{}
		cfg.CheckIntervalMinutes = &v
		return cfg
	}
	cases := []struct {
		name string
		cfg  *store.AlertConfigWithUser
		want time.Duration
	}{
		{"nil config", nil, 30 * time.Minute},
		{"unset", &store.AlertConfigWithUser{}, 30 * time.Minute},
		{"zero", n(0), 30 * time.Minute},
		{"ninety minutes is not floored to an hour", n(90), 90 * time.Minute},
		{"forty-five", n(45), 45 * time.Minute},
		{"two days", n(2880), 48 * time.Hour},
	}
	for _, tc := range cases {
		if got := thresholdInterval(tc.cfg); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestDecideThreshold(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		count    int
		hasAlert bool
		entered  bool
		want     thresholdAction
	}{
		{"newly exceeded, no alert row", 5, false, true, thresholdNotify},
		{"still exceeded, already notified", 5, false, false, thresholdNone},
		{"newly exceeded with an active alert row", 5, true, true, thresholdTouch},
		{"still exceeded with an active alert row", 5, true, false, thresholdTouch},
		{"at threshold with an active alert row", 2, true, false, thresholdResolve},
		{"below threshold, nothing to resolve", 1, false, false, thresholdNone},
	}
	for _, tc := range cases {
		if got := decideThreshold(tc.count, 2, tc.hasAlert, tc.entered); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestExceededHostIDs(t *testing.T) {
	t.Parallel()
	hosts := []db.Host{
		{ID: "over", Status: "active"},
		{ID: "at", Status: "active"},
		{ID: "inactive", Status: "pending"},
		{ID: "no-packages", Status: "active"},
	}
	counts := map[string]db.GetPendingUpdateCountsPerHostRow{
		"over":     {HostID: "over", SecurityCount: 3},
		"at":       {HostID: "at", SecurityCount: 1},
		"inactive": {HostID: "inactive", SecurityCount: 9},
	}
	got := exceededHostIDs(hosts, counts, thresholdChecks[0].count, 1)
	if len(got) != 1 || got[0] != "over" {
		t.Fatalf("exceeded = %v, want [over]", got)
	}
}
