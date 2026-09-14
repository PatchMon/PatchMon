package packages

import (
	"testing"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAPTManager_parseInstalledPackages(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewAPTManager(logger, CacheRefreshConfig{Mode: "never"})

	tests := []struct {
		name     string
		input    string
		expected map[string]models.Package
	}{
		{
			name: "valid single package",
			input: `vim 2:8.2.3995-1ubuntu2.17 Vi IMproved - enhanced vi editor
`,
			expected: map[string]models.Package{
				"vim": {
					Name:           "vim",
					CurrentVersion: "2:8.2.3995-1ubuntu2.17",
					Description:    "Vi IMproved - enhanced vi editor",
				},
			},
		},
		{
			name: "multiple packages",
			input: `vim 2:8.2.3995-1ubuntu2.17 Vi IMproved
libc6 2.35-0ubuntu3.8 GNU C Library
bash 5.1-6ubuntu1.1 GNU Bourne Again SHell
`,
			expected: map[string]models.Package{
				"vim": {
					Name:           "vim",
					CurrentVersion: "2:8.2.3995-1ubuntu2.17",
					Description:    "Vi IMproved",
				},
				"libc6": {
					Name:           "libc6",
					CurrentVersion: "2.35-0ubuntu3.8",
					Description:    "GNU C Library",
				},
				"bash": {
					Name:           "bash",
					CurrentVersion: "5.1-6ubuntu1.1",
					Description:    "GNU Bourne Again SHell",
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

func TestAPTManager_parseAPTUpgrade(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewAPTManager(logger, CacheRefreshConfig{Mode: "never"})

	tests := []struct {
		name     string
		input    string
		expected []models.Package
	}{
		{
			name:  "standard upgrade",
			input: `Inst vim [2:8.2.3995-1ubuntu2.16] (2:8.2.3995-1ubuntu2.17 Ubuntu:22.04/jammy-updates [amd64])`,
			expected: []models.Package{
				{
					Name:             "vim",
					CurrentVersion:   "2:8.2.3995-1ubuntu2.16",
					AvailableVersion: "2:8.2.3995-1ubuntu2.17",
					NeedsUpdate:      true,
					IsSecurityUpdate: false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.parseAPTUpgrade(tt.input)
			assert.Equal(t, len(tt.expected), len(result))
		})
	}
}

// Verbatim `apt -s -o Debug::NoLocking=1 upgrade` output from an Ubuntu 24.04
// host with a pending kernel ABI bump. Two shapes matter here:
//
//   - Upgrades carry [current] and are the only real outdated packages. This
//     set must match `apt list --upgradable`, which lists exactly these five.
//   - New installs (the 6.8.0-137 kernel packages apt pulls in) have no
//     [current] at all. They are not installed, so they cannot be outdated,
//     and reporting them inflated this host's count from 5 to 12 whilst
//     showing "amd64])" as the installed version.
//
// Note the trailing "[]" on some lines and the bracketed architecture on all
// of them: both are why the current version has to be located by position
// rather than by finding the first bracketed field.
func TestAPTManager_parseAPTUpgrade_NewInstallsAreNotOutdated(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewAPTManager(logger, CacheRefreshConfig{Mode: "never"})

	const output = `Reading package lists...
Building dependency tree...
Inst linux-modules-6.8.0-137-generic (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64])
Inst linux-image-6.8.0-137-generic (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64])
Inst linux-modules-extra-6.8.0-137-generic (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64])
Inst linux-generic [6.8.0-136.136] (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64]) []
Inst linux-image-generic [6.8.0-136.136] (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64]) []
Inst linux-headers-6.8.0-137 (6.8.0-137.137 Ubuntu:24.04/noble-updates [all]) []
Inst linux-headers-6.8.0-137-generic (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64]) []
Inst linux-headers-generic [6.8.0-136.136] (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64])
Inst linux-libc-dev [6.8.0-136.136] (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64])
Inst linux-tools-common [6.8.0-136.136] (6.8.0-137.137 Ubuntu:24.04/noble-updates [all])
Inst linux-tools-6.8.0-137 (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64])
Inst linux-tools-6.8.0-137-generic (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64])
Conf linux-generic (6.8.0-137.137 Ubuntu:24.04/noble-updates [amd64])`

	got := manager.parseAPTUpgrade(output)

	want := []models.Package{
		{Name: "linux-generic", CurrentVersion: "6.8.0-136.136", AvailableVersion: "6.8.0-137.137", NeedsUpdate: true},
		{Name: "linux-image-generic", CurrentVersion: "6.8.0-136.136", AvailableVersion: "6.8.0-137.137", NeedsUpdate: true},
		{Name: "linux-headers-generic", CurrentVersion: "6.8.0-136.136", AvailableVersion: "6.8.0-137.137", NeedsUpdate: true},
		{Name: "linux-libc-dev", CurrentVersion: "6.8.0-136.136", AvailableVersion: "6.8.0-137.137", NeedsUpdate: true},
		{Name: "linux-tools-common", CurrentVersion: "6.8.0-136.136", AvailableVersion: "6.8.0-137.137", NeedsUpdate: true},
	}
	assert.Equal(t, want, got)
}

func TestAPTManager_parseAPTUpgrade_Shapes(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewAPTManager(logger, CacheRefreshConfig{Mode: "never"})

	tests := []struct {
		name string
		line string
		want []models.Package
	}{
		{
			name: "epoch in both versions",
			line: `Inst vim [2:8.2.3995-1ubuntu2.16] (2:8.2.3995-1ubuntu2.17 Ubuntu:22.04/jammy-updates [amd64])`,
			want: []models.Package{{
				Name: "vim", CurrentVersion: "2:8.2.3995-1ubuntu2.16",
				AvailableVersion: "2:8.2.3995-1ubuntu2.17", NeedsUpdate: true,
			}},
		},
		{
			name: "security pocket sets the flag",
			line: `Inst openssl [3.0.2-0ubuntu1.15] (3.0.2-0ubuntu1.16 Ubuntu:22.04/jammy-security [amd64])`,
			want: []models.Package{{
				Name: "openssl", CurrentVersion: "3.0.2-0ubuntu1.15",
				AvailableVersion: "3.0.2-0ubuntu1.16", NeedsUpdate: true, IsSecurityUpdate: true,
			}},
		},
		{
			name: "multiarch package name is preserved",
			line: `Inst libc6:i386 [2.35-0ubuntu3.6] (2.35-0ubuntu3.8 Ubuntu:22.04/jammy-updates [i386])`,
			want: []models.Package{{
				Name: "libc6:i386", CurrentVersion: "2.35-0ubuntu3.6",
				AvailableVersion: "2.35-0ubuntu3.8", NeedsUpdate: true,
			}},
		},
		{
			name: "two origins listed for one candidate",
			line: `Inst curl [7.81.0-1ubuntu1.15] (7.81.0-1ubuntu1.16 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [amd64])`,
			want: []models.Package{{
				Name: "curl", CurrentVersion: "7.81.0-1ubuntu1.15",
				AvailableVersion: "7.81.0-1ubuntu1.16", NeedsUpdate: true, IsSecurityUpdate: true,
			}},
		},
		{
			name: "new install is skipped",
			line: `Inst linux-headers-6.8.0-137 (6.8.0-137.137 Ubuntu:24.04/noble-updates [all]) []`,
			want: nil,
		},
		{
			name: "non-Inst line is ignored",
			line: `Conf vim (2:8.2.3995-1ubuntu2.17 Ubuntu:22.04/jammy-updates [amd64])`,
			want: nil,
		},
		{
			name: "malformed line is ignored",
			line: `Inst something-odd`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, manager.parseAPTUpgrade(tt.line))
		})
	}
}
