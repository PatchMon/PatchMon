package commands

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"

	"patchmon-agent/internal/packages"
)

type fakeAPKResponse struct {
	out string
	err error
}

type fakeAPK struct {
	responses   map[string][]fakeAPKResponse
	calls       []string
	notes       []string
	inv         packages.APKInventory
	invAfterPre *packages.APKInventory
}

func (f *fakeAPK) io() apkPatchIO {
	return apkPatchIO{
		run: func(args ...string) (string, error) {
			key := strings.Join(args, " ")
			f.calls = append(f.calls, key)
			if queue := f.responses[key]; len(queue) > 0 {
				r := queue[0]
				if len(queue) > 1 {
					f.responses[key] = queue[1:]
				}
				return r.out, r.err
			}
			return "OK: 8 MiB in 16 packages\n", nil
		},
		note: func(msg string) { f.notes = append(f.notes, msg) },
		inventory: func() (packages.APKInventory, error) {
			if f.invAfterPre != nil && slices.ContainsFunc(f.calls, func(c string) bool { return strings.HasSuffix(c, "-only") }) {
				return *f.invAfterPre, nil
			}
			return f.inv, nil
		},
	}
}

func (f *fakeAPK) ran(prefix string) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

const (
	apkSimulateAll      = "upgrade --no-progress --simulate"
	apkUpgradeAll       = "upgrade --no-progress --wait 30"
	apkHeldBusyboxPlan  = "( 1/2) Upgrading libcrypto3 (3.5.4-r0 -> 3.5.9-r0)\n( 2/2) Upgrading libssl3 (3.5.4-r0 -> 3.5.9-r0)\nOK: 8 MiB in 16 packages\n"
	apkUnreliableV3Plan = "WARNING: This simulation might not reliable as a preupgrade is available.\n( 1/3) Upgrading libapk (3.0.1-r1 -> 3.0.8-r0)\n( 2/3) Upgrading apk-tools (3.0.1-r1 -> 3.0.8-r0)\n( 3/3) Upgrading zlib (1.3.1-r2 -> 1.3.2-r1)\n"
)

func apkTestInventory(world string) packages.APKInventory {
	return packages.APKInventory{
		Major:     3,
		Installed: map[string]string{"busybox": "1.37.0-r29", "busybox-binsh": "1.37.0-r29", "libssl3": "3.5.4-r0", "libcrypto3": "3.5.4-r0"},
		Available: map[string]string{"busybox": "1.37.0-r30", "busybox-binsh": "1.37.0-r30", "libssl3": "3.5.9-r0", "libcrypto3": "3.5.9-r0"},
		Origin:    map[string]string{"busybox": "busybox", "busybox-binsh": "busybox", "libssl3": "openssl", "libcrypto3": "openssl"},
		World:     packages.ParseAPKWorld(world),
	}
}

func TestRunAPKPatchSkipsWorldHolds(t *testing.T) {
	f := &fakeAPK{
		inv:       apkTestInventory("busybox=1.37.0-r29"),
		responses: map[string][]fakeAPKResponse{apkSimulateAll: {{out: apkHeldBusyboxPlan}}},
	}
	if err := runAPKPatch(f.io(), "patch_all", nil, false); err != nil {
		t.Fatal(err)
	}
	wantNotes := []string{
		"Held by /etc/apk/world: busybox=1.37.0-r29 (1.37.0-r30 available)",
		"Not upgraded by apk: busybox-binsh 1.37.0-r29 (1.37.0-r30 available), held back with busybox by /etc/apk/world",
	}
	if !slices.Equal(f.notes, wantNotes) {
		t.Errorf("notes = %q, want %q", f.notes, wantNotes)
	}
	if want := []string{"update --wait 30", apkSimulateAll, apkUpgradeAll}; !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
}

func TestRunAPKPatchWorldConflictFailsBeforeChanges(t *testing.T) {
	f := &fakeAPK{inv: apkTestInventory("busybox<1.0")}
	err := runAPKPatch(f.io(), "patch_all", nil, false)
	if err == nil || !strings.Contains(err.Error(), "/etc/apk/world requires busybox<1.0 but busybox 1.37.0-r29 is installed") {
		t.Fatalf("err = %v", err)
	}
	if f.ran("upgrade") != 0 {
		t.Errorf("nothing may be planned or applied after a world conflict: %q", f.calls)
	}
}

