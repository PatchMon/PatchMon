package repositories

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPTManager_parseSourceLine(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewAPTManager(logger)

	tests := []struct {
		name         string
		input        string
		expectNil    bool
		expectedURL  string
		expectedDist string
		expectedComp string
	}{
		{
			name:         "standard deb line",
			input:        "deb http://archive.ubuntu.com/ubuntu jammy main restricted",
			expectNil:    false,
			expectedURL:  "http://archive.ubuntu.com/ubuntu",
			expectedDist: "jammy",
			expectedComp: "main restricted",
		},
		{
			name:         "with signed-by option",
			input:        "deb [signed-by=/usr/share/keyrings/ubuntu-archive-keyring.gpg] http://archive.ubuntu.com/ubuntu jammy main",
			expectNil:    false,
			expectedURL:  "http://archive.ubuntu.com/ubuntu",
			expectedDist: "jammy",
			expectedComp: "main",
		},
		{
			name:      "insufficient fields",
			input:     "deb http://example.com",
			expectNil: true,
		},
		{
			name:      "local file URL",
			input:     "deb file:///mnt/repo jammy main",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.parseSourceLine(tt.input)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedURL, result.URL)
				assert.Equal(t, tt.expectedDist, result.Distribution)
				assert.Equal(t, tt.expectedComp, result.Components)
				assert.True(t, result.IsEnabled)
			}
		})
	}
}

func TestAPTManager_parseSourcesList(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewAPTManager(logger)

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "sources.list")

	content := `# Comment line
deb http://archive.ubuntu.com/ubuntu jammy main restricted
deb-src http://archive.ubuntu.com/ubuntu jammy main restricted

deb http://security.ubuntu.com/ubuntu jammy-security main
`
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0644))

	repos, err := manager.parseSourcesList(testFile)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(repos), 2)
}

func TestAPTManager_parseDEB822Sources_FieldCasing(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager := NewAPTManager(logger)

	tests := []struct {
		name            string
		content         string
		expectedURL     string
		expectedDist    string
		expectedComp    string
		expectedType    string
		expectedEnabled bool
	}{
		{
			name: "canonical casing",
			content: `Types: deb
URIs: http://archive.ubuntu.com/ubuntu
Suites: noble
Components: main restricted
`,
			expectedURL:     "http://archive.ubuntu.com/ubuntu",
			expectedDist:    "noble",
			expectedComp:    "main restricted",
			expectedType:    "deb",
			expectedEnabled: true,
		},
		{
			name: "lowercase field names",
			content: `types: deb
uris: http://archive.ubuntu.com/ubuntu
suites: noble
components: main
`,
			expectedURL:     "http://archive.ubuntu.com/ubuntu",
			expectedDist:    "noble",
			expectedComp:    "main",
			expectedType:    "deb",
			expectedEnabled: true,
		},
		{
			name: "mixed casing",
			content: `Types: deb-src
Uris: https://deb.debian.org/debian
SUITES: trixie
CoMpOnEnTs: main contrib
Enabled: Yes
`,
			expectedURL:     "https://deb.debian.org/debian",
			expectedDist:    "trixie",
			expectedComp:    "main contrib",
			expectedType:    "deb-src",
			expectedEnabled: true,
		},
		{
			name: "disabled with capitalised value",
			content: `Types: deb
Uris: http://archive.ubuntu.com/ubuntu
Suites: noble
Components: main
enabled: No
`,
			expectedURL:     "http://archive.ubuntu.com/ubuntu",
			expectedDist:    "noble",
			expectedComp:    "main",
			expectedType:    "deb",
			expectedEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(t.TempDir(), "test.sources")
			require.NoError(t, os.WriteFile(testFile, []byte(tt.content), 0644))

			repos, err := manager.parseDEB822Sources(testFile)
			require.NoError(t, err)
			require.Len(t, repos, 1)
			assert.Equal(t, tt.expectedURL, repos[0].URL)
			assert.Equal(t, tt.expectedDist, repos[0].Distribution)
			assert.Equal(t, tt.expectedComp, repos[0].Components)
			assert.Equal(t, tt.expectedType, repos[0].RepoType)
			assert.Equal(t, tt.expectedEnabled, repos[0].IsEnabled)
		})
	}
}

func TestIsValidSuiteComponents(t *testing.T) {
	tests := []struct {
		name       string
		suite      string
		components string
		expected   bool
	}{
		{
			name:       "distribution with components",
			suite:      "jammy",
			components: "main restricted",
			expected:   true,
		},
		{
			name:       "exact path without components",
			suite:      "jammy/",
			components: "",
			expected:   true,
		},
		{
			name:       "exact path with components - invalid",
			suite:      "jammy/",
			components: "main",
			expected:   false,
		},
		{
			name:       "distribution without components - invalid",
			suite:      "jammy",
			components: "",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSuiteComponents(tt.suite, tt.components)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidRepoURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"http URL", "http://example.com", true},
		{"https URL", "https://example.com", true},
		{"ftp URL", "ftp://example.com", true},
		{"mirror URL", "mirror://example.com", true},
		{"file URL", "file:///local/path", false},
		{"cdrom URL", "cdrom:[label]", false},
		{"copy URL", "copy:///path", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidRepoURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
