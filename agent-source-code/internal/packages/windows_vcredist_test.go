package packages

import (
	"strings"
	"testing"

	"patchmon-agent/pkg/models"
)

// Issue #760: registry names from the reporter's Programs and Features, winget
// truncating both architectures onto the same 40-column name as seen on CI.
func TestVCRedistRowsSharingATruncatedName(t *testing.T) {
	const (
		x64      = "Microsoft Visual C++ v14 Redistributable (x64) - 14.50.35719"
		x86      = "Microsoft Visual C++ v14 Redistributable (x86) - 14.50.35719"
		trunc    = "Microsoft Visual C++ v14 Redistributabl…"
		x64ID    = "Microsoft.VCRedist.2015+.x64"
		x86ID    = "Microsoft.VCRedist.2015+.x86"
		vclibs   = "Microsoft.VCLibs.140.00.UWPDesktop"
		vclibsID = "Microsoft.VCLibs.Desktop.14"
	)

	cases := []struct {
		name       string
		reg        []models.Package
		rows       []wingetTestRow
		upgrades   map[string]string
		wantUpdate map[string]string
	}{
		{
			name: "both architectures behind",
			reg: []models.Package{
				{Name: x64, CurrentVersion: "14.50.35719.0"},
				{Name: x86, CurrentVersion: "14.50.35719.0"},
			},
			rows: []wingetTestRow{
				{trunc, x64ID, "14.50.35719.0", "14.51.36247.0", "winget"},
				{trunc, x86ID, "14.50.35719.0", "14.51.36247.0", "winget"},
			},
			upgrades:   map[string]string{x64ID: "14.51.36247.0", x86ID: "14.51.36247.0"},
			wantUpdate: map[string]string{x64: "14.51.36247.0", x86: "14.51.36247.0"},
		},
		{
			name: "only x64 behind",
			reg: []models.Package{
				{Name: x64, CurrentVersion: "14.50.35719.0"},
				{Name: "Microsoft Visual C++ v14 Redistributable (x86) - 14.51.36247", CurrentVersion: "14.51.36247.0"},
			},
			rows: []wingetTestRow{
				{trunc, x86ID, "14.51.36247.0", "", "winget"},
				{trunc, x64ID, "14.50.35719.0", "14.51.36247.0", "winget"},
			},
			upgrades:   map[string]string{x64ID: "14.51.36247.0"},
			wantUpdate: map[string]string{x64: "14.51.36247.0", "Microsoft Visual C++ v14 Redistributable (x86) - 14.51.36247": ""},
		},
		{
			name: "UWP VCLibs framework never lends its version to the redistributable",
			reg: []models.Package{
				{Name: x64, CurrentVersion: "14.50.35719.0"},
				{Name: x86, CurrentVersion: "14.50.35719.0"},
			},
			rows: []wingetTestRow{
				{trunc, x64ID, "14.50.35719.0", "", "winget"},
				{trunc, x86ID, "14.50.35719.0", "", "winget"},
				{vclibs, vclibsID, "14.0.33519.0", "14.0.33728.0", "winget"},
			},
			upgrades:   map[string]string{vclibsID: "14.0.33728.0"},
			wantUpdate: map[string]string{x64: "", x86: "", vclibs: "14.0.33728.0"},
		},
		{
			name: "available lower than installed is not an update",
			reg:  []models.Package{{Name: "Algodoo", CurrentVersion: "2.2.4"}},
			rows: []wingetTestRow{
				{"Algodoo", "Algoryx.Algodoo", "2.2.4", "2.2.1", "winget"},
			},
			upgrades:   map[string]string{},
			wantUpdate: map[string]string{"Algodoo": ""},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			merged := collectWindowsApps(t, tc.reg, wingetTable(40, tc.rows), tc.upgrades)
			for name, want := range tc.wantUpdate {
				got := findPackage(t, merged, name)
				if got.NeedsUpdate != (want != "") || got.AvailableVersion != want {
					t.Errorf("%q: NeedsUpdate=%v AvailableVersion=%q, want NeedsUpdate=%v AvailableVersion=%q",
						name, got.NeedsUpdate, got.AvailableVersion, want != "", want)
				}
			}
			for _, p := range merged {
				if strings.HasPrefix(trunc, p.Name) && p.Name != trunc {
					t.Errorf("truncated winget row %q reported alongside the registry entries it belongs to", p.Name)
				}
			}
		})
	}
}

func TestCompareDottedVersions(t *testing.T) {
	cases := []struct {
		a, b   string
		want   int
		wantOK bool
	}{
		{"14.0.33728.0", "14.50.35719.0", -1, true},
		{"14.51.36247.0", "14.50.35719.0", 1, true},
		{"14.50.35719", "14.50.35719.0", 0, true},
		{"2.2.1", "2.2.4", -1, true},
		{"v1.10", "1.9", 1, true},
		{"Unknown", "17.0.1000.7", 0, false},
		{"1.2.3-beta", "1.2.4", 0, false},
	}
	for _, tc := range cases {
		got, ok := compareDottedVersions(tc.a, tc.b)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("compareDottedVersions(%q, %q) = %d, %v; want %d, %v", tc.a, tc.b, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestWingetOnlyRowsSharingANamePreferTheOneBehind(t *testing.T) {
	const vclibs = "Microsoft.VCLibs.140.00.UWPDesktop"
	table := wingetTable(40, []wingetTestRow{
		{vclibs, "Microsoft.VCLibs.Desktop.14", "14.0.33728.0", "", "winget"},
		{vclibs, "Microsoft.VCLibs.Desktop.14", "14.0.33519.0", "14.0.33728.0", "winget"},
	})
	merged := collectWindowsApps(t, nil, table, map[string]string{})
	if len(merged) != 1 {
		t.Fatalf("got %d packages, want 1: %+v", len(merged), merged)
	}
	if !merged[0].NeedsUpdate || merged[0].CurrentVersion != "14.0.33519.0" {
		t.Errorf("reported %+v, want the 14.0.33519.0 copy needing 14.0.33728.0", merged[0])
	}
}

func TestTruncatedPrefixNeedsMatchingVersion(t *testing.T) {
	const reg = "Microsoft Visual C++ v14 Redistributable (x64) - 14.50.35719"
	table := wingetTable(40, []wingetTestRow{
		{"Microsoft Visual C++ v14 Redistributabl…", "Microsoft.VCRedist.2015+.x64", "14.40.33810.0", "14.51.36247.0", "winget"},
	})
	merged := collectWindowsApps(t, []models.Package{{Name: reg, CurrentVersion: "14.50.35719.0"}}, table, map[string]string{})
	got := findPackage(t, merged, reg)
	if got.NeedsUpdate || got.SourceRepository != "local" {
		t.Errorf("registry entry matched a truncated row at another version: %+v", got)
	}
}

func TestFallbackWithoutUpgradeListKeepsNonNumericInequality(t *testing.T) {
	table := wingetTable(40, []wingetTestRow{
		{"Contoso Tool", "Contoso.Tool", "24.08-r1", "24.08-r2", "winget"},
	})
	merged := collectWindowsApps(t, nil, table, nil)
	if got := findPackage(t, merged, "Contoso Tool"); !got.NeedsUpdate || got.AvailableVersion != "24.08-r2" {
		t.Errorf("non-numeric available version lost on the fallback path: %+v", got)
	}
}
