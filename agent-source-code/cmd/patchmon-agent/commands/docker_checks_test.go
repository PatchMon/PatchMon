package commands

import (
	"testing"
	"time"
)

func TestServerCheckTooSoon(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		last time.Time
		want bool
	}{
		{"never run", time.Time{}, false},
		{"just finished", now.Add(-time.Minute), true},
		{"inside cooldown", now.Add(-9*time.Minute - 59*time.Second), true},
		{"cooldown elapsed", now.Add(-10 * time.Minute), false},
		{"long ago", now.Add(-6 * time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := serverCheckTooSoon(tt.last, now); got != tt.want {
				t.Errorf("serverCheckTooSoon = %v, want %v", got, tt.want)
			}
		})
	}
}
