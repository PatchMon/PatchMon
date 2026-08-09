package packages

import (
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// recordedCmd captures what the fallback actually tried to run.
type recordedCmd struct {
	name string
	args []string
}

// stubCommandLookup makes checkupdates appear absent and stubs the command
// runner with fixed output.
//
// It swaps package-level vars, so callers must NOT call t.Parallel(): the
// restore is scoped to the test and a parallel test would observe the swap.
func stubCommandLookup(t *testing.T, stdout, stderr string, exitCode int) *recordedCmd {
	t.Helper()

	origLook, origRun := lookPath, runCommand
	t.Cleanup(func() { lookPath, runCommand = origLook, origRun })

	called := &recordedCmd{}

	lookPath = func(file string) (string, error) {
		if file == "checkupdates" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + file, nil
	}

	runCommand = func(name string, args ...string) *exec.Cmd {
		called.name = name
		called.args = args
		// stdout and stderr are passed as positional arguments so no quoting
		// of the payload leaks into the shell command itself.
		script := `printf '%s' "$1"; printf '%s' "$2" >&2; exit ` + strconv.Itoa(exitCode)
		return exec.Command("sh", "-c", script, "sh", stdout, stderr)
	}

	return called
}

func newTestPacmanManager() *PacmanManager {
	log := logrus.New()
	log.SetOutput(io.Discard)
	return &PacmanManager{logger: log}
}

// Without pacman-contrib installed, checkupdates is absent. That used to return
// an error, which aborted the entire report and left the host with no OS,
// architecture or agent version recorded, not merely no packages.
func TestGetUpgradablePackages_FallsBackWhenCheckupdatesMissing(t *testing.T) {
	called := stubCommandLookup(t, "device-mapper 2.03.41-1 -> 2.03.42-1\nlinux 6.9.1-1 -> 6.9.2-1\n", "", 0)

	m := newTestPacmanManager()
	pkgs, err := m.getUpgradablePackages()
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}

	if called.name != "pacman" || len(called.args) != 1 || called.args[0] != "-Qu" {
		t.Errorf("expected fallback to run `pacman -Qu`, got %q %v", called.name, called.args)
	}

	if len(pkgs) != 2 {
		t.Fatalf("expected 2 upgradable packages, got %d: %+v", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "device-mapper" {
		t.Errorf("first package name = %q, want %q", pkgs[0].Name, "device-mapper")
	}
	if pkgs[0].CurrentVersion != "2.03.41-1" || pkgs[0].AvailableVersion != "2.03.42-1" {
		t.Errorf("first package versions = %q -> %q, want %q -> %q",
			pkgs[0].CurrentVersion, pkgs[0].AvailableVersion, "2.03.41-1", "2.03.42-1")
	}
}

// pacman -Qu exits 1 with no stderr when nothing is upgradable. That is a
// normal empty result, not a failure.
func TestGetUpgradablePackages_FallbackTreatsSilentExit1AsEmpty(t *testing.T) {
	stubCommandLookup(t, "", "", 1)

	m := newTestPacmanManager()
	pkgs, err := m.getUpgradablePackages()
	if err != nil {
		t.Fatalf("silent exit 1 should mean no updates, got error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected no packages, got %d: %+v", len(pkgs), pkgs)
	}
}

// pacman -Qu also exits 1 on a genuine failure, and the only thing separating
// the two is stderr. Reporting a broken package database as "no updates" would
// make an unhealthy host look green, which is worse than surfacing the error.
func TestGetUpgradablePackages_FallbackExit1WithStderrIsAnError(t *testing.T) {
	stubCommandLookup(t, "", "error: could not open file /var/lib/pacman/sync/core.db\n", 1)

	m := newTestPacmanManager()
	_, err := m.getUpgradablePackages()
	if err == nil {
		t.Fatal("expected an error when pacman -Qu writes to stderr, got nil")
	}
	if !strings.Contains(err.Error(), "core.db") {
		t.Errorf("expected stderr to be surfaced in the error, got: %v", err)
	}
}

// pacman -Qu appends " [ignored]" for IgnorePkg entries, whereas checkupdates
// filters them out itself. The shared parser happens to drop them because its
// pattern is anchored and the trailing marker contains a space.
//
// That agreement is load-bearing but not obvious: relaxing checkUpdateRe to
// tolerate trailing content would silently start reporting ignored packages as
// pending updates, which is exactly the false positive this must never emit.
func TestGetUpgradablePackages_FallbackExcludesIgnoredPackages(t *testing.T) {
	stubCommandLookup(t, "device-mapper 2.03.41-1 -> 2.03.42-1\nlinux 6.9.1-1 -> 6.9.2-1 [ignored]\n", "", 0)

	m := newTestPacmanManager()
	pkgs, err := m.getUpgradablePackages()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected ignored package to be excluded, got %d: %+v", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "device-mapper" {
		t.Errorf("wrong package survived: %q", pkgs[0].Name)
	}
}
