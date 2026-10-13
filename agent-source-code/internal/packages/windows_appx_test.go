package packages

import (
	"io"
	"reflect"
	"testing"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
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
		wantDrop  bool
		origin    appOrigin
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
			name: "staged copy newer than every installed one takes the installed version and stays flagged",
			pkg:  models.Package{Name: "Microsoft.WindowsTerminal", CurrentVersion: "1.24.12741.0", AvailableVersion: "1.25.2733.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.WindowsTerminal", "1.18.10301.0", "Installed"},
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
			},
			wantVer:   "1.18.10301.0",
			wantAvail: "1.25.2733.0",
			wantNeeds: true,
		},
		{
			name: "unflagged row reporting a newer staged copy is flagged for the staged version",
			pkg:  models.Package{Name: "Microsoft.WindowsTerminal", CurrentVersion: "1.24.12741.0"},
			appx: []appxPackage{
				{"Microsoft.WindowsTerminal", "1.18.10301.0", "Installed"},
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
			},
			wantVer:   "1.18.10301.0",
			wantAvail: "1.24.12741.0",
			wantNeeds: true,
		},
		{
			name:    "registry row matching a staged-only package is kept",
			pkg:     models.Package{Name: "Contoso.Widget", CurrentVersion: "3.1.0.0"},
			origin:  appOrigin{registry: true},
			appx:    []appxPackage{{"Contoso.Widget", "3.1.0.0", "Staged"}},
			wantVer: "3.1.0.0",
		},
		{
			name: "a paused copy counts as present and is never dropped",
			pkg:  models.Package{Name: "Microsoft.WindowsTerminal", CurrentVersion: "1.24.12741.0"},
			appx: []appxPackage{
				{"Microsoft.WindowsTerminal", "1.18.10301.0", "Paused"},
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
			},
			wantVer: "1.24.12741.0",
		},
		{
			name:    "a copy paused for one user and staged for another is not staged-only",
			pkg:     models.Package{Name: "Microsoft.BingNews", CurrentVersion: "4.55.62231.0"},
			appx:    []appxPackage{{"Microsoft.BingNews", "4.55.62231.0", "Staged,Paused"}},
			wantVer: "4.55.62231.0",
		},
		{
			name:     "NotInstalled alongside Staged is still staged-only",
			pkg:      models.Package{Name: "Microsoft.BingNews", CurrentVersion: "4.55.62231.0"},
			appx:     []appxPackage{{"Microsoft.BingNews", "4.55.62231.0", "Staged,NotInstalled"}},
			wantDrop: true,
		},
		{
			name:   "friendly name matched by its package family Id",
			pkg:    models.Package{Name: "Windows Terminal", CurrentVersion: "1.24.12741.0", AvailableVersion: "1.25.2733.0", NeedsUpdate: true},
			origin: appOrigin{wingetID: "Microsoft.WindowsTerminal_8wekyb3d8bbwe"},
			appx: []appxPackage{
				{"Microsoft.WindowsTerminal", "1.18.10301.0", "Installed"},
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
			},
			wantVer:   "1.18.10301.0",
			wantAvail: "1.25.2733.0",
			wantNeeds: true,
		},
		{
			name:     "staged-only friendly name matched by its package family Id is dropped",
			pkg:      models.Package{Name: "Microsoft News", CurrentVersion: "4.55.62231.0"},
			origin:   appOrigin{wingetID: "Microsoft.BingNews_8wekyb3d8bbwe"},
			appx:     []appxPackage{{"Microsoft.BingNews", "4.55.62231.0", "Staged"}},
			wantDrop: true,
		},
		{
			name:   "friendly name with a non-family Id is untouched",
			pkg:    models.Package{Name: "Windows Terminal", CurrentVersion: "1.24.12741.0", AvailableVersion: "1.25.2733.0", NeedsUpdate: true},
			origin: appOrigin{wingetID: "Microsoft.WindowsTerminal"},
			appx: []appxPackage{
				{"Microsoft.WindowsTerminal", "1.18.10301.0", "Installed"},
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
			},
			wantVer:   "1.24.12741.0",
			wantAvail: "1.25.2733.0",
			wantNeeds: true,
		},
		{
			name:   "registry row carrying a matched winget family Id keeps its own version",
			pkg:    models.Package{Name: "Microsoft Teams classic", CurrentVersion: "1.7.00.13456", AvailableVersion: "1.8.00.1234", NeedsUpdate: true},
			origin: appOrigin{registry: true, wingetID: "MSTeams_8wekyb3d8bbwe"},
			appx: []appxPackage{
				{"MSTeams", "24295.605.3225.8804", "Installed"},
				{"MSTeams", "24243.1309.3132.617", "Staged"},
			},
			wantVer:   "1.7.00.13456",
			wantAvail: "1.8.00.1234",
			wantNeeds: true,
		},
		{
			name:   "family Id is matched exactly before the name prefix",
			pkg:    models.Package{Name: "Microsoft.Bing", CurrentVersion: "4.55.62231.0"},
			origin: appOrigin{wingetID: "Microsoft.BingNews_8wekyb3d8bbwe"},
			appx: []appxPackage{
				{"Microsoft.BingNews", "4.55.62231.0", "Staged"},
				{"Microsoft.BingWeather", "4.54.63045.0", "Staged"},
			},
			wantDrop: true,
		},
		{
			name:     "friendly name with a family Id that is only a prefix is untouched",
			pkg:      models.Package{Name: "Microsoft News", CurrentVersion: "4.55.62231.0"},
			origin:   appOrigin{wingetID: "Microsoft.Bing_8wekyb3d8bbwe"},
			appx:     []appxPackage{{"Microsoft.BingNews", "4.55.62231.0", "Staged"}},
			wantVer:  "4.55.62231.0",
			wantDrop: false,
		},
		{
			name: "mixed architectures take the highest installed version",
			pkg:  models.Package{Name: "Microsoft.WindowsTerminal", CurrentVersion: "1.24.12741.0", AvailableVersion: "1.25.2733.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.WindowsTerminal", "1.18.10301.0", "Installed"},
				{"Microsoft.WindowsTerminal", "1.20.11781.0", "Installed"},
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
			},
			wantVer:   "1.20.11781.0",
			wantAvail: "1.25.2733.0",
			wantNeeds: true,
		},
		{
			name: "same version installed for one architecture and staged for another is untouched",
			pkg:  models.Package{Name: "Microsoft.WindowsTerminal", CurrentVersion: "1.24.12741.0", AvailableVersion: "1.25.2733.0", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Installed"},
				{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
			},
			wantVer:   "1.24.12741.0",
			wantAvail: "1.25.2733.0",
			wantNeeds: true,
		},
		{
			name:     "staged for every user and installed for none is dropped",
			pkg:      models.Package{Name: "Microsoft.BingNews", CurrentVersion: "4.55.62231.0", AvailableVersion: "4.56.0.0", NeedsUpdate: true},
			appx:     []appxPackage{{"Microsoft.BingNews", "4.55.62231.0", "Staged"}},
			wantDrop: true,
		},
		{
			name:     "staged-only in a different version scheme is dropped",
			pkg:      models.Package{Name: "Microsoft.WindowsAppRuntime.1.8", CurrentVersion: "1.8.3", AvailableVersion: "1.8.4", NeedsUpdate: true},
			appx:     []appxPackage{{"Microsoft.WindowsAppRuntime.1.8", "8000.616.304.0", "Staged"}},
			wantDrop: true,
		},
		{
			name:     "staged-only matched by a truncated name and the same version is dropped",
			pkg:      models.Package{Name: "Microsoft.BingWea", CurrentVersion: "4.54.63045.0"},
			appx:     []appxPackage{{"Microsoft.BingWeather", "4.54.63045.0", "Staged"}},
			wantDrop: true,
		},
		{
			name:    "staged-only prefix match with a different version is kept",
			pkg:     models.Package{Name: "Microsoft.Edge", CurrentVersion: "120.0.1.0"},
			appx:    []appxPackage{{"Microsoft.EdgeWebView", "1.0.0.0", "Staged"}},
			wantVer: "120.0.1.0",
		},
		{
			name:     "non-numeric winget version with an exact staged-only match is dropped",
			pkg:      models.Package{Name: "Microsoft.WindowsAppRuntime.1.8", CurrentVersion: "> 1.8.10"},
			appx:     []appxPackage{{"Microsoft.WindowsAppRuntime.1.8", "8000.616.304.0", "Staged"}},
			wantDrop: true,
		},
		{
			name:     "non-numeric winget version with an exact family Id staged-only match is dropped",
			pkg:      models.Package{Name: "Windows App Runtime 1.8", CurrentVersion: "> 1.8.10"},
			origin:   appOrigin{wingetID: "Microsoft.WindowsAppRuntime.1.8_8wekyb3d8bbwe"},
			appx:     []appxPackage{{"Microsoft.WindowsAppRuntime.1.8", "8000.616.304.0", "Staged"}},
			wantDrop: true,
		},
		{
			name:    "non-numeric winget version with only a prefix staged-only match is kept",
			pkg:     models.Package{Name: "Microsoft.WindowsAppRuntime.1", CurrentVersion: "> 1.8.10"},
			appx:    []appxPackage{{"Microsoft.WindowsAppRuntime.1.8", "8000.616.304.0", "Staged"}},
			wantVer: "> 1.8.10",
		},
		{
			name:    "non-numeric winget version on a registry row with an exact staged-only match is kept",
			pkg:     models.Package{Name: "Microsoft.WindowsAppRuntime.1.8", CurrentVersion: "> 1.8.10"},
			origin:  appOrigin{registry: true},
			appx:    []appxPackage{{"Microsoft.WindowsAppRuntime.1.8", "8000.616.304.0", "Staged"}},
			wantVer: "> 1.8.10",
		},
		{
			name: "non-numeric winget version with an installed copy is untouched",
			pkg:  models.Package{Name: "Microsoft.WindowsAppRuntime.1.8", CurrentVersion: "> 1.8.10", AvailableVersion: "1.8.11", NeedsUpdate: true},
			appx: []appxPackage{
				{"Microsoft.WindowsAppRuntime.1.8", "8000.616.304.0", "Staged"},
				{"Microsoft.WindowsAppRuntime.1.8", "8000.731.1532.0", "Installed"},
			},
			wantVer:   "> 1.8.10",
			wantAvail: "1.8.11",
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
			kept, _, dropped := applyAppxInstallState([]models.Package{tc.pkg}, []appOrigin{tc.origin}, tc.appx)
			if tc.wantDrop {
				if len(kept) != 0 || len(dropped) != 1 || dropped[0] != tc.pkg.Name {
					t.Errorf("want dropped, got kept %+v, dropped %v", kept, dropped)
				}
				return
			}
			if len(kept) != 1 || len(dropped) != 0 {
				t.Fatalf("want kept, got kept %+v, dropped %v", kept, dropped)
			}
			got := kept[0]
			if got.CurrentVersion != tc.wantVer || got.AvailableVersion != tc.wantAvail || got.NeedsUpdate != tc.wantNeeds {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)", got.CurrentVersion, got.AvailableVersion, got.NeedsUpdate, tc.wantVer, tc.wantAvail, tc.wantNeeds)
			}
		})
	}
}

