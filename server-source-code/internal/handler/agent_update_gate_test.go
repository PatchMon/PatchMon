package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestAgentAutoUpdateGate(t *testing.T) {
	cases := []struct {
		name                   string
		server, host, patching bool
		wantDisabled           bool
		wantReason             string
	}{
		{"enabled and idle", true, true, false, false, ""},
		{"enabled but patching", true, true, true, true, store.PatchRunInProgressMessage},
		{"server off wins over patching", false, true, true, true, "Auto-update is disabled in server settings"},
		{"host off", true, false, false, true, "Auto-update is disabled for this host"},
		{"both off", false, false, false, true, "Auto-update is disabled in server settings and for this host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			disabled, reason := agentAutoUpdateGate(tc.server, tc.host, tc.patching)
			if disabled != tc.wantDisabled || reason != tc.wantReason {
				t.Fatalf("got (%v, %q), want (%v, %q)", disabled, reason, tc.wantDisabled, tc.wantReason)
			}
		})
	}
}
