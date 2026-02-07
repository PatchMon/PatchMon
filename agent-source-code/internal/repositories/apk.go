package repositories

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// APKManager handles APK repository information collection
type APKManager struct {
	logger *logrus.Logger
}

// NewAPKManager creates a new APK repository manager
func NewAPKManager(logger *logrus.Logger) *APKManager {
	return &APKManager{
		logger: logger,
	}
}

// GetRepositories gets APK repository information
func (m *APKManager) GetRepositories() ([]models.Repository, error) {
	var repositories []models.Repository

	m.logger.Debug("Discovering APK repositories...")
	repoFile, err := m.findRepoFile()
	if err != nil {
		m.logger.WithError(err).Error("Failed to find APK repositories file")
		return repositories, err
	}

	if repoFile == "" {
		m.logger.Debug("No APK repositories file found")
		return repositories, nil
	}

	m.logger.WithField("file", repoFile).Debug("Parsing APK repositories file")
	repos, err := m.parseRepoFile(repoFile)
	if err != nil {
		m.logger.WithError(err).WithField("file", repoFile).Warn("Failed to parse repositories file")
		return repositories, err
	}

	m.logger.WithFields(logrus.Fields{
		"file":  repoFile,
		"count": len(repos),
	}).Debug("Extracted repositories from APK repositories file")
	repositories = append(repositories, repos...)

	return repositories, nil
}

// findRepoFile locates the APK repositories file
func (m *APKManager) findRepoFile() (string, error) {
	repoFile := "/etc/apk/repositories"

	// Check if file exists
	if _, err := os.Stat(repoFile); err != nil {
		if os.IsNotExist(err) {
			m.logger.WithField("file", repoFile).Debug("APK repositories file does not exist")
			return "", nil
		}
		return "", err
	}

	return repoFile, nil
}

// parseRepoFile parses the APK repositories file
// Format: http://dl-cdn.alpinelinux.org/alpine/v3.19/main
// Or with tags: @edge http://dl-cdn.alpinelinux.org/alpine/edge/main
func (m *APKManager) parseRepoFile(filename string) ([]models.Repository, error) {
	var repositories []models.Repository

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			m.logger.WithError(err).WithField("file", filename).Debug("Failed to close file")
		}
	}()

	// Regex to match repository URL pattern
	// Matches: http://... or https://... followed by path
	urlRegex := regexp.MustCompile(`^(@\S+\s+)?(https?://[^\s]+)`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse repository line
		repo := m.parseRepoLine(line, urlRegex)
		if repo != nil {
			repositories = append(repositories, *repo)
		}
	}

	return repositories, scanner.Err()
}

// parseRepoLine parses a single APK repository line
// Format: http://dl-cdn.alpinelinux.org/alpine/v3.19/main
// Or: @edge http://dl-cdn.alpinelinux.org/alpine/edge/main
func (m *APKManager) parseRepoLine(line string, urlRegex *regexp.Regexp) *models.Repository {
	// Match the URL pattern
	matches := urlRegex.FindStringSubmatch(line)
	if len(matches) < 3 {
		m.logger.WithField("line", line).Debug("Skipping malformed repository line")
		return nil
	}

	tag := strings.TrimSpace(matches[1]) // Tag like @edge (may be empty)
	url := strings.TrimSpace(matches[2]) // Full URL

	// Skip if URL doesn't look valid
	if !m.isValidRepoURL(url) {
		m.logger.WithField("url", url).Debug("Skipping unsupported repository URL")
		return nil
	}

	// Extract distribution and components from URL
	// Format: http://.../alpine/v3.19/main
	// Or: http://.../alpine/edge/main
	distribution, components := m.extractDistributionAndComponents(url)

	if distribution == "" {
		m.logger.WithField("url", url).Debug("Failed to extract distribution from URL")
		return nil
	}

	// Generate repository name
	repoName := m.generateRepoName(url, distribution, components, tag)

	return &models.Repository{
		Name:         repoName,
		URL:          url,
		Distribution: distribution,
		Components:   components,
		RepoType:     constants.RepoTypeAPK,
		IsEnabled:    true,
		IsSecure:     m.isSecureURL(url),
	}
}

// extractDistributionAndComponents extracts distribution and components from URL
// Example: http://dl-cdn.alpinelinux.org/alpine/v3.19/main -> distribution: "v3.19", components: "main"
// Example: http://dl-cdn.alpinelinux.org/alpine/edge/main -> distribution: "edge", components: "main"
func (m *APKManager) extractDistributionAndComponents(url string) (distribution, components string) {
	// Split URL by "/"
	parts := strings.Split(url, "/")

	// Find "alpine" in the path
	alpineIndex := -1
	for i, part := range parts {
		if part == "alpine" && i+1 < len(parts) {
			alpineIndex = i
			break
		}
	}

	if alpineIndex == -1 || alpineIndex+1 >= len(parts) {
		return "", ""
	}

	// Distribution is the part after "alpine"
	distribution = parts[alpineIndex+1]

	// Components is the part after distribution (if present)
	if alpineIndex+2 < len(parts) {
		components = parts[alpineIndex+2]
		// There might be more components, join them
		if alpineIndex+3 < len(parts) {
			components = strings.Join(parts[alpineIndex+2:], " ")
		}
	}

	return distribution, components
}

// generateRepoName generates a repository name from URL, distribution, components, and tag
func (m *APKManager) generateRepoName(url, distribution, components, tag string) string {
	// If tag is present, use it in the name
	if tag != "" {
		tagName := strings.TrimPrefix(tag, "@")
		if components != "" {
			return strings.ToLower(tagName + "-" + distribution + "-" + components)
		}
		return strings.ToLower(tagName + "-" + distribution)
	}

	// Otherwise, use distribution and components
	if components != "" {
		return strings.ToLower(distribution + "-" + components)
	}
	return strings.ToLower(distribution)
}

// isValidRepoURL checks if a URL is a valid remote repository URL
// Excludes local-only schemes like file://
func (m *APKManager) isValidRepoURL(url string) bool {
	supportedPrefixes := []string{
		"http://",
		"https://",
		"ftp://",
	}

	for _, prefix := range supportedPrefixes {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}

// isSecureURL checks if a URL uses HTTPS
func (m *APKManager) isSecureURL(url string) bool {
	return strings.HasPrefix(url, "https://")
}
