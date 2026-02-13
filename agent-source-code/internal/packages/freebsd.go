package packages

import (
	"bufio"
	"os/exec"
	"regexp"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// FreeBSDManager handles FreeBSD package information collection
type FreeBSDManager struct {
	logger *logrus.Logger
}

// NewFreeBSDManager creates a new FreeBSD package manager
func NewFreeBSDManager(logger *logrus.Logger) *FreeBSDManager {
	return &FreeBSDManager{
		logger: logger,
	}
}

// GetPackages gets package information for FreeBSD systems
// Collects from: pkg (binary packages), freebsd-update (base system), and pkg audit (security)
func (m *FreeBSDManager) GetPackages() ([]models.Package, error) {
	var allPackages []models.Package

	// 1. Get pkg binary packages (primary)
	pkgPackages, err := m.getPkgPackages()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get pkg packages")
	} else {
		allPackages = append(allPackages, pkgPackages...)
	}

	// 2. Get freebsd-update base system updates
	basePackage := m.getFreeBSDUpdates()
	if basePackage != nil {
		allPackages = append(allPackages, *basePackage)
	}

	// 3. Get security audit information and mark vulnerable packages
	m.markSecurityVulnerabilities(allPackages)

	return allPackages, nil
}

// getPkgPackages gets installed and upgradable packages from pkg
func (m *FreeBSDManager) getPkgPackages() ([]models.Package, error) {
	// Get installed packages: pkg info
	m.logger.Debug("Getting installed packages with pkg info...")
	installedCmd := exec.Command("pkg", "info")
	installedOutput, err := installedCmd.Output()

	var installedPackages map[string]string
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get installed packages")
		installedPackages = make(map[string]string)
	} else {
		installedPackages = m.parseInstalledPackages(string(installedOutput))
		m.logger.WithField("count", len(installedPackages)).Debug("Found installed packages")
	}

	// Get upgradable packages: pkg upgrade -n
	m.logger.Debug("Checking for package upgrades...")
	upgradeCmd := exec.Command("pkg", "upgrade", "-n")
	upgradeOutput, err := upgradeCmd.Output()

	var upgradablePackages []models.Package
	if err != nil {
		// Exit code 1 can mean no upgrades available or error, check output
		if exitErr, ok := err.(*exec.ExitError); ok {
			m.logger.WithField("exit_code", exitErr.ExitCode()).Debug("pkg upgrade -n returned non-zero")
			// Try to parse output anyway in case there's useful info
			if len(upgradeOutput) > 0 {
				upgradablePackages = m.parseUpgradeOutput(string(upgradeOutput), installedPackages)
			} else {
				upgradablePackages = []models.Package{}
			}
		} else {
			upgradablePackages = []models.Package{}
		}
	} else {
		upgradablePackages = m.parseUpgradeOutput(string(upgradeOutput), installedPackages)
		m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")
	}

	// Combine installed and upgradable packages
	packages := CombinePackageData(installedPackages, upgradablePackages)
	return packages, nil
}

// parseInstalledPackages parses pkg info output
// Format: package-name-version    Description
// Example: bash-5.3.9                     GNU Project's Bourne Again SHell
func (m *FreeBSDManager) parseInstalledPackages(output string) map[string]string {
	installedPackages := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Split on whitespace: first field is package-version
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}

		packageWithVersion := fields[0]
		packageName, version := m.extractPackageNameAndVersion(packageWithVersion)

		if packageName != "" && version != "" {
			installedPackages[packageName] = version
		}
	}

	return installedPackages
}

// parseUpgradeOutput parses pkg upgrade -n output
// Example output:
// The following 5 package(s) will be affected (of 0 checked):
//
// Installed packages to be UPGRADED:
//         curl: 8.9.1 -> 8.10.0
//         git: 2.46.0 -> 2.46.1
func (m *FreeBSDManager) parseUpgradeOutput(output string, installedPackages map[string]string) []models.Package {
	var packages []models.Package

	// Regex to match upgrade lines: packagename: oldversion -> newversion
	upgradeRegex := regexp.MustCompile(`^\s+(\S+):\s+(\S+)\s+->\s+(\S+)`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	inUpgradeSection := false

	for scanner.Scan() {
		line := scanner.Text()

		// Detect upgrade section
		if strings.Contains(line, "to be UPGRADED") {
			inUpgradeSection = true
			continue
		}

		// End of upgrade section markers
		if inUpgradeSection && (strings.Contains(line, "to be INSTALLED") ||
			strings.Contains(line, "to be REINSTALLED") ||
			strings.HasPrefix(strings.TrimSpace(line), "Number of")) {
			inUpgradeSection = false
			continue
		}

		// Skip empty lines in upgrade section (they're just spacing)
		if inUpgradeSection && strings.TrimSpace(line) == "" {
			continue
		}

		if !inUpgradeSection {
			continue
		}

		// Parse upgrade line
		matches := upgradeRegex.FindStringSubmatch(line)
		if len(matches) == 4 {
			packageName := matches[1]
			currentVersion := matches[2]
			availableVersion := matches[3]

			packages = append(packages, models.Package{
				Name:             packageName,
				CurrentVersion:   currentVersion,
				AvailableVersion: availableVersion,
				NeedsUpdate:      true,
				IsSecurityUpdate: false, // Will be set by markSecurityVulnerabilities
			})
		}
	}

	return packages
}

// markSecurityVulnerabilities uses pkg audit to mark packages with known vulnerabilities
func (m *FreeBSDManager) markSecurityVulnerabilities(packages []models.Package) {
	// Run pkg audit (fetch vulnerability database if needed)
	m.logger.Debug("Running pkg audit to check for vulnerabilities...")

	// First update the vulnerability database
	fetchCmd := exec.Command("pkg", "audit", "-F")
	if err := fetchCmd.Run(); err != nil {
		m.logger.WithError(err).Debug("Failed to fetch vulnerability database (may require root)")
	}

	// Run the audit
	auditCmd := exec.Command("pkg", "audit")
	auditOutput, err := auditCmd.CombinedOutput()

	if err != nil {
		// pkg audit returns non-zero if vulnerabilities found, which is expected
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Exit code 1 means vulnerabilities were found, this is normal
			m.logger.Debug("pkg audit found vulnerabilities")
		} else {
			m.logger.WithError(err).Debug("pkg audit failed")
			return
		}
	}

	if len(auditOutput) == 0 {
		m.logger.Debug("No vulnerabilities found")
		return
	}

	// Parse vulnerable packages
	vulnerablePackages := m.parseAuditOutput(string(auditOutput))

	// Mark packages as security updates
	for i := range packages {
		if vulnerablePackages[packages[i].Name] {
			packages[i].IsSecurityUpdate = true
		}
	}

	m.logger.WithField("vulnerable_count", len(vulnerablePackages)).Debug("Identified vulnerable packages")
}

