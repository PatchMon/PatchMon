package commands

import (
	"slices"
	"sync/atomic"

	"patchmon-agent/pkg/models"
)

const capabilityWingetPinned = "wingetPinned"

var serverAcceptsWingetPinned atomic.Bool

func recordServerCapabilities(resp *models.PingResponse) {
	if resp == nil {
		return
	}
	serverAcceptsWingetPinned.Store(slices.Contains(resp.Capabilities, capabilityWingetPinned))
}

// withServerSupportedFields drops fields an older server would reject. wingetPinned is in the
// packages hash, so sending it to a server that does not hash it fails every report.
func withServerSupportedFields(pkgs []models.Package) []models.Package {
	if serverAcceptsWingetPinned.Load() {
		return pkgs
	}
	for i := range pkgs {
		pkgs[i].WingetPinned = false
	}
	return pkgs
}
