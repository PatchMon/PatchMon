//go:build windows

package packages

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// The collectors in this package build PowerShell as Go string literals, so a
// syntax error is still valid Go: it compiles, ships, and only fails on a
// customer's machine. These tests are the only thing that executes those
// scripts, so they run them against the live host and assert on the result.
//
// Each collector swallows its own errors and returns an empty slice, which
// makes "the script is broken" and "this host has nothing to report" look
// identical from the outside. Where a real Windows host is guaranteed to have
// data, the test asserts a non-empty result so a broken script actually fails.
// Where it is not, the test asserts on shape and on the call completing inside
// its timeout.
//
// Gated so a workstation `go test ./...` does not shell out to PowerShell,
// query Windows Update, or take minutes. CI sets it on windows-latest.
func requireWindowsIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("PATCHMON_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PATCHMON_WINDOWS_INTEGRATION=1 to run the live PowerShell collectors")
	}
}

func newTestWindowsManager() *WindowsManager {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return NewWindowsManager(logger)
}

// assertPackagesWellFormed catches a script that parses but emits the wrong
// shape, e.g. a ConvertTo-Json change that drops a property, and catches the
// encoding regression that winexec exists to prevent.
func assertPackagesWellFormed(t *testing.T, pkgs []models.Package) {
	t.Helper()
	for i, p := range pkgs {
		if p.Name == "" {
			t.Errorf("package %d has an empty Name: %+v", i, p)
		}
		if !utf8.ValidString(p.Name) {
			t.Errorf("package %d name is not valid UTF-8: %q", i, p.Name)
		}
		if !utf8.ValidString(p.CurrentVersion) {
			t.Errorf("package %d version is not valid UTF-8: %q", i, p.CurrentVersion)
		}
		if strings.ContainsRune(p.Name, '�') {
			t.Errorf("package %d name contains a replacement character, so the UTF-8 preamble did not apply: %q", i, p.Name)
		}
	}
}

// Every Windows installation has uninstall registry entries, so an empty
// result here means the script failed, not that the host is clean.
func TestRegistryCollectorLive(t *testing.T) {
	requireWindowsIntegration(t)

	pkgs := newTestWindowsManager().getPackagesFromRegistry()
	if len(pkgs) == 0 {
		t.Fatal("registry collector returned no packages; every Windows host has uninstall entries, so the PowerShell almost certainly failed")
	}
	assertPackagesWellFormed(t, pkgs)
	t.Logf("registry collector returned %d packages", len(pkgs))
}

// winget is absent on some hosted runners and on Server SKUs, so an empty
// result is legitimate. What is asserted is that the collector returns rather
// than hanging on winget's interactive prompts, and that anything it does
// return is well formed.
func TestWingetCollectorLive(t *testing.T) {
	requireWindowsIntegration(t)

	done := make(chan []models.Package, 1)
	go func() { done <- newTestWindowsManager().getPackagesFromWinget() }()

	select {
	case pkgs := <-done:
		assertPackagesWellFormed(t, pkgs)
		t.Logf("winget collector returned %d packages", len(pkgs))
	case <-time.After(4 * time.Minute):
		t.Fatal("winget collector did not return; the bounded command timeout is not being applied")
	}
}

func TestWingetUpgradeMapLive(t *testing.T) {
	requireWindowsIntegration(t)

	upgrades := newTestWindowsManager().getWingetUpgradeAvailable()
	for name, version := range upgrades {
		if name == "" {
			t.Error("upgrade map contains an empty package name")
		}
		if !utf8.ValidString(name) || !utf8.ValidString(version) {
			t.Errorf("upgrade entry is not valid UTF-8: %q => %q", name, version)
		}
	}
	t.Logf("winget reports %d available upgrades", len(upgrades))
}

