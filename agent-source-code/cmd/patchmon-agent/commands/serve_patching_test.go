package commands

import (
	"reflect"
	"strings"
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
			name: "upgrade disables retries and forces conffile defaults",
			got:  aptUpgradeArgs(false, true),
			want: []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "-o", "Acquire::Retries=0", "--with-new-pkgs", "upgrade", "-y"},
		},
		{
			name: "upgrade keeps retries when disabled is off",
			got:  aptUpgradeArgs(false, false),
			want: []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "--with-new-pkgs", "upgrade", "-y"},
		},
		{
			name: "simulated upgrade stays unchanged",
			got:  aptUpgradeArgs(true, true),
			want: []string{"-s", "--with-new-pkgs", "upgrade"},
		},
		{
			name: "package upgrade disables retries and forces conffile defaults",
			got:  aptOnlyUpgradeArgs(false, true, []string{"curl", "git"}),
			want: []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "-o", "Acquire::Retries=0", "--only-upgrade", "install", "-y", "curl", "git"},
		},
		{
			name: "package upgrade keeps retries when disabled is off",
			got:  aptOnlyUpgradeArgs(false, false, []string{"curl", "git"}),
			want: []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "--only-upgrade", "install", "-y", "curl", "git"},
		},
		{
			name: "simulated package upgrade stays unchanged",
			got:  aptOnlyUpgradeArgs(true, true, []string{"curl"}),
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
	dpkgWant := []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}
	retriesWant := []string{"-o", "Acquire::Retries=0"}

	got := aptOnlyUpgradeArgs(false, true, []string{"curl"})
	for i := range got {
		got[i] = "mutated"
	}
	if !reflect.DeepEqual(dpkgConfOptions, dpkgWant) {
		t.Fatalf("dpkgConfOptions mutated: %v", dpkgConfOptions)
	}
	if !reflect.DeepEqual(aptRetriesOff, retriesWant) {
		t.Fatalf("aptRetriesOff mutated: %v", aptRetriesOff)
	}

	first := aptOnlyUpgradeArgs(false, true, []string{"curl"})
	second := aptUpgradeArgs(false, true)
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

func TestPatchTypeRefusal(t *testing.T) {
	cases := []struct {
		pkgManager, patchType string
		dryRun                bool
		refused               bool
	}{
		{"pacman", "patch_package", false, true},
		{"pacman", "patch_package", true, true},
		{"pacman", "patch_all", true, true},
		{"pacman", "patch_all", false, false},
		{"apt", "patch_package", true, false},
		{"dnf", "patch_package", false, false},
		{"apk", "patch_package", false, false},
	}
	for _, c := range cases {
		got := patchTypeRefusal(c.pkgManager, c.patchType, c.dryRun)
		if (got != "") != c.refused {
			t.Errorf("patchTypeRefusal(%q, %q, %v) = %q, refused want %v", c.pkgManager, c.patchType, c.dryRun, got, c.refused)
		}
	}
	if got := patchTypeRefusal("pacman", "patch_package", false); !strings.Contains(got, "partial upgrades") {
		t.Errorf("refusal must explain partial upgrades, got %q", got)
	}
}

func TestUCSUpgradeArgs(t *testing.T) {
	got := ucsUpgradeArgs("5.2-7")
	want := []string{"--noninteractive", "--disable-app-updates", "--updateto", "5.2-7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ucsUpgradeArgs() = %v, want %v", got, want)
	}
}

func TestUCSUpgradeRefusalWithoutRelease(t *testing.T) {
	if msg := ucsUpgradeRefusal(""); msg == "" {
		t.Fatal("expected a refusal when the UCS release is unknown")
	}
}
