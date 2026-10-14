package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readZypperFixture(t *testing.T, name string) zypperStream {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseZypperStream(b)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestZypperListUpdates(t *testing.T) {
	cases := []struct {
		fixture string
		count   int
		name    string
		current string
		avail   string
		repo    string
	}{
		{"zypper-leap155-lu.xml", 22, "curl", "8.0.1-150400.5.50.1", "8.0.1-150400.5.59.1", "repo-sle-update"},
		{"zypper-leap160-lu.xml", 2, "patterns-base-fips", "20241218-lp160.5.1", "20241218+leap-lp160.1.1", "openSUSE:repo-oss"},
		{"zypper-tumbleweed-lu.xml", 1, "openSUSE-build-key", "1.0-68.1", "1.0-69.1", "repo-update"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			got := zypperUpgradablePackages(readZypperFixture(t, tc.fixture).Updates, nil)
			if len(got) != tc.count {
				t.Fatalf("got %d upgradable packages, want %d", len(got), tc.count)
			}
			for _, p := range got {
				if p.Name != tc.name {
					continue
				}
				if p.CurrentVersion != tc.current || p.AvailableVersion != tc.avail || p.SourceRepository != tc.repo || !p.NeedsUpdate {
					t.Errorf("got %+v", p)
				}
				return
			}
			t.Errorf("%s missing", tc.name)
		})
	}
}

func TestZypperSecurityPatches(t *testing.T) {
	lp := readZypperFixture(t, "zypper-leap155-lp-security.xml")
	if got := neededSecurityPatches(lp); len(got) != 4 {
		t.Errorf("needed security patches = %v, want 4 (stack pending, so they sit in blocked-update-list)", got)
	}
	if lp.noUpdateRepositories() {
		t.Error("leap reports update repositories")
	}

	bci := readZypperFixture(t, "zypper-bci156-lp.xml")
	if !bci.noUpdateRepositories() || len(neededSecurityPatches(bci)) != 0 {
		t.Error("BCI has no update repositories")
	}

	info, err := os.ReadFile("testdata/zypper-leap155-info-security-patches.txt")
	if err != nil {
		t.Fatal(err)
	}
	fixes := parseZypperPatchConflicts(string(info))
	if got := fixes["libopenssl1_1-hmac"]; len(got) != 1 || got[0] != "1.1.1l-150500.17.37.1" {
		t.Errorf("libopenssl1_1-hmac fixes = %v", got)
	}
	var flagged []string
	for _, p := range zypperUpgradablePackages(readZypperFixture(t, "zypper-leap155-lu.xml").Updates, nil) {
		if zypperFixesSecurity(fixes[p.Name], p.CurrentVersion, p.AvailableVersion) {
			flagged = append(flagged, p.Name)
		}
	}
	want := "curl,libcurl4,libglib-2_0-0,libopenssl1_1,libopenssl1_1-hmac,openssl-1_1"
	if got := strings.Join(flagged, ","); got != want {
		t.Errorf("flagged %s, want %s (what zypper patch --dry-run --category security upgrades)", got, want)
	}
}

func TestZypperFixesSecurity(t *testing.T) {
	cases := []struct {
		fixes              []string
		current, available string
		want               bool
	}{
		{[]string{"8.0.1-150400.5.59.1"}, "8.0.1-150400.5.50.1", "8.0.1-150400.5.59.1", true},
		{[]string{"8.0.1-150400.5.59.1"}, "8.0.1-150400.5.59.1", "8.0.1-150400.5.62.1", false},
		{[]string{"8.0.1-150400.5.59.1"}, "8.0.1-150400.5.50.1", "8.0.1-150400.5.55.1", false},
		{[]string{"1.0-1", "2.0-1"}, "1.5-1", "2.1-1", true},
		{nil, "1.0-1", "2.0-1", false},
	}
	for _, tc := range cases {
		if got := zypperFixesSecurity(tc.fixes, tc.current, tc.available); got != tc.want {
			t.Errorf("zypperFixesSecurity(%v, %s, %s) = %v, want %v", tc.fixes, tc.current, tc.available, got, tc.want)
		}
	}
}

