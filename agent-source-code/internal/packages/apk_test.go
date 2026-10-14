package packages

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"patchmon-agent/internal/repositories"

	"github.com/sirupsen/logrus"
)

func quietLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

// Captured from alpine:3.21.0 (apk-tools 2.14.6); 3.23.0 (apk-tools 3.0.1) prints the same format.
const apkInstalledV2 = `.tmp-test-20261014.131424-20261014.134913 noarch {.tmp-test-20261014.131424} () [installed]
busybox-1.37.0-r8 aarch64 {busybox} (GPL-2.0-only) [installed]
busybox-binsh-1.37.0-r8 aarch64 {busybox} (GPL-2.0-only) [installed]
ca-certificates-bundle-20241010-r0 aarch64 {ca-certificates} (MPL-2.0 AND MIT) [installed]
font-adobe-100dpi-1.0.4-r2 aarch64 {font-adobe-100dpi} (MIT) [installed]
libstdc++-14.2.0-r4 aarch64 {gcc} (GPL-2.0-or-later AND LGPL-2.1-or-later) [installed]
lua5.4-5.4.7-r0 aarch64 {lua5.4} (MIT) [installed]
py3-pip-24.3.1-r0 aarch64 {py3-pip} (MIT) [installed]
ssl_client-1.37.0-r8 aarch64 {busybox} (GPL-2.0-only) [installed]
`

const apkUpgradableV3 = `alpine-baselayout-3.7.2-r0 aarch64 {alpine-baselayout} (GPL-2.0-only) [upgradable from: alpine-baselayout-3.7.1-r8]
alpine-release-3.23.6-r0 aarch64 {alpine-base} (MIT) [upgradable from: alpine-release-3.23.0-r0]
busybox-1.37.0-r30 aarch64 {busybox} (GPL-2.0-only) [upgradable from: busybox-1.37.0-r29]
busybox-binsh-1.37.0-r30 aarch64 {busybox} (GPL-2.0-only) [upgradable from: busybox-binsh-1.37.0-r29]
libcrypto3-3.5.9-r0 aarch64 {openssl} (Apache-2.0) [upgradable from: libcrypto3-3.5.4-r0]
libssl3-3.5.9-r0 aarch64 {openssl} (Apache-2.0) [upgradable from: libssl3-3.5.4-r0]
perl-dbd-sqlite-1.76-r0 aarch64 {perl-dbd-sqlite} (Artistic-1.0-Perl OR GPL-1.0-or-later) [upgradable from: perl-dbd-sqlite-1.74-r0]
mesa-gl-25.1.0-r1 aarch64 {mesa} (MIT AND SGI-B-2.0 AND (MIT OR BSD-3-Clause)) [upgradable from: mesa-gl-25.0.7-r0]
`

func TestParseAPKListLines(t *testing.T) {
	installed := parseAPKListLines(apkInstalledV2)
	want := map[string][2]string{
		".tmp-test-20261014.131424": {"20261014.134913", ".tmp-test-20261014.131424"},
		"busybox":                   {"1.37.0-r8", "busybox"},
		"ca-certificates-bundle":    {"20241010-r0", "ca-certificates"},
		"font-adobe-100dpi":         {"1.0.4-r2", "font-adobe-100dpi"},
		"libstdc++":                 {"14.2.0-r4", "gcc"},
		"lua5.4":                    {"5.4.7-r0", "lua5.4"},
		"py3-pip":                   {"24.3.1-r0", "py3-pip"},
		"ssl_client":                {"1.37.0-r8", "busybox"},
	}
	got := map[string][2]string{}
	for _, e := range installed {
		if e.From != "" {
			t.Errorf("%s: installed line parsed as an upgrade", e.Name)
		}
		got[e.Name] = [2]string{e.Version, e.Origin}
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %v, want %v", name, got[name], w)
		}
	}

	upgradable := parseAPKListLines(apkUpgradableV3)
	if len(upgradable) != 8 {
		t.Fatalf("got %d upgradable entries, want 8: %+v", len(upgradable), upgradable)
	}
	mesa := upgradable[7]
	if mesa.Name != "mesa-gl" || mesa.Version != "25.1.0-r1" || mesa.From != "25.0.7-r0" || mesa.Origin != "mesa" {
		t.Errorf("licence with nested parentheses misparsed: %+v", mesa)
	}
	ssl := upgradable[5]
	if ssl.Name != "libssl3" || ssl.Origin != "openssl" || ssl.From != "3.5.4-r0" {
		t.Errorf("libssl3 misparsed: %+v", ssl)
	}
}

