package packages

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestExtractPackageNameAndVersion(t *testing.T) {
	logger := logrus.New()
	manager := NewFreeBSDManager(logger)

	tests := []struct {
		input       string
		wantName    string
		wantVersion string
	}{
		{"bash-5.3.9", "bash", "5.3.9"},
		{"go125-1.25.7", "go125", "1.25.7"},
		{"libX11-1.8.12,1", "libX11", "1.8.12,1"},
		{"python311-3.11.9", "python311", "3.11.9"},
		{"noto-emoji-2.042", "noto-emoji", "2.042"},
		{"curl-8.17.0", "curl", "8.17.0"},
		{"git-2.46.0", "git", "2.46.0"},
		{"pkg-1.21.3", "pkg", "1.21.3"},
		{"openssl-3.0.15,1", "openssl", "3.0.15,1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, version := manager.extractPackageNameAndVersion(tt.input)
			if name != tt.wantName || version != tt.wantVersion {
				t.Errorf("extractPackageNameAndVersion(%q) = (%q, %q), want (%q, %q)",
					tt.input, name, version, tt.wantName, tt.wantVersion)
			}
		})
	}
}

func TestParseInstalledPackages(t *testing.T) {
	logger := logrus.New()
	manager := NewFreeBSDManager(logger)

	input := `bash-5.3.9                     GNU Project's Bourne Again SHell
curl-8.17.0                    Command line tool and library for transferring data with URLs
go125-1.25.7                   Go programming language
libX11-1.8.12,1                X11 library`

	result := manager.parseInstalledPackages(input)

	expectedPackages := map[string]struct {
		version string
	}{
		"bash":   {"5.3.9"},
		"curl":   {"8.17.0"},
		"go125":  {"1.25.7"},
		"libX11": {"1.8.12,1"},
	}

	if len(result) != len(expectedPackages) {
		t.Errorf("Expected %d packages, got %d", len(expectedPackages), len(result))
	}

	for pkg, expected := range expectedPackages {
		if result[pkg] != expected.version {
			t.Errorf("Expected %s version %s, got %s", pkg, expected.version, result[pkg])
		}
	}
}

func TestParseUpgradeOutput(t *testing.T) {
	logger := logrus.New()
	manager := NewFreeBSDManager(logger)

	input := `Checking for upgrades (5 candidates): ..... done
Processing candidates (5 candidates): ..... done
The following 2 package(s) will be affected (of 0 checked):

Installed packages to be UPGRADED:
	curl: 8.9.1 -> 8.10.0
	git: 2.46.0 -> 2.46.1

Number of packages to be upgraded: 2`

	result := manager.parseUpgradeOutput(input, nil)

	if len(result) != 2 {
		t.Fatalf("Expected 2 upgradable packages, got %d", len(result))
	}

	// Check curl upgrade
	foundCurl := false
	for _, pkg := range result {
		if pkg.Name == "curl" {
			foundCurl = true
			if pkg.CurrentVersion != "8.9.1" {
				t.Errorf("curl current version: expected 8.9.1, got %s", pkg.CurrentVersion)
			}
			if pkg.AvailableVersion != "8.10.0" {
				t.Errorf("curl available version: expected 8.10.0, got %s", pkg.AvailableVersion)
			}
			if !pkg.NeedsUpdate {
				t.Error("curl should need update")
			}
		}
	}

	if !foundCurl {
		t.Error("curl package not found in upgrades")
	}

	// Check git upgrade
	foundGit := false
	for _, pkg := range result {
		if pkg.Name == "git" {
			foundGit = true
			if pkg.CurrentVersion != "2.46.0" {
				t.Errorf("git current version: expected 2.46.0, got %s", pkg.CurrentVersion)
			}
			if pkg.AvailableVersion != "2.46.1" {
				t.Errorf("git available version: expected 2.46.1, got %s", pkg.AvailableVersion)
			}
		}
	}

	if !foundGit {
		t.Error("git package not found in upgrades")
	}
}

func TestParseAuditOutput(t *testing.T) {
	logger := logrus.New()
	manager := NewFreeBSDManager(logger)

	input := `curl-8.9.1 is vulnerable:
  curl -- multiple vulnerabilities
  CVE: CVE-2024-XXXX
  WWW: https://vuxml.FreeBSD.org/freebsd/abcd1234-5678-90ab-cdef-1234567890ab.html

openssl-3.0.14,1 is vulnerable:
  openssl -- security update
  CVE: CVE-2024-YYYY
  WWW: https://vuxml.FreeBSD.org/freebsd/1234abcd-5678-90ab-cdef-abcdef123456.html

2 problem(s) in 2 installed package(s) found.`

	result := manager.parseAuditOutput(input)

	expectedVulnerable := map[string]bool{
		"curl":    true,
		"openssl": true,
	}

	if len(result) != len(expectedVulnerable) {
		t.Errorf("Expected %d vulnerable packages, got %d", len(expectedVulnerable), len(result))
	}

	for pkg := range expectedVulnerable {
		if !result[pkg] {
			t.Errorf("Expected %s to be vulnerable", pkg)
		}
	}
}

func TestParseUpgradeOutputEmpty(t *testing.T) {
	logger := logrus.New()
	manager := NewFreeBSDManager(logger)

	input := `Checking for upgrades (0 candidates): done
Processing candidates (0 candidates): done
Checking integrity... done (0 conflicting)
Your packages are up to date.`

	result := manager.parseUpgradeOutput(input, nil)

	if len(result) != 0 {
		t.Errorf("Expected 0 upgradable packages, got %d", len(result))
	}
}
