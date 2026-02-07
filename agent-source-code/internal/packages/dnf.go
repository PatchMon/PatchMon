package packages

import (
	"bufio"
	"os/exec"
	"slices"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// DNFManager handles dnf/yum package information collection
type DNFManager struct {
	logger *logrus.Logger
}

// NewDNFManager creates a new DNF package manager
func NewDNFManager(logger *logrus.Logger) *DNFManager {
	return &DNFManager{
		logger: logger,
	}
}

// detectPackageManager detects whether to use dnf or yum
func (m *DNFManager) detectPackageManager() string {
	// Prefer dnf over yum for modern RHEL-based systems
	packageManager := "dnf"
	if _, err := exec.LookPath("dnf"); err != nil {
		// Fall back to yum if dnf is not available (legacy systems)
		packageManager = "yum"
	}
	return packageManager
}

// GetPackages gets package information for RHEL-based systems
func (m *DNFManager) GetPackages() []models.Package {
	// Determine package manager
	packageManager := m.detectPackageManager()

	m.logger.WithField("manager", packageManager).Debug("Using package manager")

	// Note: We don't run 'makecache' because:
	// 1. It causes delays on systems without internet (tries to reach remote repos)
	// 2. It's not needed for listing installed packages
	// 3. The 'check-update' command already refreshes metadata when needed
	// 4. Fedora's cache issue (if any) is resolved by using proper update checks

	// Get installed packages
	m.logger.Debug("Getting installed packages...")
	listCmd := exec.Command(packageManager, "list", "--installed")
	installedOutput, err := listCmd.Output()
	var installedPackages map[string]models.Package
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get installed packages")
		installedPackages = make(map[string]models.Package)
	} else {
		m.logger.WithField("outputSize", len(installedOutput)).Debug("Received output from list installed command")
		m.logger.Debug("Parsing installed packages...")
		installedPackages = m.parseInstalledPackages(string(installedOutput))
		m.logger.WithField("count", len(installedPackages)).Info("Found installed packages")

		if len(installedPackages) == 0 {
			m.logger.Warn("No installed packages found - this may indicate a parsing issue")
		}
	}

	// Get security updates first to identify which packages are security updates
	m.logger.Debug("Getting security updates...")
	securityPackages := m.getSecurityPackages(packageManager)
	m.logger.WithField("count", len(securityPackages)).Debug("Found security packages")

	// Get upgradable packages
	m.logger.Debug("Getting upgradable packages...")
	checkCmd := exec.Command(packageManager, "check-update")
	checkOutput, _ := checkCmd.Output() // This command returns exit code 100 when updates are available

	var upgradablePackages []models.Package
	if len(checkOutput) > 0 {
		m.logger.Debug("Parsing DNF/yum check-update output...")
		upgradablePackages = m.parseUpgradablePackages(string(checkOutput), packageManager, installedPackages, securityPackages)
		m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")
	} else {
		m.logger.Debug("No updates available")
		upgradablePackages = []models.Package{}
	}

	// Merge and deduplicate packages
	packages := CombinePackageData(installedPackages, upgradablePackages)
	m.logger.WithFields(logrus.Fields{
		"total":             len(packages),
		"installed":         len(installedPackages),
		"upgradable":        len(upgradablePackages),
		"securityAvailable": len(securityPackages),
	}).Info("Package collection completed")

	if len(packages) == 0 {
		m.logger.Error("WARNING: Returning 0 packages - this will show as empty in PatchMon UI")
	}

	return packages
}