func TestParseAPKListLinesSkipsNoise(t *testing.T) {
	out := "WARNING: opening from cache https://dl-cdn.alpinelinux.org/alpine/v3.21/main: No such file or directory\n" +
		"busybox-1.37.0-r30 aarch64 {busybox} (GPL-2.0-only)\n\n"
	if got := parseAPKListLines(out); len(got) != 0 {
		t.Errorf("expected nothing, got %+v", got)
	}
}

func TestSplitAPKNameVersion(t *testing.T) {
	cases := []struct{ in, name, version string }{
		{"busybox-1.37.0-r30", "busybox", "1.37.0-r30"},
		{"font-adobe-100dpi-1.0.4-r2", "font-adobe-100dpi", "1.0.4-r2"},
		{"py3-pip-24.3.1-r0", "py3-pip", "24.3.1-r0"},
		{"lua5.4-libs-5.4.7-r0", "lua5.4-libs", "5.4.7-r0"},
		{"libstdc++-14.2.0-r4", "libstdc++", "14.2.0-r4"},
		{"ca-certificates-bundle-20260909-r0", "ca-certificates-bundle", "20260909-r0"},
		{"alpine-release-3.25.0_alpha20260805-r0", "alpine-release", "3.25.0_alpha20260805-r0"},
		{".tmp-test-20261014.131424-20261014.134913", ".tmp-test-20261014.131424", "20261014.134913"},
		{"busybox", "", ""},
		{"-1.0-r0", "", ""},
		{"foo-", "", ""},
		{"-", "", ""},
		{"foo-r1", "", ""},
		{"-r1", "", ""},
	}
	for _, tc := range cases {
		name, version := splitAPKNameVersion(tc.in)
		if name != tc.name || version != tc.version {
			t.Errorf("splitAPKNameVersion(%q) = %q, %q; want %q, %q", tc.in, name, version, tc.name, tc.version)
		}
	}
}

func TestAPKParsersSurviveTrailingDash(t *testing.T) {
	for _, out := range []string{
		"foo- aarch64 {foo} (MIT) [installed]\n",
		"foo-1.0-r1 aarch64 {foo} (MIT) [upgradable from: foo-]\n",
		"- aarch64 {x} () [upgradable from: -]\n",
	} {
		if got := parseAPKListLines(out); len(got) != 0 {
			t.Errorf("%q parsed as %+v", out, got)
		}
	}
	if _, ok := APKConflictFromError("ERROR: unable to select packages:\n  foo-:\n    breaks: world[foo<1.0]\n"); ok {
		t.Error("a nameless conflict line must not parse")
	}
}

func TestParseApkPolicy(t *testing.T) {
	// v2 and v3 print the repository base URL; older apk printed the index URL with the arch.
	out := `busybox policy:
  1.37.0-r29:
    lib/apk/db/installed
  1.37.0-r30:
    https://dl-cdn.alpinelinux.org/alpine/v3.23/main
lua5.4 policy:
  5.4.8-r0:
    lib/apk/db/installed
    https://dl-cdn.alpinelinux.org/alpine/v3.23/community
curl policy:
  8.12.1-r0:
    lib/apk/db/installed
    https://dl-cdn.alpinelinux.org/alpine/v3.22/main/x86_64/APKINDEX.tar.gz
nginx policy:
  1.29.0-r0:
    @nginx https://nginx.org/packages/mainline/alpine/v3.23/main
custom policy:
  1.0-r0:
    https://packages.example.com/apk/extras
local policy:
  1.0-r0:
    lib/apk/db/installed
`
	m := &APKManager{logger: quietLogger()}
	got := map[string]string{}
	m.parseApkPolicy(out, got)
	want := map[string]string{"busybox": "main", "lua5.4": "community", "curl": "main", "nginx": "main", "custom": "extras"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseApkPolicy = %v, want %v", got, want)
	}
}

