//go:build windows

package packages

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run against an existing Chocolatey installation; never installs packages or
// changes sources. PATCHMON_CHOCO_EXPECT_FIXTURES enables the local-feed fixture
// assertions described in README.md.
func TestChocolateyCollectorLive(t *testing.T) {
	if os.Getenv("PATCHMON_CHOCO_INTEGRATION") != "1" {
		t.Skip("set PATCHMON_CHOCO_INTEGRATION=1 to query the host's Chocolatey installation")
	}
	exe, err := findChocolatey()
	if err != nil {
		t.Fatal(err)
	}
	if exe == "" {
		t.Skip("Chocolatey is not installed")
	}
	pkgs, err := collectChocolateyPackages()
	if err != nil {
		t.Fatal(err)
	}
	updates, fixtures := 0, 0
	for _, p := range pkgs {
		if p.WingetID != "" || p.WUAGuid != "" {
			t.Fatalf("Chocolatey record is patchable through another manager: %+v", p)
		}
		if p.NeedsUpdate {
			updates++
		}
		if strings.HasSuffix(p.Name, "patchmon-inventory-fixture") || strings.HasSuffix(p.Name, "patchmon-pinned-fixture") {
			if p.CurrentVersion != "1.0.0" || p.AvailableVersion != "2.0.0" || !p.NeedsUpdate {
				t.Fatalf("fixture update not detected: %+v", p)
			}
			fixtures++
		}
	}
	if os.Getenv("PATCHMON_CHOCO_EXPECT_FIXTURES") == "1" && fixtures != 2 {
		t.Fatalf("expected two outdated fixtures, found %d", fixtures)
	}
	t.Logf("Chocolatey inventory: %d packages, %d outdated", len(pkgs), updates)
	if target := os.Getenv("PATCHMON_CHOCO_TEST_OUTPUT"); target != "" {
		data, err := json.MarshalIndent(pkgs, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// Exercise the production Manager/report error boundary with an executable that
// cannot start. No real package manager or Windows Update query should run.
func TestChocolateyFailureBlocksWindowsInventory(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "choco.exe"), []byte("not an executable"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ChocolateyInstall", root)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	pkgs, err := New(logger, CacheRefreshConfig{}).GetPackages()
	if err == nil || len(pkgs) != 0 {
		t.Fatalf("failed Chocolatey collection accepted: %+v %v", pkgs, err)
	}
}
