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
	manager := NewAPTManager(logger)

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
	manager := NewAPTManager(logger)

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