func TestParseAPKWorld(t *testing.T) {
	world := `alpine-baselayout
busybox=1.37.0-r29
!nano
openssl<3.6
musl>=1.2.5-r0
zlib~1.3
curl@edge>8.0
.tmp-test=20261014.134913
pkg><Q1abc=
`
	got := ParseAPKWorld(world)
	var raw []string
	for _, c := range got {
		raw = append(raw, c.Name+" "+c.Op+" "+c.Version)
	}
	want := []string{"busybox = 1.37.0-r29", "openssl < 3.6", "musl >= 1.2.5-r0", "zlib ~ 1.3", "curl > 8.0", ".tmp-test = 20261014.134913"}
	if !reflect.DeepEqual(raw, want) {
		t.Errorf("ParseAPKWorld = %v, want %v", raw, want)
	}
}

func TestAPKWorldConstraintAllows(t *testing.T) {
	cases := []struct {
		entry, version string
		want           bool
	}{
		{"busybox=1.37.0-r29", "1.37.0-r29", true},
		{"busybox=1.37.0-r29", "1.37.0-r30", false},
		{"busybox<1.37.0-r10", "1.37.0-r8", true},
		{"busybox<1.37.0-r10", "1.37.0-r14", false},
		{"openssl<=3.5.9-r0", "3.5.9-r0", true},
		{"musl>1.2.5-r10", "1.2.5-r11", true},
		{"musl>=1.2.5-r11", "1.2.5-r10", false},
		{"zlib~1.3", "1.3.2-r1", true},
		{"zlib~1.3", "1.30-r0", false},
		{"zlib~1.3", "1.4-r0", false},
		{"zlib>~1.3", "1.4-r0", true},
		{"zlib<~1.3", "1.3.2-r1", true},
	}
	for _, tc := range cases {
		c := ParseAPKWorld(tc.entry)
		if len(c) != 1 {
			t.Fatalf("%s did not parse", tc.entry)
		}
		if got := c[0].Allows(tc.version, CompareAPKVersions); got != tc.want {
			t.Errorf("%s allows %s = %v, want %v", tc.entry, tc.version, got, tc.want)
		}
	}
}

