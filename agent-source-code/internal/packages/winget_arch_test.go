package packages

import (
	"context"
	"debug/pe"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const detailsThunderbirdX86 = "Mozilla Thunderbird (x86 de) [Mozilla.Thunderbird.de]\r\n" +
	"Version: 158.0.1\r\n" +
	"Publisher: Mozilla\r\n" +
	"Local Identifier: ARP\\Machine\\X86\\Mozilla Thunderbird 158.0.1 (x86 de)\r\n" +
	"Product Code: mozilla thunderbird 158.0.1 (x86 de)\r\n" +
	"Installer Category: exe\r\n" +
	"Installed Scope: Machine\r\n" +
	"Installed Location: C:\\Program Files (x86)\\Mozilla Thunderbird\r\n" +
	"Available Upgrades:\r\n" +
	"  winget [159.0]\r\n"

const detailsThunderbirdBoth = "(1/2) Mozilla Thunderbird (x64 de) [Mozilla.Thunderbird.de]\n" +
	"Local Identifier: ARP\\Machine\\X64\\Mozilla Thunderbird 159.0 (x64 de)\n" +
	"(2/2) Mozilla Thunderbird (x86 de) [Mozilla.Thunderbird.de]\n" +
	"Local Identifier: ARP\\Machine\\X86\\Mozilla Thunderbird 158.0.1 (x86 de)\n"

func TestMachineX86ARPKey(t *testing.T) {
	cases := []struct {
		name, out, key string
		ok             bool
	}{
		{"x86 machine install", detailsThunderbirdX86, "Mozilla Thunderbird 158.0.1 (x86 de)", true},
		{"localised labels", strings.ReplaceAll(detailsThunderbirdX86, "Local Identifier", "Lokaler Bezeichner"), "Mozilla Thunderbird 158.0.1 (x86 de)", true},
		{"msi product code", "Lokale ID: ARP\\Machine\\X86\\{d8bbe9f9-7c5b-42c6-b715-9ee898a2e515}\n", "{d8bbe9f9-7c5b-42c6-b715-9ee898a2e515}", true},
		{"x64 install", "Local Identifier: ARP\\Machine\\X64\\Mozilla Thunderbird 159.0 (x64 de)\n", "", false},
		{"both architectures", detailsThunderbirdBoth, "", false},
		{"user scope", "Local Identifier: ARP\\User\\X86\\Some App\n", "", false},
		{"msix", "Local Identifier: MSIX\\Microsoft.WindowsTerminal_1.0_x64__8wekyb3d8bbwe\n", "", false},
		{"no output", "", "", false},
	}
	for _, c := range cases {
		key, ok := machineX86ARPKey(c.out)
		if key != c.key || ok != c.ok {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, key, ok, c.key, c.ok)
		}
	}
}

