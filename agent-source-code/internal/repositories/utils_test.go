package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateRepoName(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		distribution string
		components   string
		expected     string
	}{
		{
			name:         "Ubuntu main repository",
			url:          "http://archive.ubuntu.com/ubuntu",
			distribution: "jammy",
			components:   "main restricted",
			expected:     "archive-ubuntu-jammy",
		},
		{
			name:         "Security repository",
			url:          "http://security.ubuntu.com/ubuntu",
			distribution: "jammy-security",
			components:   "main",
			expected:     "security-ubuntu-jammy-security",
		},
		{
			name:         "Updates repository",
			url:          "http://archive.ubuntu.com/ubuntu",
			distribution: "jammy-updates",
			components:   "main",
			expected:     "archive-ubuntu-jammy-updates",
		},
		{
			name:         "Single non-main component",
			url:          "http://archive.ubuntu.com/ubuntu",
			distribution: "jammy",
			components:   "universe",
			expected:     "archive-ubuntu-jammy-universe",
		},
		{
			name:         "Backports",
			url:          "http://archive.ubuntu.com/ubuntu",
			distribution: "jammy-backports",
			components:   "main",
			expected:     "archive-ubuntu-jammy-backports",
		},
		{
			name:         "Debian repository",
			url:          "https://deb.debian.org/debian",
			distribution: "bookworm",
			components:   "main contrib non-free",
			expected:     "deb-debian-bookworm",
		},
		{
			name:         "URL with port",
			url:          "http://mirror.example.com:8080/ubuntu",
			distribution: "jammy",
			components:   "main",
			expected:     "mirror-example-jammy",
		},
		{
			name:         "Domain with dots",
			url:          "http://us.archive.ubuntu.com/ubuntu",
			distribution: "jammy",
			components:   "main",
			expected:     "us-archive-ubuntu-jammy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateRepoName(tt.url, tt.distribution, tt.components)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateRepoName_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		distribution   string
		components     string
		shouldNotPanic bool
	}{
		{
			name:           "Empty URL",
			url:            "",
			distribution:   "jammy",
			components:     "main",
			shouldNotPanic: true,
		},
		{
			name:           "Malformed URL",
			url:            "not-a-url",
			distribution:   "jammy",
			components:     "main",
			shouldNotPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				generateRepoName(tt.url, tt.distribution, tt.components)
			})
		})
	}
}