func TestAPKWorldHolds(t *testing.T) {
	world := ParseAPKWorld("busybox=1.37.0-r29\nzlib<1.0\nmusl>=1.2.5-r0\nlibssl3<3.5.9-r0\n")
	installed := map[string]string{"busybox": "1.37.0-r29", "zlib": "1.3.1-r2", "musl": "1.2.5-r21", "libssl3": "3.5.4-r0"}
	available := map[string]string{"busybox": "1.37.0-r30", "musl": "1.2.5-r23", "libssl3": "3.5.9-r0"}
	holds, conflicts := apkWorldHolds(world, installed, available, CompareAPKVersions)

	if len(holds) != 2 || holds[0].Name != "busybox" || holds[1].Name != "libssl3" {
		t.Fatalf("holds = %+v", holds)
	}
	if got := holds[0].String(); got != "busybox=1.37.0-r29 (1.37.0-r30 available)" {
		t.Errorf("hold message = %q", got)
	}
	if len(conflicts) != 1 || conflicts[0].Name != "zlib" || conflicts[0].Installed != "1.3.1-r2" {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	msg := APKConflictMessage(conflicts[0])
	for _, want := range []string{"zlib<1.0", "zlib 1.3.1-r2 is installed", "zlib=1.3.1-r2", "Nothing was changed"} {
		if !strings.Contains(msg, want) {
			t.Errorf("conflict message lacks %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "apk fix") {
		t.Errorf("conflict message must not suggest apk fix: %s", msg)
	}
}

// Captured from alpine:3.21.0 and alpine:3.23.0.
const apkSimulateV2 = `WARNING: This simulation is not reliable as apk-tools upgrade is available.
(1/10) Upgrading alpine-baselayout-data (3.6.8-r0 -> 3.6.8-r1)
(2/10) Upgrading musl (1.2.5-r8 -> 1.2.5-r11)
(9/10) Upgrading apk-tools (2.14.6-r2 -> 2.14.6-r3)
(10/10) Upgrading musl-utils (1.2.5-r8 -> 1.2.5-r11)
OK: 7 MiB in 15 packages
`

const apkSimulateV3 = `WARNING: This simulation might not reliable as a preupgrade is available.
( 1/11) Upgrading alpine-baselayout-data (3.7.1-r8 -> 3.7.2-r0)
( 9/11) Upgrading ca-certificates-bundle (20251003-r0 -> 20260909-r0)
(10/11) Upgrading apk-tools (3.0.1-r1 -> 3.0.8-r0)
(11/11) Upgrading musl-utils (1.2.5-r21 -> 1.2.5-r23)
OK: 8 MiB in 16 packages
`

func TestParseAPKPlan(t *testing.T) {
	cases := []struct {
		name       string
		output     string
		names      []string
		unreliable bool
		refused    int
	}{
		{"v2 with apk-tools update", apkSimulateV2, []string{"alpine-baselayout-data", "musl", "apk-tools", "musl-utils"}, true, 0},
		{"v3 padded counter", apkSimulateV3, []string{"alpine-baselayout-data", "ca-certificates-bundle", "apk-tools", "musl-utils"}, true, 0},
		{"nothing to do", "OK: 7 MiB in 15 packages\n", nil, false, 0},
		{"downgrade", "(1/2) Upgrading zlib (1.3.2-r0 -> 1.3.2-r1)\n(2/2) Downgrading scanelf (1.3.11-r0 -> 1.3.8-r2)\n", []string{"zlib", "scanelf"}, false, 1},
		{"purge and tagged", "(1/3) Purging busybox-binsh (1.37.0-r14)\n(2/3) Installing dash (0.5.12-r3)\n(3/3) Upgrading busybox@edge (1.37.0-r14 -> 1.38.0-r7)\n", []string{"busybox-binsh", "dash", "busybox"}, false, 1},
		{"pinning change", "(1/2) Upgrading apk-tools (2.14.6-r2 -> 2.14.6-r3)\n(2/2) Updating pinning scanelf (1.3.11-r0)\n", []string{"apk-tools", "scanelf"}, false, 0},
		{"unknown verb", "(1/1) Frobnicating busybox (1.0-r0)\n", []string{"busybox"}, false, 1},
		{"same-version replace", "(1/1) Replacing busybox (1.37.0-r30 -> 1.37.0-r30)\nOK: 8 MiB in 16 packages\n", []string{"busybox"}, false, 1},
		{"real run with preupgrade and scripts", "Preupgrading:\n(1/2) Upgrading libapk (3.0.1-r1 -> 3.0.8-r0)\n(2/2) Upgrading apk-tools (3.0.1-r1 -> 3.0.8-r0)\nExecuting busybox-1.37.0-r29.trigger\n  Installing file to etc/hosts.apk-new\n", []string{"libapk", "apk-tools"}, false, 0},
	}
	world := ParseAPKWorldNames("alpine-base\nbusybox-binsh\n")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := ParseAPKPlan(tc.output)
			var names []string
			for _, s := range plan.Steps {
				names = append(names, s.Name)
			}
			if !reflect.DeepEqual(names, tc.names) {
				t.Errorf("names = %v, want %v", names, tc.names)
			}
			if plan.Unreliable != tc.unreliable {
				t.Errorf("unreliable = %v, want %v", plan.Unreliable, tc.unreliable)
			}
			if got := len(plan.Refused(world)); got != tc.refused {
				t.Errorf("refused = %d, want %d", got, tc.refused)
			}
		})
	}
	if !ParseAPKPlan("OK: 7 MiB in 15 packages\n").Complete || ParseAPKPlan("WARNING: something\n").Complete {
		t.Error("Complete must follow apk's OK line")
	}
	if got := ParseAPKPlan("(1/1) Downgrading scanelf (1.3.11-r0 -> 1.3.8-r2)\n").Refused(nil)[0].String(); got != "Downgrading scanelf (1.3.11-r0 -> 1.3.8-r2)" {
		t.Errorf("refused step renders as %q", got)
	}
}

func TestParseAPKWorldNames(t *testing.T) {
	got := ParseAPKWorldNames("alpine-base\nbusybox=1.37.0-r29\n!nano\ncurl@edge>8.0\nzlib~1.3\n.tmp-test=20261014.134913\nfoo<2\nbar><Q1abc=\n!baz@edge\nso:libz.so.1\n")
	want := map[string]bool{
		"alpine-base": true, "busybox": true, "nano": true, "curl": true, "zlib": true, ".tmp-test": true,
		"foo": true, "bar": true, "baz": true, "so:libz.so.1": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseAPKWorldNames = %v, want %v", got, want)
	}
}

