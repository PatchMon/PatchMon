package packages

import (
	"bufio"
	"os/exec"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// OpenBSDManager handles OpenBSD package information collection
type OpenBSDManager struct {
	logger *logrus.Logger
}

// NewOpenBSDManager creates a new OpenBSD package manager
func NewOpenBSDManager(logger *logrus.Logger) *OpenBSDManager {
	return &OpenBSDManager{
		logger: logger,
	}
}

// GetPackages gets package information for OpenBSD systems.
// Collects from: pkg_info (binary packages) and syspatch (base system patches).
func (m *OpenBSDManager) GetPackages() ([]models.Package, error) {
	var allPackages []models.Package

	// 1. Get pkg binary packages
	pkgPackages, err := m.getPkgPackages()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get OpenBSD pkg packages")
	} else {
		allPackages = append(allPackages, pkgPackages...)
	}

	// 2. Get syspatch base-system update info
	sysPackage := m.getSyspatchInfo()
	if sysPackage != nil {
		allPackages = append(allPackages, *sysPackage)
	}

	return allPackages, nil
}

// getPkgPackages collects installed packages via pkg_info and available upgrades
// via pkg_add -uqn (dry-run).
func (m *OpenBSDManager) getPkgPackages() ([]models.Package, error) {
	// --- installed packages ---
	m.logger.Debug("Getting installed packages with pkg_info...")
	installedOut, err := exec.Command("pkg_info").Output()
	installedPackages := make(map[string]string)
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get installed packages via pkg_info")
	} else {
		installedPackages = m.parseInstalledPackages(string(installedOut))
		m.logger.WithField("count", len(installedPackages)).Debug("Found installed packages")
	}

	// --- upgradable packages ---
	// pkg_add -uqn: dry-run upgrade, quiet, no interaction.
	// CombinedOutput captures stderr too (pkg_add prints progress to stderr).
	m.logger.Debug("Checking for package upgrades via pkg_add -uqn...")
	upgradeOut, _ := exec.Command("pkg_add", "-uqn").CombinedOutput()
	upgradablePackages := m.parseUpgradeOutput(string(upgradeOut))
	m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")

	packages := CombinePackageData(installedPackages, upgradablePackages)
	return packages, nil
}

// parseInstalledPackages parses pkg_info output.
// Format: pkgname-version    Description
// Example: bash-5.2.15          GNU Project's Bourne Again SHell
func (m *OpenBSDManager) parseInstalledPackages(output string) map[string]string {
	installedPackages := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		pkgName, version := splitOpenBSDPackage(fields[0])
		if pkgName != "" && version != "" {
			installedPackages[pkgName] = version
		}
	}
	return installedPackages
}

// parseUpgradeOutput parses pkg_add -uqn output.
// pkg_add prints candidate lines in the form:
//
//	Update candidates: curl-8.7.1 -> curl-8.10.0
func (m *OpenBSDManager) parseUpgradeOutput(output string) []models.Package {
	var packages []models.Package
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Update candidates:") {
			continue
		}
		// "Update candidates: pkgold-X.Y -> pkgnew-A.B"
		rest := strings.TrimSpace(strings.TrimPrefix(line, "Update candidates:"))
		parts := strings.SplitN(rest, "->", 2)
		if len(parts) != 2 {
			continue
		}
		oldPkg := strings.TrimSpace(parts[0])
		newPkg := strings.TrimSpace(parts[1])

		oldName, oldVersion := splitOpenBSDPackage(oldPkg)
		newName, newVersion := splitOpenBSDPackage(newPkg)

		if oldName == "" || oldVersion == "" || newVersion == "" {
			continue
		}
		name := oldName
		if newName != "" {
			name = newName
		}
		packages = append(packages, models.Package{
			Name:             name,
			CurrentVersion:   oldVersion,
			AvailableVersion: newVersion,
			NeedsUpdate:      true,
			IsSecurityUpdate: false,
		})
	}
	return packages
}

// getSyspatchInfo checks for available OpenBSD base-system patches via syspatch -c.
// Returns a synthetic package entry when patches are pending, nil otherwise.
func (m *OpenBSDManager) getSyspatchInfo() *models.Package {
	cmd := exec.Command("syspatch", "-c")
	output, err := cmd.Output()
	if err != nil {
		// syspatch not available or no connectivity — not an error we surface to the user
		return nil
	}
	patches := strings.TrimSpace(string(output))
	if patches == "" {
		return nil
	}
	// One or more patch IDs are listed (e.g. "001_xyz\n002_abc")
	return &models.Package{
		Name:             "syspatch",
		CurrentVersion:   "applied",
		AvailableVersion: patches,
		NeedsUpdate:      true,
		IsSecurityUpdate: true,
	}
}

// splitOpenBSDPackage splits an OpenBSD package token "pkgname-version" into its
// name and version components.  The version always starts with a digit.
//
// Examples:
//
//	"bash-5.2.15"        -> ("bash", "5.2.15")
//	"py3-setuptools-68.0.0" -> ("py3-setuptools", "68.0.0")
func splitOpenBSDPackage(s string) (name, version string) {
	for i := len(s) - 1; i >= 1; i-- {
		if s[i] == '-' && s[i+1] >= '0' && s[i+1] <= '9' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
