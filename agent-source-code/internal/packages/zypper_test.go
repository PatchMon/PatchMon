package packages

import (
	"os"
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
	if got := neededSecurityPatches(lp); got != 4 {
		t.Errorf("needed security patches = %d, want 4 (stack pending, so they sit in blocked-update-list)", got)
	}
	if lp.noUpdateRepositories() {
		t.Error("leap reports update repositories")
	}

	bci := readZypperFixture(t, "zypper-bci156-lp.xml")
	if !bci.noUpdateRepositories() || neededSecurityPatches(bci) != 0 {
		t.Error("BCI has no update repositories")
	}

	names := readZypperFixture(t, "zypper-leap155-patch-security-dry.xml").InstallSummary.packageNames()
	want := []string{"curl", "libcurl4", "libglib-2_0-0", "libopenssl1_1", "libopenssl1_1-hmac", "openssl-1_1"}
	if len(names) != len(want) {
		t.Errorf("got %v, want %v", names, want)
	}
	for _, n := range want {
		if !names[n] {
			t.Errorf("%s not flagged", n)
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
