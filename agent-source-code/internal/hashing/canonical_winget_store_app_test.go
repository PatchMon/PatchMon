package hashing

import (
	"testing"

	"patchmon-agent/pkg/models"
)

// The server's matching test must pin the same value.
const storeAppRowFixtureHash = "8eaaf876cf5fbc86692a8759c41e016c6fb1d6fe8496d7db3b9ad3dfca9b33d3"

func storeAppFixtureRow() models.Package {
	return models.Package{Name: "Windows Terminal", CurrentVersion: "1.20", AvailableVersion: "1.21", NeedsUpdate: true, WingetID: "Microsoft.WindowsTerminal", WingetStoreApp: true}
}

func TestPackagesHashWingetStoreApp(t *testing.T) {
	row := storeAppFixtureRow()
	flagged, err := PackagesHash([]models.Package{row})
	if err != nil {
		t.Fatal(err)
	}
	row.WingetStoreApp = false
	plain, err := PackagesHash([]models.Package{row})
	if err != nil {
		t.Fatal(err)
	}
	if flagged == plain {
		t.Fatal("flagging a Store app does not change the packages hash")
	}
	if flagged != storeAppRowFixtureHash {
		t.Fatalf("store app row hash = %s, want %s", flagged, storeAppRowFixtureHash)
	}
}