// Captured with apk upgrade --simulate on an Alpine edge VM (apk-tools 3.0.8): the new
// python3 dropped its gdbm dependency, so apk purges gdbm as an orphan.
func TestAPKPlanEdgeOrphanPurge(t *testing.T) {
	sim, err := os.ReadFile("testdata/apk-edge-simulate.txt")
	if err != nil {
		t.Fatal(err)
	}
	worldFile, err := os.ReadFile("testdata/apk-edge-world.txt")
	if err != nil {
		t.Fatal(err)
	}
	plan := ParseAPKPlan(string(sim))
	world := ParseAPKWorldNames(string(worldFile))
	if len(plan.Steps) != 141 || !plan.Complete {
		t.Fatalf("parsed %d steps, complete=%v", len(plan.Steps), plan.Complete)
	}
	if refused := plan.Refused(world); len(refused) != 0 {
		t.Errorf("orphan purge refused: %v", refused)
	}
	orphans := plan.Orphans(world)
	if len(orphans) != 1 || orphans[0].Name != "gdbm" || orphans[0].Detail != "(1.26-r0)" {
		t.Errorf("orphans = %+v", orphans)
	}

	world["gdbm"] = true
	if refused := plan.Refused(world); len(refused) != 1 || refused[0].Name != "gdbm" {
		t.Errorf("purging a world package must be refused, got %+v", refused)
	}
	if len(plan.Orphans(world)) != 0 {
		t.Error("a world package is never an orphan")
	}
}

