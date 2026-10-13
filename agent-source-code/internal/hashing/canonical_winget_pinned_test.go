package hashing

import (
	"testing"

	"patchmon-agent/pkg/models"
)

// Same value the server's TestCanonicalPackagesHashWingetPinned pins.
const pinnedRowFixtureHash = "adf14314e514bd9be86ef1419ee4b1ef8584ac5b602ce712b13fc9be2326cdeb"

func TestPackagesHashWingetPinned(t *testing.T) {
	row := models.Package{Name: "7-Zip", CurrentVersion: "24.08", NeedsUpdate: true, WingetID: "7zip.7zip"}
	unpinned, err := PackagesHash([]models.Package{row})
	if err != nil {
		t.Fatal(err)
	}
	legacy := row
	legacy.WingetID = ""
	if h, _ := PackagesHash([]models.Package{legacy}); h != unpinned {
		t.Fatal("an unpinned row no longer hashes as it did before wingetPinned existed")
	}
	row.WingetPinned = true
	pinned, err := PackagesHash([]models.Package{row})
	if err != nil {
		t.Fatal(err)
	}
	if pinned == unpinned {
		t.Fatal("pinning an app does not change the packages hash")
	}
	if pinned != pinnedRowFixtureHash {
		t.Fatalf("pinned row hash = %s, server pins %s", pinned, pinnedRowFixtureHash)
	}
}
