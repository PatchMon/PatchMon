package repositories

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"
)

// PacmanManager handles repository collection for pacman-based systems
type PacmanManager struct {
	logger *logrus.Logger
}

// NewPacmanManager creates a new PacmanManager
func NewPacmanManager(logger *logrus.Logger) *PacmanManager {
	return &PacmanManager{logger: logger}
}

// GetRepositories parses /etc/pacman.conf and included files for repositories
func (p *PacmanManager) GetRepositories() ([]models.Repository, error) {
	return p.parsePacmanConf("/etc/pacman.conf")
}

// parsePacmanConf parses the main pacman configuration file and collects repositories
func (p *PacmanManager) parsePacmanConf(filename string) ([]models.Repository, error) {
	// pacman.conf allows bare boolean keys like "Color"; enable AllowBooleanKeys.
	// Also make key names case-insensitive to be resilient.
	cfg, err := ini.LoadSources(ini.LoadOptions{
		AllowBooleanKeys: true,
		Insensitive:      true,
	}, filename)
	if err != nil {
		p.logger.WithError(err).WithField("file", filename).Warn("Failed to load pacman.conf as INI")
		return []models.Repository{}, nil
	}

	var repos []models.Repository

	// Iterate through sections; any section other than [options] is a repository
	for _, section := range cfg.Sections() {
		name := section.Name()
		// Default section is often named DEFAULT by ini lib; skip it
		if name == ini.DefaultSection || strings.EqualFold(name, "options") {
			continue
		}

		repoName := strings.ToLower(strings.TrimSpace(name))

		// Collect Server entries (there might be 0 or 1 typically in pacman.conf)
		if key, err := section.GetKey("Server"); err == nil {
			// ini can keep shadow values if repeated; include all if present
			values := key.ValueWithShadows()
			if len(values) == 0 {
				values = []string{key.String()}
			}
			for _, v := range values {
				url := strings.TrimSpace(v)
				if !p.isValidRepoURL(url) {
					continue
				}
				repos = append(repos, p.buildRepoEntry(repoName, url))
			}
		}

		// Follow Include entries, which may contain globs to mirrorlist files
		if key, err := section.GetKey("Include"); err == nil {
			includes := key.ValueWithShadows()
			if len(includes) == 0 {
				includes = []string{key.String()}
			}
			for _, incPattern := range includes {
				for _, inc := range p.expandIncludeGlobs(strings.TrimSpace(incPattern)) {
					incRepos := p.parseMirrorList(inc, repoName)
					repos = append(repos, incRepos...)
				}
			}
		}
	}

	return repos, nil
}

// parseMirrorList parses an included mirrorlist file and extracts Server URLs
func (p *PacmanManager) parseMirrorList(filename string, repoName string) []models.Repository {
	file, err := os.Open(filename)
	if err != nil {
		p.logger.WithError(err).WithField("file", filename).Debug("Failed to open include file")
		return nil
	}
	defer file.Close()

	var repos []models.Repository
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		// Expect lines like: Server = https://mirror/archlinux/$repo/os/$arch
		// Do a simple case-insensitive prefix check and split on '='
		// Allow extra spaces around '='
		// Identify key
		// Find '=' position
		eq := strings.IndexRune(trimmed, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		val := strings.TrimSpace(trimmed[eq+1:])
		if strings.EqualFold(key, "Server") {
			url := val
			if !p.isValidRepoURL(url) {
				continue
			}
			repos = append(repos, p.buildRepoEntry(repoName, url))
		}
	}

	if err := scanner.Err(); err != nil {
		p.logger.WithError(err).WithField("file", filename).Debug("Error reading include file")
	}

	return repos
}

// buildRepoEntry builds a Repository object for pacman server URL
func (p *PacmanManager) buildRepoEntry(section string, url string) models.Repository {
	// In pacman, the section name is the repository name
	name := section

	// Distribution/Components are not applicable in pacman; set distribution to section for context
	distribution := section
	components := ""

	return models.Repository{
		Name:         name,
		URL:          url,
		Distribution: distribution,
		Components:   components,
		RepoType:     constants.RepoTypePacman,
		IsEnabled:    true, // if present and not commented, it's enabled
		IsSecure:     p.isSecureURL(url),
	}
}

// expandIncludeGlobs handles Include directives that may contain globs
func (p *PacmanManager) expandIncludeGlobs(pattern string) []string {
	// Pacman allows simple file paths; if globbing fails, return the pattern itself if file exists
	matches, err := filepath.Glob(pattern)
	if err == nil && len(matches) > 0 {
		return matches
	}
	if _, err := os.Stat(pattern); err == nil {
		return []string{pattern}
	}
	return nil
}

// isValidRepoURL does a basic sanity check for URLs we can report
func (p *PacmanManager) isValidRepoURL(url string) bool {
	u := strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(u, "http://") ||
		strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "ftp://")
}

// isSecureURL checks if URL uses HTTPS
func (p *PacmanManager) isSecureURL(url string) bool {
	return strings.HasPrefix(strings.ToLower(url), "https://")
}
