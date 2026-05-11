package repositories

import (
	"net/url"
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

	mirrorURL := strings.TrimSpace(string(data))
	if mirrorURL == "" || strings.HasPrefix(mirrorURL, "#") {
		m.logger.Debug("No repository URL found in /etc/installurl")
		return []models.Repository{}, nil
	}

	// Derive a human-readable name from the hostname, e.g.
	// "https://ftp.lip6.fr/pub/OpenBSD" -> "installurl-ftp.lip6.fr"
	repoName := "installurl"
	if parsed, err := url.Parse(mirrorURL); err == nil && parsed.Host != "" {
		repoName = "installurl-" + parsed.Host
	}

	repo := models.Repository{
		Name:      repoName,
		URL:       mirrorURL,
		RepoType:  constants.RepoTypeOpenBSD,
		IsEnabled: true,
		IsSecure:  strings.HasPrefix(strings.ToLower(mirrorURL), "https://"),
	}

	m.logger.WithField("url", mirrorURL).Debug("Detected OpenBSD repository from /etc/installurl")
	return []models.Repository{repo}, nil
}
