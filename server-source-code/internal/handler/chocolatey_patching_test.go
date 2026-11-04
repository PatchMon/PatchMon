package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

func TestChocolateyTargetResolution(t *testing.T) {
	for _, tc := range []struct {
		id        string
		patchable bool
		reason    string
		valid     bool
	}{
		{"app.install", true, "", true}, {"all", true, "", false}, {"--force", true, "", false}, {"app", false, "chocolatey_pinned", false}, {"", false, "no_winget_id", false},
	} {
		rows := []db.ResolveWindowsPatchTargetsRow{{Name: "chocolatey:app", ChocolateyID: strp(tc.id), NeedsUpdate: true, Patchable: tc.patchable, NotPatchableReason: tc.reason}}
		items, rejects := classifyWindowsTargets([]string{"chocolatey:app"}, rows)
		if tc.valid {
			if len(items) != 1 || items[0].Kind != store.PatchItemKindChocolatey || items[0].Identifier != tc.id {
				t.Fatalf("bad target: %+v", items)
			}
		} else if len(items) != 0 || len(rejects) != 1 {
			t.Fatalf("unsafe target accepted: %+v", items)
		}
	}
}
