package packages

import (
	"bufio"
	"os/exec"
	"slices"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// APTManager handles APT package information collection
type APTManager struct {
	logger *logrus.Logger
}

// NewAPTManager creates a new APT package manager
func NewAPTManager(logger *logrus.Logger) *APTManager {
	return &APTManager{
		logger: logger,
	}
}

// detectPackageManager detects whether to use apt or apt-get
func (m *APTManager) detectPackageManager() string {
	// Prefer apt over apt-get for modern Debian-based systems
	packageManager := "apt"
	if _, err := exec.LookPath("apt"); err != nil {
		packageManager = "apt-get"
	}
	return packageManager
}

// GetPackages gets package information for APT-based systems
func (m *APTManager) GetPackages() []models.Package {
	// Determine package manager
	packageManager := m.detectPackageManager()

	// Update package lists using detected package manager
	m.logger.WithField("manager", packageManager).Debug("Updating package lists")
	updateCmd := exec.Command(packageManager, "update", "-qq")

	if err := updateCmd.Run(); err != nil {
		m.logger.WithError(err).WithField("manager", packageManager).Warn("Failed to update package lists")
	}

	// Get installed packages
	m.logger.Debug("Getting installed packages...")
	// Note: Description can be multiline. Multiline descriptions in debian packages usually have subsequent lines indented.
	installedCmd := exec.Command("dpkg-query", "-W", "-f", "${Package} ${Version} ${Description}\n")
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

	// Get upgradable packages using apt simulation
	m.logger.Debug("Getting upgradable packages...")
	upgradeCmd := exec.Command(packageManager, "-s", "-o", "Debug::NoLocking=1", "upgrade")

	upgradeOutput, err := upgradeCmd.Output()
	var upgradablePackages []models.Package
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get upgrade simulation")
		upgradablePackages = []models.Package{}
	} else {
		m.logger.Debug("Parsing apt upgrade simulation output...")
		upgradablePackages = m.parseAPTUpgrade(string(upgradeOutput))
		m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")
	}

	// Merge and deduplicate packages
	packages := CombinePackageData(installedPackages, upgradablePackages)

	return packages
}

// parseAPTUpgrade parses apt/apt-get upgrade simulation output
func (m *APTManager) parseAPTUpgrade(output string) []models.Package {
	var packages []models.Package

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Look for lines starting with "Inst"
		if !strings.HasPrefix(line, "Inst ") {
			continue
		}

		// Parse the line: Inst package [current_version] (new_version source)
		fields := slices.Collect(strings.FieldsSeq(line))
		if len(fields) < 4 {
			m.logger.WithField("line", line).Debug("Skipping 'Inst' line due to insufficient fields")
			continue
		}

		packageName := fields[1]

		// Extract current version (in brackets)
		var currentVersion string
		for i, field := range fields {
			if strings.HasPrefix(field, "[") && strings.HasSuffix(field, "]") {
				currentVersion = strings.Trim(field, "[]")
				break
			} else if after, found := strings.CutPrefix(field, "["); found {
				// Multi-word version, continue until we find the closing bracket
				versionParts := []string{after}
				for j := i + 1; j < len(fields); j++ {
					if strings.HasSuffix(fields[j], "]") {
						versionParts = append(versionParts, strings.TrimSuffix(fields[j], "]"))
						break
					} else {
						versionParts = append(versionParts, fields[j])
					}
				}
				currentVersion = strings.Join(versionParts, " ")
				break
			}
		}

		// Extract available version (in parentheses)
		var availableVersion string
		for _, field := range fields {
			if after, found := strings.CutPrefix(field, "("); found {
				availableVersion = after
				break
			}
		}

		// Check if it's a security update
		isSecurityUpdate := strings.Contains(strings.ToLower(line), "security")

		if packageName != "" && currentVersion != "" && availableVersion != "" {
			packages = append(packages, models.Package{
				Name:             packageName,
				CurrentVersion:   currentVersion,
				AvailableVersion: availableVersion,
				NeedsUpdate:      true,
				IsSecurityUpdate: isSecurityUpdate,
			})
		}
	}

	return packages
}

// parseInstalledPackages parses dpkg-query output and returns a map of package name to version
func (m *APTManager) parseInstalledPackages(output string) map[string]models.Package {
	installedPackages := make(map[string]models.Package)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentPkg *models.Package

	for scanner.Scan() {
		line := scanner.Text() // Preserve whitespace for description continuation detection
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		// Check if this line is a continuation of the description (starts with space)
		if strings.HasPrefix(line, " ") && currentPkg != nil {
			// It's a description continuation
			// For now, we can append it or just skip if we only want the summary.
			// Let's append it to have full description, joining with newline
			currentPkg.Description += "\n" + trimmedLine
			installedPackages[currentPkg.Name] = *currentPkg // Update map
			continue
		}

		// New package line: Package Version Description
		// We use SplitN with 3 parts. Description is the rest.
		parts := strings.SplitN(trimmedLine, " ", 3)
		if len(parts) < 2 {
			m.logger.WithField("line", line).Debug("Skipping malformed installed package line")
			currentPkg = nil
			continue
		}

		packageName := parts[0]
		version := parts[1]
		description := ""
		if len(parts) == 3 {
			description = parts[2]
		}

		pkg := models.Package{
			Name:           packageName,
			CurrentVersion: version,
			Description:    description,
			NeedsUpdate:    false,
		}
		installedPackages[packageName] = pkg
		currentPkg = &pkg
	}

	return installedPackages
}
