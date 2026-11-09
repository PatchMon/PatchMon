package packages

import (
	"strings"
	"testing"
	"time"

	"patchmon-agent/pkg/models"
)

const testWindowsApps = `C:\Program Files\WindowsApps`

func storeEntry(name, family, loc, kind string) storeAppx {
	return storeAppx{Name: name, PackageFamilyName: family, InstallLocation: loc, SignatureKind: kind}
}

func TestIsStoreApp(t *testing.T) {
	appx := []storeAppx{
		storeEntry("Microsoft.WindowsTerminal", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", testWindowsApps+`\Microsoft.WindowsTerminal_1.21.0.0_x64__8wekyb3d8bbwe`, "Store"),
		storeEntry("NotepadPlusPlus", "NotepadPlusPlus_7njy0v32s6xk6", `C:\Program Files\Notepad++\contextMenu`, "Developer"),
		storeEntry("Microsoft.VCLibs.140.00.UWPDesktop", "Microsoft.VCLibs.140.00.UWPDesktop_8wekyb3d8bbwe", testWindowsApps+`\Microsoft.VCLibs.140.00.UWPDesktop_14.0.33728.0_x64__8wekyb3d8bbwe`, "Store"),
		storeEntry("Microsoft.WindowsCalculator", "Microsoft.WindowsCalculator_8wekyb3d8bbwe", testWindowsApps+`\Microsoft.WindowsCalculator_11.2.0.0_x64__8wekyb3d8bbwe`, "System"),
		storeEntry("Contoso.Tool", "Contoso.Tool_abcdefghijklm", `C:\Program Files\WindowsAppsEvil\Contoso.Tool`, "Store"),
	}
	appx[2].IsFramework = true
	cases := []struct {
		name string
		pkg  models.Package
		want bool
	}{
		{"terminal by Appx name", models.Package{Name: "Windows Terminal", WingetID: "Microsoft.WindowsTerminal"}, true},
		{"store product id", models.Package{Name: "Some app", WingetID: "9NBLGGH4NNS1"}, true},
		{"xp product id", models.Package{Name: "Other", WingetID: "XPDC2RH70K22MN"}, true},
		{"msstore source", models.Package{Name: "App", WingetID: "Foo.Bar", SourceRepository: "msstore"}, true},
		{"family name id", models.Package{Name: "App", WingetID: "Foo.Bar_8wekyb3d8bbwe"}, true},
		{"system-signed inbox app", models.Package{Name: "Calculator", WingetID: "Microsoft.WindowsCalculator"}, true},
		{"sparse shell package is not a Store app", models.Package{Name: "Notepad++", WingetID: "NotepadPlusPlus"}, false},
		{"desktop Notepad++", models.Package{Name: "Notepad++ (64-bit x64)", WingetID: "Notepad++.Notepad++"}, false},
		{"framework package", models.Package{Name: "VCLibs", WingetID: "Microsoft.VCLibs.140.00.UWPDesktop"}, false},
		{"lookalike install location", models.Package{Name: "Tool", WingetID: "Contoso.Tool"}, false},
		{"prefix of a Store app is not a match", models.Package{Name: "Terminal Preview", WingetID: "Microsoft.WindowsTerminal.Preview"}, false},
		{"desktop app", models.Package{Name: "7-Zip", WingetID: "7zip.7zip", SourceRepository: "winget"}, false},
		{"no winget id", models.Package{Name: "Windows Terminal"}, false},
		{"display name alone never matches", models.Package{Name: "Microsoft.WindowsCalculator", WingetID: "Vendor.DesktopCalc"}, false},
	}
	for _, c := range cases {
		if got := isStoreApp(c.pkg, appx, testWindowsApps); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMarkStoreAppsKeepsTheUpdateVisible(t *testing.T) {
	pkgs := []models.Package{{Name: "Windows Terminal", WingetID: "9N0DX20HK701", NeedsUpdate: true, AvailableVersion: "1.21"}}
	markStoreApps(pkgs, nil, testWindowsApps)
	if !pkgs[0].WingetStoreApp || !pkgs[0].NeedsUpdate || pkgs[0].AvailableVersion != "1.21" {
		t.Errorf("got %+v", pkgs[0])
	}
}

func TestParseStoreAppx(t *testing.T) {
	one := parseStoreAppx([]byte("\xEF\xBB\xBF" + `{"Name":"A","PackageFamilyName":"A_x","InstallLocation":"L","SignatureKind":"Store","IsFramework":false,"IsResourcePackage":true}`))
	if len(one) != 1 || !one[0].IsResourcePackage || one[0].SignatureKind != "Store" {
		t.Errorf("single = %+v", one)
	}
	if got := parseStoreAppx([]byte(`[{"Name":"A"},{"Name":"B"}]`)); len(got) != 2 {
		t.Errorf("array = %+v", got)
	}
	if got := parseStoreAppx([]byte("null")); got != nil {
		t.Errorf("null = %+v", got)
	}
}

func TestStoreTarget(t *testing.T) {
	appx := []storeAppx{
		{Name: "Microsoft.WindowsTerminal", PackageFamilyName: "Microsoft.WindowsTerminal_8wekyb3d8bbwe", InstallLocation: testWindowsApps + `\Microsoft.WindowsTerminal_1.18.10301.0_x64__8wekyb3d8bbwe`, SignatureKind: "Store"},
		{Name: "Microsoft.DesktopAppInstaller", PackageFamilyName: "Microsoft.DesktopAppInstaller_8wekyb3d8bbwe", InstallLocation: testWindowsApps + `\Microsoft.DesktopAppInstaller_1.29.379.0_x64__8wekyb3d8bbwe`, SignatureKind: "Store"},
		{Name: "Microsoft.MicrosoftOfficeHub", PackageFamilyName: "Microsoft.MicrosoftOfficeHub_8wekyb3d8bbwe", InstallLocation: testWindowsApps + `\Microsoft.MicrosoftOfficeHub_19.2.0.0_x64__8wekyb3d8bbwe`, SignatureKind: "Store"},
	}
	cases := []struct {
		pkg    models.Package
		target string
		ok     bool
	}{
		{models.Package{WingetID: "9MSMLRH6LZF3"}, "9MSMLRH6LZF3", true},
		{models.Package{WingetID: "Microsoft.Paint_8wekyb3d8bbwe"}, "Microsoft.Paint_8wekyb3d8bbwe", true},
		{models.Package{WingetID: "Microsoft.WindowsTerminal"}, "Microsoft.WindowsTerminal_8wekyb3d8bbwe", true},
		{models.Package{WingetID: "Microsoft.AppInstaller"}, "Microsoft.DesktopAppInstaller_8wekyb3d8bbwe", true},
		{models.Package{WingetID: "microsoft.365copilot"}, "Microsoft.MicrosoftOfficeHub_8wekyb3d8bbwe", true},
		{models.Package{WingetID: "Some.App", SourceRepository: "msstore"}, "", true},
		{models.Package{WingetID: "7zip.7zip"}, "", false},
		{models.Package{}, "", false},
	}
	for _, c := range cases {
		target, ok := storeTarget(c.pkg, appx, testWindowsApps)
		if target != c.target || ok != c.ok {
			t.Errorf("storeTarget(%q) = %q, %v; want %q, %v", c.pkg.WingetID, target, ok, c.target, c.ok)
		}
	}
}

func TestParseStoreUpdateOutput(t *testing.T) {
	out := "\ufeffnoise\r\n{\"state\":\"Completed\",\"pfn\":\"A_b\",\"before\":\"1.0\",\"after\":\"2.0\",\"installedForUser\":true}\r\n"
	res, err := parseStoreUpdateOutput([]byte(out))
	if err != nil || res.State != "Completed" || res.After != "2.0" || !res.InstalledForUser {
		t.Fatalf("got %+v, %v", res, err)
	}
	if _, err := parseStoreUpdateOutput([]byte("nothing")); err == nil {
		t.Fatal("no JSON line parsed without error")
	}
}

func TestClassifyStoreUpdate(t *testing.T) {
	wait := 29 * time.Minute
	cases := []struct {
		res     storeUpdateResult
		status  string
		message string
		hresult string
	}{
		{storeUpdateResult{State: "Completed", Before: "1.0", After: "2.0", InstalledForUser: true}, itemInstalled, msgStoreUpdated, ""},
		{storeUpdateResult{State: "Completed", Before: "1.0", After: "2.0"}, itemInstalled, msgStoreStaged, ""},
		{storeUpdateResult{State: "Completed", Before: "1.0", After: "1.0"}, itemFailed, "Microsoft Store reported the update complete, but the installed version did not change", ""},
		{storeUpdateResult{State: "NoUpdate"}, itemNotApplicable, msgStoreNoUpdate, ""},
		{storeUpdateResult{State: "Unavailable"}, itemSkipped, msgStoreUnavailable, ""},
		{storeUpdateResult{State: "Error", ErrorCode: "0x80073CF9"}, itemFailed, "Microsoft Store update failed", "0x80073cf9"},
		{storeUpdateResult{State: "Canceled"}, itemFailed, "Microsoft Store cancelled the update", ""},
		{storeUpdateResult{State: "ReadyToDownload"}, itemFailed, "Microsoft Store did not finish the update within 29 minutes (last state: ReadyToDownload); it may still complete", ""},
		{storeUpdateResult{State: "PausedWiFiRequired"}, itemFailed, "Microsoft Store paused the update (PausedWiFiRequired)", ""},
	}
	for _, c := range cases {
		status, message, hr := classifyStoreUpdate(c.res, wait)
		if status != c.status || message != c.message || hr != c.hresult {
			t.Errorf("classifyStoreUpdate(%+v) = %q, %q, %q", c.res, status, message, hr)
		}
	}
}

// These WinGet IDs differ from their installed Appx names. The patch runner
// also calls the classifier with only a title and ID, so names are not evidence.
func TestStoreAppWingetAliases(t *testing.T) {
	for _, tc := range []struct{ id, name string }{
		{"Microsoft.AppInstaller", "Microsoft.DesktopAppInstaller"},
		{"Microsoft.365Copilot", "Microsoft.MicrosoftOfficeHub"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			family := tc.name + "_8wekyb3d8bbwe"
			app := storeEntry(tc.name, family, testWindowsApps+`\`+tc.name+`_1.0.0.0_x64__8wekyb3d8bbwe`, "Store")
			pkg := models.Package{Name: tc.name, WingetID: tc.id, NeedsUpdate: true, AvailableVersion: "2.0"}
			wrongPublisher := app
			wrongPublisher.PackageFamilyName = tc.name + "_abcdefghijklm"
			cases := []struct {
				name string
				app  storeAppx
				id   string
				want bool
			}{
				{"verified family", app, tc.id, true},
				{"case insensitive ID", app, strings.ToLower(tc.id), true},
				{"different ID", app, "Vendor.DesktopApp", false},
				{"different publisher", wrongPublisher, tc.id, false},
			}
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					p := pkg
					p.WingetID = c.id
					p.Name = ""
					if got := isStoreApp(p, []storeAppx{c.app}, testWindowsApps); got != c.want {
						t.Fatalf("got %v, want %v", got, c.want)
					}
				})
			}
			for _, mutate := range []func(*storeAppx){
				func(a *storeAppx) { a.SignatureKind = "Developer" },
				func(a *storeAppx) { a.InstallLocation = `C:\Other\App` },
				func(a *storeAppx) { a.IsFramework = true },
				func(a *storeAppx) { a.IsResourcePackage = true },
			} {
				changed := app
				mutate(&changed)
				if isStoreApp(pkg, []storeAppx{changed}, testWindowsApps) {
					t.Error("accepted non-Store installation")
				}
			}
			if isStoreApp(pkg, nil, testWindowsApps) {
				t.Error("alias alone is insufficient")
			}
			pkgs := []models.Package{pkg}
			markStoreApps(pkgs, []storeAppx{app}, testWindowsApps)
			if !pkgs[0].WingetStoreApp || !pkgs[0].NeedsUpdate || pkgs[0].AvailableVersion != "2.0" {
				t.Fatalf("inventory update lost: %+v", pkgs[0])
			}
		})
	}
}
