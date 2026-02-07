package repositories

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// APTManager handles APT repository information collection
type APTManager struct {
	logger *logrus.Logger
}

// NewAPTManager creates a new APT repository manager
func NewAPTManager(logger *logrus.Logger) *APTManager {
	return &APTManager{
		logger: logger,
	}
}

// GetRepositories gets APT repository information
func (m *APTManager) GetRepositories() ([]models.Repository, error) {
	var repositories []models.Repository

	// Find repository files
	m.logger.Debug("Discovering package repositories...")
	aptSrcFiles, err := m.findAptListFiles()
	if err != nil {
		m.logger.WithError(err).Error("Failed to find apt list files")
		return repositories, err
	}
	m.logger.WithField("count", len(aptSrcFiles)).Debug("Found apt list files")

	sourcesFiles, err := m.findDeb822SourcesFiles()
	if err != nil {
		m.logger.WithError(err).Error("Failed to find deb822 sources files")
		return repositories, err
	}
	m.logger.WithField("count", len(sourcesFiles)).Debug("Found deb822 sources files")

	// Parse apt list files
	for _, file := range aptSrcFiles {
		m.logger.WithField("file", file).Debug("Parsing apt list file")
		repos, err := m.parseSourcesList(file)
		if err != nil {
			m.logger.WithError(err).WithField("file", file).Warn("Failed to parse sources list")
			continue
		}
		m.logger.WithFields(logrus.Fields{
			"file":  file,
			"count": len(repos),
		}).Debug("Extracted repositories from apt list file")
		repositories = append(repositories, repos...)
	}

	// Parse modern DEB822 format (.sources files)
	for _, file := range sourcesFiles {
		m.logger.WithField("file", file).Debug("Parsing deb822 sources file")
		repos, err := m.parseDEB822Sources(file)
		if err != nil {
			m.logger.WithError(err).WithField("file", file).Warn("Failed to parse deb822 sources")
			continue
		}
		m.logger.WithFields(logrus.Fields{
			"file":  file,
			"count": len(repos),
		}).Debug("Extracted repositories from deb822 file")
		repositories = append(repositories, repos...)
	}
	return repositories, nil
}

// findAptListFiles finds all apt list files in common locations
func (m *APTManager) findAptListFiles() ([]string, error) {
	var listFiles []string

	// Add main sources.list file
	listFiles = append(listFiles, "/etc/apt/sources.list")

	// Add .list files from sources.list.d
	sourcesDir := "/etc/apt/sources.list.d"
	if entries, err := os.ReadDir(sourcesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			if strings.HasSuffix(entry.Name(), ".list") {
				fullPath := filepath.Join(sourcesDir, entry.Name())
				listFiles = append(listFiles, fullPath)
			}
		}
	} else {
		m.logger.WithError(err).WithField("path", sourcesDir).Warn("Failed to read sources directory for .list files")
	}

	return listFiles, nil
}

// findDeb822SourcesFiles finds all Deb822 .sources files in common locations
func (m *APTManager) findDeb822SourcesFiles() ([]string, error) {
	var sourcesFiles []string

	// Add .sources files from sources.list.d
	sourcesDir := "/etc/apt/sources.list.d"
	if entries, err := os.ReadDir(sourcesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			if strings.HasSuffix(entry.Name(), ".sources") {
				fullPath := filepath.Join(sourcesDir, entry.Name())
				sourcesFiles = append(sourcesFiles, fullPath)
			}
		}
	} else {
		m.logger.WithError(err).WithField("path", sourcesDir).Warn("Failed to read sources directory for .sources files")
	}

	return sourcesFiles, nil
}

// parseSourcesList parses traditional APT sources.list files
func (m *APTManager) parseSourcesList(filename string) ([]models.Repository, error) {
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

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse repository line (deb or deb-src)
		if strings.HasPrefix(line, constants.RepoTypeDeb) {
			repo := m.parseSourceLine(line)
			if repo != nil {
				repositories = append(repositories, *repo)
			}
		}
	}

	return repositories, scanner.Err()
}

// parseSourceLine parses a single APT source line
func (m *APTManager) parseSourceLine(line string) *models.Repository {
	fields := slices.Collect(strings.FieldsSeq(line))
	if len(fields) < 4 {
		return nil
	}

	repoType := fields[0]
	var url, distribution, components string
	var fieldIndex int

	// Handle modern format with options like [signed-by=...]
	if len(fields) > 1 && strings.HasPrefix(fields[1], "[") {
		// Find the end of options
		optionsEnd := 1
		for i := 1; i < len(fields); i++ {
			if strings.HasSuffix(fields[i], "]") {
				optionsEnd = i
				break
			}
		}
		fieldIndex = optionsEnd + 1
	} else {
		fieldIndex = 1
	}

	if fieldIndex+2 >= len(fields) {
		m.logger.WithFields(logrus.Fields{
			"line":       line,
			"fieldCount": len(fields),
			"fieldIndex": fieldIndex,
		}).Debug("Skipping malformed entry: not enough fields")
		return nil
	}

	url = fields[fieldIndex]
	distribution = fields[fieldIndex+1]
	if fieldIndex+2 < len(fields) {
		components = strings.Join(fields[fieldIndex+2:], " ")
	}

	// Skip if URL doesn't look valid
	if !isValidRepoURL(url) {
		m.logger.WithField("url", url).Debug("Skipping unsupported source URL")
		return nil
	}

	// Skip if distribution is empty or looks malformed
	if distribution == "" || strings.Contains(distribution, "[") {
		m.logger.WithField("distribution", distribution).Debug("Skipping malformed distribution")
		return nil
	}

	// Validate suite/components relationship per sources.list(5)
	if !isValidSuiteComponents(distribution, components) {
		m.logger.WithFields(logrus.Fields{
			"suite":      distribution,
			"components": components,
		}).Debug("Skipping invalid suite/components combination")
		return nil
	}

	// Determine repository name
	repoName := generateRepoName(url, distribution, components)

	return &models.Repository{
		Name:         repoName,
		URL:          url,
		Distribution: distribution,
		Components:   components,
		RepoType:     repoType,
		IsEnabled:    true,
		IsSecure:     isSecureURL(url),
	}
}

