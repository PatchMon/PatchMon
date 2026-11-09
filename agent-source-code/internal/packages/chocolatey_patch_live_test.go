//go:build windows

package packages

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// Explicit opt-in: upgrades only the harmless local-feed fixtures documented in
// the README. Never runs on normal test hosts without that fixture setup.
func TestChocolateyPatchLive(t *testing.T) {
	if os.Getenv("PATCHMON_CHOCO_PATCH_INTEGRATION") != "1" {
		t.Skip("set PATCHMON_CHOCO_PATCH_INTEGRATION=1 with the local patching fixtures installed")
	}
	ctx := context.Background()
	backend := &chocolateyExecBackend{}
	before, err := backend.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{"patchmon-upgrade-fixture", "patchmon-pinned-fixture", "patchmon-failure-fixture", "patchmon-absent-fixture"}
	for _, id := range ids[:3] {
		p, ok := findChocolateyPackage(before, id)
		if !ok || p.CurrentVersion != "1.0.0" || p.AvailableVersion != "2.0.0" {
			t.Fatalf("fixture setup missing: %+v", p)
		}
	}
	pinned, _ := findChocolateyPackage(before, ids[1])
	if !pinned.ChocolateyPinned {
		t.Fatal("pin fixture first")
	}
	if _, ok := findChocolateyPackage(before, ids[3]); ok {
		t.Fatal("absent fixture must not be installed")
	}
	var results []WindowsPatchResult
	run := func(dry bool, targets []string) WindowsPatchResult {
		req := WindowsPatchRequest{DryRun: dry}
		for _, id := range targets {
			req.Targets = append(req.Targets, WindowsPatchTarget{Kind: WindowsTargetChocolatey, ID: id})
		}
		r := newWindowsPatchRun(req, WindowsPatchHooks{Output: func(s string) { t.Log(s) }}, nil, nil)
		r.chocolatey = backend
		res := r.run(ctx, ctx)
		results = append(results, res)
		return res
	}
	dry := run(true, ids)
	for i, want := range []string{itemWouldInstall, itemSkipped, itemWouldInstall, itemNotApplicable} {
		if dry.Items[i].Status != want {
			t.Fatalf("dry run: %+v", dry)
		}
	}
	afterDry, err := backend.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids[:3] {
		p, _ := findChocolateyPackage(afterDry, id)
		if p.CurrentVersion != "1.0.0" {
			t.Fatal("dry run changed packages")
		}
	}
	actual := run(false, ids)
	for i, want := range []string{itemInstalled, itemSkipped, itemFailed, itemNotApplicable} {
		if actual.Items[i].Status != want {
			t.Fatalf("real upgrade: %+v", actual)
		}
	}
	after, err := backend.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"2.0.0", "1.0.0"} {
		p, _ := findChocolateyPackage(after, ids[i])
		if p.CurrentVersion != want {
			t.Fatalf("unexpected final version: %+v", p)
		}
	}
	// Chocolatey may remove a failed package from its inventory rather than
	// restore its previous version. Never assert rollback the manager does not guarantee.
	if failed, ok := findChocolateyPackage(after, ids[2]); ok && failed.CurrentVersion == "2.0.0" {
		t.Fatal("failed fixture unexpectedly installed")
	}
	if _, ok := findChocolateyPackage(after, ids[3]); ok {
		t.Fatal("installed absent package")
	}
	repeat := run(false, ids[:1])
	if repeat.Items[0].Status != itemNotApplicable {
		t.Fatalf("repeat: %+v", repeat)
	}
	if target := os.Getenv("PATCHMON_CHOCO_PATCH_OUTPUT"); target != "" {
		b, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
