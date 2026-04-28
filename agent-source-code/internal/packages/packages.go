package packages

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// CacheRefreshConfig controls whether package managers refresh their cache before collecting packages.
type CacheRefreshConfig struct {
	Mode   string // "always", "if_stale", "never"
	MaxAge int    // minutes, only used when Mode == "if_stale"
}

// Manager handles package information collection
type Manager struct {
	logger         *logrus.Logger
	aptManager     *APTManager
	dnfManager     *DNFManager
	apkManager     *APKManager
	pacmanManager  *PacmanManager
	freebsdManager *FreeBSDManager
	openbsdManager *OpenBSDManager
	winManager     *WindowsManager
}

// New creates a new package manager
func New(logger *logrus.Logger, cacheRefresh CacheRefreshConfig) *Manager {
	aptManager := NewAPTManager(logger, cacheRefresh)
	dnfManager := NewDNFManager(logger)
	apkManager := NewAPKManager(logger)
	pacmanManager := NewPacmanManager(logger)
	freebsdManager := NewFreeBSDManager(logger)
	openbsdManager := NewOpenBSDManager(logger)
	winManager := NewWindowsManager(logger)

	return &Manager{
		logger:         logger,
		aptManager:     aptManager,
		dnfManager:     dnfManager,
		apkManager:     apkManager,
		pacmanManager:  pacmanManager,
		freebsdManager: freebsdManager,
		openbsdManager: openbsdManager,
		winManager:     winManager,
	}
}

// GetPackages gets package information based on detected package manager
func (m *Manager) GetPackages() ([]models.Package, error) {
	packageManager := m.DetectPackageManager()

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
	case "pkg":
		return m.freebsdManager.GetPackages()
	case "pkg_info":
		return m.openbsdManager.GetPackages()
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", packageManager)
	}
}

// DetectPackageManager detects which package manager is available on the system.
// Returns one of: apt, dnf, yum, apk, pacman, pkg, pkg_info, windows, or unknown.
func (m *Manager) DetectPackageManager() string {
	// Check for Windows first (runtime check, no exec)
	if runtime.GOOS == "windows" {
		return "windows"
	}
	// Check for OpenBSD: pkg_info is the package tool
	if runtime.GOOS == "openbsd" {
		if _, err := exec.LookPath("pkg_info"); err == nil {
			return "pkg_info"
		}
	}
	// Check for FreeBSD pkg first (avoid confusion with other 'pkg' tools).
	// When the agent runs as an rc.d service, PATH may be minimal, so also check
	// standard FreeBSD paths explicitly so package reports still work on pfSense/FreeBSD.
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

// GetPkgBinaryPath returns the path to the FreeBSD pkg binary.
// Used when running patch commands on FreeBSD (PATH may be minimal under rc.d).
func GetPkgBinaryPath() string {
	if path, err := exec.LookPath("pkg"); err == nil {
		return path
	}
	for _, p := range []string{"/usr/sbin/pkg", "/usr/local/sbin/pkg"} {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
			return p
		}
	}
	return "pkg"
}

// stringMapToPackageMap converts name->version map to name->Package for package managers
// that don't provide descriptions (pacman, freebsd pkg).
func stringMapToPackageMap(m map[string]string) map[string]models.Package {
	out := make(map[string]models.Package, len(m))
	for name, version := range m {
		out[name] = models.Package{
			Name:           name,
			CurrentVersion: version,
			NeedsUpdate:    false,
		}
	}
	return out
}

// CombinePackageData combines and deduplicates installed and upgradable package lists.
// installedPackages must contain full package info (including Description from dpkg-query).
// Descriptions and SourceRepository are preserved from installed packages for both upgradable and non-upgradable.
func CombinePackageData(installedPackages map[string]models.Package, upgradablePackages []models.Package) []models.Package {
	packages := make([]models.Package, 0)
	upgradableMap := make(map[string]bool)

	// First, add upgradable packages, merging in description and repo from installed if available
	for _, pkg := range upgradablePackages {
		if installed, ok := installedPackages[pkg.Name]; ok {
			if installed.Description != "" {
				pkg.Description = installed.Description
			}
			if pkg.SourceRepository == "" && installed.SourceRepository != "" {
				pkg.SourceRepository = installed.SourceRepository
			}
		}
		packages = append(packages, pkg)
		upgradableMap[pkg.Name] = true
	}

	// Then add installed packages that are not upgradable (with full info including description).
	// AvailableVersion is set to CurrentVersion because if the package manager did not report an
	// upgrade candidate, the installed version is already the latest available from the repository.
	for packageName, installed := range installedPackages {
		if !upgradableMap[packageName] {
			packages = append(packages, models.Package{
				Name:             packageName,
				Description:      installed.Description,
				CurrentVersion:   installed.CurrentVersion,
				AvailableVersion: installed.CurrentVersion,
				SourceRepository: installed.SourceRepository,
				NeedsUpdate:      false,
				IsSecurityUpdate: false,
			})
		}
	}

	return packages
}
