package packages

import (
	"testing"

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
