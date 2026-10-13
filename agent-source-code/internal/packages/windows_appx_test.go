package packages

import (
	"testing"

	"patchmon-agent/pkg/models"
)

func issue1149Appx() []appxPackage {
	return []appxPackage{
		{"Microsoft.WindowsAppRuntime.1.7", "7000.522.1444.0", "Installed"},
		{"Microsoft.WindowsAppRuntime.1.7", "7000.785.2325.0", "Installed"},
		{"Microsoft.WindowsAppRuntime.1.7", "7000.522.1444.0", "Installed"},
		{"Microsoft.WindowsAppRuntime.1.7", "7000.770.750.0", "Staged"},
		{"Microsoft.WindowsAppRuntime.1.7", "7000.785.2325.0", "Installed"},
		{"Microsoft.VCLibs.140.00.UWPDesktop", "14.0.30704.0", "Staged"},
		{"Microsoft.VCLibs.140.00", "14.0.30704.0", "Staged"},
		{"Microsoft.VCLibs.140.00", "14.0.33519.0", "Installed"},
		{"Microsoft.VCLibs.140.00.UWPDesktop", "14.0.33728.0", "Installed"},
		{"Microsoft.VCLibs.140.00", "14.0.33519.0", "Installed"},
		{"Microsoft.VCLibs.140.00.UWPDesktop", "14.0.33728.0", "Installed"},
	}
}

