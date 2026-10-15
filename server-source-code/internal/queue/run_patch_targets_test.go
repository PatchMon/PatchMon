package queue

import (
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

func TestBuildRunPatchMessageLinuxUnchanged(t *testing.T) {
	msg := buildRunPatchMessage(RunPatchPayload{PatchRunID: "r", PatchType: "patch_package", PackageNames: []string{"curl"}}, &db.GetPatchRunByIDRow{}, false, nil)
	if names, ok := msg["package_names"].([]string); !ok || len(names) != 1 || names[0] != "curl" {
		t.Fatalf("Linux run_patch lost package_names: %+v", msg)
	}
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
	name := "7-Zip"
	msg := buildRunPatchMessage(RunPatchPayload{PatchType: "patch_package", PackageName: &name, PackageNames: []string{"2026-10 Cumulative Update (KB5044284)", "7-Zip"}}, &db.GetPatchRunByIDRow{}, true, pending)
	for _, k := range []string{"package_name", "package_names"} {
		if _, ok := msg[k]; ok {
			t.Errorf("Windows run_patch carries %s; only validated targets may reach the agent", k)
		}
	}
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

func TestSplitDispatchableItemsSeparatesInvalidIdentifiers(t *testing.T) {
	valid, invalid := splitDispatchableItems([]db.PatchRunItem{
		{ID: "1", Kind: "wua", Identifier: "7f1b0c5e-1111-2222-3333-444455556666"},
		{ID: "2", Kind: "wua", Identifier: "KB5044284"},
		{ID: "3", Kind: "winget", Identifier: "7zip.7zip"},
		{ID: "4", Kind: "winget", Identifier: "x';calc;'"},
		{ID: "5", Kind: "apt", Identifier: "curl"},
	})
	if len(valid) != 2 || valid[0].ID != "1" || valid[1].ID != "3" {
		t.Fatalf("valid = %+v", valid)
	}
	if strings.Join(invalid, ",") != "2,4,5" {
		t.Fatalf("invalid = %v", invalid)
	}
}

func TestNoTargetsMessage(t *testing.T) {
	for n, want := range map[int]string{
		0: noPatchTargetsMessage,
		1: "No valid patch targets: 1 item had an invalid identifier",
		3: "No valid patch targets: 3 items had an invalid identifier",
	} {
		if got := noTargetsMessage(n); got != want {
			t.Errorf("noTargetsMessage(%d) = %q, want %q", n, got, want)
		}
	}
}