func TestAPKToolsMajor(t *testing.T) {
	cases := map[string]int{
		"apk-tools 2.14.6, compiled for aarch64.\n":  2,
		"apk-tools 3.0.8-r0, compiled for x86_64.\n": 3,
		"sh: apk: not found":                         0,
		"":                                           0,
	}
	for in, want := range cases {
		if got := APKToolsMajor(in); got != want {
			t.Errorf("APKToolsMajor(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestAPKArgs(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"update", APKUpdateArgs(), []string{"update", "--wait", "30"}},
		{"simulate all", APKUpgradeArgs(true, nil), []string{"upgrade", "--no-progress", "--simulate"}},
		{"simulate selected", APKUpgradeArgs(true, []string{"libssl3", "libcrypto3"}), []string{"upgrade", "--no-progress", "--simulate", "libssl3", "libcrypto3"}},
		{"upgrade all", APKUpgradeArgs(false, nil), []string{"upgrade", "--no-progress", "--wait", "30"}},
		{"upgrade selected", APKUpgradeArgs(false, []string{"busybox"}), []string{"upgrade", "--no-progress", "--wait", "30", "busybox"}},
		{"preupgrade v2", APKPreupgradeArgs(2), []string{"upgrade", "--no-progress", "--wait", "30", "--self-upgrade-only"}},
		{"preupgrade v3", APKPreupgradeArgs(3), []string{"upgrade", "--no-progress", "--wait", "30", "--preupgrade-only"}},
		{"preupgrade unknown", APKPreupgradeArgs(0), nil},
	}
	for _, tc := range cases {
		if !reflect.DeepEqual(tc.got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
	for _, args := range [][]string{APKUpgradeArgs(false, []string{"x"}), APKUpgradeArgs(true, []string{"x"})} {
		if slices.Contains(args, "add") || slices.Contains(args, "--available") || slices.Contains(args, "--latest") {
			t.Errorf("upgrade must not touch world or reset holds: %v", args)
		}
	}
}

func testInventory() APKInventory {
	return APKInventory{
		Major: 3,
		Installed: map[string]string{
			"busybox": "1.37.0-r29", "busybox-binsh": "1.37.0-r29", "ssl_client": "1.37.0-r29",
			"libssl3": "3.5.4-r0", "libcrypto3": "3.5.4-r0", "zlib": "1.3.1-r2",
		},
		Available: map[string]string{
			"busybox": "1.37.0-r30", "busybox-binsh": "1.37.0-r30", "ssl_client": "1.37.0-r30",
			"libssl3": "3.5.9-r0", "libcrypto3": "3.5.9-r0", "zlib": "1.3.2-r1",
		},
		Origin: map[string]string{
			"busybox": "busybox", "busybox-binsh": "busybox", "ssl_client": "busybox",
			"libssl3": "openssl", "libcrypto3": "openssl", "zlib": "zlib",
		},
		World: ParseAPKWorld("busybox=1.37.0-r29"),
	}
}

func TestAPKInventoryExpandTargets(t *testing.T) {
	inv := testInventory()
	holds, _ := inv.Holds()
	got := inv.ExpandTargets([]string{"libssl3", "ssl_client"}, holds)
	want := []string{"libssl3", "ssl_client", "busybox-binsh", "libcrypto3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExpandTargets = %v, want %v", got, want)
	}
}

func TestAPKInventoryNotUpgraded(t *testing.T) {
	inv := testInventory()
	holds, _ := inv.Holds()
	plan := ParseAPKPlan("(1/3) Upgrading libcrypto3 (3.5.4-r0 -> 3.5.9-r0)\n(2/3) Upgrading libssl3 (3.5.4-r0 -> 3.5.9-r0)\n(3/3) Upgrading zlib (1.3.1-r2 -> 1.3.2-r1)\n")

	all := inv.NotUpgraded(plan, holds, nil)
	want := []string{
		"Not upgraded by apk: busybox-binsh 1.37.0-r29 (1.37.0-r30 available), held back with busybox by /etc/apk/world",
		"Not upgraded by apk: ssl_client 1.37.0-r29 (1.37.0-r30 available), held back with busybox by /etc/apk/world",
	}
	if !reflect.DeepEqual(all, want) {
		t.Errorf("patch_all NotUpgraded = %q, want %q", all, want)
	}

	inv.World = nil
	selected := inv.NotUpgraded(ParseAPKPlan("OK: 8 MiB in 16 packages\n"), nil, []string{"libssl3"})
	if len(selected) != 1 || !strings.Contains(selected[0], "libssl3 3.5.4-r0 (3.5.9-r0 available). Its new version needs other packages") {
		t.Errorf("selected NotUpgraded = %q", selected)
	}
}

func TestAPKConflictFromError(t *testing.T) {
	cases := []struct {
		name, output, pkg, installed, constraint string
	}{
		{"v2", "WARNING: Failed to perform initial self-upgrade, continuing with full upgrade.\nERROR: unable to select packages:\n  busybox-1.37.0-r8:\n    breaks: world[busybox<1.0]\n", "busybox", "1.37.0-r8", "busybox<1.0"},
		{"v3", "ERROR: unable to select packages:\n  busybox-1.37.0-r29:\n    breaks: world[busybox<1.0]\n            busybox-binsh-1.37.0-r30[busybox=1.37.0-r30]\n", "busybox", "1.37.0-r29", "busybox<1.0"},
	}
	for _, tc := range cases {
		c, ok := APKConflictFromError(tc.output)
		if !ok || c.Name != tc.pkg || c.Installed != tc.installed || c.Constraint != tc.constraint {
			t.Errorf("%s: got %+v %v", tc.name, c, ok)
		}
	}
	if _, ok := APKConflictFromError("ERROR: Package 'nosuchpkg' not found\n"); ok {
		t.Error("an unknown package is not a world conflict")
	}
}

func TestAPKSecDBSources(t *testing.T) {
	lines := []repositories.APKRepoLine{
		{URL: "https://dl-cdn.alpinelinux.org/alpine/v3.22/main"},
		{URL: "https://dl-cdn.alpinelinux.org/alpine/v3.22/community"},
		{Tag: "@edge", URL: "https://dl-cdn.alpinelinux.org/alpine/edge/main"},
		{URL: "https://mirror.example.com/alpine/v3.21/main"},
		{URL: "https://packages.example.com/apk"},
	}
	branch, repos := apkSecDBSources(lines)
	if branch != "v3.22" || !reflect.DeepEqual(repos, []string{"main", "community"}) {
		t.Errorf("got %q %v", branch, repos)
	}
	if b, _ := apkSecDBSources(lines[2:3]); b != "" {
		t.Errorf("a tagged repository must not choose the branch, got %q", b)
	}
}

func TestAPKFixesSecurity(t *testing.T) {
	fixes := []string{"3.3.5-r0", "3.3.7-r2"}
	cases := []struct {
		installed, available string
		want                 bool
	}{
		{"3.3.2-r4", "3.3.7-r2", true},
		{"3.3.6-r0", "3.3.7-r1", false},
		{"3.3.7-r2", "3.3.8-r0", false},
		{"3.3.5-r0", "3.3.7-r2", true},
		{"bogus", "3.3.7-r2", false},
	}
	for _, tc := range cases {
		if got := apkFixesSecurity(fixes, tc.installed, tc.available, CompareAPKVersions); got != tc.want {
			t.Errorf("%s -> %s = %v, want %v", tc.installed, tc.available, got, tc.want)
		}
	}
	if apkFixesSecurity(nil, "1.0-r0", "2.0-r0", CompareAPKVersions) {
		t.Error("no secfixes means no security update")
	}
}

const secdbMain = `{"distroversion":"v3.23","reponame":"main","packages":[
{"pkg":{"name":"openssl","secfixes":{"0":["CVE-2000-0001"],"3.5.5-r0":["CVE-2025-9230"],"3.5.9-r0":["CVE-2026-1"]}}},
{"pkg":{"name":"zlib","secfixes":{"1.3.2-r0":["CVE-2026-2"]}}},
{"pkg":{"name":"xen","secfixes":{"4.20.1-r0":["XSA-470"],"not-a-version":["X"]}}}]}`

func TestAPKSecDBFixesAndCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the secdb cache is never trusted on Windows")
	}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/v3.23/main.json":
			_, _ = io.WriteString(w, secdbMain)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	now := time.Now()
	db := &apkSecDB{logger: quietLogger(), client: srv.Client(), baseURL: srv.URL, cacheDir: t.TempDir(), cacheOwner: os.Getuid(), now: func() time.Time { return now }}

	fixes := db.fixes(context.Background(), "v3.23", []string{"main", "community"})
	slices.Sort(fixes["openssl"])
	if !reflect.DeepEqual(fixes["openssl"], []string{"3.5.5-r0", "3.5.9-r0"}) {
		t.Errorf("openssl fixes = %v, the \"0\" key must be skipped", fixes["openssl"])
	}
	if !reflect.DeepEqual(fixes["xen"], []string{"4.20.1-r0"}) {
		t.Errorf("xen fixes = %v", fixes["xen"])
	}
	first := hits.Load()

	db.fixes(context.Background(), "v3.23", []string{"main"})
	if hits.Load() != first {
		t.Error("a fresh cache must not be fetched again")
	}

	now = now.Add(apkSecDBTTL + time.Minute)
	srv.Close()
	stale := db.fixes(context.Background(), "v3.23", []string{"main"})
	if len(stale["zlib"]) != 1 {
		t.Errorf("an expired cache must still be used when secdb is unreachable, got %v", stale)
	}

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	empty := &apkSecDB{logger: quietLogger(), client: gone.Client(), baseURL: gone.URL, cacheDir: t.TempDir(), cacheOwner: os.Getuid(), now: time.Now}
	if got := empty.fixes(context.Background(), "v3.23", []string{"main"}); len(got) != 0 {
		t.Errorf("unreachable secdb with no cache must yield nothing, got %v", got)
	}
}

func TestAPKSecDBUntrustedCacheDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, secdbMain)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	db := &apkSecDB{logger: quietLogger(), client: srv.Client(), baseURL: srv.URL, cacheDir: dir, cacheOwner: os.Getuid(), now: time.Now}
	if len(db.fixes(context.Background(), "v3.23", []string{"main"})) == 0 {
		t.Fatal("an untrusted cache must not stop the fetch")
	}
	if _, err := os.Stat(filepath.Join(dir, "v3.23-main.json")); !os.IsNotExist(err) {
		t.Error("a world-writable cache directory must not be written")
	}

	otherOwner := &apkSecDB{logger: quietLogger(), client: srv.Client(), baseURL: srv.URL, cacheDir: t.TempDir(), cacheOwner: os.Getuid() + 1, now: time.Now}
	if otherOwner.cacheDirTrusted() {
		t.Error("a cache directory owned by another user must not be trusted")
	}

	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	viaLink := &apkSecDB{logger: quietLogger(), cacheDir: link, cacheOwner: os.Getuid(), now: time.Now}
	if viaLink.cacheDirTrusted() {
		t.Error("a symlinked cache directory must not be trusted")
	}
}