// getSecurityPackages gets the list of security packages from dnf/yum updateinfo
func (m *DNFManager) getSecurityPackages(packageManager string) map[string]bool {
	securityPackages := make(map[string]bool)

	// Try dnf updateinfo list security (works for dnf)
	updateInfoCmd := exec.Command(packageManager, "updateinfo", "list", "security")
	updateInfoOutput, err := updateInfoCmd.Output()
	if err != nil {
		// Fall back to "sec" if "security" doesn't work
		updateInfoCmd = exec.Command(packageManager, "updateinfo", "list", "sec")
		updateInfoOutput, err = updateInfoCmd.Output()
	}

	if err != nil {
		m.logger.WithError(err).Debug("Failed to get security updates, will not mark packages as security updates")
		return securityPackages
	}

	// Parse the output to extract package names
	scanner := bufio.NewScanner(strings.NewReader(string(updateInfoOutput)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip header lines and empty lines
		if line == "" || strings.Contains(line, "Last metadata") ||
			strings.Contains(line, "expiration") || strings.HasPrefix(line, "Loading") {
			continue
		}

		// Format: ALSA-2025:11140 Moderate/Sec.  glib2-2.68.4-16.el9_6.2.x86_64
		// We need to extract the package name (3rd field) and get the base name
		fields := slices.Collect(strings.FieldsSeq(line))
		if len(fields) < 3 {
			continue
		}

		// Skip lines that don't start with advisory IDs
		// Common advisory ID prefixes: RHSA (Red Hat), ALSA (AlmaLinux), ELSA (Oracle/Enterprise Linux), CESA (CentOS)
		// This filters out header lines like "expiration"
		advisoryID := fields[0]
		isAdvisory := strings.HasPrefix(advisoryID, "RHSA") ||
			strings.HasPrefix(advisoryID, "ALSA") ||
			strings.HasPrefix(advisoryID, "ELSA") ||
			strings.HasPrefix(advisoryID, "CESA")

		if !isAdvisory {
			continue
		}

		// The package name is in the format: package-name-version-release.arch
		// We need to extract just the base package name
		packageNameWithVersion := fields[2]
		basePackageName := m.extractBasePackageName(packageNameWithVersion)

		if basePackageName != "" {
			securityPackages[basePackageName] = true
			m.logger.WithFields(logrus.Fields{
				"advisory": advisoryID,
				"package":  basePackageName,
			}).Debug("Detected security package")
		}
	}

	m.logger.WithField("count", len(securityPackages)).Info("Security packages identified")
	return securityPackages
}

// extractBasePackageName extracts the base package name from a package string
// Handles formats like:
// - package-name-version-release.arch (from updateinfo)
// - package-name.arch (from check-update)
func (m *DNFManager) extractBasePackageName(packageString string) string {
	// Remove architecture suffix first (e.g., .x86_64, .noarch)
	baseName := packageString
	if idx := strings.LastIndex(packageString, "."); idx > 0 {
		archSuffix := packageString[idx+1:]
		// Check if it's a known architecture
		if archSuffix == "x86_64" || archSuffix == "i686" || archSuffix == "i386" ||
			archSuffix == "noarch" || archSuffix == "aarch64" || archSuffix == "arm64" {
			baseName = packageString[:idx]
		}
	}

	// If the base name contains a version pattern (starts with a digit after a dash),
	// extract just the package name part
	// Format: package-name-version-release
	// We look for the FIRST dash that's followed by a digit (version starts)
	// This handles packages with dashes in their names like "glibc-common-2.34-168.el9_6.19"
	for i := 0; i < len(baseName); i++ {
		if baseName[i] == '-' && i+1 < len(baseName) {
			nextChar := baseName[i+1]
			// Check if the next character is a digit (version starts)
			if nextChar >= '0' && nextChar <= '9' {
				// This is the start of version, return everything before this dash
				return baseName[:i]
			}
		}
	}

	return baseName
}

// parseUpgradablePackages parses dnf/yum check-update output
func (m *DNFManager) parseUpgradablePackages(output string, packageManager string, installedPackages map[string]models.Package, securityPackages map[string]bool) []models.Package {
	var packages []models.Package

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip header lines and empty lines
		if line == "" || strings.Contains(line, "Loaded plugins") ||
			strings.Contains(line, "Last metadata") || strings.HasPrefix(line, "Loading") {
			continue
		}

		fields := slices.Collect(strings.FieldsSeq(line))
		if len(fields) < 3 {
			continue
		}

		packageName := fields[0]
		availableVersion := fields[1]

		// Get current version from installed packages map (already collected)
		// Try exact match first
		var currentVersion string
		if p, ok := installedPackages[packageName]; ok {
			currentVersion = p.CurrentVersion
		}

		// If not found, try to find by base name (handles architecture suffixes)
		// e.g., if packageName is "package" but installed has "package.x86_64"
		// or if packageName is "package.x86_64" but installed has "package"
		if currentVersion == "" {
			// Try to find by removing architecture suffix from packageName (if present)
			basePackageName := packageName
			if idx := strings.LastIndex(packageName, "."); idx > 0 {
				archSuffix := packageName[idx+1:]
				if archSuffix == "x86_64" || archSuffix == "i686" || archSuffix == "i386" ||
					archSuffix == "noarch" || archSuffix == "aarch64" || archSuffix == "arm64" {
					basePackageName = packageName[:idx]
					if p, ok := installedPackages[basePackageName]; ok {
						currentVersion = p.CurrentVersion
					}
				}
			}

			// If still not found, search through installed packages for matching base name
			if currentVersion == "" {
				for installedName, p := range installedPackages {
					// Remove architecture suffix if present (e.g., .x86_64, .noarch, .i686)
					baseName := installedName
					if idx := strings.LastIndex(installedName, "."); idx > 0 {
						// Check if the part after the last dot looks like an architecture
						archSuffix := installedName[idx+1:]
						if archSuffix == "x86_64" || archSuffix == "i686" || archSuffix == "i386" ||
							archSuffix == "noarch" || archSuffix == "aarch64" || archSuffix == "arm64" {
							baseName = installedName[:idx]
						}
					}

					// Compare base names (handles both cases: package vs package.x86_64)
					if baseName == basePackageName || baseName == packageName {
						currentVersion = p.CurrentVersion
						break
					}
				}
			}
		}

		// If still not found in installed packages, try to get it with a command as fallback
		if currentVersion == "" {
			getCurrentCmd := exec.Command(packageManager, "list", "--installed", packageName)
			getCurrentOutput, err := getCurrentCmd.Output()
			if err == nil {
				for _, currentLine := range strings.Split(string(getCurrentOutput), "\n") {
					if strings.Contains(currentLine, packageName) && !strings.Contains(currentLine, "Installed") && !strings.Contains(currentLine, "Available") {
						currentFields := slices.Collect(strings.FieldsSeq(currentLine))
						if len(currentFields) >= 2 {
							currentVersion = currentFields[1]
							break
						}
					}
				}
			}
		}

		// Only add package if we have both current and available versions
		// This prevents empty currentVersion errors on the server
		if packageName != "" && currentVersion != "" && availableVersion != "" {
			// Extract base package name to check against security packages
			basePackageName := m.extractBasePackageName(packageName)
			isSecurityUpdate := securityPackages[basePackageName]

			packages = append(packages, models.Package{
				Name:             packageName,
				CurrentVersion:   currentVersion,
				AvailableVersion: availableVersion,
				NeedsUpdate:      true,
				IsSecurityUpdate: isSecurityUpdate,
			})
		} else {
			m.logger.WithFields(logrus.Fields{
				"package":          packageName,
				"currentVersion":   currentVersion,
				"availableVersion": availableVersion,
			}).Debug("Skipping package due to missing version information")
		}
	}

	return packages
}

// parseInstalledPackages parses dnf list installed output
func (m *DNFManager) parseInstalledPackages(output string) map[string]models.Package {
	installedPackages := make(map[string]models.Package)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Installed Packages") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		packageName := strings.Split(parts[0], ".")[0] // Remove architecture
		version := parts[1]

		installedPackages[packageName] = models.Package{
			Name:           packageName,
			CurrentVersion: version,
			NeedsUpdate:    false,
		}
	}

	return installedPackages
}
