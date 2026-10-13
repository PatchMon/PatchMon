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

const pinnedRowFixtureHash = "adf14314e514bd9be86ef1419ee4b1ef8584ac5b602ce712b13fc9be2326cdeb"

func TestCanonicalPackagesHashWingetPinned(t *testing.T) {
	row := store.ReportPackage{Name: "7-Zip", CurrentVersion: "24.08", NeedsUpdate: true, WingetID: "7zip.7zip"}
	unpinned, err := CanonicalPackagesHash([]store.ReportPackage{row})
	if err != nil {
		t.Fatal(err)
	}
	legacy := row
	legacy.WingetID = ""
	if h, _ := CanonicalPackagesHash([]store.ReportPackage{legacy}); h != unpinned {
		t.Fatal("an unpinned row no longer hashes as it did before wingetPinned existed")
	}
	row.WingetPinned = true
	pinned, err := CanonicalPackagesHash([]store.ReportPackage{row})
	if err != nil {
		t.Fatal(err)
	}
	if pinned == unpinned {
		t.Fatal("pinning an app does not change the packages hash")
	}
	if pinned != pinnedRowFixtureHash {
		t.Fatalf("pinned row hash = %s; the agent's hashing test pins the same value", pinned)
	}
}
