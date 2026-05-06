//go:build darwin

package repositories

import (
	"os"
	"os/exec"
	"strings"

	"patchmon-agent/pkg/models"
)

func newBrewCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("brew", args...)
	cmd.Env = append(os.Environ(),
		"HOMEBREW_ALLOW_RUN_AS_ROOT=1",
		"HOMEBREW_NO_AUTO_UPDATE=1",
	)
	return cmd
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
