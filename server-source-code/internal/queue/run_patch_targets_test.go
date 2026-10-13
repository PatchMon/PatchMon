package queue

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

func TestBuildRunPatchMessageLinuxUnchanged(t *testing.T) {
	msg := buildRunPatchMessage(RunPatchPayload{PatchRunID: "r", PatchType: "patch_package", PackageNames: []string{"curl"}}, &db.GetPatchRunByIDRow{}, false, nil)
	for _, k := range []string{"targets", "include_updates", "include_apps"} {
		if _, ok := msg[k]; ok {
			t.Errorf("Linux run_patch carries %s", k)
		}
	}
	if msg["type"] != "run_patch" || msg["patch_type"] != "patch_package" {
		t.Fatalf("msg = %+v", msg)
	}
}

func TestBuildRunPatchMessageWindowsTargets(t *testing.T) {
	pending := []db.PatchRunItem{{Kind: "wua", Identifier: "7f1b0c5e-1111-2222-3333-444455556666"}, {Kind: "winget", Identifier: "7zip.7zip"}}
	msg := buildRunPatchMessage(RunPatchPayload{PatchType: "patch_package"}, &db.GetPatchRunByIDRow{}, true, pending)
	targets, ok := msg["targets"].([]runPatchTarget)
	if !ok || len(targets) != 2 || targets[0] != (runPatchTarget{Kind: "wua", ID: pending[0].Identifier}) || targets[1] != (runPatchTarget{Kind: "winget", ID: "7zip.7zip"}) {
		t.Fatalf("targets = %#v", msg["targets"])
	}
	if _, ok := msg["include_updates"]; ok {
		t.Error("patch_package carries include_updates")
	}
}

func TestBuildRunPatchMessageWindowsScope(t *testing.T) {
	f := false
	cases := []struct {
		name             string
		updates, apps    *bool
		wantUpd, wantApp bool
	}{
		{"null sent as true", nil, nil, true, true},
		{"apps off", nil, &f, true, false},
		{"updates off", &f, nil, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := buildRunPatchMessage(RunPatchPayload{PatchType: "patch_all"}, &db.GetPatchRunByIDRow{WindowsIncludeUpdates: tc.updates, WindowsIncludeApps: tc.apps}, true, nil)
			if msg["include_updates"] != tc.wantUpd || msg["include_apps"] != tc.wantApp {
				t.Fatalf("msg = %+v", msg)
			}
			if _, ok := msg["targets"]; ok {
				t.Error("patch_all carries targets")
			}
		})
	}
}
