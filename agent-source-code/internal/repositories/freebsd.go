package repositories

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// FreeBSDManager handles FreeBSD repository information collection
type FreeBSDManager struct {
	logger *logrus.Logger
}

// NewFreeBSDManager creates a new FreeBSD repository manager
func NewFreeBSDManager(logger *logrus.Logger) *FreeBSDManager {
	return &FreeBSDManager{
		logger: logger,
	}
}

// getPkgPath returns the path to the pkg binary (works when PATH is minimal, e.g. under rc.d)
func (m *FreeBSDManager) getPkgPath() string {
	if path, err := exec.LookPath("pkg"); err == nil {
		return path
	}
	for _, p := range []string{"/usr/sbin/pkg", "/usr/local/sbin/pkg"} {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
			return p
		}
	}
	return "pkg"
}

// GetRepositories gets FreeBSD repository information
// Sources: pkg -vv output, /etc/pkg/*.conf, /usr/local/etc/pkg/repos/*.conf
func (m *FreeBSDManager) GetRepositories() ([]models.Repository, error) {
	var repositories []models.Repository

	// Primary method: parse pkg -vv output which shows resolved repositories
	pkgRepos, err := m.getPkgRepositories()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get pkg repositories from pkg -vv")
	} else {
		repositories = append(repositories, pkgRepos...)
	}

	// If pkg -vv failed, fallback to config file parsing
	if len(repositories) == 0 {
		configRepos, err := m.parseConfigFiles()
		if err != nil {
			m.logger.WithError(err).Warn("Failed to parse pkg config files")
		} else {
			repositories = append(repositories, configRepos...)
		}
	}

	m.logger.WithField("count", len(repositories)).Debug("Total FreeBSD repositories found")
	return repositories, nil
}

// getPkgRepositories parses pkg -vv output to get repository information
// This is the most reliable method as it shows resolved/active repositories
func (m *FreeBSDManager) getPkgRepositories() ([]models.Repository, error) {
	var repositories []models.Repository

	cmd := exec.Command(m.getPkgPath(), "-vv")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse the Repositories: section
	// Format:
	// Repositories:
	//   FreeBSD-ports: {
	//     url             : "pkg+https://pkg.FreeBSD.org/FreeBSD:15:amd64/quarterly",
	//     enabled         : yes,
	//     priority        : 0,
	//     ...
	//   }

	inRepoSection := false
	var currentRepo *models.Repository

	// Regex patterns
	repoNameRegex := regexp.MustCompile(`^\s+(\S+):\s*\{`)
	urlRegex := regexp.MustCompile(`url\s*:\s*"([^"]+)"`)
	enabledRegex := regexp.MustCompile(`enabled\s*:\s*(yes|no)`)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()

		// Detect start of Repositories section
		if strings.TrimSpace(line) == "Repositories:" {
			inRepoSection = true
			continue
		}

		if !inRepoSection {
			continue
		}

		// End of repositories section (next major section or end)
		if !strings.HasPrefix(line, " ") && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "}") {
			break
		}

		// New repository block
		if matches := repoNameRegex.FindStringSubmatch(line); len(matches) >= 2 {
			// Save previous repo if exists
			if currentRepo != nil {
				repositories = append(repositories, *currentRepo)
			}
			currentRepo = &models.Repository{
				Name:     matches[1],
				RepoType: constants.RepoTypeFreeBSD,
			}
			continue
		}

		if currentRepo == nil {
			continue
		}

		// Parse URL
		if matches := urlRegex.FindStringSubmatch(line); len(matches) >= 2 {
			url := matches[1]
			// Remove pkg+ prefix if present
			url = strings.TrimPrefix(url, "pkg+")
			currentRepo.URL = url
			currentRepo.IsSecure = strings.HasPrefix(url, "https://")

			// Extract distribution info from URL
			currentRepo.Distribution, currentRepo.Components = m.extractDistributionFromURL(url)
		}

		// Parse enabled status
		if matches := enabledRegex.FindStringSubmatch(line); len(matches) >= 2 {
			currentRepo.IsEnabled = (matches[1] == "yes")
		}

		// End of current repo block
		if strings.TrimSpace(line) == "}" {
			if currentRepo != nil {
				repositories = append(repositories, *currentRepo)
				currentRepo = nil
			}
		}
	}

	// Don't forget last repo
	if currentRepo != nil {
		repositories = append(repositories, *currentRepo)
	}

	return repositories, nil
}