func TestAPKSecDBCheckRedirect(t *testing.T) {
	first, _ := http.NewRequest(http.MethodGet, "https://secdb.alpinelinux.org/v3.23/main.json", nil)
	cases := []struct {
		to   string
		want bool
	}{
		{"https://secdb.alpinelinux.org/v3.23/main.json?x", true},
		{"http://secdb.alpinelinux.org/v3.23/main.json", false},
		{"https://evil.example.com/main.json", false},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(http.MethodGet, tc.to, nil)
		if got := apkSecDBCheckRedirect(req, []*http.Request{first}) == nil; got != tc.want {
			t.Errorf("redirect to %s allowed = %v, want %v", tc.to, got, tc.want)
		}
	}
}

func TestAPKSecDBWarnsOncePerTTL(t *testing.T) {
	apkSecDBMu.Lock()
	apkSecDBLastWarn = time.Time{}
	apkSecDBMu.Unlock()
	now := time.Now()
	db := &apkSecDB{now: func() time.Time { return now }}
	if !db.shouldWarn() || db.shouldWarn() {
		t.Fatal("expected one warning, then silence")
	}
	now = now.Add(apkSecDBTTL + time.Second)
	if !db.shouldWarn() {
		t.Error("expected a fresh warning after the TTL")
	}
}

