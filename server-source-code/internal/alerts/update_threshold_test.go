package alerts

import (
	"testing"
	"time"

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
