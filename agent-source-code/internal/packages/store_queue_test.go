package packages

import (
	"testing"

	"patchmon-agent/pkg/models"
)

func queued(name, family, version string, installed bool) pendingStoreUpdate {
	return pendingStoreUpdate{
		storeAppx: storeEntry(name, family, testWindowsApps+`\`+name+`_`+version+`_x64__8wekyb3d8bbwe`, "Store"),
		Version:   version,
		Installed: installed,
	}
}

func TestParsePendingStoreUpdates(t *testing.T) {
	one := []byte("\ufeff{\"Name\":\"Microsoft.ScreenSketch\",\"PackageFamilyName\":\"Microsoft.ScreenSketch_8wekyb3d8bbwe\",\"Version\":\"11.2307.64.0\",\"Installed\":true,\"SignatureKind\":\"Store\"}\r\n")
	got := parsePendingStoreUpdates(one)
	if len(got) != 1 || got[0].PackageFamilyName != "Microsoft.ScreenSketch_8wekyb3d8bbwe" || got[0].Version != "11.2307.64.0" || !got[0].Installed {
		t.Fatalf("single object = %+v", got)
	}
	if got := parsePendingStoreUpdates([]byte(`[{"Name":"A"},{"Name":"B"}]`)); len(got) != 2 {
		t.Errorf("array = %+v", got)
	}
	for _, empty := range []string{"", "null", "[]", "not json"} {
		if got := parsePendingStoreUpdates([]byte(empty)); len(got) != 0 {
			t.Errorf("%q = %+v", empty, got)
		}
	}
}

func TestAddPendingStoreUpdatesAddsOutdatedStoreApps(t *testing.T) {
	pkgs := []models.Package{{Name: "7-Zip", CurrentVersion: "24.08", WingetID: "7zip.7zip"}}
	pending := []pendingStoreUpdate{
		queued("Microsoft.ScreenSketch", "Microsoft.ScreenSketch_8wekyb3d8bbwe", "11.2307.64.0", true),
		queued("Microsoft.ScreenSketch", "Microsoft.ScreenSketch_8wekyb3d8bbwe", "11.2210.1.0", true),
		queued("Microsoft.BingWeather", "Microsoft.BingWeather_8wekyb3d8bbwe", "4.53.0.0", false),
	}
	got := addPendingStoreUpdates(pkgs, pending, testWindowsApps)
	if len(got) != 2 {
		t.Fatalf("got %d rows: %+v", len(got), got)
	}
	row := got[1]
	want := models.Package{Name: "Microsoft.ScreenSketch", CurrentVersion: "11.2307.64.0", NeedsUpdate: true, SourceRepository: "msstore", WingetID: "Microsoft.ScreenSketch_8wekyb3d8bbwe", WingetStoreApp: true}
	if row.Name != want.Name || row.CurrentVersion != want.CurrentVersion || row.NeedsUpdate != want.NeedsUpdate || row.SourceRepository != want.SourceRepository || row.WingetID != want.WingetID || row.WingetStoreApp != want.WingetStoreApp || row.AvailableVersion != "" {
		t.Fatalf("row = %+v", row)
	}
	if !wingetIDPattern.MatchString(row.WingetID) {
		t.Fatalf("%q is not a valid winget Id", row.WingetID)
	}
	if got[0].NeedsUpdate || got[0].WingetStoreApp {
		t.Errorf("unrelated app changed: %+v", got[0])
	}
}

func TestAddPendingStoreUpdatesMergesWithWingetRows(t *testing.T) {
	pkgs := []models.Package{
		{Name: "Windows Terminal", CurrentVersion: "1.24.12741.0", WingetID: "Microsoft.WindowsTerminal", SourceRepository: "winget"},
		{Name: "App Installer", CurrentVersion: "1.29.379.0", WingetID: "Microsoft.AppInstaller", AvailableVersion: "1.29.380", NeedsUpdate: true, SourceRepository: "winget"},
	}
	pending := []pendingStoreUpdate{
		queued("Microsoft.WindowsTerminal", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "1.24.12741.0", true),
		queued("Microsoft.DesktopAppInstaller", "Microsoft.DesktopAppInstaller_8wekyb3d8bbwe", "1.29.379.0", true),
	}
	got := addPendingStoreUpdates(pkgs, pending, testWindowsApps)
	if len(got) != 2 {
		t.Fatalf("duplicated rows: %+v", got)
	}
	for _, p := range got {
		if !p.NeedsUpdate || !p.WingetStoreApp {
			t.Errorf("%s not marked: %+v", p.Name, p)
		}
	}
	if got[1].AvailableVersion != "1.29.380" {
		t.Errorf("winget's available version lost: %+v", got[1])
	}
}

func TestAddPendingStoreUpdatesIgnoresNonStoreInstalls(t *testing.T) {
	dev := queued("Contoso.Tool", "Contoso.Tool_abcdefghijklm", "1.0.0.0", true)
	dev.SignatureKind = "Developer"
	framework := queued("Microsoft.VCLibs", "Microsoft.VCLibs_8wekyb3d8bbwe", "14.0.0.0", true)
	framework.IsFramework = true
	elsewhere := queued("Contoso.Other", "Contoso.Other_abcdefghijklm", "1.0.0.0", true)
	elsewhere.InstallLocation = `C:\Program Files\Contoso`
	if got := addPendingStoreUpdates(nil, []pendingStoreUpdate{dev, framework, elsewhere}, testWindowsApps); len(got) != 0 {
		t.Fatalf("non-Store installs added: %+v", got)
	}
}

func TestPendingStoreUpdatesStayOutOfPatchAll(t *testing.T) {
	got := addPendingStoreUpdates(nil, []pendingStoreUpdate{queued("Microsoft.ScreenSketch", "Microsoft.ScreenSketch_8wekyb3d8bbwe", "11.2307.64.0", true)}, testWindowsApps)
	if len(got) != 1 || !got[0].WingetStoreApp {
		t.Fatalf("got %+v", got)
	}
	if updatableWingetApps(got) != nil {
		t.Error("a queued Store update would be part of Patch all")
	}
}
