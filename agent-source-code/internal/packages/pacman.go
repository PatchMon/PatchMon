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

var installedPackageRe = regexp.MustCompile(`^(\S+)\s+(\S+)$`)
var checkUpdateRe = regexp.MustCompile(`^(\S+)\s+(\S+)\s+->\s+(\S+)$`)

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
	// Get installed packages
	installedCmd := runCommand("pacman", "-Q")
	installedOutput, err := installedCmd.Output()
	var installedPackages map[string]string
	if err != nil {
		m.logger.WithError(err).Error("Failed to get installed packages")
		installedPackages = make(map[string]string)
	} else {
		installedPackages = m.parseInstalledPackages(string(installedOutput))
	}

	upgradablePackages, err := m.getUpgradablePackages()
	if err != nil {
		return nil, err
	}

	// Merge and deduplicate packages
	packages := CombinePackageData(installedPackages, upgradablePackages)
	return packages, nil
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

// parseInstalledPackages parses pacman -Q output and returns a map of package name to version
func (m *PacmanManager) parseInstalledPackages(output string) map[string]string {
	installedPackages := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		matches := installedPackageRe.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}

		packageName := matches[1]
		version := matches[2]
		installedPackages[packageName] = version
	}

	return installedPackages
}
