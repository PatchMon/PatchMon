package packages

import (
	"testing"

	"patchmon-agent/pkg/models"

	"github.com/stretchr/testify/assert"
)

func TestCombinePackageData(t *testing.T) {
	tests := []struct {
		name               string
		installedPackages  map[string]models.Package
		upgradablePackages []models.Package
		expectedCount      int
		expectedUpgradable int
	}{
		{
			name: "merge installed and upgradable",
			installedPackages: map[string]models.Package{
				"vim": {
					Name:           "vim",
					CurrentVersion: "2:8.2.3995-1ubuntu2.16",
				},
				"bash": {
					Name:           "bash",
					CurrentVersion: "5.1-6ubuntu1",
				},
				"curl": {
					Name:           "curl",
					CurrentVersion: "7.81.0-1ubuntu1.15",
				},
			},
			upgradablePackages: []models.Package{
				{
					Name:             "vim",
					CurrentVersion:   "2:8.2.3995-1ubuntu2.16",
					AvailableVersion: "2:8.2.3995-1ubuntu2.17",
					NeedsUpdate:      true,
				},
			},
			expectedCount:      3,
			expectedUpgradable: 1,
		},
		{
			name:               "empty inputs",
			installedPackages:  map[string]models.Package{},
			upgradablePackages: []models.Package{},
			expectedCount:      0,
			expectedUpgradable: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CombinePackageData(tt.installedPackages, tt.upgradablePackages)
			assert.Equal(t, tt.expectedCount, len(result))

			upgradable := 0
			for _, pkg := range result {
				if pkg.NeedsUpdate {
					upgradable++
				}
			}
			assert.Equal(t, tt.expectedUpgradable, upgradable)
		})
	}
}