func TestRunAPKPatchConflictFromSimulation(t *testing.T) {
	f := &fakeAPK{
		inv: apkTestInventory(""),
		responses: map[string][]fakeAPKResponse{apkSimulateAll: {{
			out: "ERROR: unable to select packages:\n  busybox-1.37.0-r29:\n    breaks: world[busybox<1.0]\n",
			err: errors.New("exit status 3"),
		}}},
	}
	err := runAPKPatch(f.io(), "patch_all", nil, false)
	if err == nil || !strings.Contains(err.Error(), "Correct that line in /etc/apk/world") {
		t.Fatalf("err = %v", err)
	}
	if f.ran(apkUpgradeAll) != 0 {
		t.Error("a failed simulation must not be followed by an upgrade")
	}
}

func TestRunAPKPatchRefusesDowngrades(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		f := &fakeAPK{
			inv: apkTestInventory(""),
			responses: map[string][]fakeAPKResponse{apkSimulateAll: {{
				out: "(1/2) Upgrading zlib (1.3.2-r0 -> 1.3.2-r1)\n(2/2) Downgrading scanelf (1.3.11-r0 -> 1.3.8-r2)\n",
			}}},
		}
		err := runAPKPatch(f.io(), "patch_all", nil, dryRun)
		if err == nil || !strings.Contains(err.Error(), "Downgrading scanelf (1.3.11-r0 -> 1.3.8-r2)") {
			t.Fatalf("dryRun=%v err = %v", dryRun, err)
		}
		if f.ran(apkUpgradeAll) != 0 {
			t.Errorf("dryRun=%v: a downgrade plan must not be applied: %q", dryRun, f.calls)
		}
	}
}

func TestRunAPKPatchPreupgradesOnRealRunsOnly(t *testing.T) {
	after := apkTestInventory("")
	delete(after.Available, "busybox")
	f := &fakeAPK{
		inv:         apkTestInventory(""),
		invAfterPre: &after,
		responses: map[string][]fakeAPKResponse{apkSimulateAll: {
			{out: apkUnreliableV3Plan},
			{out: "( 1/1) Upgrading zlib (1.3.1-r2 -> 1.3.2-r1)\n"},
		}},
	}
	if err := runAPKPatch(f.io(), "patch_all", nil, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"update --wait 30", apkSimulateAll, "upgrade --no-progress --wait 30 --preupgrade-only", apkSimulateAll, apkUpgradeAll}
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}

	dry := &fakeAPK{
		inv:       apkTestInventory(""),
		responses: map[string][]fakeAPKResponse{apkSimulateAll: {{out: apkUnreliableV3Plan}}},
	}
	if err := runAPKPatch(dry.io(), "patch_all", nil, true); err != nil {
		t.Fatal(err)
	}
	if dry.ran("upgrade --no-progress --wait") != 0 {
		t.Errorf("a dry run must not change anything, not even apk-tools: %q", dry.calls)
	}
	if len(dry.notes) == 0 || !strings.Contains(dry.notes[0], "apk-tools has an update of its own") {
		t.Errorf("dry run notes = %q", dry.notes)
	}
}

func TestRunAPKPatchRefusesUnreliablePlanWithUnknownApk(t *testing.T) {
	inv := apkTestInventory("")
	inv.Major = 0
	f := &fakeAPK{inv: inv, responses: map[string][]fakeAPKResponse{apkSimulateAll: {{out: apkUnreliableV3Plan}}}}
	err := runAPKPatch(f.io(), "patch_all", nil, false)
	if err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("err = %v", err)
	}
	if f.ran("upgrade --no-progress --wait") != 0 {
		t.Errorf("nothing may be applied: %q", f.calls)
	}
}

func TestRunAPKPatchRejectsUnparseableSimulation(t *testing.T) {
	for _, out := range []string{"", "WARNING: something new\n", "( 1/2) Frobnicating busybox\n"} {
		f := &fakeAPK{inv: apkTestInventory(""), responses: map[string][]fakeAPKResponse{apkSimulateAll: {{out: out}}}}
		err := runAPKPatch(f.io(), "patch_all", nil, false)
		want := "printed no plan"
		if strings.Contains(out, "Frobnicating") {
			want = "Frobnicating busybox"
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v", out, err)
		}
		if f.ran(apkUpgradeAll) != 0 {
			t.Errorf("%q: nothing may be applied: %q", out, f.calls)
		}
	}
}

