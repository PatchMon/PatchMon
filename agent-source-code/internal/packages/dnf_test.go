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
		{
			name: "subscription-manager messages are not packages (issue #864)",
			input: `Updating Subscription Management repositories.
Unable to read consumer identity

This system is not registered with an entitlement server. You can use subscription-manager to register.

Installed Packages
bash.x86_64                          5.1.8-6.el9_1                        @baseos`,
			expected: map[string]models.Package{
				"bash": {
					Name:           "bash",
					CurrentVersion: "5.1.8-6.el9_1",
					NeedsUpdate:    false,
				},
			},
		},
		{
			name: "package name containing dots keeps full name",
			input: `Installed Packages
python3.11.x86_64                    3.11.9-7.el9_5.2                     @appstream`,
			expected: map[string]models.Package{
				"python3.11": {
					Name:           "python3.11",
					CurrentVersion: "3.11.9-7.el9_5.2",
					NeedsUpdate:    false,
				},
			},
		},
		{
			name: "wrapped long package name (legacy yum)",
			input: `Installed Packages
NetworkManager-config-server.noarch
                                     1:1.18.8-2.el7_9                     @updates
bash.x86_64                          4.2.46-35.el7_9                      @updates`,
			expected: map[string]models.Package{
				"NetworkManager-config-server": {
					Name:           "NetworkManager-config-server",
					CurrentVersion: "1:1.18.8-2.el7_9",
					NeedsUpdate:    false,
				},
				"bash": {
					Name:           "bash",
					CurrentVersion: "4.2.46-35.el7_9",
					NeedsUpdate:    false,
				},
			},
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
		{
			name: "subscription-manager messages are not upgradable packages (issue #864)",
			input: `Updating Subscription Management repositories.
Unable to read consumer identity

This system is not registered with an entitlement server. You can use subscription-manager to register.

Last metadata expiration check: 0:19:27 ago on Mon Jul  6 10:00:00 2026.
kernel.x86_64                     5.14.0-284.30.1.el9_2           baseos`,
			pkgMgr: "dnf",
			// Simulate phantom entries created by older agents from the same
			// noise lines: they must not turn the messages into packages.
			installedPackages: map[string]models.Package{
				"kernel.x86_64": {
					Name:           "kernel.x86_64",
					CurrentVersion: "5.14.0-284.30.1.el9_1",
				},
				"This":     {Name: "This", CurrentVersion: "system"},
				"Unable":   {Name: "Unable", CurrentVersion: "to"},
				"Updating": {Name: "Updating", CurrentVersion: "Subscription"},
			},
			securityPackages: map[string]bool{},
			expected:         1,
			expectedSecurity: 0,
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
