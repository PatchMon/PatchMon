package system

import (
	"testing"
)

func TestCompareKernelVersions(t *testing.T) {
	// Définition des cas de test
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{
			name:     "Same version with extra",
			v1:       "6.14.11-2-pve",
			v2:       "6.14.11-2-pve",
			expected: 0,
		},
		{
			name:     "v1 newest (major version)",
			v1:       "6.14.11-2-pve",
			v2:       "5.19.0-1-generic",
			expected: 1,
		},
		{
			name:     "v1 oldest (minor version)",
			v1:       "6.8.12-9-pve",
			v2:       "6.14.11-2-pve",
			expected: -1,
		},
		{
			name:     "v1 newest (patch)",
			v1:       "6.8.15-1-pve",
			v2:       "6.8.12-9-pve",
			expected: 1,
		},
		{
			name:     "v1 oldest (build)",
			v1:       "6.8.12-2-pve",
			v2:       "6.8.12-9-pve",
			expected: -1,
		},
		{
			name:     "Same versions in Debian format",
			v1:       "6.12.90+deb13.1-amd64",
			v2:       "6.12.90+deb13.1-amd64",
			expected: 0,
		},
		{
			name:     "v1 oldest, versions in Debian format with Debian patch",
			v1:       "6.12.90+deb13-amd64",
			v2:       "6.12.90+deb13.1-amd64",
			expected: -1,
		},
		{
			name:     "v1 newest versions in Debian format",
			v1:       "6.12.90+deb13.2-amd64",
			v2:       "6.12.90+deb13.1-amd64",
			expected: 1,
		},
	}

	// Exécution des cas de test
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareKernelVersions(tt.v1, tt.v2)
			if got != tt.expected {
				t.Errorf("compareKernelVersions(%q, %q) = %d; got %d", tt.v1, tt.v2, tt.expected, got)
			}
		})
	}
}
