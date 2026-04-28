//go:build darwin

package repositories

import (
    "os/exec"
    "strings"

    "patchmon-agent/pkg/models"
)

// CollectRepositories returns configured Homebrew taps,
// which are the macOS equivalent of apt/yum repos.
func CollectRepositories() ([]models.Repository, error) {
    out, err := exec.Command("brew", "tap").Output()
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