package handler

import (
	"testing"
	"time"
)

func TestInstallStatusAwaitingAgent(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		dispatched time.Time
		want       string
		wantErr    bool
	}{
		{name: "just dispatched", dispatched: now.Add(-5 * time.Second), want: "active"},
		{name: "apt still running", dispatched: now.Add(-10 * time.Minute), want: "active"},
		{name: "agent never reported", dispatched: now.Add(-installAgentReportWindow), want: "failed", wantErr: true},
		{name: "dispatch time unknown", want: "failed", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errMsg := installStatusAwaitingAgent(tt.dispatched, now)
			if got != tt.want || (errMsg != "") != tt.wantErr {
				t.Fatalf("got (%q, %q), want %q with error=%v", got, errMsg, tt.want, tt.wantErr)
			}
		})
	}
}