func TestCompareRPMEVR(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0-1", "1.0-1", 0},
		{"1.0-1", "1.0-2", -1},
		{"1.10-1", "1.9-1", 1},
		{"1.0a-1", "1.0-1", 1},
		{"1.0~rc1-1", "1.0-1", -1},
		{"1.0^git1-1", "1.0-1", 1},
		{"1:1.0-1", "2.0-1", 1},
		{"0:2.0-1", "2.0-1", 0},
		{"84.87+git20180409.04c9dae-150300.10.20.1", "84.87+git20180409.04c9dae-150300.10.23.1", -1},
		{"13.3.0+git8781-150000.1.12.1", "14.2.0+git10526-150000.1.6.1", -1},
		{"20241218-lp160.5.1", "20241218+leap-lp160.1.1", -1},
		{"1.0.010-1", "1.0.9-1", 1},
		{"2.0", "2.0-5", 0},
		{"1.0a", "1.0.1", -1},
	}
	for _, tc := range cases {
		if got := compareRPMEVR(tc.a, tc.b); got != tc.want {
			t.Errorf("compareRPMEVR(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
		if got := compareRPMEVR(tc.b, tc.a); got != -tc.want {
			t.Errorf("compareRPMEVR(%q, %q) = %d, want %d", tc.b, tc.a, got, -tc.want)
		}
	}
}

func TestParseRPMQueryOutput(t *testing.T) {
	out := "bash\t(none)\t4.4\t150400.25.22\t1700000000\tThe GNU Bourne-Again Shell\n" +
		"kernel-default\t(none)\t6.4.0\t150600.23.25.1\t1700000000\tThe Standard Kernel\n" +
		"kernel-default\t(none)\t6.4.0\t150600.23.30.1\t1700000500\tThe Standard Kernel\n" +
		"kernel-default\t(none)\t6.4.0\t150600.23.20.1\t1600000000\tThe Standard Kernel\n" +
		"gpg-pubkey\t(none)\t29b700a4\t62b07e22\t1700000000\tgpg(SUSE)\n" +
		"perl-Foo\t2\t1.0\t1.1\t1700000000\t\n" +
		"short\tline\n"
	got := parseRPMQueryOutput(out)
	if len(got) != 3 {
		t.Fatalf("got %d packages: %v", len(got), got)
	}
	if got["bash"].CurrentVersion != "4.4-150400.25.22" || got["bash"].Description != "The GNU Bourne-Again Shell" {
		t.Errorf("bash = %+v", got["bash"])
	}
	if got["kernel-default"].CurrentVersion != "6.4.0-150600.23.30.1" {
		t.Errorf("kernel-default = %s, want the most recently installed", got["kernel-default"].CurrentVersion)
	}
	if got["perl-Foo"].CurrentVersion != "2:1.0-1.1" {
		t.Errorf("perl-Foo = %s, want the epoch kept", got["perl-Foo"].CurrentVersion)
	}
}

func TestParseZypperStreamWithoutXML(t *testing.T) {
	if _, err := parseZypperStream([]byte("zypper: command not found")); err == nil {
		t.Error("want an error")
	}
}

func readZypperPlan(t *testing.T, name string) ZypperPlan {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ParseZypperPlan(b)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestParseZypperPlan(t *testing.T) {
	cases := []struct {
		fixture  string
		lines    int
		first    string
		removals []string
		patches  int
	}{
		{"zypper-leap155-patch-dry-stack.xml", 4, "  upgrade libsolv-tools 0.7.30-150400.3.27.2 -> 0.7.31-150500.6.5.1", nil, 4},
		{"zypper-leap155-patch-security-dry.xml", 6, "  upgrade curl 8.0.1-150400.5.50.1 -> 8.0.1-150400.5.59.1", nil, 4},
		{"zypper-tumbleweed-dup-dry.xml", 13, "  upgrade openSUSE-build-key 1.0-68.1 -> 1.0-69.1", []string{"libcurl-mini4"}, 0},
		{"zypper-tumbleweed-update-dry.xml", 13, "", []string{"libcurl-mini4"}, 0},
		{"zypper-bci156-lp.xml", 0, "", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			plan := readZypperPlan(t, tc.fixture)
			lines := plan.Lines()
			if len(lines) != tc.lines {
				t.Fatalf("got %d lines: %q", len(lines), lines)
			}
			if tc.first != "" && lines[0] != tc.first {
				t.Errorf("first line %q, want %q", lines[0], tc.first)
			}
			if got := plan.Removals(); len(got) != len(tc.removals) || (len(got) > 0 && got[0] != tc.removals[0]) {
				t.Errorf("removals %v, want %v", got, tc.removals)
			}
			if len(plan.Patches) != tc.patches {
				t.Errorf("patches %v, want %d", plan.Patches, tc.patches)
			}
			if len(plan.Refused()) != 0 {
				t.Errorf("refused %v", plan.Refused())
			}
		})
	}
}

func TestZypperPlanNothingAndErrors(t *testing.T) {
	nothing := `<?xml version='1.0'?>
<stream>
<install-summary download-size="0" space-usage-diff="0" packages-to-change="0" need-restart="0" need-reboot="0"></install-summary>
<message type="info">Nothing to do.</message>
</stream>`
	plan, err := ParseZypperPlan([]byte(nothing))
	if err != nil || !plan.Nothing {
		t.Errorf("nothing to do not detected: %+v %v", plan, err)
	}

	problem := readZypperPlan(t, "zypper-leap160-patch-dry-problem.xml")
	if len(problem.Problems) != 1 || !strings.Contains(problem.Problems[0], "systemd-sysvcompat") || !strings.Contains(problem.Problems[0], "Solution 2") {
		t.Errorf("problems %q", problem.Problems)
	}

	for _, f := range []string{"zypper-leap155-update-notfound.xml", "zypper-leap155-locked.xml"} {
		if p := readZypperPlan(t, f); len(p.Errors) == 0 {
			t.Errorf("%s: no error messages read", f)
		}
	}
}

func TestZypperPlanRefused(t *testing.T) {
	out := `<?xml version='1.0'?>
<stream>
<install-summary packages-to-change="3">
<to-downgrade><solvable type="package" name="vim" edition="9.0-1.1" edition-old="9.1-2.1" arch="x86_64"/></to-downgrade>
<to-change-arch><solvable type="package" name="glibc" edition="2.38-1" arch="x86_64" arch-old="i586"/></to-change-arch>
<to-change-vendor><solvable type="package" name="ffmpeg-6" edition="6.1-1"/></to-change-vendor>
</install-summary>
</stream>`
	plan, err := ParseZypperPlan([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"downgrade vim 9.1-2.1 -> 9.0-1.1",
		"change the architecture of glibc from i586 to x86_64",
		"change the vendor of ffmpeg-6",
	}
	got := plan.Refused()
	if len(got) != len(want) {
		t.Fatalf("got %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %q, want %q", got[i], want[i])
		}
	}
}

func TestZypperTransactionArgs(t *testing.T) {
	cases := []struct {
		name    string
		rolling bool
		targets []string
		plan    bool
		want    string
	}{
		{"leap patch all", false, nil, false, "--non-interactive --non-interactive-include-reboot-patches --no-refresh patch --with-update --auto-agree-with-licenses"},
		{"leap patch all plan", false, nil, true, "--non-interactive --non-interactive-include-reboot-patches --no-refresh --xmlout patch --with-update --auto-agree-with-licenses --dry-run"},
		{"tumbleweed patch all", true, nil, false, "--non-interactive --non-interactive-include-reboot-patches --no-refresh dist-upgrade --auto-agree-with-licenses"},
		{"selected", false, []string{"curl", "libcurl4"}, false, "--non-interactive --non-interactive-include-reboot-patches --no-refresh update --auto-agree-with-licenses -- curl libcurl4"},
		{"selected on tumbleweed plan", true, []string{"curl"}, true, "--non-interactive --non-interactive-include-reboot-patches --no-refresh --xmlout update --auto-agree-with-licenses --dry-run -- curl"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(ZypperTransactionArgs(tc.rolling, tc.targets, tc.plan), " "); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestZypperLocks(t *testing.T) {
	locks := readZypperFixture(t, "zypper-leap155-locks.xml").Locks
	if len(locks) != 1 || locks[0].Name != "bash" || locks[0].Type != "package" {
		t.Fatalf("got %+v", locks)
	}
	locks = append(locks, ZypperLock{Name: "kernel-*", Type: "package"}, ZypperLock{Name: "curl", Type: "pattern"})
	locked, free := ZypperLockedTargets(locks, []string{"bash", "kernel-default", "curl", "zsh"})
	if strings.Join(locked, ",") != "bash,kernel-default" || strings.Join(free, ",") != "curl,zsh" {
		t.Errorf("locked %v free %v", locked, free)
	}
}

func TestZypperHostKind(t *testing.T) {
	dir := t.TempDir()
	oldRelease, oldMounts, oldTU := zypperOSReleasePath, zypperMountsPath, transactionalUpdateAt
	t.Cleanup(func() { zypperOSReleasePath, zypperMountsPath, transactionalUpdateAt = oldRelease, oldMounts, oldTU })
	zypperOSReleasePath = filepath.Join(dir, "os-release")
	zypperMountsPath = filepath.Join(dir, "mounts")
	transactionalUpdateAt = filepath.Join(dir, "transactional-update")

	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for id, want := range map[string]bool{`"opensuse-tumbleweed"`: true, "opensuse-slowroll": true, `"opensuse-leap"`: false, `"sles"`: false} {
		write(zypperOSReleasePath, "NAME=x\nID="+id+"\nID_LIKE=\"suse\"\n")
		if got := ZypperRolling(); got != want {
			t.Errorf("ID=%s rolling = %v, want %v", id, got, want)
		}
	}

	write(zypperMountsPath, "/dev/vda3 / btrfs ro,relatime 0 0\n/dev/vda3 /var btrfs rw 0 0\n")
	if ZypperTransactional() {
		t.Error("read-only root without transactional-update is not transactional")
	}
	write(transactionalUpdateAt, "")
	if !ZypperTransactional() {
		t.Error("MicroOS layout not detected")
	}
	write(zypperMountsPath, "/dev/vda3 / btrfs rw,relatime 0 0\n")
	if ZypperTransactional() {
		t.Error("writable root is not transactional")
	}
}
