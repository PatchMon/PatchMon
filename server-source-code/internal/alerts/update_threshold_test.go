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
		silenced bool
		entered  bool
		remind   bool
		want     thresholdDecision
	}{
		{"newly exceeded, no alert row", 5, false, false, true, false, thresholdDecision{notify: true}},
		{"still exceeded, already notified", 5, false, false, false, false, thresholdDecision{}},
		{"still exceeded, reminder due", 5, false, false, false, true, thresholdDecision{notify: true, reminder: true}},
		{"newly exceeded with an active alert row", 5, true, false, true, false, thresholdDecision{touch: true}},
		{"still exceeded with an active alert row", 5, true, false, false, false, thresholdDecision{touch: true}},
		{"reminder due with an active alert row", 5, true, false, false, true, thresholdDecision{touch: true, notify: true, reminder: true}},
		{"reminder due but the alert is silenced", 5, true, true, false, true, thresholdDecision{touch: true}},
		{"silenced flag without an active row does not hold reminders", 5, false, true, false, true, thresholdDecision{notify: true, reminder: true}},
		{"new exceedance with no row is never held", 5, false, true, true, false, thresholdDecision{notify: true}},
		{"silenced alert still resolves when cleared", 2, true, true, false, false, thresholdDecision{resolve: true}},
		{"at threshold with an active alert row", 2, true, false, false, false, thresholdDecision{resolve: true}},
		{"below threshold, nothing to resolve", 1, false, false, false, false, thresholdDecision{}},
		{"reminder flag ignored once cleared", 1, false, false, false, true, thresholdDecision{}},
	}
	for _, tc := range cases {
		if got := decideThreshold(tc.count, 2, tc.hasAlert, tc.silenced, tc.entered, tc.remind); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestSilencedHostIDs(t *testing.T) {
	t.Parallel()
	got := silencedHostIDs(map[string]activeThresholdAlert{
		"h1": {id: "a1", silenced: true},
		"h2": {id: "a2"},
	})
	if len(got) != 1 || got[0] != "h1" {
		t.Fatalf("silenced = %v, want [h1]", got)
	}
}

func TestReminderInterval(t *testing.T) {
	t.Parallel()
	h := func(v int) *store.AlertConfigWithUser {
		cfg := &store.AlertConfigWithUser{}
		cfg.ReminderIntervalHours = &v
		return cfg
	}
	cases := []struct {
		name string
		cfg  *store.AlertConfigWithUser
		want time.Duration
	}{
		{"nil config", nil, 0},
		{"unset means no reminders", &store.AlertConfigWithUser{}, 0},
		{"zero means no reminders", h(0), 0},
		{"negative treated as none", h(-3), 0},
		{"daily", h(24), 24 * time.Hour},
		{"huge values clamped to a year", h(1 << 30), 24 * 365 * time.Hour},
	}
	for _, tc := range cases {
		if got := reminderInterval(tc.cfg); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
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
