// Package packages provides package management functionality for APK package manager
package packages

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// APKManager handles APK package information collection
type APKManager struct {
	logger *logrus.Logger
}

// NewAPKManager creates a new APK package manager
func NewAPKManager(logger *logrus.Logger) *APKManager {
	return &APKManager{
		logger: logger,
	}
}

// GetPackages gets package information for APK-based systems
func (m *APKManager) GetPackages() []models.Package {
	// Update package index
	m.logger.Debug("Updating package index...")
	updateCmd := exec.Command("apk", "update", "-q")
	if err := updateCmd.Run(); err != nil {
		m.logger.WithError(err).Warn("Failed to update package index")
	}

	// Get installed packages
	m.logger.Debug("Getting installed packages...")
	installedCmd := exec.Command("apk", "list", "--installed")
	installedOutput, err := installedCmd.Output()
	var installedPackages map[string]models.Package
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get installed packages")
		installedPackages = make(map[string]models.Package)
	} else {
		m.logger.Debug("Parsing installed packages...")
		installedPackages = m.parseInstalledPackages(string(installedOutput))
		m.logger.WithField("count", len(installedPackages)).Debug("Found installed packages")
	}

	// Get upgradable packages (must run after apk update)
	m.logger.Debug("Getting upgradable packages...")
	upgradableCmd := exec.Command("apk", "-u", "list")
	upgradableOutput, err := upgradableCmd.Output()
	var upgradablePackages []models.Package
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get upgradable packages")
		upgradablePackages = []models.Package{}
	} else {
		m.logger.Debug("Parsing apk upgradable packages output...")
		upgradablePackages = m.parseUpgradablePackages(string(upgradableOutput), installedPackages)
		m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")
	}

	// Merge and deduplicate packages (pass full installed packages to preserve descriptions)
	packages := CombinePackageData(installedPackages, upgradablePackages)

	// Enrich packages with repository attribution
	m.enrichWithRepoAttribution(packages)

	m.logger.WithField("total", len(packages)).Debug("Total packages collected")

	return packages
}

// enrichWithRepoAttribution populates SourceRepository for each package by running
// apk policy in batches and extracting the logical repo name from the URL.
func (m *APKManager) enrichWithRepoAttribution(packages []models.Package) {
	if len(packages) == 0 {
		return
	}

	names := make([]string, len(packages))
	for i := range packages {
		names[i] = packages[i].Name
	}

	pkgIdx := make(map[string][]int, len(packages))
	for i, p := range packages {
		pkgIdx[p.Name] = append(pkgIdx[p.Name], i)
	}

	const batchSize = 200
	repoMap := make(map[string]string, len(packages))

	for start := 0; start < len(names); start += batchSize {
		end := start + batchSize
		if end > len(names) {
			end = len(names)
		}
		batch := names[start:end]

		args := append([]string{"policy"}, batch...)
		cmd := exec.Command("apk", args...)
		cmd.Env = append(os.Environ(), "LANG=C")
		output, err := cmd.Output()
		if err != nil {
			m.logger.WithError(err).Warn("apk policy failed, skipping repo attribution for batch")
			continue
		}

		m.parseApkPolicy(string(output), repoMap)
	}

	// Apply results
	for name, repo := range repoMap {
		for _, idx := range pkgIdx[name] {
			packages[idx].SourceRepository = repo
		}
	}

	m.logger.WithField("attributed", len(repoMap)).Debug("Enriched packages with repository attribution")
}

// parseApkPolicy parses apk policy output and populates repoMap.
// Output format per package:
//
//	curl policy:
//	  8.12.1-r0:
//	    lib/apk/db/installed
//	    https://dl-cdn.alpinelinux.org/alpine/v3.22/main/x86_64/APKINDEX.tar.gz
//
// Extracts the logical repo name (e.g. "main", "community") from the URL path.
func (m *APKManager) parseApkPolicy(output string, repoMap map[string]string) {
	lines := strings.Split(output, "\n")

	var currentPkg string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Package header: "packagename policy:"
		if strings.HasSuffix(trimmed, " policy:") {
			currentPkg = strings.TrimSuffix(trimmed, " policy:")
			continue
		}

		if currentPkg == "" {
			continue
		}

		// Already resolved this package
		if _, ok := repoMap[currentPkg]; ok {
			continue
		}

		// Skip version lines (end with ":")
		if strings.HasSuffix(trimmed, ":") {
			continue
		}

		// Skip local install marker
		if strings.Contains(trimmed, "lib/apk/db/installed") {
			continue
		}

		// URL line: extract logical repo name from path
		// e.g. https://dl-cdn.alpinelinux.org/alpine/v3.22/main/x86_64/APKINDEX.tar.gz -> "main"
		repo := m.extractRepoNameFromURL(trimmed)
		if repo != "" {
			repoMap[currentPkg] = repo
		}
	}
}

// extractRepoNameFromURL extracts the logical repository name from an APK repository URL.
// The repo name is the path segment before the architecture directory.
// e.g. .../v3.22/main/x86_64/APKINDEX.tar.gz -> "main"
// e.g. .../v3.22/community/aarch64/APKINDEX.tar.gz -> "community"
func (m *APKManager) extractRepoNameFromURL(url string) string {
	// Known architectures to look for
	archTokens := map[string]bool{
		"x86_64": true, "x86": true, "aarch64": true, "armhf": true,
		"armv7": true, "ppc64le": true, "s390x": true, "riscv64": true,
	}

	parts := strings.Split(url, "/")
	for i, part := range parts {
		if archTokens[part] && i > 0 {
			return parts[i-1]
		}
	}
	return ""
}

