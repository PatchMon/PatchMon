package repositories

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// Manager handles repository information collection
type Manager struct {
	logger          *logrus.Logger
	aptManager      *APTManager
	dnfManager      *DNFManager
	apkManager      *APKManager
	pacmanManager   *PacmanManager
	freebsdManager  *FreeBSDManager
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
	case "pacman":
		return m.pacmanManager.GetRepositories()
	case "pkg":
		return m.freebsdManager.GetRepositories()
	default:
		m.logger.WithField("package_manager", packageManager).Warn("Unsupported package manager")
		return []models.Repository{}, nil
	}
}

// detectPackageManager detects which package manager is available on the system
func (m *Manager) detectPackageManager() string {
	// Check for FreeBSD pkg first. When the agent runs as rc.d service, PATH may be minimal.
	if runtime.GOOS == "freebsd" {
		for _, pkgPath := range []string{"/usr/sbin/pkg", "/usr/local/sbin/pkg"} {
			if info, err := os.Stat(pkgPath); err == nil && info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
				return "pkg"
			}
		}
	}
	if _, err := exec.LookPath("pkg"); err == nil {
		if output, err := exec.Command("uname", "-s").Output(); err == nil {
			if strings.TrimSpace(string(output)) == "FreeBSD" {
				return "pkg"
			}
		}
	}

	// Check for APK (Alpine Linux)
	if _, err := exec.LookPath("apk"); err == nil {
		return "apk"
	}

	// Check for Pacman (Arch Linux and derivatives)
	if _, err := exec.LookPath("pacman"); err == nil {
		return "pacman"
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