func TestApplyAppxInstallState(t *testing.T) {
	cases := []struct {
		name      string
		pkg       models.Package
		appx      []appxPackage
		wantVer   string
		wantAvail string
		wantNeeds bool
	}{
		{
			name:    "staged copy in the same scheme takes the installed version",
			pkg:     models.Package{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true},
			appx:    issue1149Appx(),
			wantVer: "14.0.33519.0",
		},
		{
			name:    "UWPDesktop is matched by its own name",
			pkg:     models.Package{Name: "Microsoft.VCLibs.140.00.UWPDesktop", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33728.0", NeedsUpdate: true},
			appx:    issue1149Appx(),
			wantVer: "14.0.33728.0",
		},
		{
			name:    "different version scheme reports the installed package version, not flagged",
			pkg:     models.Package{Name: "Microsoft.WindowsAppRuntime.1.7", CurrentVersion: "1.7.8", AvailableVersion: "1.7.9", NeedsUpdate: true},
			appx:    issue1149Appx(),
			wantVer: "7000.785.2325.0",
		},
		{
			name:    "truncated winget name matches a unique package",
			pkg:     models.Package{Name: "Microsoft.VCLibs.140.00.UWPDesk", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33728.0", NeedsUpdate: true},
			appx:    issue1149Appx(),
			wantVer: "14.0.33728.0",
		},
		{
			name:      "installed version still behind stays flagged",
			pkg:       models.Package{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.40000.0", NeedsUpdate: true},
			appx:      issue1149Appx(),
			wantVer:   "14.0.33519.0",
			wantAvail: "14.0.40000.0",
			wantNeeds: true,
		},
		{
			name:      "no staged copy leaves the row untouched",
			pkg:       models.Package{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.33519.0", AvailableVersion: "14.0.40000.0", NeedsUpdate: true},
			appx:      []appxPackage{{"Microsoft.VCLibs.140.00", "14.0.33519.0", "Installed"}},
			wantVer:   "14.0.33519.0",
			wantAvail: "14.0.40000.0",
			wantNeeds: true,
		},
		{
			name: "staged copy newer than installed leaves the row untouched",
			pkg:  models.Package{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.VCLibs.140.00", "14.0.30704.0", "Installed"},
				{"Microsoft.VCLibs.140.00", "14.0.33519.0", "Staged"},
			},
			wantVer:   "14.0.30704.0",
			wantAvail: "14.0.33519.0",
			wantNeeds: true,
		},
		{
			name: "copy installed for one user and staged for another counts as installed",
			pkg:  models.Package{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.VCLibs.140.00", "14.0.30704.0", "Staged"},
				{"Microsoft.VCLibs.140.00", "14.0.33519.0", "Installed,Staged"},
			},
			wantVer: "14.0.33519.0",
		},
		{
			name: "winget version that is not a staged copy is untouched",
			pkg:  models.Package{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.40000.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.VCLibs.140.00", "14.0.32000.0", "Staged"},
				{"Microsoft.VCLibs.140.00", "14.0.31000.0", "Staged"},
				{"Microsoft.VCLibs.140.00", "14.0.33519.0", "Installed"},
			},
			wantVer:   "14.0.30704.0",
			wantAvail: "14.0.40000.0",
			wantNeeds: true,
		},
		{
			name: "any of several staged copies is recognised",
			pkg:  models.Package{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.VCLibs.140.00", "14.0.32000.0", "Staged"},
				{"Microsoft.VCLibs.140.00", "14.0.30704.0", "Staged"},
				{"Microsoft.VCLibs.140.00", "14.0.33519.0", "Installed"},
			},
			wantVer: "14.0.33519.0",
		},
		{
			name: "different scheme with a staged copy newer than installed stays flagged",
			pkg:  models.Package{Name: "Microsoft.WindowsAppRuntime.1.7", CurrentVersion: "1.7.8", AvailableVersion: "1.7.9", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.WindowsAppRuntime.1.7", "7000.770.750.0", "Installed"},
				{"Microsoft.WindowsAppRuntime.1.7", "7000.785.2325.0", "Staged"},
			},
			wantVer:   "1.7.8",
			wantAvail: "1.7.9",
			wantNeeds: true,
		},
		{
			name:      "non-numeric winget version is never suppressed",
			pkg:       models.Package{Name: "Microsoft.WindowsAppRuntime.1.7", CurrentVersion: "Unknown", AvailableVersion: "1.7.9", NeedsUpdate: true},
			appx:      issue1149Appx(),
			wantVer:   "Unknown",
			wantAvail: "1.7.9",
			wantNeeds: true,
		},
		{
			name: "display names with spaces are never touched",
			pkg:  models.Package{Name: "Microsoft Visual C++ 2015 UWP Desktop Runtime Package", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33728.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft Visual C++ 2015 UWP Desktop Runtime Package", "14.0.30704.0", "Staged"},
				{"Microsoft Visual C++ 2015 UWP Desktop Runtime Package", "14.0.33728.0", "Installed"},
			},
			wantVer:   "14.0.30704.0",
			wantAvail: "14.0.33728.0",
			wantNeeds: true,
		},
		{
			name: "prefix match with a different version is untouched",
			pkg:  models.Package{Name: "Microsoft.Edge", CurrentVersion: "120.0.1.0", AvailableVersion: "121.0.1.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.EdgeWebView", "1.0.0.0", "Staged"},
				{"Microsoft.EdgeWebView", "2.0.0.0", "Installed"},
			},
			wantVer:   "120.0.1.0",
			wantAvail: "121.0.1.0",
			wantNeeds: true,
		},
		{
			name:    "unflagged row reporting a staged copy takes the installed version",
			pkg:     models.Package{Name: "Microsoft.WindowsAppRuntime.1.7", CurrentVersion: "7000.770.750.0"},
			appx:    issue1149Appx(),
			wantVer: "7000.785.2325.0",
		},
		{
			name:    "unflagged row in a different scheme is untouched",
			pkg:     models.Package{Name: "Microsoft.WindowsAppRuntime.1.7", CurrentVersion: "1.7.8"},
			appx:    issue1149Appx(),
			wantVer: "1.7.8",
		},
		{
			name:      "unrelated app is untouched",
			pkg:       models.Package{Name: "Mozilla Firefox", CurrentVersion: "120.0", AvailableVersion: "121.0", NeedsUpdate: true},
			appx:      issue1149Appx(),
			wantVer:   "120.0",
			wantAvail: "121.0",
			wantNeeds: true,
		},
		{
			name:      "no Appx data leaves the row untouched",
			pkg:       models.Package{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true},
			wantVer:   "14.0.30704.0",
			wantAvail: "14.0.33519.0",
			wantNeeds: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkgs := []models.Package{tc.pkg}
			_ = applyAppxInstallState(pkgs, tc.appx)
			got := pkgs[0]
			if got.CurrentVersion != tc.wantVer || got.AvailableVersion != tc.wantAvail || got.NeedsUpdate != tc.wantNeeds {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)", got.CurrentVersion, got.AvailableVersion, got.NeedsUpdate, tc.wantVer, tc.wantAvail, tc.wantNeeds)
			}
		})
	}
}

func TestApplyAppxInstallStateAmbiguousPrefix(t *testing.T) {
	pkgs := []models.Package{{Name: "Microsoft.VCLibs.140", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true}}
	_ = applyAppxInstallState(pkgs, issue1149Appx())
	if !pkgs[0].NeedsUpdate || pkgs[0].CurrentVersion != "14.0.30704.0" {
		t.Errorf("ambiguous prefix should not match, got %+v", pkgs[0])
	}
}

func TestApplyAppxInstallStateReportsSuppressed(t *testing.T) {
	pkgs := []models.Package{
		{Name: "Microsoft.WindowsAppRuntime.1.7", CurrentVersion: "1.7.8", AvailableVersion: "1.7.9", NeedsUpdate: true},
		{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true},
	}
	pkgs = append(pkgs, models.Package{Name: "Microsoft.VCLibs.140.00.UWPDesktop", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.40000.0", NeedsUpdate: true})
	got := applyAppxInstallState(pkgs, issue1149Appx())
	want := []string{"Microsoft.WindowsAppRuntime.1.7", "Microsoft.VCLibs.140.00"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("suppressed = %v, want %v", got, want)
	}
}

func TestHasAppxUpdateCandidate(t *testing.T) {
	cases := []struct {
		name string
		pkgs []models.Package
		want bool
	}{
		{"flagged Appx name", []models.Package{{Name: "Microsoft.VCLibs.140.00", NeedsUpdate: true}}, true},
		{"unflagged Appx name", []models.Package{{Name: "Microsoft.VCLibs.140.00"}}, true},
		{"display name", []models.Package{{Name: "Mozilla Firefox", NeedsUpdate: true}}, false},
		{"name without a dot", []models.Package{{Name: "7zip", NeedsUpdate: true}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasAppxStyleName(tc.pkgs); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseAppxPackages(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"array", `[{"Name":"A","Version":"1.0.0.0","States":"Installed"},{"Name":"B","Version":"2.0.0.0","States":"Staged"}]`, 2},
		{"single object", `{"Name":"A","Version":"1.0.0.0","States":"Installed"}`, 1},
		{"BOM", "\ufeff" + `[{"Name":"A","Version":"1.0.0.0","States":""}]`, 1},
		{"empty", "", 0},
		{"null", "null", 0},
		{"garbage", "not json", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseAppxPackages([]byte(tc.in)); len(got) != tc.want {
				t.Errorf("got %d packages, want %d", len(got), tc.want)
			}
		})
	}
}