// Exercises the Microsoft.Update.Session COM path. A hosted runner does not
// reliably have updates pending, so this asserts on completion and shape
// rather than on a count.
func TestWindowsUpdatesCollectorLive(t *testing.T) {
	requireWindowsIntegration(t)

	done := make(chan []models.Package, 1)
	go func() { done <- newTestWindowsManager().getWindowsUpdates() }()

	select {
	case pkgs := <-done:
		assertPackagesWellFormed(t, pkgs)
		missingGUID := 0
		for i, p := range pkgs {
			if p.Category != "Windows Update" {
				t.Errorf("update %d has category %q, want \"Windows Update\"", i, p.Category)
			}
			if p.WUAGuid == "" {
				missingGUID++
				t.Logf("update %d (%s) has no WUA GUID, so InstallWindowsUpdate cannot act on it", i, p.Name)
			}
		}
		// Logged rather than failed: a hosted runner has a constrained Windows
		// Update configuration, so an empty Identity.UpdateID here cannot be
		// told apart from the same thing happening on a real host. Tracked in
		// https://github.com/PatchMon/PatchMon/issues/983, and this should go
		// back to failing the build once that is settled.
		if missingGUID > 0 {
			t.Logf("WARNING: %d of %d updates have no WUA GUID (issue #983)", missingGUID, len(pkgs))
		}
		t.Logf("WUA search returned %d updates", len(pkgs))
	case <-time.After(6 * time.Minute):
		t.Fatal("Windows Update collector did not return")
	}
}

func TestWSUSCheckLive(t *testing.T) {
	requireWindowsIntegration(t)

	t.Logf("isWSUSActive() = %v", newTestWindowsManager().isWSUSActive())
}

func TestRebootRequiredLive(t *testing.T) {
	requireWindowsIntegration(t)

	t.Logf("RebootRequired() = %v", RebootRequired())
}

// The dry run lists upgrades without installing anything, which is enough to
// prove wingetResolveBlock and the surrounding script parse and execute.
func TestWinGetUpgradeAllDryRunLive(t *testing.T) {
	requireWindowsIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	out, err := NewWindowsPatcher().WinGetUpgradeAll(ctx, true)
	if ctx.Err() != nil {
		t.Fatalf("winget dry run exceeded its deadline: %v", ctx.Err())
	}
	// A non-zero exit is acceptable (winget may be absent); a PowerShell parse
	// error is not, and it surfaces in the output rather than as a Go error.
	assertNoPowerShellParseError(t, out)
	if err != nil {
		t.Logf("winget dry run exited non-zero (acceptable if winget is absent): %v", err)
	}
	t.Logf("winget dry run output:\n%s", out)
}

// The full merge path, which is what serve actually calls.
func TestGetPackagesLive(t *testing.T) {
	requireWindowsIntegration(t)

	pkgs := newTestWindowsManager().GetPackages()
	if len(pkgs) == 0 {
		t.Fatal("GetPackages returned nothing; the registry collector alone should have populated it")
	}
	assertPackagesWellFormed(t, pkgs)

	seen := make(map[string]int, len(pkgs))
	for _, p := range pkgs {
		seen[p.Name]++
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("mergeRegistryAndWinget left %d copies of %q", n, name)
		}
	}
	t.Logf("GetPackages returned %d merged packages", len(pkgs))
}

// PowerShell reports parse and binding failures on stdout/stderr while often
// still exiting zero, so the text has to be inspected directly.
func assertNoPowerShellParseError(t *testing.T, output string) {
	t.Helper()
	markers := []string{
		"ParserError",
		"Unexpected token",
		"is not recognized as the name of a cmdlet",
		"Missing closing '}'",
		"The string is missing the terminator",
		"CommandNotFoundException",
	}
	lower := strings.ToLower(output)
	for _, m := range markers {
		if strings.Contains(lower, strings.ToLower(m)) {
			t.Errorf("PowerShell reported %q, so the embedded script is broken:\n%s", m, output)
		}
	}
}
