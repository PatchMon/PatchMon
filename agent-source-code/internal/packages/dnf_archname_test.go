package packages

import (
	"sort"
	"testing"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

func TestStripRPMArchSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"strips x86_64", "bash.x86_64", "bash"},
		{"strips noarch", "python3-pip.noarch", "python3-pip"},
		{"strips aarch64", "glibc.aarch64", "glibc"},
		{"strips i686", "glibc.i686", "glibc"},
		{"strips ppc64le", "kernel.ppc64le", "kernel"},
		{"strips s390x", "kernel.s390x", "kernel"},
		{"bare name untouched", "bash", "bash"},

		// The reason this is not a naive split on ".": these are real RPM
		// names whose own name contains a dot. strings.Split(n, ".")[0] would
		// turn both into "python3" and merge two distinct packages.
		{"dotted name without arch is untouched", "python3.11", "python3.11"},
		{"dotted name with arch keeps its dot", "python3.11.x86_64", "python3.11"},
		{"dotted name with noarch keeps its dot", "python3.12.noarch", "python3.12"},

		// A trailing dot-token that is not an architecture must survive.
		{"non-arch suffix untouched", "libfoo.so", "libfoo.so"},
		{"version-like suffix untouched", "package.el9", "package.el9"},

		// Degenerate inputs.
		{"empty", "", ""},
		{"leading dot untouched", ".x86_64", ".x86_64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := stripRPMArchSuffix(tt.in); got != tt.want {
				t.Errorf("stripRPMArchSuffix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestDNFNoDuplicateRowsAfterCombine is the end-to-end regression guard.
func TestDNFNoDuplicateRowsAfterCombine(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	m := NewDNFManager(logger)

	installedOutput := `Installed Packages
bash.x86_64                       5.1.8-6.el9                     @baseos
systemd.x86_64                    252-14.el9_2.1                  @baseos
python3.11.x86_64                 3.11.2-2.el9                    @appstream
curl.x86_64                       7.76.1-23.el9                   @baseos`

	checkUpdateOutput := `bash.x86_64                       5.1.8-9.el9                     baseos
python3.11.x86_64                 3.11.5-1.el9                    appstream`

	installed := m.parseInstalledPackages(installedOutput)
	if len(installed) != 4 {
		t.Fatalf("expected 4 installed packages, got %d: %v", len(installed), installed)
	}
	// The dotted name must survive arch stripping intact.
	if _, ok := installed["python3.11"]; !ok {
		t.Fatalf("expected installed key \"python3.11\", got keys %v", keysOf(installed))
	}

	upgradable := m.parseUpgradablePackages(checkUpdateOutput, "dnf", installed, map[string]bool{})
	if len(upgradable) != 2 {
		t.Fatalf("expected 2 upgradable packages, got %d", len(upgradable))
	}

	combined := CombinePackageData(installed, upgradable)

	// One row per installed package, no phantom duplicates.
	if len(combined) != 4 {
		t.Fatalf("expected 4 combined packages, got %d: %v", len(combined), namesOf(combined))
	}

	seen := make(map[string]int, len(combined))
	for _, p := range combined {
		seen[p.Name]++
	}
	for name, count := range seen {
		if count != 1 {
			t.Errorf("package %q appears %d times, expected exactly once", name, count)
		}
	}

	// The two upgradable packages must be flagged, and must carry both versions.
	for _, name := range []string{"bash", "python3.11"} {
		var found bool
		for _, p := range combined {
			if p.Name != name {
				continue
			}
			found = true
			if !p.NeedsUpdate {
				t.Errorf("%s should be flagged NeedsUpdate", name)
			}
			if p.CurrentVersion == "" || p.AvailableVersion == "" {
				t.Errorf("%s missing versions: current=%q available=%q", name, p.CurrentVersion, p.AvailableVersion)
			}
		}
		if !found {
			t.Errorf("expected %q in the combined set, got %v", name, namesOf(combined))
		}
	}

	// The untouched packages must not be flagged.
	for _, p := range combined {
		if (p.Name == "systemd" || p.Name == "curl") && p.NeedsUpdate {
			t.Errorf("%s should not be flagged NeedsUpdate", p.Name)
		}
	}
}

func keysOf(m map[string]models.Package) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func namesOf(pkgs []models.Package) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}

// TestMultilibDoesNotProduceDuplicateNames is the regression guard for the
// defect arch-stripping introduced.
func TestMultilibDoesNotProduceDuplicateNames(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	m := NewDNFManager(logger)

	installed := m.parseInstalledPackages(`Installed Packages
bash.x86_64                       5.1.8-6.el9                     @baseos
glibc.x86_64                      2.34-100.el9                    @baseos
glibc.i686                        2.34-100.el9                    @baseos`)

	upgradable := m.parseUpgradablePackages(`glibc.x86_64                      2.34-125.el9                    baseos
glibc.i686                        2.34-125.el9                    baseos`,
		"dnf", installed, map[string]bool{})

	combined := CombinePackageData(installed, upgradable)

	counts := make(map[string]int, len(combined))
	for _, p := range combined {
		counts[p.Name]++
	}
	for name, n := range counts {
		if n != 1 {
			t.Errorf("package %q appears %d times in the report payload; "+
				"duplicate names abort the server's bulk upsert with Postgres 21000", name, n)
		}
	}

	// The pending update must survive de-duplication.
	var glibc *models.Package
	for i := range combined {
		if combined[i].Name == "glibc" {
			glibc = &combined[i]
		}
	}
	if glibc == nil {
		t.Fatalf("glibc missing from the combined set: %v", namesOf(combined))
	}
	if !glibc.NeedsUpdate {
		t.Error("de-duplication must keep the pending update, not drop it")
	}
}

// TestMultilibKeepsTheSecurityFlaggedEntry: where one arch is a security
// update and the other is not, the host's security count must not be silently
// reduced.
func TestMultilibKeepsTheSecurityFlaggedEntry(t *testing.T) {
	t.Parallel()

	installed := map[string]models.Package{
		"glibc": {Name: "glibc", CurrentVersion: "2.34-100.el9"},
	}
	// Plain entry first, so the retained one must be replaced by the later
	// security-flagged one.
	upgradable := []models.Package{
		{Name: "glibc", CurrentVersion: "2.34-100.el9", AvailableVersion: "2.34-125.el9", NeedsUpdate: true},
		{Name: "glibc", CurrentVersion: "2.34-100.el9", AvailableVersion: "2.34-125.el9", NeedsUpdate: true, IsSecurityUpdate: true},
	}

	combined := CombinePackageData(installed, upgradable)
	if len(combined) != 1 {
		t.Fatalf("expected exactly one glibc row, got %d: %v", len(combined), namesOf(combined))
	}
	if !combined[0].IsSecurityUpdate {
		t.Error("the security-flagged entry must win, otherwise the security count under-reports")
	}
}