func TestAPKSecDBBacksOffAfterFailure(t *testing.T) {
	var hits atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if fail.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, secdbMain)
	}))
	defer srv.Close()
	now := time.Now()
	db := &apkSecDB{logger: quietLogger(), client: srv.Client(), baseURL: srv.URL, cacheDir: t.TempDir(), cacheOwner: os.Getuid(), now: func() time.Time { return now }}

	db.fixes(context.Background(), "v3.23", []string{"main"})
	db.fixes(context.Background(), "v3.23", []string{"main"})
	if hits.Load() != 1 {
		t.Fatalf("a failed source must not be retried within the backoff, got %d requests", hits.Load())
	}
	fail.Store(false)
	now = now.Add(apkSecDBBackoff + time.Second)
	if got := db.fixes(context.Background(), "v3.23", []string{"main"}); len(got["zlib"]) != 1 || hits.Load() != 2 {
		t.Fatalf("expected a retry after the backoff, got %v after %d requests", got, hits.Load())
	}
	now = now.Add(time.Minute)
	fail.Store(true)
	db.cacheDir = t.TempDir()
	db.fixes(context.Background(), "v3.23", []string{"main"})
	if hits.Load() != 3 {
		t.Errorf("a success must clear the backoff, got %d requests", hits.Load())
	}
}

func TestAPKSecDBCancelledFetchIsNotABackoff(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, secdbMain)
	}))
	defer srv.Close()
	db := &apkSecDB{logger: quietLogger(), client: srv.Client(), baseURL: srv.URL, cacheDir: t.TempDir(), cacheOwner: os.Getuid(), now: time.Now}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := db.fixes(ctx, "v3.23", []string{"main"}); len(got) != 0 {
		t.Fatalf("a cancelled fetch must yield nothing, got %v", got)
	}
	if got := db.fixes(context.Background(), "v3.23", []string{"main"}); len(got["zlib"]) != 1 || hits.Load() != 1 {
		t.Errorf("a cancelled fetch must not start a backoff: %v after %d requests", got, hits.Load())
	}
}

func TestAPKSecDBRejectsNonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html>captive portal</html>")
	}))
	defer srv.Close()
	dir := t.TempDir()
	db := &apkSecDB{logger: quietLogger(), client: srv.Client(), baseURL: srv.URL, cacheDir: dir, cacheOwner: os.Getuid(), now: time.Now}
	if got := db.fixes(context.Background(), "v3.23", []string{"main"}); len(got) != 0 {
		t.Errorf("got %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "v3.23-main.json")); !os.IsNotExist(err) {
		t.Error("a non-JSON response must not be cached")
	}
}

func TestAcquireAPK(t *testing.T) {
	waited := 0
	release, err := AcquireAPK(context.Background(), func() { waited++ })
	if err != nil || waited != 0 {
		t.Fatalf("a free slot must not wait: err=%v waited=%d", err, waited)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := AcquireAPK(ctx, func() { waited++ }); err == nil {
		t.Fatal("a second patch run must wait for the first")
	}
	if waited != 1 {
		t.Errorf("onWait ran %d times, want 1", waited)
	}
	m := &APKManager{logger: quietLogger()}
	m.refreshIndexes()
	release()
	release()

	second, err := AcquireAPK(context.Background(), nil)
	if err != nil {
		t.Fatal("release must free the slot")
	}
	select {
	case apkBusy <- struct{}{}:
		t.Fatal("a repeated release must not free a slot someone else holds")
	default:
	}
	second()
}