// parseDEB822Sources parses modern DEB822 format sources files
func (m *APTManager) parseDEB822Sources(filename string) ([]models.Repository, error) {
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

	var currentEntry map[string]string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Empty line indicates end of entry
		if line == "" {
			if currentEntry != nil {
				repos := m.processDEB822Entry(currentEntry)
				repositories = append(repositories, repos...)
				currentEntry = nil
			}
			continue
		}

		// Parse key-value pairs
		if strings.Contains(line, ":") {
			if currentEntry == nil {
				currentEntry = make(map[string]string)
			}

			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				currentEntry[key] = value
			}
		}
	}

	// Process last entry if file doesn't end with empty line
	if currentEntry != nil {
		repos := m.processDEB822Entry(currentEntry)
		repositories = append(repositories, repos...)
	}

	return repositories, scanner.Err()
}

// processDEB822Entry processes a single DEB822 repository entry
func (m *APTManager) processDEB822Entry(entry map[string]string) []models.Repository {
	var repositories []models.Repository

	// Check if explicitly disabled (Enabled: no)
	// Per sources.list(5), Enabled defaults to yes if not specified
	enabled := entry["Enabled"]
	isEnabled := enabled != "no" && enabled != "false"

	types := entry["Types"]
	uris := entry["URIs"]
	suites := entry["Suites"]
	components := entry["Components"]
	name := entry["X-Repolib-Name"]

	if uris == "" || suites == "" {
		return repositories
	}

	// Split multiple values
	uriList := slices.Collect(strings.FieldsSeq(uris))
	suiteList := slices.Collect(strings.FieldsSeq(suites))
	typeList := slices.Collect(strings.FieldsSeq(types))

	// If no types specified, default to "deb"
	if len(typeList) == 0 {
		typeList = []string{constants.RepoTypeDeb}
	}

	for _, repoType := range typeList {
		// Skip if not a supported repo type
		if repoType != constants.RepoTypeDeb && repoType != constants.RepoTypeDebSrc {
			m.logger.WithField("type", repoType).Debug("Skipping unsupported repo type")
			continue
		}

		for _, uri := range uriList {
			// Skip invalid URIs
			if !isValidRepoURL(uri) {
				m.logger.WithField("uri", uri).Debug("Skipping unsupported source URL")
				continue
			}

			for _, suite := range suiteList {
				if suite == "" {
					m.logger.Debug("Skipping malformed entry: empty suite")
					continue
				}

				// Validate suite/components relationship per sources.list(5)
				if !isValidSuiteComponents(suite, components) {
					m.logger.WithFields(logrus.Fields{
						"suite":      suite,
						"components": components,
					}).Debug("Skipping invalid suite/components combination")
					continue
				}

				// Generate repository name
				repoName := name
				if repoName == "" {
					repoName = generateRepoName(uri, suite, components)
				} else {
					repoName = strings.ToLower(strings.ReplaceAll(repoName, " ", "-"))
				}

				repositories = append(repositories, models.Repository{
					Name:         repoName,
					URL:          uri,
					Distribution: suite,
					Components:   components,
					RepoType:     repoType,
					IsEnabled:    isEnabled,
					IsSecure:     isSecureURL(uri),
				})
			}
		}
	}

	return repositories
}

// isValidSuiteComponents validates the suite/components relationship per sources.list(5).
// If suite ends with / (exact path), components must be empty.
// If suite doesn't end with / (distribution), at least one component must be present.
func isValidSuiteComponents(suite, components string) bool {
	isExactPath := strings.HasSuffix(suite, "/")
	hasComponents := components != ""

	// Exact path must have no components
	if isExactPath && hasComponents {
		return false
	}

	// Distribution must have at least one component
	if !isExactPath && !hasComponents {
		return false
	}

	return true
}

// isValidRepoURL checks if a URL is a valid remote repository URL.
// Excludes local-only schemes like file, cdrom, copy.
func isValidRepoURL(url string) bool {
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
func isSecureURL(url string) bool {
	return strings.HasPrefix(url, "https://")
}
