package commands

import (
	"reflect"
	"testing"
)

func TestSplitFreeBSDPatchTargets(t *testing.T) {
	t.Run("separates base package from pkg targets", func(t *testing.T) {
		gotTargets, gotBase := splitFreeBSDPatchTargets([]string{"freebsd-base", "curl", "git"})
		wantTargets := []string{"curl", "git"}

		if !gotBase {
			t.Fatal("expected freebsd-base target to be detected")
		}
		if !reflect.DeepEqual(gotTargets, wantTargets) {
			t.Fatalf("splitFreeBSDPatchTargets() targets = %v, want %v", gotTargets, wantTargets)
		}
	})

	t.Run("treats case-insensitive base target as base update", func(t *testing.T) {
		gotTargets, gotBase := splitFreeBSDPatchTargets([]string{"FreeBSD-Base"})

		if !gotBase {
			t.Fatal("expected freebsd-base target to be detected case-insensitively")
		}
		if len(gotTargets) != 0 {
			t.Fatalf("expected no pkg targets, got %v", gotTargets)
		}
	})
}

func TestFreeBSDUpdateOutputHasPendingUpdates(t *testing.T) {
	t.Run("detects pending updates", func(t *testing.T) {
		output := `The following files will be updated as part of updating to 14.2-RELEASE-p3:
/bin/freebsd-version`

		if !freeBSDUpdateOutputHasPendingUpdates(output) {
			t.Fatal("expected pending base-system updates")
		}
	})

	t.Run("detects no updates", func(t *testing.T) {
		output := `No updates are available to install.`

		if freeBSDUpdateOutputHasPendingUpdates(output) {
			t.Fatal("expected no pending base-system updates")
		}
	})
}
