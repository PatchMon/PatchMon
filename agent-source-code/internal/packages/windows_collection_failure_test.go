//go:build !windows

package packages

// Executable fixtures exercise the real collectors without requiring Windows.
import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestWindowsApplicationScanFailures(t *testing.T) {
	root := t.TempDir()
	ps := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if err := os.MkdirAll(filepath.Dir(ps), 0700); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
if [ "$PATCHMON_TEST_REGISTRY_FAIL" = "1" ]; then exit 1; fi
if [ "$PATCHMON_TEST_REGISTRY_FAIL" = "json" ]; then echo invalid; exit 0; fi
printf '%s\n' '[{"Name":"Review App","Version":"1.0","Publisher":"Fixture"}]'
`
	if err := os.WriteFile(ps, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	winget := filepath.Join(root, "winget-fixture")
	tablePath := filepath.Join(root, "table.txt")
	table := wingetTable(20, []wingetTestRow{{"Review App", "Fixture.App", "1.0", "2.0", "winget"}})
	if err := os.WriteFile(tablePath, []byte(table), 0600); err != nil {
		t.Fatal(err)
	}
	scriptWinget := `#!/bin/sh
if [ "$PATCHMON_TEST_WINGET_FAIL" = "1" ]; then exit 1; fi
case "$*" in
  *--upgrade-available*)
    if [ "$PATCHMON_TEST_WINGET_FAIL" = "upgrade" ]; then exit 1; fi
    if [ "$PATCHMON_TEST_WINGET_FAIL" = "none" ]; then exit 0; fi
    ;;
esac
/bin/cat "$PATCHMON_TEST_TABLE"
`
	if err := os.WriteFile(winget, []byte(scriptWinget), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SystemRoot", root)
	t.Setenv("PATCHMON_TEST_TABLE", tablePath)
	t.Setenv("PATCHMON_TEST_REGISTRY_FAIL", "0")
	t.Setenv("PATCHMON_TEST_WINGET_FAIL", "0")
	original := sharedWingetResolver
	sharedWingetResolver = &wingetResolver{resolve: func() (string, []string, error) { return winget, os.Environ(), nil }, now: time.Now}
	t.Cleanup(func() { sharedWingetResolver = original })
	log := logrus.New()
	log.SetOutput(io.Discard)
	manager := NewWindowsManager(log)

	checkHealthy := func(t *testing.T) {
		t.Helper()
		pkgs, err := manager.appPackages()
		if err != nil || len(pkgs) != 1 || !pkgs[0].NeedsUpdate || pkgs[0].AvailableVersion != "2.0" {
			t.Fatalf("healthy inventory: packages=%+v error=%v", pkgs, err)
		}
	}
	checkHealthy(t)
	for _, tc := range []struct{ name, registry, winget, want string }{
		{"list failure", "0", "1", "installed-package scan failed"},
		{"upgrade failure", "0", "upgrade", "available-upgrade scan failed"},
		{"registry failure", "1", "0", "query Windows Uninstall registry"},
		{"registry malformed", "json", "0", "parse Windows Uninstall registry"},
		{"both fail", "1", "1", "query Windows Uninstall registry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PATCHMON_TEST_REGISTRY_FAIL", tc.registry)
			t.Setenv("PATCHMON_TEST_WINGET_FAIL", tc.winget)
			pkgs, err := manager.appPackages()
			if err == nil || !strings.Contains(err.Error(), tc.want) || len(pkgs) != 0 {
				t.Fatalf("must reject incomplete inventory: packages=%+v error=%v", pkgs, err)
			}
		})
		checkHealthy(t)
	}
	t.Run("no upgrades", func(t *testing.T) {
		t.Setenv("PATCHMON_TEST_WINGET_FAIL", "none")
		noUpdates := filepath.Join(root, "no-updates.txt")
		table := wingetTable(20, []wingetTestRow{{"Review App", "Fixture.App", "1.0", "", "winget"}})
		if err := os.WriteFile(noUpdates, []byte(table), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATCHMON_TEST_TABLE", noUpdates)
		pkgs, err := manager.appPackages()
		if err != nil || len(pkgs) != 1 || pkgs[0].NeedsUpdate || pkgs[0].AvailableVersion != "" {
			t.Fatalf("valid scan with no updates: packages=%+v error=%v", pkgs, err)
		}
	})
	t.Run("WinGet absent", func(t *testing.T) {
		resolver := sharedWingetResolver
		sharedWingetResolver = &wingetResolver{resolve: func() (string, []string, error) {
			return "", nil, errWingetNotFound
		}, now: time.Now}
		t.Cleanup(func() { sharedWingetResolver = resolver })
		pkgs, err := manager.appPackages()
		if err != nil || len(pkgs) != 1 || pkgs[0].Name != "Review App" {
			t.Fatalf("registry-only inventory: packages=%+v error=%v", pkgs, err)
		}
	})
}
