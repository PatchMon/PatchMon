package packages

import (
	"patchmon-agent/pkg/models"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestDNFManager_parseInstalledPackages(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewDNFManager(logger)

	tests := []struct {
		name     string
		input    string
		expected map[string]models.Package
	}{
		{
			name: "valid packages",
			input: `Installed Packages
vim-enhanced.x86_64                  2:8.2.2637-20.el9_1                  @baseos
bash.x86_64                          5.1.8-6.el9_1                        @baseos`,
			expected: map[string]models.Package{
				"vim-enhanced": {
					Name:           "vim-enhanced",
					CurrentVersion: "2:8.2.2637-20.el9_1",
					NeedsUpdate:    false,
				},
				"bash": {
					Name:           "bash",
					CurrentVersion: "5.1.8-6.el9_1",
					NeedsUpdate:    false,
				},
			},
		},
		{
			name:     "empty input",
			input:    "",
			expected: map[string]models.Package{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.parseInstalledPackages(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDNFManager_parseUpgradablePackages(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewDNFManager(logger)

	tests := []struct {
		name              string
		input             string
		pkgMgr            string
		installedPackages map[string]models.Package
		securityPackages  map[string]bool
		expected          int
		expectedSecurity  int
	}{
		{
			name: "upgradable packages",
			input: `kernel.x86_64                     5.14.0-284.30.1.el9_2           baseos
systemd.x86_64                    252-14.el9_2.2                  baseos`,
			pkgMgr: "dnf",
			installedPackages: map[string]models.Package{
				"kernel.x86_64": {
					Name:           "kernel.x86_64",
					CurrentVersion: "5.14.0-284.30.1.el9_1",
				},
				"systemd.x86_64": {
					Name:           "systemd.x86_64",
					CurrentVersion: "252-14.el9_2.1",
				},
			},
			securityPackages: map[string]bool{
				"kernel": true,
			},
			expected:         2,
			expectedSecurity: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.parseUpgradablePackages(tt.input, tt.pkgMgr, tt.installedPackages, tt.securityPackages)
			assert.Equal(t, tt.expected, len(result))
			securityCount := 0
			for _, pkg := range result {
				if pkg.IsSecurityUpdate {
					securityCount++
				}
			}
			assert.Equal(t, tt.expectedSecurity, securityCount)
		})
	}
}

func TestDNFManager_extractBasePackageName(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewDNFManager(logger)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "package with version and arch from updateinfo",
			input:    "glib2-2.68.4-16.el9_6.2.x86_64",
			expected: "glib2",
		},
		{
			name:     "package with dashes in name",
			input:    "glibc-common-2.34-168.el9_6.19.x86_64",
			expected: "glibc-common",
		},
		{
			name:     "package with arch from check-update",
			input:    "glib2.x86_64",
			expected: "glib2",
		},
		{
			name:     "package with noarch",
			input:    "firewalld-filesystem.noarch",
			expected: "firewalld-filesystem",
		},
		{
			name:     "package with version but no arch",
			input:    "glib2-2.68.4-16.el9_6.2",
			expected: "glib2",
		},
		{
			name:     "simple package name",
			input:    "kernel",
			expected: "kernel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.extractBasePackageName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
