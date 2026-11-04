package hashing

import (
	"patchmon-agent/pkg/models"
	"testing"
)

func TestChocolateyHash(t *testing.T) {
	p := models.Package{Name: "chocolatey:git", CurrentVersion: "1.0", ChocolateyID: "git", ChocolateyPinned: true, NeedsUpdate: true}
	h, err := PackagesHash([]models.Package{p})
	if err != nil {
		t.Fatal(err)
	}
	if h != "22fc96d9c009502d7e894b69bcae34ad2441e4ac18915a301792ec4ffda6d68d" {
		t.Fatalf("agent/server fixture hash: %s", h)
	}
	p.ChocolateyPinned = false
	unpinned, _ := PackagesHash([]models.Package{p})
	if h == unpinned {
		t.Fatal("pin missing from hash")
	}
	p.ChocolateyID = ""
	legacy, _ := PackagesHash([]models.Package{p})
	if legacy == unpinned {
		t.Fatal("capability missing from hash")
	}
}
