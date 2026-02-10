package repositories

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// DNFManager handles dnf/yum repository information collection
type DNFManager struct {
	logger *logrus.Logger
}

// repoEntry represents a parsed repository entry before processing
type repoEntry struct {
	id         string
	name       string
	baseurls   []string
	mirrorlist string
	metalink   string
	enabled    *bool // Pointer to distinguish between unset and false
}

// NewDNFManager creates a new DNF repository manager
func NewDNFManager(logger *logrus.Logger) *DNFManager {
	return &DNFManager{
		logger: logger,
	}
}

// GetRepositories gets dnf/yum repository information
func (d *DNFManager) GetRepositories() []models.Repository {
	var repositories []models.Repository

	d.logger.Debug("Searching for RPM repository files...")
	repoFiles, err := d.findRepoFiles()
	if err != nil {
		d.logger.WithError(err).Error("Failed to find repository files")
		return repositories
	}
	d.logger.WithField("count", len(repoFiles)).Debug("Found repo files")

	for _, file := range repoFiles {
		d.logger.WithField("file", file).Debug("Parsing repository file")
		repos, err := d.parseRepoFile(file)
		if err != nil {
			d.logger.WithError(err).WithField("file", file).Error("Failed to parse repository file")
			continue
		}
		d.logger.WithFields(logrus.Fields{
			"file":  file,
			"count": len(repos),
		}).Debug("Extracted repositories from file")
		repositories = append(repositories, repos...)
	}

	d.logger.WithField("total", len(repositories)).Debug("Total repositories collected")
	return repositories
}

// findRepoFiles finds all .repo files in common locations
func (d *DNFManager) findRepoFiles() ([]string, error) {
	var repoFiles []string
	searchPaths := []string{
		"/etc/yum.repos.d",
		"/etc/dnf/repos.d",
	}

	for _, path := range searchPaths {
		d.logger.WithField("path", path).Debug("Searching for repo files")
		files, err := filepath.Glob(filepath.Join(path, "*.repo"))
		if err != nil {
			d.logger.WithError(err).WithField("path", path).Warn("Failed to search for repo files")
			continue
		}
		d.logger.WithFields(logrus.Fields{
			"path":  path,
			"count": len(files),
		}).Debug("Found repo files in path")
		repoFiles = append(repoFiles, files...)
	}

	return repoFiles, nil
}

// parseRepoFile parses a .repo file and extracts repository information
func (d *DNFManager) parseRepoFile(filename string) ([]models.Repository, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			d.logger.WithError(err).WithField("file", filename).Debug("Failed to close file")
		}
	}()

	var repositories []models.Repository
	var currentRepo *repoEntry
	var inSection bool

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section header [repo-name]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// Save previous repository if it was valid
			if inSection && currentRepo != nil {
				repos := d.processRepoEntry(currentRepo)
				repositories = append(repositories, repos...)
			}

			// Start new repository
			currentRepo = &repoEntry{
				id:       strings.Trim(line, "[]"),
				enabled:  nil, // Will default to true if not specified
				baseurls: []string{},
			}
			inSection = true
			continue
		}

		if !inSection || currentRepo == nil {
			continue
		}

		// Parse key=value pairs
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				d.logger.WithField("line", line).Debug("Skipping malformed line")
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "name":
				currentRepo.name = value
			case "baseurl":
				// baseurl is a list - can have multiple URLs separated by whitespace
				urls := strings.Fields(value)
				currentRepo.baseurls = append(currentRepo.baseurls, urls...)
			case "mirrorlist":
				currentRepo.mirrorlist = value
			case "metalink":
				currentRepo.metalink = value
			case "enabled":
				enabled := (value == "1" || strings.ToLower(value) == "true")
				currentRepo.enabled = &enabled
			}
		}
	}

	// Don't forget the last repository
	if inSection && currentRepo != nil {
		repos := d.processRepoEntry(currentRepo)
		repositories = append(repositories, repos...)
	}

	return repositories, scanner.Err()
}

// processRepoEntry processes a repository entry and creates Repository models
// Per dnf.conf(5), priority is: baseurl first (in order), then metalink, then mirrorlist
func (d *DNFManager) processRepoEntry(entry *repoEntry) []models.Repository {
	var repositories []models.Repository

	// Check if repository is enabled (defaults to true per dnf.conf(5))
	isEnabled := true
	if entry.enabled != nil {
		isEnabled = *entry.enabled
	}

	// Skip disabled repositories
	if !isEnabled {
		d.logger.WithField("repoId", entry.id).Debug("Skipping disabled repository")
		return repositories
	}

	// Collect all URLs in priority order
	var urls []string

	// 1. baseurl entries (highest priority, in listed order)
	for _, url := range entry.baseurls {
		if d.isValidRepoURL(url) {
			urls = append(urls, url)
		}
	}

	// 2. metalink
	if entry.metalink != "" && d.isValidRepoURL(entry.metalink) {
		urls = append(urls, entry.metalink)
	}

	// 3. mirrorlist (lowest priority)
	if entry.mirrorlist != "" && d.isValidRepoURL(entry.mirrorlist) {
		urls = append(urls, entry.mirrorlist)
	}

	// Create a repository entry for each valid URL
	for _, url := range urls {
		repositories = append(repositories, models.Repository{
			Name:         entry.id,
			URL:          url,
			Distribution: entry.name,
			RepoType:     constants.RepoTypeRPM,
			IsEnabled:    isEnabled,
			IsSecure:     d.isSecureURL(url),
		})
	}

	if len(repositories) == 0 {
		d.logger.WithField("repoId", entry.id).Debug("No valid remote URLs found for repository")
	}

	return repositories
}

// isValidRepoURL checks if a URL is a valid remote repository URL
// Excludes local-only schemes like file://.
func (d *DNFManager) isValidRepoURL(url string) bool {
	supportedPrefixes := []string{
		"http://", "https://", "ftp://",
		"mirror://", "mirror+",
	}

	for _, prefix := range supportedPrefixes {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}

// isSecureURL checks if a URL uses HTTPS
func (d *DNFManager) isSecureURL(url string) bool {
	return strings.HasPrefix(url, "https://")
}
