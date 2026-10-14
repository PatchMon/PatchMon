package handler

import (
	"net/http"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestSelectedPatchRefusal(t *testing.T) {
	pm := func(s string) *string { return &s }
	cases := []struct {
		name      string
		pkgMgr    *string
		patchType string
		refused   bool
	}{
		{"pacman selected", pm("pacman"), "patch_package", true},
		{"pacman patch all", pm("pacman"), "patch_all", false},
		{"apt selected", pm("apt"), "patch_package", false},
		{"unknown manager", nil, "patch_package", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := selectedPatchRefusal(&models.Host{PackageManager: c.pkgMgr}, c.patchType)
			if (e != nil) != c.refused {
				t.Fatalf("refused = %v, want %v", e != nil, c.refused)
			}
			if e != nil && (e.Status != http.StatusBadRequest || e.Body["error"] != store.PacmanSelectedPatchUnsupported) {
				t.Fatalf("unexpected refusal %+v", e)
			}
		})
	}
}