func writePE(t *testing.T, path string, machine uint16) {
	t.Helper()
	b := make([]byte, 64)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 64)
	b = append(b, 'P', 'E', 0, 0)
	hdr := make([]byte, 20)
	binary.LittleEndian.PutUint16(hdr, machine)
	b = append(b, hdr...)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestX86OnlyBinaries(t *testing.T) {
	dir := func(files map[string]uint16, extra ...string) string {
		d := t.TempDir()
		for name, m := range files {
			writePE(t, filepath.Join(d, name), m)
		}
		for _, name := range extra {
			if err := os.WriteFile(filepath.Join(d, name), []byte("not a PE"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}
	nested := dir(map[string]uint16{"thunderbird.exe": pe.IMAGE_FILE_MACHINE_I386})
	if err := os.Mkdir(filepath.Join(nested, "uninstall"), 0o700); err != nil {
		t.Fatal(err)
	}
	writePE(t, filepath.Join(nested, "uninstall", "helper.exe"), pe.IMAGE_FILE_MACHINE_AMD64)

	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{"32-bit app", dir(map[string]uint16{"thunderbird.exe": pe.IMAGE_FILE_MACHINE_I386, "updater.exe": pe.IMAGE_FILE_MACHINE_I386}, "license.txt"), true},
		{"64-bit app with a 32-bit uninstaller", dir(map[string]uint16{"msedge.exe": pe.IMAGE_FILE_MACHINE_AMD64, "uninstall.exe": pe.IMAGE_FILE_MACHINE_I386}), false},
		{"arm64 app", dir(map[string]uint16{"app.exe": pe.IMAGE_FILE_MACHINE_ARM64}), false},
		{"unreadable exe ignored", dir(map[string]uint16{"app.EXE": pe.IMAGE_FILE_MACHINE_I386}, "broken.exe"), true},
		{"subdirectories ignored", nested, true},
		{"no executables", dir(nil, "readme.txt"), false},
		{"missing directory", filepath.Join(t.TempDir(), "gone"), false},
		{"no location", "", false},
	}
	for _, c := range cases {
		if got := x86OnlyBinaries(c.dir); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWingetDetailsArgs(t *testing.T) {
	want := []string{"list", "--id", "Mozilla.Thunderbird.de", "--exact", "--source", "winget", "--details", "--accept-source-agreements", "--disable-interactivity"}
	if got := wingetDetailsArgs("Mozilla.Thunderbird.de"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func x86Winget() *fakeWinget {
	return &fakeWinget{
		states: map[string]wingetUpgradeState{"Mozilla.Thunderbird.de": wingetStateAvailable, "7zip.7zip": wingetStateAvailable},
		x86:    map[string]bool{"Mozilla.Thunderbird.de": true},
		codes:  map[string]uint32{},
	}
}

func upgradeCallsFor(g *fakeWinget, id string) [][]string {
	var out [][]string
	for _, c := range g.calls {
		if c[0] == "upgrade" && c[2] == id {
			out = append(out, c)
		}
	}
	return out
}

func TestRunUpgradesX86AppWithX86Installer(t *testing.T) {
	g := x86Winget()
	rec := &recorder{}
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{
		{Kind: WindowsTargetWinget, ID: "Mozilla.Thunderbird.de"},
		{Kind: WindowsTargetWinget, ID: "7zip.7zip"},
	}}, rec, &fakeWUA{}, g)

	tb := upgradeCallsFor(g, "Mozilla.Thunderbird.de")
	if want := append(wingetUpgradeArgs("Mozilla.Thunderbird.de"), "--architecture", "x86"); len(tb) != 1 || !reflect.DeepEqual(tb[0], want) {
		t.Errorf("thunderbird calls = %q", tb)
	}
	if zip := upgradeCallsFor(g, "7zip.7zip"); len(zip) != 1 || !reflect.DeepEqual(zip[0], wingetUpgradeArgs("7zip.7zip")) {
		t.Errorf("7zip calls = %q", zip)
	}
	for _, id := range []string{"Mozilla.Thunderbird.de", "7zip.7zip"} {
		if it := itemByID(res, id); it.Status != itemInstalled {
			t.Errorf("%s = %+v", id, it)
		}
	}
	if !strings.Contains(rec.out.String(), "Mozilla.Thunderbird.de: installed as 32-bit") || strings.Contains(rec.out.String(), "7zip.7zip: installed as 32-bit") {
		t.Errorf("output:\n%s", rec.out.String())
	}
}

func TestRunDoesNotSwitchX86AppWithoutX86Installer(t *testing.T) {
	g := x86Winget()
	g.codes["Mozilla.Thunderbird.de"] = wingetUpdateNotApplicable
	g.codes["7zip.7zip"] = wingetUpdateNotApplicable
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{
		{Kind: WindowsTargetWinget, ID: "Mozilla.Thunderbird.de"},
		{Kind: WindowsTargetWinget, ID: "7zip.7zip"},
	}}, &recorder{}, &fakeWUA{}, g)

	if it := itemByID(res, "Mozilla.Thunderbird.de"); it.Status != itemNotApplicable || it.Message != msgNo32BitUpgrade {
		t.Errorf("thunderbird = %+v", it)
	}
	if n := len(upgradeCallsFor(g, "Mozilla.Thunderbird.de")); n != 1 {
		t.Errorf("thunderbird upgraded %d times, want 1 with no retry", n)
	}
	if it := itemByID(res, "7zip.7zip"); it.Status != itemNotApplicable || it.Message != "No applicable upgrade" {
		t.Errorf("7zip = %+v", it)
	}
}

func TestDryRunSkipsArchitectureCheck(t *testing.T) {
	g := x86Winget()
	runFake(WindowsPatchRequest{DryRun: true, Targets: []WindowsPatchTarget{{Kind: WindowsTargetWinget, ID: "Mozilla.Thunderbird.de"}}}, &recorder{}, &fakeWUA{}, g)
	if len(g.archChecks) != 0 {
		t.Errorf("dry run checked architecture for %q", g.archChecks)
	}
}

func TestArchitectureCheckOnlyForAvailableUpgrades(t *testing.T) {
	g := x86Winget()
	g.states["Mozilla.Thunderbird.de"] = wingetStateNone
	runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWinget, ID: "Mozilla.Thunderbird.de"}}}, &recorder{}, &fakeWUA{}, g)
	if len(g.archChecks) != 0 || len(upgradeCallsFor(g, "Mozilla.Thunderbird.de")) != 0 {
		t.Errorf("arch checks %q, calls %q", g.archChecks, g.calls)
	}
}

func TestStopDuringArchitectureCheckDoesNotUpgrade(t *testing.T) {
	g := x86Winget()
	rec := &recorder{}
	g.archErr = context.Canceled
	g.onArch = func() { rec.mu.Lock(); rec.stopped = true; rec.mu.Unlock() }
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWinget, ID: "Mozilla.Thunderbird.de"}}}, rec, &fakeWUA{}, g)
	if it := itemByID(res, "Mozilla.Thunderbird.de"); it.Status != itemSkipped || it.Message != msgStopped {
		t.Errorf("item = %+v", it)
	}
	if calls := upgradeCallsFor(g, "Mozilla.Thunderbird.de"); len(calls) != 0 {
		t.Errorf("upgraded after a stop: %q", calls)
	}
}

func TestFailedArchitectureCheckDoesNotUpgrade(t *testing.T) {
	g := x86Winget()
	g.archErr = errors.New("winget exited unexpectedly")
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWinget, ID: "Mozilla.Thunderbird.de"}}}, &recorder{}, &fakeWUA{}, g)
	if it := itemByID(res, "Mozilla.Thunderbird.de"); it.Status != itemFailed || !strings.Contains(it.Message, "winget exited unexpectedly") {
		t.Errorf("item = %+v", it)
	}
	if calls := upgradeCallsFor(g, "Mozilla.Thunderbird.de"); len(calls) != 0 {
		t.Errorf("upgraded after a failed check: %q", calls)
	}
}
