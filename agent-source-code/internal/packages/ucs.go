package packages

import (
	"os"
	"os/exec"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// UCSManager handles package information collection on Univention Corporate Server.
// UCS is Debian-based; package listing delegates to APTManager.
type UCSManager struct {
	logger     *logrus.Logger
	aptManager *APTManager
}

// NewUCSManager creates a new UCS package manager.
func NewUCSManager(logger *logrus.Logger, cacheRefresh CacheRefreshConfig) *UCSManager {
	return &UCSManager{
		logger:     logger,
		aptManager: NewAPTManager(logger, cacheRefresh),
	}
}

// GetPackages returns installed packages on a UCS host via the underlying apt layer.
func (m *UCSManager) GetPackages() []models.Package {
	return m.aptManager.GetPackages()
}

// IsUCS returns true when running on Univention Corporate Server.
// The /etc/univention/ directory is the canonical marker for a UCS installation.
func IsUCS() bool {
	if info, err := os.Stat("/etc/univention"); err == nil && info.IsDir() {
		return true
	}
	if _, err := exec.LookPath("ucr"); err == nil {
		return true
	}
	return false
}