// parseConfigFiles parses pkg configuration files directly
// Locations: /etc/pkg/*.conf, /usr/local/etc/pkg/repos/*.conf
func (m *FreeBSDManager) parseConfigFiles() ([]models.Repository, error) {
	var repositories []models.Repository

	configDirs := []string{
		"/etc/pkg",
		"/usr/local/etc/pkg/repos",
	}

	for _, dir := range configDirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.conf"))
		if err != nil {
			continue
		}

		for _, file := range matches {
			repos, err := m.parseConfigFile(file)
			if err != nil {
				m.logger.WithError(err).WithField("file", file).Debug("Failed to parse config file")
				continue
			}
			repositories = append(repositories, repos...)
		}
	}

	return repositories, nil
}

// parseConfigFile parses a single pkg configuration file
// Format (UCL-like):
//
//	RepoName: {
//	  url: "https://...",
//	  enabled: yes
//	}
func (m *FreeBSDManager) parseConfigFile(filename string) ([]models.Repository, error) {
	var repositories []models.Repository

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var currentRepo *models.Repository

	repoNameRegex := regexp.MustCompile(`^(\S+):\s*\{`)
	urlRegex := regexp.MustCompile(`url\s*:\s*"([^"]+)"`)
	enabledRegex := regexp.MustCompile(`enabled\s*:\s*(yes|no|true|false)`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// New repository block
		if matches := repoNameRegex.FindStringSubmatch(line); len(matches) >= 2 {
			if currentRepo != nil {
				repositories = append(repositories, *currentRepo)
			}
			currentRepo = &models.Repository{
				Name:      matches[1],
				RepoType:  constants.RepoTypeFreeBSD,
				IsEnabled: true, // Default to enabled
			}
			continue
		}

		if currentRepo == nil {
			continue
		}

		// Parse URL
		if matches := urlRegex.FindStringSubmatch(line); len(matches) >= 2 {
			url := strings.TrimPrefix(matches[1], "pkg+")
			currentRepo.URL = url
			currentRepo.IsSecure = strings.HasPrefix(url, "https://")
			currentRepo.Distribution, currentRepo.Components = m.extractDistributionFromURL(url)
		}

		// Parse enabled status
		if matches := enabledRegex.FindStringSubmatch(line); len(matches) >= 2 {
			val := strings.ToLower(matches[1])
			currentRepo.IsEnabled = (val == "yes" || val == "true")
		}

		// End of block
		if line == "}" {
			if currentRepo != nil {
				repositories = append(repositories, *currentRepo)
				currentRepo = nil
			}
		}
	}

	if currentRepo != nil {
		repositories = append(repositories, *currentRepo)
	}

	return repositories, nil
}

// extractDistributionFromURL extracts distribution and components from FreeBSD pkg URL
// Example: https://pkg.FreeBSD.org/FreeBSD:15:amd64/quarterly
// Distribution: "FreeBSD:15:amd64", Components: "quarterly"
func (m *FreeBSDManager) extractDistributionFromURL(url string) (distribution, components string) {
	// Remove scheme
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	parts := strings.Split(url, "/")

	// Look for FreeBSD:XX:arch pattern
	for i, part := range parts {
		if strings.HasPrefix(part, "FreeBSD:") {
			distribution = part
			if i+1 < len(parts) {
				components = parts[i+1]
			}
			return
		}
	}

	// Fallback: use last two path components
	if len(parts) >= 2 {
		distribution = parts[len(parts)-2]
		components = parts[len(parts)-1]
	}

	return
}
