package packages

import (
	"encoding/json"
	"testing"
)

const testWingetDir = `C:\Program Files\WindowsApps\Microsoft.DesktopAppInstaller_1.21.10121.0_x64__8wekyb3d8bbwe`

func trustedPackage() wingetAppxPackage {
	return wingetAppxPackage{
		Publisher:       microsoftPublisherName,
		SignatureKind:   "Store",
		Family:          wingetPackageFamily,
		InstallLocation: testWingetDir,
	}
}

func TestEvaluateWingetTrust(t *testing.T) {
	exe := testWingetDir + `\winget.exe`
	with := func(f func(*wingetAppxPackage)) wingetTrustReport {
		p := trustedPackage()
		f(&p)
		return wingetTrustReport{Authenticode: "NotSigned", Packages: wingetAppxPackages{p}}
	}
	cases := []struct {
		name   string
		exe    string
		report wingetTrustReport
		ok     bool
	}{
		{"Server 2025: package trusted, file NotSigned", exe, with(func(*wingetAppxPackage) {}), true},
		{"System signature kind", exe, with(func(p *wingetAppxPackage) { p.SignatureKind = "System" }), true},
		{"path case and slashes differ", `c:/program files/windowsapps/microsoft.desktopappinstaller_1.21.10121.0_x64__8wekyb3d8bbwe/WINGET.EXE`, with(func(p *wingetAppxPackage) { p.InstallLocation += `\` }), true},
		{"developer signed", exe, with(func(p *wingetAppxPackage) { p.SignatureKind = "Developer" }), false},
		{"wrong publisher", exe, with(func(p *wingetAppxPackage) { p.Publisher = "CN=Microsoft Corporation" }), false},
		{"wrong family", exe, with(func(p *wingetAppxPackage) { p.Family = "Microsoft.DesktopAppInstaller_evil" }), false},
		{"exe outside the package", `C:\Users\Public\winget.exe`, with(func(*wingetAppxPackage) {}), false},
		{"sibling dir sharing a prefix", testWingetDir + `evil\winget.exe`, with(func(*wingetAppxPackage) {}), false},
		{"dot-dot escape", testWingetDir + `\..\evil\winget.exe`, with(func(*wingetAppxPackage) {}), false},
		{"empty install location", exe, with(func(p *wingetAppxPackage) { p.InstallLocation = "" }), false},
		{"no package, file signed by Microsoft", exe, wingetTrustReport{Authenticode: "Valid", Signer: "CN=Microsoft Corporation, O=Microsoft Corporation, L=Redmond, S=Washington, C=US"}, true},
		{"no package, file signed by someone else", exe, wingetTrustReport{Authenticode: "Valid", Signer: "CN=Evil, O=Evil"}, false},
		{"no package, NotSigned", exe, wingetTrustReport{Authenticode: "NotSigned"}, false},
	}
	for _, c := range cases {
		err := evaluateWingetTrust(c.exe, c.report)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestWingetTrustReportJSON(t *testing.T) {
	var one wingetTrustReport
	if err := json.Unmarshal([]byte(`{"authenticode":"NotSigned","signer":"","packages":{"publisher":"P","signature_kind":"Store","family":"F","install_location":"L"}}`), &one); err != nil || len(one.Packages) != 1 || one.Packages[0].SignatureKind != "Store" {
		t.Errorf("single package object = %+v, %v", one, err)
	}
	var many wingetTrustReport
	if err := json.Unmarshal([]byte(`{"authenticode":"Valid","packages":[{"family":"A"},{"family":"B"}]}`), &many); err != nil || len(many.Packages) != 2 {
		t.Errorf("package array = %+v, %v", many, err)
	}
	var none wingetTrustReport
	if err := json.Unmarshal([]byte(`{"authenticode":"NotSigned","packages":[]}`), &none); err != nil || len(none.Packages) != 0 {
		t.Errorf("empty = %+v, %v", none, err)
	}
}

func TestChooseWingetDirSkipsSplitResourceDirs(t *testing.T) {
	root := "C:/Program Files/WindowsApps/"
	dirs := []string{
		root + "Microsoft.DesktopAppInstaller_1.21.10121.0_neutral_split.scale-100_8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_1.21.10121.0_neutral_split.language-en_8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_2026.917.151.0_neutral_~_8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_1.21.10121.0_x64__8wekyb3d8bbwe",
	}
	d, ok := chooseWingetDir(dirs, "x64")
	if !ok || d.path != dirs[3] {
		t.Errorf("chose %+v, want the x64 package dir", d)
	}
	if _, ok := chooseWingetDir(dirs[:3], "x64"); ok {
		t.Error("split resource and bundle dirs must never be chosen")
	}
}