func TestRunAPKPatchRefusesSameVersionReplace(t *testing.T) {
	f := &fakeAPK{inv: apkTestInventory(""), responses: map[string][]fakeAPKResponse{apkSimulateAll: {{out: "(1/1) Replacing busybox (1.37.0-r30 -> 1.37.0-r30)\nOK: 8 MiB in 16 packages\n"}}}}
	if err := runAPKPatch(f.io(), "patch_all", nil, false); err == nil || !strings.Contains(err.Error(), "Replacing busybox") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunAPKPatchDropsInvalidExpandedNames(t *testing.T) {
	inv := apkTestInventory("")
	inv.Available["libssl3;rm"] = "3.5.9-r0"
	inv.Origin["libssl3;rm"] = "openssl"
	f := &fakeAPK{inv: inv}
	if err := runAPKPatch(f.io(), "patch_package", []string{"libssl3"}, true); err != nil {
		t.Fatal(err)
	}
	if f.calls[1] != "upgrade --no-progress --simulate libssl3 libcrypto3" {
		t.Errorf("calls = %q", f.calls)
	}
}

func TestRunAPKPatchV2UsesSelfUpgradeOnly(t *testing.T) {
	inv := apkTestInventory("")
	inv.Major = 2
	f := &fakeAPK{
		inv: inv,
		responses: map[string][]fakeAPKResponse{apkSimulateAll: {
			{out: "WARNING: This simulation is not reliable as apk-tools upgrade is available.\n(1/1) Upgrading apk-tools (2.14.6-r2 -> 2.14.6-r3)\n"},
		}},
	}
	if err := runAPKPatch(f.io(), "patch_all", nil, false); err != nil {
		t.Fatal(err)
	}
	if f.ran("upgrade --no-progress --wait 30 --self-upgrade-only") != 1 {
		t.Errorf("calls = %q", f.calls)
	}
}

func TestRunAPKPatchSelectedExpandsOrigin(t *testing.T) {
	f := &fakeAPK{inv: apkTestInventory("")}
	if err := runAPKPatch(f.io(), "patch_package", []string{"libssl3"}, true); err != nil {
		t.Fatal(err)
	}
	if want := []string{"update --wait 30", "upgrade --no-progress --simulate libssl3 libcrypto3"}; !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
	if len(f.notes) != 1 || !strings.Contains(f.notes[0], "libssl3 3.5.4-r0 (3.5.9-r0 available). Its new version needs other packages") {
		t.Errorf("notes = %q", f.notes)
	}
}

func TestRunAPKPatchLockTimeout(t *testing.T) {
	f := &fakeAPK{
		inv: apkTestInventory(""),
		responses: map[string][]fakeAPKResponse{"update --wait 30": {{
			out: "Waiting for repository lock\nERROR: Unable to lock database: Interrupted system call\nERROR: Failed to open apk database: Interrupted system call\n",
			err: errors.New("exit status 99"),
		}}},
	}
	err := runAPKPatch(f.io(), "patch_all", nil, false)
	if err == nil || !strings.Contains(err.Error(), "another apk process held the package database for more than 30 seconds") {
		t.Fatalf("err = %v", err)
	}
}

func TestAPKStepError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	exit99 := exec.Command("sh", "-c", "exit 99").Run()
	cases := []struct {
		name, output string
		err          error
		want         string
	}{
		{"stale indexes", "ERROR: Not continuing due to stale/unavailable repositories.Use --force-missing-repositories to continue.", exit99, "a repository in /etc/apk/repositories could not be reached"},
		{"update partially failed", "2 unavailable, 0 stale; 16 distinct packages available", errors.New("exit status 2"), "could not be reached"},
		{"bare 99", "", exit99, "apk could not open its database"},
		{"unknown package", "ERROR: Package 'nosuchpkg' not found", errors.New("exit status 1"), "apk upgrade failed: exit status 1"},
	}
	for _, tc := range cases {
		if got := apkStepError("apk upgrade", tc.output, tc.err).Error(); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q lacks %q", tc.name, got, tc.want)
		}
	}
}

func TestRunPatchAPKRequiresPackageNames(t *testing.T) {
	err := runPatchAPK(context.Background(), nil, nil, "patch_package", nil, false)
	if err == nil || !strings.Contains(err.Error(), "package_names required") {
		t.Fatalf("err = %v", err)
	}
}
