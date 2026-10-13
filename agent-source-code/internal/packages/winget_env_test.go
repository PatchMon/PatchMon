package packages

import (
	"io"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
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

func TestWingetListOutputParsesAfterProgressAndSpinner(t *testing.T) {
	m := &WindowsManager{logger: logrus.New()}
	m.logger.SetOutput(io.Discard)
	raw := "   - \r   \\ \r\x1b[2K  ███▒▒  1024 KB / 8.30 MB\r\n" +
		"Name               Id                  Version  Available Source\r\n" +
		"-----------------------------------------------------------------\r\n" +
		"7-Zip 24.08 (x64)  7zip.7zip           24.08    24.09     winget\r\n" +
		"Notepad++          Notepad++.Notepad++ 8.6      8.7       winget\r\n"
	entries := m.parseWingetTable(stripTerminalControls(raw))
	if len(entries) != 2 || entries[0].ID != "7zip.7zip" || entries[1].ID != "Notepad++.Notepad++" || entries[1].Available != "8.7" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestWithTempDirReplacesTempAndTmp(t *testing.T) {
	got := withTempDir([]string{`TEMP=C:\Windows\Temp`, "A=1", `tmp=C:\Windows\Temp`, `Temp=x`}, `C:\Windows\SystemTemp`)
	want := []string{"A=1", `TEMP=C:\Windows\SystemTemp`, `TMP=C:\Windows\SystemTemp`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
