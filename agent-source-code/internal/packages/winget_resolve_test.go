package packages

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
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
		{"no package, signer only contains the Microsoft O=", exe, wingetTrustReport{Authenticode: "Valid", Signer: "CN=Microsoft Corporation, O=Microsoft Corporation Clones Ltd, C=GB"}, false},
		{"no package, Microsoft signer but not Valid", exe, wingetTrustReport{Authenticode: "HashMismatch", Signer: microsoftPublisherName}, false},
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

func TestWingetCandidates(t *testing.T) {
	root := "C:/Program Files/WindowsApps/"
	dirs := []string{
		root + "Microsoft.DesktopAppInstaller_1.9.25200.0_x64__8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_1.10.3.0_x64__8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_1.24.25199.0_arm64__8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_9.0.0.0_x64__notmicrosoft",
		root + "Microsoft.DesktopAppInstaller_2.0.0.0_neutral_~_8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_1.21.10121.0_neutral_split.scale-100_8wekyb3d8bbwe",
	}
	versions := func(c []appxDir) []string {
		var v []string
		for _, d := range c {
			v = append(v, d.version+"/"+d.arch)
		}
		return v
	}
	if got, want := versions(wingetCandidates(dirs, "x64")), []string{"1.10.3.0/x64", "1.9.25200.0/x64", "1.24.25199.0/arm64"}; !reflect.DeepEqual(got, want) {
		t.Errorf("x64 order = %q, want %q", got, want)
	}
	if got, want := versions(wingetCandidates(dirs, "arm64")), []string{"1.24.25199.0/arm64", "1.10.3.0/x64", "1.9.25200.0/x64"}; !reflect.DeepEqual(got, want) {
		t.Errorf("arm64 order = %q, want %q", got, want)
	}
	if c := wingetCandidates(nil, "x64"); len(c) != 0 {
		t.Errorf("empty = %+v", c)
	}
	if c := wingetCandidates(dirs[4:], "x64"); len(c) != 0 {
		t.Errorf("bundle and split resource dirs must never be candidates: %+v", c)
	}
}

func TestFirstTrustedWingetSkipsAnUntrustedNewerDir(t *testing.T) {
	root := "C:/Program Files/WindowsApps/"
	cands := wingetCandidates([]string{
		root + "Microsoft.DesktopAppInstaller_1.27.0.0_x64__8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_1.26.0.0_x64__8wekyb3d8bbwe",
	}, "x64")
	staged := errors.New("not registered")
	verify := func(exe string) error {
		if strings.Contains(exe, "1.27.0.0") {
			return staged
		}
		return nil
	}
	d, err := firstTrustedWinget(cands, verify)
	if err != nil || d.version != "1.26.0.0" {
		t.Fatalf("got %+v, %v; want the registered 1.26.0.0", d, err)
	}
	if _, err := firstTrustedWinget(cands, func(string) error { return staged }); !errors.Is(err, staged) {
		t.Errorf("all untrusted: err = %v", err)
	}
	if _, err := firstTrustedWinget(nil, verify); !errors.Is(err, errWingetNotFound) {
		t.Errorf("no candidates: err = %v", err)
	}
}

func TestWingetResolverCachesFailureBriefly(t *testing.T) {
	now := time.Unix(0, 0)
	calls := 0
	fail := errors.New("untrusted")
	r := &wingetResolver{
		now: func() time.Time { return now },
		resolve: func() (string, []string, error) {
			calls++
			if calls == 1 {
				return "", nil, fail
			}
			return "winget.exe", nil, nil
		},
	}
	if _, _, err := r.get(); !errors.Is(err, fail) || errors.Is(err, errWingetRecentlyFailed) {
		t.Fatalf("first call err = %v", err)
	}
	now = now.Add(wingetRetryAfter - time.Second)
	if _, _, err := r.get(); !errors.Is(err, errWingetRecentlyFailed) || !errors.Is(err, fail) || calls != 1 {
		t.Fatalf("within the retry window: err = %v, calls = %d", err, calls)
	}
	now = now.Add(time.Second)
	if exe, _, err := r.get(); err != nil || exe != "winget.exe" || calls != 2 {
		t.Fatalf("after the retry window: exe = %q, err = %v, calls = %d", exe, err, calls)
	}
	if _, _, err := r.get(); err != nil || calls != 3 {
		t.Fatalf("success is not cached: err = %v, calls = %d", err, calls)
	}
}

func TestNewestAppxDirKeepsVCLibsFlavoursApart(t *testing.T) {
	dirs := []string{
		`/wa/Microsoft.VCLibs.140.00.UWPDesktop_14.0.33519.0_arm64__8wekyb3d8bbwe`,
		`/wa/Microsoft.VCLibs.140.00.UWPDesktop_14.0.33728.0_arm64__8wekyb3d8bbwe`,
		`/wa/Microsoft.VCLibs.140.00.UWPDesktop_14.0.40000.0_x64__8wekyb3d8bbwe`,
		`/wa/Microsoft.VCLibs.140.00_14.0.33519.0_arm64__8wekyb3d8bbwe`,
		`/wa/Microsoft.VCLibs.140.00_14.0.33728.0_x64__8wekyb3d8bbwe`,
	}
	d, ok := newestAppxDir(dirs, vclibsPackagePrefix, "arm64")
	if !ok || d.path != dirs[1] {
		t.Fatalf("UWPDesktop VCLibs = %+v, %v", d, ok)
	}
	d, ok = newestAppxDir(dirs, vclibsAppPrefix, "arm64")
	if !ok || d.path != dirs[3] {
		t.Fatalf("app VCLibs = %+v, %v", d, ok)
	}
}

func TestWingetChildEnv(t *testing.T) {
	env := wingetChildEnv([]string{"Path=C:\\Windows", "TERM=xterm", "A=1"}, `C:\VCLibs`)
	want := []string{`Path=C:\VCLibs;C:\Windows`, "A=1", "TERM=dumb"}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("env = %q, want %q", env, want)
	}
	if env := wingetChildEnv([]string{"A=1"}, ""); !reflect.DeepEqual(env, []string{"A=1", "TERM=dumb"}) {
		t.Errorf("env without vclibs = %q", env)
	}
	got := wingetChildEnv([]string{"Path=C:\\Windows", "A=1"}, `C:\UWPDesktop`, "", `C:\VCLibs`)
	want = []string{`Path=C:\UWPDesktop;C:\VCLibs;C:\Windows`, "A=1", "TERM=dumb"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("both VCLibs dirs = %q, want %q", got, want)
	}
}
