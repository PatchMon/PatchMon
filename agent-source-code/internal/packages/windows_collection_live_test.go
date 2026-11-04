//go:build windows

package packages

import (
	"os"
	"strings"
	"testing"
	"time"
)

// The test executable doubles as a controlled WinGet subprocess. The registry
// collector still runs real PowerShell against the Windows host.
func init() {
	if os.Getenv("PATCHMON_TEST_WINGET_HELPER") != "1" {
		return
	}
	mode := os.Getenv("PATCHMON_TEST_WINGET_MODE")
	if mode == "list-failure" || (mode == "upgrade-failure" && strings.Contains(strings.Join(os.Args[1:], " "), "--upgrade-available")) {
		os.Exit(1)
	}
	_, _ = os.Stdout.WriteString(os.Getenv("PATCHMON_TEST_WINGET_TABLE"))
	os.Exit(0)
}

func TestApplicationScanFailureLive(t *testing.T) {
	requireWindowsIntegration(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	original := sharedWingetResolver
	sharedWingetResolver = &wingetResolver{resolve: func() (string, []string, error) {
		env := append(os.Environ(), "PATCHMON_TEST_WINGET_HELPER=1")
		return exe, env, nil
	}, now: time.Now}
	t.Cleanup(func() { sharedWingetResolver = original })
	t.Setenv("PATCHMON_TEST_WINGET_TABLE", wingetTable(24, []wingetTestRow{{"Scan Fixture", "Fixture.Scan", "1.0", "2.0", "winget"}}))
	m := newTestWindowsManager()
	healthy := func() {
		t.Helper()
		pkgs, err := m.appPackages()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range pkgs {
			if p.Name == "Fixture.Scan" || p.Name == "Scan Fixture" {
				if !p.NeedsUpdate || p.AvailableVersion != "2.0" {
					t.Fatalf("lost fixture update: %+v", p)
				}
				t.Logf("successful collection: %d applications, fixture update preserved", len(pkgs))
				return
			}
		}
		t.Fatalf("fixture absent: %+v", pkgs)
	}
	healthy()
	for _, mode := range []string{"list-failure", "upgrade-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("PATCHMON_TEST_WINGET_MODE", mode)
			pkgs, _, err := m.GetPackages()
			if err == nil || len(pkgs) != 0 {
				t.Fatalf("incomplete inventory accepted: %+v, %v", pkgs, err)
			}
			t.Logf("incomplete report rejected: %v", err)
		})
		healthy()
	}
}
