package commands

import (
	"encoding/json"
	"strings"
	"testing"

	"patchmon-agent/internal/hashing"
	"patchmon-agent/pkg/models"
)

func pinnedPackages() []models.Package {
	return []models.Package{{Name: "7-Zip", CurrentVersion: "26.02", AvailableVersion: "26.04", NeedsUpdate: true, WingetID: "7zip.7zip", WingetPinned: true}}
}

func TestWingetPinnedOnlyReachesServersThatAcceptIt(t *testing.T) {
	t.Cleanup(func() { serverAcceptsWingetPinned.Store(false); serverAcceptsWingetStoreApp.Store(false) })

	legacy := pinnedPackages()
	legacy[0].WingetPinned = false
	legacyHash, err := hashing.PackagesHash(legacy)
	if err != nil {
		t.Fatal(err)
	}
	pinnedHash, err := hashing.PackagesHash(pinnedPackages())
	if err != nil {
		t.Fatal(err)
	}
	if legacyHash == pinnedHash {
		t.Fatal("a pinned app must change the packages hash")
	}

	for _, tc := range []struct {
		name       string
		resp       *models.PingResponse
		wantHash   string
		wantPinned bool
	}{
		{"old server", &models.PingResponse{HashGate: true}, legacyHash, false},
		{"new server", &models.PingResponse{HashGate: true, Capabilities: []string{"other", "wingetPinned"}}, pinnedHash, true},
		{"server downgraded", &models.PingResponse{}, legacyHash, false},
	} {
		recordServerCapabilities(tc.resp)
		pkgs := withServerSupportedFields(pinnedPackages())
		got, err := hashing.PackagesHash(pkgs)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.wantHash {
			t.Errorf("%s: hash = %s, want %s", tc.name, got, tc.wantHash)
		}
		body, _ := json.Marshal(pkgs)
		if strings.Contains(string(body), "wingetPinned") != tc.wantPinned {
			t.Errorf("%s: payload = %s", tc.name, body)
		}
	}

	serverAcceptsWingetPinned.Store(true)
	recordServerCapabilities(nil)
	if !serverAcceptsWingetPinned.Load() {
		t.Error("a failed ping must not change what the server is known to accept")
	}
}

func TestWingetStoreAppGatedOnItsOwnCapability(t *testing.T) {
	t.Cleanup(func() { serverAcceptsWingetPinned.Store(false); serverAcceptsWingetStoreApp.Store(false) })
	pkgs := func() []models.Package {
		return []models.Package{{Name: "Windows Terminal", WingetID: "Microsoft.WindowsTerminal", NeedsUpdate: true, WingetPinned: true, WingetStoreApp: true}}
	}
	for _, tc := range []struct {
		caps          []string
		pinned, store bool
	}{
		{nil, false, false},
		{[]string{"wingetPinned"}, true, false},
		{[]string{"wingetStoreApp"}, false, true},
		{[]string{"wingetPinned", "wingetStoreApp"}, true, true},
	} {
		recordServerCapabilities(&models.PingResponse{Capabilities: tc.caps})
		got := withServerSupportedFields(pkgs())[0]
		if got.WingetPinned != tc.pinned || got.WingetStoreApp != tc.store {
			t.Errorf("caps %v: pinned %v store %v", tc.caps, got.WingetPinned, got.WingetStoreApp)
		}
	}
}
