package packages

import (
	"fmt"
	"io"
	"os/exec"
	"testing"

	"github.com/sirupsen/logrus"
)

// stubCommandLookup makes checkupdates appear absent and records what the
// fallback tries to run.
func stubCommandLookup(t *testing.T, stdout string, exitCode int) *struct {
	name string
	args []string
} {
	t.Helper()

	origLook, origRun := lookPath, runCommand
	t.Cleanup(func() { lookPath, runCommand = origLook, origRun })

	called := &struct {
		name string
		args []string
	}{}

	lookPath = func(file string) (string, error) {
		if file == "checkupdates" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + file, nil
	}

	runCommand = func(name string, args ...string) *exec.Cmd {
		called.name = name
		called.args = args
		if exitCode != 0 {
			return exec.Command("sh", "-c", fmt.Sprintf("exit %d", exitCode))
		}
		return exec.Command("printf", "%s", stdout)
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
	called := stubCommandLookup(t, "device-mapper 2.03.41-1 -> 2.03.42-1\nlinux 6.9.1-1 -> 6.9.2-1\n", 0)

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

// pacman -Qu exits 1 when nothing is upgradable. That is a normal empty
// result, not a failure.
func TestGetUpgradablePackages_FallbackTreatsExit1AsEmpty(t *testing.T) {
	stubCommandLookup(t, "", 1)

	m := newTestPacmanManager()
	pkgs, err := m.getUpgradablePackages()
	if err != nil {
		t.Fatalf("exit 1 should mean no updates, got error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected no packages, got %d: %+v", len(pkgs), pkgs)
	}
}