func TestApplyAppxInstallStateAmbiguousPrefix(t *testing.T) {
	pkgs, _, _ := applyAppxInstallState([]models.Package{{Name: "Microsoft.VCLibs.140", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true}}, nil, issue1149Appx())
	if len(pkgs) != 1 {
		t.Fatalf("want one row, got %+v", pkgs)
	}
	if !pkgs[0].NeedsUpdate || pkgs[0].CurrentVersion != "14.0.30704.0" {
		t.Errorf("ambiguous prefix should not match, got %+v", pkgs[0])
	}
}

func TestApplyAppxInstallStateReportsSuppressed(t *testing.T) {
	pkgs := []models.Package{
		{Name: "Microsoft.WindowsAppRuntime.1.7", CurrentVersion: "1.7.8", AvailableVersion: "1.7.9", NeedsUpdate: true},
		{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true},
		{Name: "Microsoft.VCLibs.140.00.UWPDesktop", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.40000.0", NeedsUpdate: true},
	}
	_, got, _ := applyAppxInstallState(pkgs, nil, issue1149Appx())
	want := []string{"Microsoft.WindowsAppRuntime.1.7", "Microsoft.VCLibs.140.00"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("suppressed = %v, want %v", got, want)
	}
}

func TestApplyAppxInstallStateDropsStagedOnlyAndKeepsOrder(t *testing.T) {
	appx := append(issue1149Appx(),
		appxPackage{"Microsoft.WindowsTerminal", "1.18.10301.0", "Installed"},
		appxPackage{"Microsoft.WindowsTerminal", "1.24.12741.0", "Staged"},
		appxPackage{"Microsoft.BingNews", "4.55.62231.0", "Staged"},
		appxPackage{"Microsoft.ZuneVideo", "10.22091.10061.0", "Staged"},
	)
	pkgs := []models.Package{
		{Name: "7-Zip 26.02 (arm64)", CurrentVersion: "26.02", AvailableVersion: "26.04", NeedsUpdate: true},
		{Name: "Microsoft.BingNews", CurrentVersion: "4.55.62231.0"},
		{Name: "Microsoft.WindowsTerminal", CurrentVersion: "1.24.12741.0", AvailableVersion: "1.25.2733.0", NeedsUpdate: true},
		{Name: "Microsoft.ZuneVideo", CurrentVersion: "10.22091.10061.0"},
		{Name: "Microsoft.VCLibs.140.00", CurrentVersion: "14.0.30704.0", AvailableVersion: "14.0.33519.0", NeedsUpdate: true},
	}
	kept, suppressed, dropped := applyAppxInstallState(pkgs, nil, appx)
	var names []string
	for _, p := range kept {
		names = append(names, p.Name+"@"+p.CurrentVersion)
	}
	wantKept := []string{"7-Zip 26.02 (arm64)@26.02", "Microsoft.WindowsTerminal@1.18.10301.0", "Microsoft.VCLibs.140.00@14.0.33519.0"}
	if !reflect.DeepEqual(names, wantKept) {
		t.Errorf("kept = %v, want %v", names, wantKept)
	}
	if want := []string{"Microsoft.BingNews", "Microsoft.ZuneVideo"}; !reflect.DeepEqual(dropped, want) {
		t.Errorf("dropped = %v, want %v", dropped, want)
	}
	if want := []string{"Microsoft.VCLibs.140.00"}; !reflect.DeepEqual(suppressed, want) {
		t.Errorf("suppressed = %v, want %v", suppressed, want)
	}
	if !kept[1].NeedsUpdate || kept[1].AvailableVersion != "1.25.2733.0" {
		t.Errorf("Terminal = %+v, want still flagged for 1.25.2733.0", kept[1])
	}
}

func TestHasAppxStyleName(t *testing.T) {
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

func TestAppxNameFromFamily(t *testing.T) {
	cases := map[string]string{
		"Microsoft.WindowsTerminal_8wekyb3d8bbwe":                        "Microsoft.WindowsTerminal",
		"Clipchamp.Clipchamp_yxz26nhyzhsrt":                              "Clipchamp.Clipchamp",
		"Microsoft.WindowsTerminal":                                      "",
		"9N0DX20HK701":                                                   "",
		"Microsoft.WindowsTerminal_8wekyb3d8bbw":                         "",
		"Microsoft.WindowsTerminal_8WEKYB3D8BBWE":                        "",
		`MSIX\Microsoft.WindowsTerminal_1.18.10301.0_x64__8wekyb3d8bbwe`: "",
		"": "",
	}
	for id, want := range cases {
		got, ok := appxNameFromFamily(id)
		if got != want || ok != (want != "") {
			t.Errorf("appxNameFromFamily(%q) = %q, %v; want %q", id, got, ok, want)
		}
	}
}

func TestNeedsAppxStateForFamilyIDOnly(t *testing.T) {
	pkgs := []models.Package{{Name: "Windows Terminal"}}
	if needsAppxState(pkgs, []appOrigin{{}}) {
		t.Error("display name without a family Id must not trigger the Appx query")
	}
	if !needsAppxState(pkgs, []appOrigin{{wingetID: "Microsoft.WindowsTerminal_8wekyb3d8bbwe"}}) {
		t.Error("a package family Id must trigger the Appx query")
	}
	if needsAppxState(pkgs, []appOrigin{{registry: true, wingetID: "Microsoft.WindowsTerminal_8wekyb3d8bbwe"}}) {
		t.Error("a family Id on a registry row must not trigger the Appx query")
	}
}

func TestMergeKeepsRegistryRowsOutOfTheStagedOnlyDrop(t *testing.T) {
	m := &WindowsManager{logger: logrus.New()}
	m.logger.SetOutput(io.Discard)
	reg := []models.Package{{Name: "Contoso.Widget", CurrentVersion: "3.1.0.0"}}
	rows := []wingetRow{
		{pkg: models.Package{Name: "Contoso.Widget", CurrentVersion: "3.1.0.0", SourceRepository: "winget"}, id: "Contoso.Widget"},
		{pkg: models.Package{Name: "Microsoft.BingNews", CurrentVersion: "4.55.62231.0", SourceRepository: "winget"}, id: "Microsoft.BingNews_8wekyb3d8bbwe"},
	}
	merged, origins := m.mergeRegistryAndWinget(reg, rows)
	if len(origins) != len(merged) || !origins[0].registry || origins[0].wingetID != "Contoso.Widget" || origins[1].registry {
		t.Fatalf("origins = %+v for %+v", origins, merged)
	}
	appx := []appxPackage{
		{"Contoso.Widget", "3.1.0.0", "Staged"},
		{"Microsoft.BingNews", "4.55.62231.0", "Staged"},
	}
	kept, _, dropped := applyAppxInstallState(merged, origins, appx)
	if len(kept) != 1 || kept[0].Name != "Contoso.Widget" || !reflect.DeepEqual(dropped, []string{"Microsoft.BingNews"}) {
		t.Errorf("kept = %+v, dropped = %v", kept, dropped)
	}
}
