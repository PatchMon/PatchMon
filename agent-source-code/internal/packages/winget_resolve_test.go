package packages

import (
	"reflect"
	"testing"
)

func TestChooseWingetDir(t *testing.T) {
	root := "C:/Program Files/WindowsApps/"
	dirs := []string{
		root + "Microsoft.DesktopAppInstaller_1.9.25200.0_x64__8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_1.10.3.0_x64__8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_1.24.25199.0_arm64__8wekyb3d8bbwe",
		root + "Microsoft.DesktopAppInstaller_9.0.0.0_x64__notmicrosoft",
		root + "Microsoft.DesktopAppInstaller_2.0.0.0_neutral_~_8wekyb3d8bbwe",
	}
	if d, ok := chooseWingetDir(dirs, "x64"); !ok || d.version != "1.10.3.0" {
		t.Errorf("x64 = %+v", d)
	}
	if d, ok := chooseWingetDir(dirs, "arm64"); !ok || d.arch != "arm64" {
		t.Errorf("arm64 = %+v", d)
	}
	if d, ok := chooseWingetDir(dirs[2:3], "x64"); !ok || d.arch != "arm64" {
		t.Errorf("fallback to another arch = %+v", d)
	}
	if _, ok := chooseWingetDir(nil, "x64"); ok {
		t.Error("empty must not resolve")
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
