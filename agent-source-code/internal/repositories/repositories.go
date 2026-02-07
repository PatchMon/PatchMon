package repositories

import (
	"os/exec"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// Manager handles repository information collection
type Manager struct {
	logger     *logrus.Logger
	aptManager *APTManager
	dnfManager *DNFManager
	apkManager *APKManager
}

// New creates a new repository manager
func New(logger *logrus.Logger) *Manager {
	return &Manager{
		logger:     logger,
		aptManager: NewAPTManager(logger),
		dnfManager: NewDNFManager(logger),
		apkManager: NewAPKManager(logger),
	}
}

// GetRepositories gets repository information based on detected package manager
func (m *Manager) GetRepositories() ([]models.Repository, error) {
	packageManager := m.detectPackageManager()

	m.logger.WithField("package_manager", packageManager).Debug("Detected package manager")

	switch packageManager {
	case "apt":
		return m.aptManager.GetRepositories()
	case "dnf", "yum":
		repos := m.dnfManager.GetRepositories()
		return repos, nil
	case "apk":
		return m.apkManager.GetRepositories()
	default:
		m.logger.WithField("package_manager", packageManager).Warn("Unsupported package manager")
		return []models.Repository{}, nil
	}
}

// detectPackageManager detects which package manager is available on the system
func (m *Manager) detectPackageManager() string {
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

	return "unknown"
}
