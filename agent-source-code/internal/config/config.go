// Package config provides configuration management functionality for the agent
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"patchmon-agent/pkg/models"

	"github.com/spf13/viper"
)

const (
	// DefaultAPIVersion is the default API version to use
	DefaultAPIVersion = "v1"
	// DefaultConfigFile is the default path to the configuration file
	DefaultConfigFile = "/etc/patchmon/config.yml"
	// DefaultCredentialsFile is the default path to the credentials file
	DefaultCredentialsFile = "/etc/patchmon/credentials.yml"
	// DefaultLogFile is the default path to the log file
	DefaultLogFile = "/etc/patchmon/logs/patchmon-agent.log"
	// DefaultLogLevel is the default logging level
	DefaultLogLevel = "info"
	// CronFilePath is the path to the cron configuration file
	CronFilePath = "/etc/cron.d/patchmon-agent"
)

// AvailableIntegrations lists all integrations that can be enabled/disabled
// Add new integrations here as they are implemented
var AvailableIntegrations = []string{
	"docker",
	"compliance",
	"ssh-proxy-enabled",
	// Future: "proxmox", "kubernetes", etc.
}

// Manager handles configuration management
type Manager struct {
	config      *models.Config
	credentials *models.Credentials
	configFile  string
}

// New creates a new configuration manager
func New() *Manager {
	return &Manager{
		config: &models.Config{
			PatchmonServer:  "", // No default server - user must provide
			APIVersion:      DefaultAPIVersion,
			CredentialsFile: DefaultCredentialsFile,
			LogFile:         DefaultLogFile,
			LogLevel:        DefaultLogLevel,
			UpdateInterval:  60, // Default to 60 minutes
			Integrations:    make(map[string]interface{}),
		},
		configFile: DefaultConfigFile,
	}
}

// SetConfigFile sets the path to the config file (called from CLI flag)
func (m *Manager) SetConfigFile(path string) {
	m.configFile = path
}

// GetConfigFile returns the path to the config file
func (m *Manager) GetConfigFile() string {
	return m.configFile
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *models.Config {
	return m.config
}

// GetCredentials returns the current credentials
func (m *Manager) GetCredentials() *models.Credentials {
	return m.credentials
}

// LoadConfig loads configuration from file
func (m *Manager) LoadConfig() error {
	// Check if config file exists
	if _, err := os.Stat(m.configFile); errors.Is(err, fs.ErrNotExist) {
		// Use defaults if config file doesn't exist
		return nil
	}

	viper.SetConfigFile(m.configFile)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	if err := viper.Unmarshal(m.config); err != nil {
		return fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Handle backward compatibility: set defaults for fields that may not exist in older configs
	// If UpdateInterval is 0 or not set, use default of 60 minutes
	if m.config.UpdateInterval <= 0 {
		m.config.UpdateInterval = 60
	}

	// If Integrations map is nil (not set in old configs), initialize it
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}

	// Ensure all available integrations are present in the map with default value
	// This ensures config.yml always shows all integrations, even if they're disabled
	for _, integrationName := range AvailableIntegrations {
		if _, exists := m.config.Integrations[integrationName]; !exists {
			if integrationName == "compliance" {
				// Default compliance to "on-demand" mode
				m.config.Integrations[integrationName] = "on-demand"
			} else {
				m.config.Integrations[integrationName] = false
			}
		}
	}

	// Validate and normalize compliance value
	if complianceVal, exists := m.config.Integrations["compliance"]; exists {
		switch v := complianceVal.(type) {
		case bool:
			// Keep bool as-is (false = disabled, true = enabled with auto-scans)
		case string:
			// Normalize string values
			switch v {
			case "on-demand", "on_demand":
				m.config.Integrations["compliance"] = "on-demand"
			case "true":
				m.config.Integrations["compliance"] = true
			case "false":
				m.config.Integrations["compliance"] = false
			}
		}
	} else {
		// Default to "on-demand" if not set
		m.config.Integrations["compliance"] = "on-demand"
	}

	// ReportOffset can be 0 - it will be recalculated if missing
	// No need to set a default here as it's calculated dynamically

	return nil
}

// LoadCredentials loads API credentials from file
func (m *Manager) LoadCredentials() error {
	if _, err := os.Stat(m.config.CredentialsFile); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("credentials file not found at %s", m.config.CredentialsFile)
	}

	viper.New()
	credViper := viper.New()
	credViper.SetConfigFile(m.config.CredentialsFile)
	credViper.SetConfigType("yaml")

	if err := credViper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading credentials file: %w", err)
	}

	m.credentials = &models.Credentials{}
	if err := credViper.Unmarshal(m.credentials); err != nil {
		return fmt.Errorf("error unmarshaling credentials: %w", err)
	}

	if m.credentials.APIID == "" || m.credentials.APIKey == "" {
		return fmt.Errorf("api_id and api_key must be configured in %s", m.config.CredentialsFile)
	}

	return nil
}

