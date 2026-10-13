package packages

import (
	"reflect"
	"testing"
)

func TestWingetChildEnvPrependsEveryVCLibsDir(t *testing.T) {
	got := wingetChildEnv([]string{"Path=C:\\Windows", "A=1"}, `C:\UWPDesktop`, "", `C:\VCLibs`)
	want := []string{`Path=C:\UWPDesktop;C:\VCLibs;C:\Windows`, "A=1", "TERM=dumb"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNewestAppxDirKeepsVCLibsFlavoursApart(t *testing.T) {
	dirs := []string{
		`/wa/Microsoft.VCLibs.140.00.UWPDesktop_14.0.33728.0_arm64__8wekyb3d8bbwe`,
		`/wa/Microsoft.VCLibs.140.00_14.0.33519.0_arm64__8wekyb3d8bbwe`,
		`/wa/Microsoft.VCLibs.140.00_14.0.33728.0_x64__8wekyb3d8bbwe`,
	}
	d, ok := newestAppxDir(dirs, vclibsAppPrefix, "arm64")
	if !ok || d.path != dirs[1] {
		t.Fatalf("app VCLibs = %+v, %v", d, ok)
	}
	d, ok = newestAppxDir(dirs, vclibsPackagePrefix, "arm64")
	if !ok || d.path != dirs[0] {
		t.Fatalf("UWPDesktop VCLibs = %+v, %v", d, ok)
	}
}
