package repositories

import (
	"patchmon-agent/internal/pkgmgr"
	"patchmon-agent/internal/textsan"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// Manager handles repository information collection
type Manager struct {
	logger         *logrus.Logger
	aptManager     *APTManager
	dnfManager     *DNFManager
	apkManager     *APKManager
	pacmanManager  *PacmanManager
	freebsdManager *FreeBSDManager
	zypperManager  *ZypperManager
	winManager     *WindowsManager
}

// New creates a new repository manager
func New(logger *logrus.Logger) *Manager {
	return &Manager{
		logger:         logger,
		aptManager:     NewAPTManager(logger),
		dnfManager:     NewDNFManager(logger),
		apkManager:     NewAPKManager(logger),
		pacmanManager:  NewPacmanManager(logger),
		freebsdManager: NewFreeBSDManager(logger),
		zypperManager:  NewZypperManager(logger),
		winManager:     NewWindowsManager(logger),
	}
}

// GetRepositories gets repository information based on detected package manager
func (m *Manager) GetRepositories() ([]models.Repository, error) {
	packageManager := m.detectPackageManager()

	m.logger.WithField("package_manager", packageManager).Debug("Detected package manager")

	var (
		repos []models.Repository
		err   error
	)
	switch packageManager {
	case "windows":
		repos, err = m.winManager.GetRepositories()
	case "apt":
		repos, err = m.aptManager.GetRepositories()
	case "dnf", "yum":
		repos = m.dnfManager.GetRepositories()
	case "apk":
		repos, err = m.apkManager.GetRepositories()
	case "pacman":
		repos, err = m.pacmanManager.GetRepositories()
	case "pkg":
		repos, err = m.freebsdManager.GetRepositories()
	case "zypper":
		repos, err = m.zypperManager.GetRepositories()
	default:
		m.logger.WithField("package_manager", packageManager).Warn("Unsupported package manager")
		return []models.Repository{}, nil
	}
	if err != nil {
		return nil, err
	}
	return textsan.CleanRepositories(repos), nil
}

func (m *Manager) detectPackageManager() string {
	return pkgmgr.Detect()
}