// SaveCredentials saves API credentials to file using atomic write to prevent TOCTOU race
func (m *Manager) SaveCredentials(apiID, apiKey string) error {
	if err := m.setupDirectories(); err != nil {
		return err
	}

	m.credentials = &models.Credentials{
		APIID:  apiID,
		APIKey: apiKey,
	}

	// Generate YAML content manually to avoid viper's default file creation
	content := fmt.Sprintf("api_id: %s\napi_key: %s\n", apiID, apiKey)

	// Use atomic write pattern to prevent TOCTOU race condition:
	// 1. Write to temp file with secure permissions from the start
	// 2. Atomically rename to target file
	dir := filepath.Dir(m.config.CredentialsFile)

	// Create temp file in same directory (required for atomic rename)
	// Use O_CREATE|O_EXCL to prevent race on temp file creation
	// File is created with 0600 permissions from the start
	tmpFile, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("error creating temp credentials file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on any error
	defer func() {
		if tmpFile != nil {
			if err := tmpFile.Close(); err != nil {
				// Use a logger if available, otherwise ignore in defer
				_ = err
			}
		}
		// Remove temp file if it still exists (rename failed or error occurred)
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			// Ignore "file not found" errors in cleanup
			_ = err
		}
	}()

	// Set secure permissions on temp file before writing content
	if err := tmpFile.Chmod(0600); err != nil {
		return fmt.Errorf("error setting temp file permissions: %w", err)
	}

	// Write credentials to temp file
	if _, err := tmpFile.WriteString(content); err != nil {
		return fmt.Errorf("error writing credentials to temp file: %w", err)
	}

	// Ensure data is flushed to disk before rename
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("error syncing temp file: %w", err)
	}

	// Close the file before rename (required on some systems)
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("error closing temp file: %w", err)
	}
	tmpFile = nil // Prevent double-close in defer

	// Atomic rename - this is the only operation that exposes the file
	// Since we set permissions before writing, no race window exists
	if err := os.Rename(tmpPath, m.config.CredentialsFile); err != nil {
		return fmt.Errorf("error renaming credentials file: %w", err)
	}

	return nil
}

// SaveConfig saves configuration to file
func (m *Manager) SaveConfig() error {
	if err := m.setupDirectories(); err != nil {
		return err
	}

	configViper := viper.New()
	configViper.Set("patchmon_server", m.config.PatchmonServer)
	configViper.Set("api_version", m.config.APIVersion)
	configViper.Set("credentials_file", m.config.CredentialsFile)
	configViper.Set("log_file", m.config.LogFile)
	configViper.Set("log_level", m.config.LogLevel)
	configViper.Set("skip_ssl_verify", m.config.SkipSSLVerify)
	configViper.Set("update_interval", m.config.UpdateInterval)
	configViper.Set("report_offset", m.config.ReportOffset)

	// Always save integrations map with all available integrations
	// This ensures config.yml always shows all integrations with their current state
	// Ensure all available integrations are present before saving
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}
	for _, integrationName := range AvailableIntegrations {
		if _, exists := m.config.Integrations[integrationName]; !exists {
			switch integrationName {
			case "compliance":
				m.config.Integrations[integrationName] = "on-demand"
			case "ssh-proxy-enabled":
				// Default SSH proxy to disabled (security: must be explicitly enabled)
				m.config.Integrations[integrationName] = false
			default:
				m.config.Integrations[integrationName] = false
			}
		}
	}
	// Ensure compliance has a valid value (normalize if needed)
	if complianceVal, exists := m.config.Integrations["compliance"]; exists {
		// Normalize compliance value
		switch v := complianceVal.(type) {
		case bool:
			// Keep bool as-is (false = disabled, true = enabled with auto-scans)
		case string:
			// Normalize string values
			switch v {
			case "on-demand", "on_demand":
				m.config.Integrations["compliance"] = "on-demand"
			case "true":
				m.config.Integrations["compliance"] = true
			case "false":
				m.config.Integrations["compliance"] = false
			}
		}
	} else {
		// Default to "on-demand" if not set
		m.config.Integrations["compliance"] = "on-demand"
	}

	configViper.Set("integrations", m.config.Integrations)

	if err := configViper.WriteConfigAs(m.configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

// SetUpdateInterval sets the update interval and saves it to config file
func (m *Manager) SetUpdateInterval(interval int) error {
	if interval <= 0 {
		return fmt.Errorf("invalid update interval: %d (must be > 0)", interval)
	}
	m.config.UpdateInterval = interval
	return m.SaveConfig()
}

// SetReportOffset sets the report offset (in seconds) and saves it to config file
func (m *Manager) SetReportOffset(offsetSeconds int) error {
	if offsetSeconds < 0 {
		return fmt.Errorf("invalid report offset: %d (must be >= 0)", offsetSeconds)
	}
	m.config.ReportOffset = offsetSeconds
	return m.SaveConfig()
}

// IsIntegrationEnabled checks if an integration is enabled
// Returns false if not specified (default behavior - integrations are disabled by default)
// For compliance, returns true if enabled (true) or on-demand ("on-demand"), false if disabled
func (m *Manager) IsIntegrationEnabled(name string) bool {
	if m.config.Integrations == nil {
		return false
	}
	val, exists := m.config.Integrations[name]
	if !exists {
		return false
	}

	// Special handling for compliance (can be false, "on-demand", or true)
	if name == "compliance" {
		switch v := val.(type) {
		case bool:
			return v // false = disabled, true = enabled
		case string:
			return v == "on-demand" || v == "on_demand" || v == "true"
		default:
			return false
		}
	}

	// For other integrations, expect bool
	if enabled, ok := val.(bool); ok {
		return enabled
	}
	return false
}

// SetIntegrationEnabled sets the enabled status for an integration
// For compliance, use SetComplianceMode() instead for three-state control
func (m *Manager) SetIntegrationEnabled(name string, enabled bool) error {
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}

	// Special handling for compliance - convert bool to appropriate mode
	if name == "compliance" {
		if enabled {
			m.config.Integrations[name] = true // true = enabled with auto-scans
		} else {
			m.config.Integrations[name] = false // false = disabled
		}
	} else {
		m.config.Integrations[name] = enabled
	}
	return m.SaveConfig()
}

