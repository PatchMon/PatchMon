// Package repositories provides repository management functionality for APK package manager
package repositories

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"patchmon-agent/internal/constants"
	"patchmon-agent/internal/logutil"
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

// APKRepoLine is one repository URL declared in the apk repositories files.
type APKRepoLine struct {
	Tag string
	URL string
}

var (
	apkRepositoriesFile = "/etc/apk/repositories"
	apkRepositoriesDirs = []string{"/etc/apk/repositories.d", "/lib/apk/repositories.d"}
)

// GetRepositories gets APK repository information
func (m *APKManager) GetRepositories() ([]models.Repository, error) {
	lines := ReadAPKRepositoryLines()
	repositories := make([]models.Repository, 0, len(lines))
	for _, line := range lines {
		if repo := apkRepository(line); repo != nil {
			repositories = append(repositories, *repo)
		} else {
			m.logger.WithField("url", logutil.Sanitize(line.URL)).Debug("Skipping unsupported repository URL")
		}
	}
	m.logger.WithField("count", len(repositories)).Debug("Extracted APK repositories")
	return repositories, nil
}

// ReadAPKRepositoryLines reads /etc/apk/repositories and then the repositories.d lists,
// where a .list name seen in /etc shadows the same name under /lib.
func ReadAPKRepositoryLines() []APKRepoLine {
	vars := map[string]string{}
	var out []APKRepoLine
	if data, err := os.ReadFile(apkRepositoriesFile); err == nil {
		out = append(out, parseAPKRepositories(string(data), vars)...)
	}
	seen := map[string]string{}
	for _, dir := range apkRepositoriesDirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.list"))
		for _, path := range matches {
			if _, ok := seen[filepath.Base(path)]; !ok {
				seen[filepath.Base(path)] = path
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if data, err := os.ReadFile(seen[name]); err == nil {
			out = append(out, parseAPKRepositories(string(data), vars)...)
		}
	}
	return out
}

var apkRepoVarRe = regexp.MustCompile(`\$\{([A-Za-z][A-Za-z0-9_]*)\}`)

const (
	apkRepoMaxExpanded = 4 << 10
	apkRepoMaxVars     = 64
)

// parseAPKRepositories accepts the v2 form "[@tag] url" and the v3 form
// "[v2|v3|ndx] [@tag] url [component...]", where components expand to url/component.
func parseAPKRepositories(content string, vars map[string]string) []APKRepoLine {
	expand := func(s string) string {
		return apkRepoVarRe.ReplaceAllStringFunc(s, func(m string) string {
			if v, ok := vars[m[2:len(m)-1]]; ok {
				return v
			}
			return m
		})
	}
	var out []APKRepoLine
	for line := range strings.Lines(content) {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if fields[0] == "set" {
			args := fields[1:]
			keepExisting := len(args) > 0 && args[0] == "-default"
			if keepExisting {
				args = args[1:]
			}
			if len(args) == 1 {
				if key, value, ok := strings.Cut(args[0], "="); ok {
					_, exists := vars[key]
					value = expand(value)
					if (!keepExisting || !exists) && len(value) <= apkRepoMaxExpanded && (exists || len(vars) < apkRepoMaxVars) {
						vars[key] = value
					}
				}
			}
			continue
		}
		switch fields[0] {
		case "v2", "v3", "ndx":
			fields = fields[1:]
		}
		tag := ""
		if len(fields) > 0 && strings.HasPrefix(fields[0], "@") {
			tag, fields = fields[0], fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		base := strings.TrimRight(expand(fields[0]), "/")
		if len(base) > apkRepoMaxExpanded {
			continue
		}
		if len(fields) == 1 {
			out = append(out, APKRepoLine{Tag: tag, URL: base})
			continue
		}
		for _, comp := range fields[1:] {
			if url := base + "/" + expand(comp); len(url) <= apkRepoMaxExpanded {
				out = append(out, APKRepoLine{Tag: tag, URL: url})
			}
		}
	}
	return out
}

var apkBranchSegmentRe = regexp.MustCompile(`^(v\d+\.\d+|edge|latest-stable)$`)

// APKBranchAndComponent finds the release branch (v3.21, edge) and repository
// (main, community) in a repository URL, wherever the mirror puts them.
func APKBranchAndComponent(rawURL string) (branch, component string) {
	segs := apkURLSegments(rawURL)
	for i, s := range segs {
		if apkBranchSegmentRe.MatchString(s) {
			if i+1 < len(segs) {
				return s, segs[i+1]
			}
			return s, ""
		}
	}
	return "", ""
}

func apkURLSegments(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	var segs []string
	for _, s := range strings.Split(u.Path, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if n := len(segs); n > 0 && (strings.HasSuffix(segs[n-1], ".tar.gz") || strings.HasSuffix(segs[n-1], ".adb")) {
		segs = segs[:n-1]
	}
	return segs
}

func apkRepository(line APKRepoLine) *models.Repository {
	if !strings.HasPrefix(line.URL, "http://") && !strings.HasPrefix(line.URL, "https://") && !strings.HasPrefix(line.URL, "ftp://") {
		return nil
	}
	distribution, components := APKBranchAndComponent(line.URL)
	if distribution == "" {
		segs := apkURLSegments(line.URL)
		switch {
		case len(segs) >= 2:
			distribution, components = segs[len(segs)-2], segs[len(segs)-1]
		case len(segs) == 1:
			distribution = segs[0]
		default:
			if u, err := url.Parse(line.URL); err == nil {
				distribution = u.Hostname()
			}
		}
	}
	if distribution == "" {
		return nil
	}
	name := distribution
	if components != "" {
		name += "-" + components
	}
	if line.Tag != "" {
		name = strings.TrimPrefix(line.Tag, "@") + "-" + name
	}
	return &models.Repository{
		Name:         strings.ToLower(name),
		URL:          line.URL,
		Distribution: distribution,
		Components:   components,
		RepoType:     constants.RepoTypeAPK,
		IsEnabled:    true,
		IsSecure:     strings.HasPrefix(line.URL, "https://"),
	}
}
