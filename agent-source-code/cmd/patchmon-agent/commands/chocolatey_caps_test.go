package commands

import (
	"testing"

	"patchmon-agent/pkg/models"
)

func TestChocolateyCapabilityGate(t *testing.T) {
	old := serverAcceptsChocolatey.Load()
	oldPinned, oldStore := serverAcceptsWingetPinned.Load(), serverAcceptsWingetStoreApp.Load()
	t.Cleanup(func() {
		serverAcceptsChocolatey.Store(old)
		serverAcceptsWingetPinned.Store(oldPinned)
		serverAcceptsWingetStoreApp.Store(oldStore)
	})
	for _, supported := range []bool{false, true} {
		caps := []string{}
		if supported {
			caps = append(caps, capabilityChocolatey)
		}
		recordServerCapabilities(&models.PingResponse{Capabilities: caps})
		p := withServerSupportedFields([]models.Package{{Name: "chocolatey:app", ChocolateyID: "app", ChocolateyPinned: true}})[0]
		if (p.ChocolateyID != "") != supported || p.ChocolateyPinned != supported {
			t.Fatalf("capability gate: %+v", p)
		}
	}
}
