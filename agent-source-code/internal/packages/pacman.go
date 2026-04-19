package packages

import (
	"bufio"
	"errors"
	"os/exec"
	"regexp"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

var checkUpdateRe = regexp.MustCompile(`^(\S+)\s+(\S+)\s+->\s+(\S+)$`)

// installedPkg holds version and repository for a parsed installed package.
type installedPkg struct {
	version string
	repo    string
}

// PacmanManager handles pacman package information collection
type PacmanManager struct {
	logger *logrus.Logger
}

// NewPacmanManager creates a new Pacman package manager
func NewPacmanManager(logger *logrus.Logger) *PacmanManager {
	return &PacmanManager{
		logger: logger,
	}
}

// indirections for testability
var (
	lookPath   = exec.LookPath
	runCommand = exec.Command
)

// GetPackages gets package information for pacman-based systems
func (m *PacmanManager) GetPackages() ([]models.Package, error) {
	// Get installed packages with repo info from pacman -Sl
	installedPackages := m.parseInstalledFromSyncList()

	// Get foreign/AUR packages from pacman -Qm
	foreignPkgs := m.getForeignPackages()
	for name, fp := range foreignPkgs {
		installedPackages[name] = fp
	}

	upgradablePackages, err := m.getUpgradablePackages()
	if err != nil {
		return nil, err
	}

	// Build package map for CombinePackageData
	pkgMap := make(map[string]models.Package, len(installedPackages))
	for name, ip := range installedPackages {
		pkgMap[name] = models.Package{
			Name:             name,
			CurrentVersion:   ip.version,
			NeedsUpdate:      false,
			SourceRepository: ip.repo,
		}
	}

	// Merge and deduplicate packages
	packages := CombinePackageData(pkgMap, upgradablePackages)

	// Carry over SourceRepository from installed data to final packages
	for i := range packages {
		if packages[i].SourceRepository == "" {
			if ip, ok := installedPackages[packages[i].Name]; ok {
				packages[i].SourceRepository = ip.repo
			}
		}
	}

	return packages, nil
}

// parseInstalledFromSyncList runs pacman -Sl and returns installed packages with their repo names.
// pacman -Sl output format: <repo> <name> <version> [installed] or [installed: <local-version>]
func (m *PacmanManager) parseInstalledFromSyncList() map[string]installedPkg {
	installed := make(map[string]installedPkg)

	cmd := runCommand("pacman", "-Sl")
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Warn("pacman -Sl failed, falling back to pacman -Q")
		return m.fallbackParseInstalled()
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		// Format: repo name version [installed] or repo name version [installed: localver]
		if len(fields) < 4 {
			continue
		}

		// Check if the 4th field indicates installed
		if !strings.HasPrefix(fields[3], "[installed") {
			continue
		}

		repo := fields[0]
		name := fields[1]
		version := fields[2]

		installed[name] = installedPkg{
			version: version,
			repo:    repo,
		}
	}

	m.logger.WithField("count", len(installed)).Debug("Found installed packages from pacman -Sl")
	return installed
}

// fallbackParseInstalled uses pacman -Q as a fallback when pacman -Sl fails.
func (m *PacmanManager) fallbackParseInstalled() map[string]installedPkg {
	installed := make(map[string]installedPkg)

	cmd := runCommand("pacman", "-Q")
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Error("Failed to get installed packages")
		return installed
	}

	re := regexp.MustCompile(`^(\S+)\s+(\S+)$`)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		matches := re.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}
		installed[matches[1]] = installedPkg{
			version: matches[2],
		}
	}

	return installed
}

// getForeignPackages runs pacman -Qm to get foreign/AUR packages.
func (m *PacmanManager) getForeignPackages() map[string]installedPkg {
	foreign := make(map[string]installedPkg)

	cmd := runCommand("pacman", "-Qm")
	output, err := cmd.Output()
	if err != nil {
		// pacman -Qm returns exit code 1 if no foreign packages exist
		m.logger.WithError(err).Debug("pacman -Qm returned error (may have no foreign packages)")
		return foreign
	}

	re := regexp.MustCompile(`^(\S+)\s+(\S+)$`)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		matches := re.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}
		foreign[matches[1]] = installedPkg{
			version: matches[2],
			repo:    "foreign",
		}
	}

	m.logger.WithField("count", len(foreign)).Debug("Found foreign/AUR packages")
	return foreign
}

// getUpgradablePackages runs checkupdates and returns parsed packages.
func (m *PacmanManager) getUpgradablePackages() ([]models.Package, error) {
	if _, err := lookPath("checkupdates"); err != nil {
		m.logger.WithError(err).Error("checkupdates not found (pacman-contrib not installed)")
		return nil, err
	}

	upgradeCmd := runCommand("checkupdates")
	upgradeOutput, err := upgradeCmd.Output()
	if err != nil {
		// 0 = success with output, 1 = unknown failure, 2 = no updates available.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 2 {
				return []models.Package{}, nil
			}
		}
		m.logger.WithError(err).Error("checkupdates failed")
		return nil, err
	}

	pkgs := m.parseCheckUpdate(string(upgradeOutput))
	return pkgs, nil
}

// parseCheckUpdate parses checkupdates output
func (m *PacmanManager) parseCheckUpdate(output string) []models.Package {
	packages := make([]models.Package, 0)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		matches := checkUpdateRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		pkg := models.Package{
			Name:             matches[1],
			CurrentVersion:   matches[2],
			AvailableVersion: matches[3],
			NeedsUpdate:      true,
			IsSecurityUpdate: false, // Data not provided
		}
		packages = append(packages, pkg)
	}

	return packages
}
