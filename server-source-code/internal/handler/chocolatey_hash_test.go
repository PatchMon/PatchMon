package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestChocolateyHashMatchesAgent(t *testing.T) {
	p := store.ReportPackage{Name: "chocolatey:git", CurrentVersion: "1.0", ChocolateyID: "git", ChocolateyPinned: true, NeedsUpdate: true}
	h, err := CanonicalPackagesHash([]store.ReportPackage{p})
	if err != nil {
		t.Fatal(err)
	}
	if h != "22fc96d9c009502d7e894b69bcae34ad2441e4ac18915a301792ec4ffda6d68d" {
		t.Fatalf("agent/server hash mismatch: %s", h)
	}
}