// parseAuditOutput parses pkg audit output to get list of vulnerable packages
// Example output:
// curl-8.9.1 is vulnerable:
//   curl -- multiple vulnerabilities
//   CVE: CVE-2024-XXXX
//   WWW: https://vuxml.FreeBSD.org/freebsd/...
func (m *FreeBSDManager) parseAuditOutput(output string) map[string]bool {
	vulnerablePackages := make(map[string]bool)

	// Match lines like: "packagename-version is vulnerable:"
	vulnRegex := regexp.MustCompile(`^(\S+)-[\d\w._,]+ is vulnerable:`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		matches := vulnRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			packageName := matches[1]
			vulnerablePackages[packageName] = true
		}
	}

	return vulnerablePackages
}

// getFreeBSDUpdates checks freebsd-update for base system updates
// Returns a special "freebsd-base" package if updates are available
func (m *FreeBSDManager) getFreeBSDUpdates() *models.Package {
	m.logger.Debug("Checking for FreeBSD base system updates...")

	// Run freebsd-update fetch (requires root, will fail gracefully otherwise)
	// We use fetch with --not-running-from-cron to avoid emails
	cmd := exec.Command("freebsd-update", "fetch", "--not-running-from-cron")
	output, err := cmd.CombinedOutput()

	if err != nil {
		// freebsd-update requires root privileges
		if exitErr, ok := err.(*exec.ExitError); ok {
			m.logger.WithField("exit_code", exitErr.ExitCode()).Debug("freebsd-update failed (may require root)")
		}
		return nil
	}

	outputStr := string(output)

	// Check if updates are available
	// freebsd-update outputs different messages:
	// "No updates needed" or "No updates are available"
	if strings.Contains(outputStr, "No updates") {
		m.logger.Debug("No FreeBSD base system updates available")
		return nil
	}

	// Check for actual updates being fetched
	// Output contains lines like "The following files will be updated as part of updating to..."
	if strings.Contains(outputStr, "will be updated") || strings.Contains(outputStr, "will be installed") {
		m.logger.Debug("FreeBSD base system updates available")

		// Get current FreeBSD version
		versionCmd := exec.Command("freebsd-version")
		versionOutput, err := versionCmd.Output()
		currentVersion := "Unknown"
		if err == nil {
			currentVersion = strings.TrimSpace(string(versionOutput))
		}

		return &models.Package{
			Name:             "freebsd-base",
			Description:      "FreeBSD base system",
			CurrentVersion:   currentVersion,
			AvailableVersion: "Updates available",
			NeedsUpdate:      true,
			IsSecurityUpdate: true, // Base system updates are typically security-related
		}
	}

	return nil
}

// extractPackageNameAndVersion extracts package name and version from FreeBSD package string
// Format: packagename-version (e.g., bash-5.3.9, go125-1.25.7, libX11-1.8.12,1)
// FreeBSD uses the last hyphen before a digit sequence as the separator
func (m *FreeBSDManager) extractPackageNameAndVersion(packageWithVersion string) (packageName, version string) {
	// Find the last hyphen that's followed by a digit
	lastHyphenIdx := -1
	for i := len(packageWithVersion) - 1; i >= 0; i-- {
		if packageWithVersion[i] == '-' && i+1 < len(packageWithVersion) {
			nextChar := packageWithVersion[i+1]
			if nextChar >= '0' && nextChar <= '9' {
				lastHyphenIdx = i
				break
			}
		}
	}

	if lastHyphenIdx == -1 {
		return packageWithVersion, ""
	}

	packageName = packageWithVersion[:lastHyphenIdx]
	version = packageWithVersion[lastHyphenIdx+1:]
	return
}