// ComplianceMode represents the three possible states for compliance integration
type ComplianceMode string

const (
	// ComplianceDisabled indicates compliance scanning is disabled
	ComplianceDisabled ComplianceMode = "disabled" // false - compliance is disabled
	// ComplianceOnDemand indicates compliance scanning runs only when triggered
	ComplianceOnDemand ComplianceMode = "on-demand" // "on-demand" - only run when triggered
	// ComplianceEnabled indicates compliance scanning is enabled with automatic scheduled scans
	ComplianceEnabled ComplianceMode = "enabled" // true - enabled with automatic scheduled scans
)

// GetComplianceMode returns the current compliance mode
// Returns: "disabled" (false), "on-demand" ("on-demand"), or "enabled" (true)
func (m *Manager) GetComplianceMode() ComplianceMode {
	if m.config.Integrations == nil {
		return ComplianceOnDemand // Default to on-demand
	}

	val, exists := m.config.Integrations["compliance"]
	if !exists {
		return ComplianceOnDemand // Default to on-demand
	}

	switch v := val.(type) {
	case bool:
		if v {
			return ComplianceEnabled // true = enabled with auto-scans
		}
		return ComplianceDisabled // false = disabled
	case string:
		if v == "on-demand" || v == "on_demand" {
			return ComplianceOnDemand
		}
		if v == "true" {
			return ComplianceEnabled
		}
		if v == "false" {
			return ComplianceDisabled
		}
		return ComplianceOnDemand // Default for unknown strings
	default:
		return ComplianceOnDemand
	}
}

// SetComplianceMode sets the compliance integration mode
// mode can be: "disabled" (false), "on-demand" ("on-demand"), or "enabled" (true)
func (m *Manager) SetComplianceMode(mode ComplianceMode) error {
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}

	switch mode {
	case ComplianceDisabled:
		m.config.Integrations["compliance"] = false
	case ComplianceOnDemand:
		m.config.Integrations["compliance"] = "on-demand"
	case ComplianceEnabled:
		m.config.Integrations["compliance"] = true
	default:
		return fmt.Errorf("invalid compliance mode: %s (must be disabled, on-demand, or enabled)", mode)
	}

	return m.SaveConfig()
}

// IsComplianceOnDemandOnly returns true if compliance is in on-demand mode
// This is a convenience method for backward compatibility
func (m *Manager) IsComplianceOnDemandOnly() bool {
	return m.GetComplianceMode() == ComplianceOnDemand
}

// SetComplianceOnDemandOnly sets compliance to on-demand mode (for backward compatibility)
// Use SetComplianceMode() for full three-state control
func (m *Manager) SetComplianceOnDemandOnly(onDemandOnly bool) error {
	if onDemandOnly {
		return m.SetComplianceMode(ComplianceOnDemand)
	}
	// If setting to false, default to enabled (auto-scans)
	return m.SetComplianceMode(ComplianceEnabled)
}

// setupDirectories creates necessary directories
// SECURITY: Use restrictive permissions (0750) for config directories
// This prevents unauthorized users from reading agent configuration
func (m *Manager) setupDirectories() error {
	dirs := []string{
		filepath.Dir(m.configFile),
		filepath.Dir(m.config.CredentialsFile),
		filepath.Dir(m.config.LogFile),
	}

	for _, dir := range dirs {
		// Use 0750 - owner full access, group read/execute, no world access
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("error creating directory %s: %w", dir, err)
		}
	}

	return nil
}
