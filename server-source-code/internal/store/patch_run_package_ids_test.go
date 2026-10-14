package store

import (
	"context"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/google/uuid"
)

func TestPackageIDsByName(t *testing.T) {
	d := pgtest.Open(t)
	ctx := context.Background()
	suffix := uuid.New().String()[:8]
	stored := map[string]string{
		"Mozilla Firefox (x64 en-GB) " + suffix: "pkgids-ff-" + suffix,
		"libstdc++6-" + suffix:                  "pkgids-std-" + suffix,
	}
	for name, id := range stored {
		if _, err := d.Exec(ctx, `INSERT INTO packages (id, name, updated_at) VALUES ($1, $2, NOW())`, id, name); err != nil {
			t.Fatal(err)
		}
	}
	missing := "2026-10 Security Update (KB5127648) (26300.9711) " + suffix
	names := []string{missing, "libstdc++6-" + suffix, "Mozilla Firefox (x64 en-GB) " + suffix, "libstdc++6-" + suffix}

	got, err := NewPatchRunsStore(d).PackageIDsByName(ctx, names)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(stored) {
		t.Fatalf("got %v, want %v", got, stored)
	}
	for name, id := range stored {
		if got[name] != id {
			t.Errorf("%q: got %q, want %q", name, got[name], id)
		}
	}
	if _, ok := got[missing]; ok {
		t.Errorf("%q is not in the inventory and must not map", missing)
	}

	if empty, err := NewPatchRunsStore(d).PackageIDsByName(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("nil names: got %v, %v", empty, err)
	}
}
