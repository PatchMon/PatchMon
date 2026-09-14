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

func TestAptArgs(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{
			name: "upgrade forces conffile defaults",
			got:  aptUpgradeArgs(false),
			want: []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "--with-new-pkgs", "upgrade", "-y"},
		},
		{
			name: "simulated upgrade stays unchanged",
			got:  aptUpgradeArgs(true),
			want: []string{"-s", "--with-new-pkgs", "upgrade"},
		},
		{
			name: "package upgrade forces conffile defaults",
			got:  aptOnlyUpgradeArgs(false, []string{"curl", "git"}),
			want: []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "--only-upgrade", "install", "-y", "curl", "git"},
		},
		{
			name: "simulated package upgrade stays unchanged",
			got:  aptOnlyUpgradeArgs(true, []string{"curl"}),
			want: []string{"-s", "--only-upgrade", "install", "curl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Fatalf("args = %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestAptArgsDoNotShareBacking(t *testing.T) {
	first := aptOnlyUpgradeArgs(false, []string{"curl"})
	second := aptUpgradeArgs(false)

	if !reflect.DeepEqual(dpkgConfOptions, []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}) {
		t.Fatalf("dpkgConfOptions mutated: %v", dpkgConfOptions)
	}
	if reflect.DeepEqual(first, second) {
		t.Fatal("expected distinct argument lists")
	}
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
