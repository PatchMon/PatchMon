package repositories

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// WindowsManager handles Windows Update source information collection
type WindowsManager struct {
	logger *logrus.Logger
}

// NewWindowsManager creates a new Windows repository manager
func NewWindowsManager(logger *logrus.Logger) *WindowsManager {
	return &WindowsManager{logger: logger}
}

// windowsUpdateSource represents a Windows Update source
type windowsUpdateSource struct {
	Name      string `json:"Name"`
	URL       string `json:"URL"`
	IsEnabled bool   `json:"IsEnabled"`
	IsManaged bool   `json:"IsManaged"`
}

// GetRepositories gets Windows Update source information
func (m *WindowsManager) GetRepositories() ([]models.Repository, error) {
	if runtime.GOOS != "windows" {
		return nil, nil // Never called on Linux
	}
	m.logger.Debug("Collecting Windows Update sources...")

	psScript := `
$ErrorActionPreference = "Stop"
$updateSession = New-Object -ComObject Microsoft.Update.Session
$updateSearcher = $updateSession.CreateUpdateSearcher()

$sources = @()
$useWUServer = (Get-ItemProperty -Path "HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate" -Name "WUServer" -ErrorAction SilentlyContinue)
if ($useWUServer) {
  $wsusServer = $useWUServer.WUServer
  $sources += @{ Name = "Windows Server Update Services (WSUS)"; URL = $wsusServer; IsEnabled = $true; IsManaged = $true }
}
$sources += @{ Name = "Microsoft Update"; URL = "https://update.microsoft.com"; IsEnabled = $true; IsManaged = $false }
$sources | ConvertTo-Json -Compress
`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to query Windows Update sources (may require admin privileges)")
		return []models.Repository{{
			Name:         "Microsoft Update",
			URL:          "https://update.microsoft.com",
			Distribution: "",
			Components:   "",
			RepoType:     constants.RepoTypeWU,
			IsEnabled:    true,
			IsSecure:     true,
		}}, nil
	}

	sourcesJSON := strings.TrimSpace(string(output))
	if sourcesJSON == "" || sourcesJSON == "[]" {
		return []models.Repository{{
			Name:         "Microsoft Update",
			URL:          "https://update.microsoft.com",
			Distribution: "",
			Components:   "",
			RepoType:     constants.RepoTypeWU,
			IsEnabled:    true,
			IsSecure:     true,
		}}, nil
	}

	var sources []windowsUpdateSource
	if err := json.Unmarshal([]byte(sourcesJSON), &sources); err != nil {
		m.logger.WithError(err).Warn("Failed to parse Windows Update sources JSON")
		return []models.Repository{{
			Name:         "Microsoft Update",
			URL:          "https://update.microsoft.com",
			Distribution: "",
			Components:   "",
			RepoType:     constants.RepoTypeWU,
			IsEnabled:    true,
			IsSecure:     true,
		}}, nil
	}

	var repositories []models.Repository
	for _, source := range sources {
		repoType := constants.RepoTypeWU
		if strings.Contains(source.Name, "WSUS") {
			repoType = constants.RepoTypeWSUS
		}
		repositories = append(repositories, models.Repository{
			Name:         source.Name,
			URL:          source.URL,
			Distribution: "",
			Components:   "",
			RepoType:     repoType,
			IsEnabled:    source.IsEnabled,
			IsSecure:     true,
		})
	}

	m.logger.WithField("count", len(repositories)).Debug("Found Windows Update sources")
	return repositories, nil
}
