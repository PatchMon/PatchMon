package packages

import (
	"fmt"
	"os/exec"
	"runtime"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// Manager handles package information collection
type Manager struct {
	logger        *logrus.Logger
	aptManager    *APTManager
	dnfManager    *DNFManager
	apkManager    *APKManager
	pacmanManager *PacmanManager
	winManager    *WindowsManager
}

// New creates a new package manager
func New(logger *logrus.Logger) *Manager {
	aptManager := NewAPTManager(logger)
	dnfManager := NewDNFManager(logger)
	apkManager := NewAPKManager(logger)
	pacmanManager := NewPacmanManager(logger)
	winManager := NewWindowsManager(logger)

	return &Manager{
		logger:        logger,
		aptManager:    aptManager,
		dnfManager:    dnfManager,
		apkManager:    apkManager,
		pacmanManager: pacmanManager,
		winManager:    winManager,
	}
}

// GetPackages gets package information based on detected package manager
func (m *Manager) GetPackages() ([]models.Package, error) {
	packageManager := m.detectPackageManager()

	m.logger.WithField("package_manager", packageManager).Debug("Detected package manager")

	switch packageManager {
	case "windows":
		return m.winManager.GetPackages(), nil
	case "apt":
		return m.aptManager.GetPackages(), nil
	case "dnf", "yum":
		return m.dnfManager.GetPackages(), nil
	case "apk":
		return m.apkManager.GetPackages(), nil
	case "pacman":
		return m.pacmanManager.GetPackages()
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", packageManager)
	}
}

// detectPackageManager detects which package manager is available on the system
func (m *Manager) detectPackageManager() string {
	// Check for Windows first (runtime check, no exec)
	if runtime.GOOS == "windows" {
		return "windows"
	}

	// Check for APK first (Alpine Linux)
	if _, err := exec.LookPath("apk"); err == nil {
		return "apk"
	}

	// Check for APT
	if _, err := exec.LookPath("apt"); err == nil {
		return "apt"
	}
	if _, err := exec.LookPath("apt-get"); err == nil {
		return "apt"
	}

	// Check for DNF/YUM
	if _, err := exec.LookPath("dnf"); err == nil {
		return "dnf"
	}
	if _, err := exec.LookPath("yum"); err == nil {
		return "yum"
	}

	// Check for Pacman
	if _, err := exec.LookPath("pacman"); err == nil {
		return "pacman"
	}

	return "unknown"
}

// CombinePackageData combines and deduplicates installed and upgradable package lists
func CombinePackageData(installedPackages map[string]string, upgradablePackages []models.Package) []models.Package {
	packages := make([]models.Package, 0)
	upgradableMap := make(map[string]bool)

	// First, add all upgradable packages
	for _, pkg := range upgradablePackages {
		packages = append(packages, pkg)
		upgradableMap[pkg.Name] = true
	}

	// Then add installed packages that are not upgradable
	for packageName, version := range installedPackages {
		if !upgradableMap[packageName] {
			packages = append(packages, models.Package{
				Name:             packageName,
				CurrentVersion:   version,
				NeedsUpdate:      false,
				IsSecurityUpdate: false,
			})
		}
	}

	return packages
}
