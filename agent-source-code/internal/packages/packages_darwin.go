//go:build darwin

package packages

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"patchmon-agent/pkg/models"
)

func newBrewCommand(args ...string) *exec.Cmd {
	brewPath := findBrewBinary()
	if brewPath == "" {
		brewPath = "brew"
	}
	cmd := exec.Command(brewPath, args...)
	cmd.Env = append(os.Environ(),
		"HOMEBREW_ALLOW_RUN_AS_ROOT=1",
		"HOMEBREW_NO_AUTO_UPDATE=1",
	)
	return cmd
}

// BrewOutdatedEntry matches the JSON output of `brew outdated --json=v2`
type BrewOutdatedEntry struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
	Pinned            bool     `json:"pinned"`
}

type BrewOutdatedResponse struct {
	Formulae []BrewOutdatedEntry `json:"formulae"`
	Casks    []BrewOutdatedEntry `json:"casks"`
}

// CollectPackages returns all installed Homebrew packages,
// flagging any that have updates available.
func CollectPackages() ([]models.Package, error) {
	return collectDarwinPackages(false)
}

// CollectDarwinPackages returns available packages for macOS hosts.
// It includes Homebrew inventory when brew is installed and always adds
// any pending system updates from softwareupdate.
func CollectDarwinPackages() ([]models.Package, error) {
	return collectDarwinPackages(false)
}

func collectDarwinPackages(requireBrew bool) ([]models.Package, error) {
	useBrew := true
	if _, err := exec.LookPath("brew"); err != nil {
		useBrew = false
		if requireBrew {
			return nil, fmt.Errorf("brew not found: %w", err)
		}
	}

	packages := make([]models.Package, 0)
	if useBrew {
		installed, err := getInstalledPackages()
		if err != nil {
			if requireBrew {
				return nil, fmt.Errorf("brew list failed: %w", err)
			}
			installed = map[string]string{}
		}

		outdated, err := getOutdatedPackages()
		if err != nil {
			outdated = map[string]string{}
		}

		for name, version := range installed {
			pkg := models.Package{
				Name:             name,
				CurrentVersion:   version,
				SourceRepository: "homebrew",
				NeedsUpdate:      false,
			}
			if newVersion, ok := outdated[name]; ok {
				pkg.AvailableVersion = newVersion
				pkg.NeedsUpdate = true
			}
			packages = append(packages, pkg)
		}
	}

	systemUpdates, err := getAvailableSystemUpdates()
	if err == nil {
		packages = append(packages, systemUpdates...)
	}

	return packages, nil
}

func getInstalledPackages() (map[string]string, error) {
	// `brew list --versions` outputs: packagename version [version...]
	cmd := newBrewCommand("list", "--versions")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("brew list --versions failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			result[parts[0]] = parts[len(parts)-1]
		}
	}
	return result, nil
}

func getOutdatedPackages() (map[string]string, error) {
	cmd := newBrewCommand("outdated", "--json=v2")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var response BrewOutdatedResponse
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, entry := range response.Formulae {
		result[entry.Name] = entry.CurrentVersion
	}
	for _, entry := range response.Casks {
		result[entry.Name] = entry.CurrentVersion
	}
	return result, nil
}

func getAvailableSystemUpdates() ([]models.Package, error) {
	if _, err := exec.LookPath("softwareupdate"); err != nil {
		return nil, nil
	}

	out, err := exec.Command("softwareupdate", "--list", "--all").CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil, err
	}

	labels := parseSoftwareUpdateList(string(out))
	if len(labels) == 0 {
		if strings.Contains(string(out), "No new software available.") || strings.Contains(string(out), "No updates are available.") {
			return nil, nil
		}
		return nil, nil
	}

	packages := make([]models.Package, 0, len(labels))
	for _, label := range labels {
		packages = append(packages, models.Package{
			Name:             label,
			Description:      "macOS system update",
			Category:         "macOS Update",
			CurrentVersion:   "installed",
			AvailableVersion: "available",
			NeedsUpdate:      true,
			SourceRepository: "softwareupdate",
		})
	}
	return packages, nil
}

func parseSoftwareUpdateList(output string) []string {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}

	labelPattern := regexp.MustCompile(`(?m)^\s*\*\s*(.+)$`)
	seen := make(map[string]struct{})
	result := make([]string, 0)

	for _, match := range labelPattern.FindAllStringSubmatch(output, -1) {
		label := strings.TrimSpace(match[1])
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		result = append(result, label)
	}

	if len(result) > 0 {
		return result
	}

	labelPattern = regexp.MustCompile(`(?m)^\s*Label:\s*(.+)$`)
	for _, match := range labelPattern.FindAllStringSubmatch(output, -1) {
		label := strings.TrimSpace(match[1])
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		result = append(result, label)
	}

	return result
}
