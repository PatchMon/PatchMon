package repositories

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDNFManager_parseRepoFile(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewDNFManager(logger)

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.repo")

	content := `[baseos]
name=Rocky Linux $releasever - BaseOS
baseurl=https://download.rockylinux.org/pub/rocky/$releasever/BaseOS/$basearch/os/
gpgcheck=1
enabled=1

[appstream]
name=Rocky Linux $releasever - AppStream
mirrorlist=https://mirrors.rockylinux.org/mirrorlist?repo=AppStream-$releasever&arch=$basearch
enabled=1

[disabled-repo]
name=Disabled Repo
baseurl=https://example.com/disabled
enabled=0
`
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0644))

	repos, err := manager.parseRepoFile(testFile)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(repos), 2, "Should have at least 2 enabled repos")

	// Check that disabled repo was skipped
	for _, repo := range repos {
		assert.NotEqual(t, "disabled-repo", repo.Name)
	}
}

func TestDNFManager_processRepoEntry(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewDNFManager(logger)

	tests := []struct {
		name          string
		entry         *repoEntry
		expectedCount int
	}{
		{
			name: "baseurl only",
			entry: &repoEntry{
				id:       "test-repo",
				name:     "Test Repository",
				baseurls: []string{"https://example.com/repo1", "https://example.com/repo2"},
			},
			expectedCount: 2,
		},
		{
			name: "mirrorlist only",
			entry: &repoEntry{
				id:         "test-mirror",
				name:       "Test Mirror",
				mirrorlist: "https://mirrors.example.com/list",
			},
			expectedCount: 1,
		},
		{
			name: "multiple URLs prioritized",
			entry: &repoEntry{
				id:         "test-multi",
				name:       "Test Multi",
				baseurls:   []string{"https://example.com/repo"},
				metalink:   "https://example.com/metalink",
				mirrorlist: "https://mirrors.example.com/list",
			},
			expectedCount: 3, // baseurl first, then metalink, then mirrorlist
		},
		{
			name: "disabled repo",
			entry: &repoEntry{
				id:       "disabled",
				name:     "Disabled",
				baseurls: []string{"https://example.com/repo"},
				enabled:  boolPtr(false),
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.processRepoEntry(tt.entry)
			assert.Equal(t, tt.expectedCount, len(result))
		})
	}
}

func TestDNFManager_isValidRepoURL(t *testing.T) {
	manager := &DNFManager{}

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"http URL", "http://example.com", true},
		{"https URL", "https://example.com", true},
		{"ftp URL", "ftp://example.com", true},
		{"file URL", "file:///local/path", false},
		{"empty URL", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isValidRepoURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDNFManager_isSecureURL(t *testing.T) {
	manager := &DNFManager{}

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"https URL", "https://example.com", true},
		{"http URL", "http://example.com", false},
		{"ftp URL", "ftp://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isSecureURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
