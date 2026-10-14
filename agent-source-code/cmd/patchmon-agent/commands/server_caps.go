package commands

import (
	"slices"
	"sync/atomic"

	"patchmon-agent/pkg/models"
)

const (
	capabilityWingetPinned   = "wingetPinned"
	capabilityWingetStoreApp = "wingetStoreApp"
)

var (
	serverAcceptsWingetPinned   atomic.Bool
	serverAcceptsWingetStoreApp atomic.Bool
)

func recordServerCapabilities(resp *models.PingResponse) {
	if resp == nil {
		return
	}
	serverAcceptsWingetPinned.Store(slices.Contains(resp.Capabilities, capabilityWingetPinned))
	serverAcceptsWingetStoreApp.Store(slices.Contains(resp.Capabilities, capabilityWingetStoreApp))
}

// withServerSupportedFields drops fields an older server would reject. These flags are in the
// packages hash, so sending one to a server that does not hash it fails every report.
func withServerSupportedFields(pkgs []models.Package) []models.Package {
	pinned, store := serverAcceptsWingetPinned.Load(), serverAcceptsWingetStoreApp.Load()
	for i := range pkgs {
		if !pinned {
			pkgs[i].WingetPinned = false
		}
		if !store {
			pkgs[i].WingetStoreApp = false
		}
	}
	return pkgs
}
