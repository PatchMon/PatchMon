//go:build darwin

package repositories

import (
	"os/exec"
	"strings"

	"patchmon-agent/pkg/models"
)

func newBrewCommand(args ...string) *exec.Cmd {
	return exec.Command("/usr/local/bin/patchmon-brew", args...)
}

// CollectRepositories returns configured Homebrew taps,
// which are the macOS equivalent of apt/yum repos.
func CollectRepositories() ([]models.Repository, error) {
	out, err := newBrewCommand("tap").Output()
	if err != nil {
		return nil, err
	}

	var repos []models.Repository
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		repos = append(repos, models.Repository{
			Name:      line,
			RepoType:  "homebrew-tap",
			IsEnabled: true,
		})
	}
	return repos, nil
}