// parseInstalledPackages parses apk list --installed output
// Format: package-name-version-release arch {origin} (license) [installed]
// Example: alpine-base-3.22.2-r0 x86_64// parseInstalledPackages parses apk info -v output
func (m *APKManager) parseInstalledPackages(output string) map[string]models.Package {
	installedPackages := make(map[string]models.Package)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Skip lines that don't have [installed] marker (shouldn't happen with --installed flag, but be safe)
		if !strings.Contains(line, "[installed]") {
			continue
		}

		// Parse the line: package-name-version-release arch {origin} (license) [installed]
		// Example: alpine-base-3.22.2-r0 x86_64 {alpine-base} (MIT) [installed]
		fields := strings.Fields(line)
		if len(fields) < 2 {
			m.logger.WithField("line", line).Debug("Skipping malformed installed package line")
			continue
		}

		// First field contains package-name-version-release
		packageWithVersion := fields[0]

		// Extract package name and version-release
		packageName, version := m.extractPackageNameAndVersion(packageWithVersion)
		if packageName == "" || version == "" {
			m.logger.WithField("line", line).Debug("Failed to extract package name or version")
			continue
		}

		installedPackages[packageName] = models.Package{
			Name:           packageName,
			CurrentVersion: version,
			NeedsUpdate:    false,
		}
	}

	return installedPackages
}

// parseUpgradablePackages parses apk -u list output
// Format: package-name-new-version arch {origin} (license) [upgradable from: package-name-old-version]
// Example: alpine-conf-3.20.0-r1 x86_64 {alpine-conf} (MIT) [upgradable from: alpine-conf-3.20.0-r0]
func (m *APKManager) parseUpgradablePackages(output string, installedPackages map[string]models.Package) []models.Package {
	var packages []models.Package

	// Regex to match the upgradable from pattern
	upgradableFromRegex := regexp.MustCompile(`\[upgradable from: (.+)\]`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Check if line contains upgradable from pattern
		matches := upgradableFromRegex.FindStringSubmatch(line)
		if len(matches) < 2 {
			// Not an upgradable package line
			continue
		}

		// Extract the old version package name
		oldPackageWithVersion := matches[1]

		// Parse the main line to get new version
		// Format: package-name-new-version arch {origin} (license) [upgradable from: ...]
		fields := strings.Fields(line)
		if len(fields) < 2 {
			m.logger.WithField("line", line).Debug("Skipping malformed upgradable package line")
			continue
		}

		// First field contains package-name-new-version
		newPackageWithVersion := fields[0]

		// Extract package name and versions
		newPackageName, newVersion := m.extractPackageNameAndVersion(newPackageWithVersion)
		oldPackageName, oldVersion := m.extractPackageNameAndVersion(oldPackageWithVersion)

		// Verify package names match
		if newPackageName == "" || newVersion == "" || oldPackageName == "" || oldVersion == "" {
			m.logger.WithField("line", line).Debug("Failed to extract package name or version from upgradable line")
			continue
		}

		if newPackageName != oldPackageName {
			m.logger.WithFields(logrus.Fields{
				"newPackage": newPackageName,
				"oldPackage": oldPackageName,
			}).Debug("Package names don't match in upgradable line, using new package name")
		}

		// Use the current version from installed packages if available, otherwise use old version
		currentVersion := oldVersion
		if installedPkg, found := installedPackages[newPackageName]; found {
			currentVersion = installedPkg.CurrentVersion
		}

		// Alpine doesn't have built-in security update tracking
		// We'll mark all updates as potentially security updates (conservative approach)
		isSecurityUpdate := false

		packages = append(packages, models.Package{
			Name:             newPackageName,
			CurrentVersion:   currentVersion,
			AvailableVersion: newVersion,
			NeedsUpdate:      true,
			IsSecurityUpdate: isSecurityUpdate,
		})
	}

	return packages
}

// extractPackageNameAndVersion extracts package name and version from a package string
// Format: package-name-version-release
// Example: alpine-conf-3.20.0-r1 -> packageName: "alpine-conf", version: "3.20.0-r1"
// Example: zzz-doc-0.2.0-r0 -> packageName: "zzz-doc", version: "0.2.0-r0"
func (m *APKManager) extractPackageNameAndVersion(packageWithVersion string) (packageName, version string) {
	// Find the first dash followed by a digit (version starts)
	// This handles packages with dashes in their names
	for i := 0; i < len(packageWithVersion); i++ {
		if packageWithVersion[i] == '-' && i+1 < len(packageWithVersion) {
			nextChar := packageWithVersion[i+1]
			// Check if the next character is a digit (version starts)
			if nextChar >= '0' && nextChar <= '9' {
				// This is the start of version
				packageName = packageWithVersion[:i]
				version = packageWithVersion[i+1:]
				return
			}
		}
	}

	// If no version pattern found, return the whole string as package name
	packageName = packageWithVersion
	return
}
