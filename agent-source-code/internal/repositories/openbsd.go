package repositories

import (
	"os"
	"strings"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// OpenBSDManager handles OpenBSD repository information collection
type OpenBSDManager struct {
	logger *logrus.Logger
}

// NewOpenBSDManager creates a new OpenBSD repository manager
func NewOpenBSDManager(logger *logrus.Logger) *OpenBSDManager {
	return &OpenBSDManager{
		logger: logger,
	}
}

// GetRepositories returns the repository configured in /etc/installurl.
// OpenBSD uses a single mirror URL stored in that file; the PKG_PATH environment
// variable can override it at runtime, but we read the persistent file here.
func (m *OpenBSDManager) GetRepositories() ([]models.Repository, error) {
	data, err := os.ReadFile("/etc/installurl")
	if err != nil {
		m.logger.WithError(err).Warn("Failed to read /etc/installurl")
		return []models.Repository{}, nil
	}

	url := strings.TrimSpace(string(data))
	if url == "" || strings.HasPrefix(url, "#") {
		m.logger.Debug("No repository URL found in /etc/installurl")
		return []models.Repository{}, nil
	}

	repo := models.Repository{
		Name:      "installurl",
		URL:       url,
		RepoType:  constants.RepoTypeOpenBSD,
		IsEnabled: true,
		IsSecure:  strings.HasPrefix(strings.ToLower(url), "https://"),
	}

	m.logger.WithField("url", url).Debug("Detected OpenBSD repository from /etc/installurl")
	return []models.Repository{repo}, nil
}
