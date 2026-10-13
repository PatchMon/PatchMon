package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestCanonicalPackagesHashIgnoresWindowsPatchFields(t *testing.T) {
	base := []store.ReportPackage{{Name: "7-Zip", CurrentVersion: "24.08", NeedsUpdate: true}}
	withFields := []store.ReportPackage{{Name: "7-Zip", CurrentVersion: "24.08", NeedsUpdate: true, WingetID: "7zip.7zip", WUABrowseOnly: true, WUAType: "Driver"}}
	a, err := CanonicalPackagesHash(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CanonicalPackagesHash(withFields)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("winget and WUA flag fields changed the canonical hash; agents and servers of different versions would reject each other's reports")
	}
}
