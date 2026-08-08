// Package config provides configuration management functionality for the agent
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"patchmon-agent/pkg/models"

	"github.com/spf13/viper"
)

const (
	// DefaultAPIVersion is the default API version to use
	DefaultAPIVersion = "v1"
	// DefaultConfigFile is the default path to the configuration file (Unix)
	DefaultConfigFile = "/etc/patchmon/config.yml"
	// DefaultCredentialsFile is the default path to the credentials file (Unix)
	DefaultCredentialsFile = "/etc/patchmon/credentials.yml"
	// DefaultLogFile is the default path to the log file (Unix)
	DefaultLogFile = "/etc/patchmon/logs/patchmon-agent.log"
	// DefaultLogLevel is the default logging level
	DefaultLogLevel = "info"
	// CronFilePath is the path to the cron configuration file (Unix only)
	CronFilePath = "/etc/cron.d/patchmon-agent"
)

// Windows default paths
const (
	DefaultConfigFileWindows      = "C:\\ProgramData\\PatchMon\\config.yml"
	DefaultCredentialsFileWindows = "C:\\ProgramData\\PatchMon\\credentials.yml"
	DefaultLogFileWindows         = "C:\\ProgramData\\PatchMon\\patchmon-agent.log"
)

// getDefaultPaths returns config, credentials, and log file paths based on OS
func getDefaultPaths() (configFile, credentialsFile, logFile string) {
	if runtime.GOOS == "windows" {
		return DefaultConfigFileWindows, DefaultCredentialsFileWindows, DefaultLogFileWindows
	}
	return DefaultConfigFile, DefaultCredentialsFile, DefaultLogFile
}

// DefaultConfigFilePath returns the default config file path for the current OS
func DefaultConfigFilePath() string {
	cfg, _, _ := getDefaultPaths()
	return cfg
}

// DefaultLogFilePath returns the default log file path for the current OS
func DefaultLogFilePath() string {
	_, _, log := getDefaultPaths()
	return log
}

// AvailableIntegrations lists all integrations that can be enabled/disabled
// Add new integrations here as they are implemented
var AvailableIntegrations = []string{
	"docker",
	"compliance",
	"ssh-proxy-enabled",
	"rdp-proxy-enabled",
	// Future: "proxmox", "kubernetes", etc.
}

// Manager handles configuration management
type Manager struct {
	// Guards config, credentials and configFile. LoadConfig replaces
	// config.Integrations while the service loop and schedulers read it, and a
	// concurrent map read/write is an unrecoverable runtime fatal.
	mu          sync.RWMutex
	config      *models.Config
	credentials *models.Credentials
	configFile  string
}

// New creates a new configuration manager
func New() *Manager {
	configFile, credentialsFile, logFile := getDefaultPaths()
	return &Manager{
		config: &models.Config{
			PatchmonServer:            "", // No default server - user must provide
			APIVersion:                DefaultAPIVersion,
			CredentialsFile:           credentialsFile,
			LogFile:                   logFile,
			LogLevel:                  DefaultLogLevel,
			UpdateInterval:            60,       // Default to 60 minutes
			PackageCacheRefreshMode:   "always", // Default to always refresh package cache
			PackageCacheRefreshMaxAge: 60,       // Default max age in minutes (used when mode is if_stale)
			Integrations:              make(map[string]interface{}),
		},
		configFile: configFile,
	}
}

// SetConfigFile sets the path to the config file (called from CLI flag)
func (m *Manager) SetConfigFile(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configFile = path
}

// GetConfigFile returns the path to the config file
func (m *Manager) GetConfigFile() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configFile
}

// GetConfig returns the current configuration
// The returned pointer is a snapshot: LoadConfig swaps in a fresh struct rather
// than mutating this one.
func (m *Manager) GetConfig() *models.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// GetCredentials returns the current credentials
func (m *Manager) GetCredentials() *models.Credentials {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.credentials
}

// LoadConfig loads configuration from file
func (m *Manager) LoadConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if config file exists
	if _, err := os.Stat(m.configFile); errors.Is(err, fs.ErrNotExist) {
		// Use defaults if config file doesn't exist
		return nil
	}

	// Manager-scoped, not the package singleton: a second Manager exists in
	// getLatestBinaryFromServer and would race on the global viper's maps.
	v := viper.New()
	v.SetConfigFile(m.configFile)
	v.SetConfigType("yaml")

	// A concurrent write of this same file locks the open out on Windows. The
	// file is intact either side of that window, so the read is retried rather
	// than failing the load.
	if err := retryTransientFile(v.ReadInConfig); err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	// Swapped in rather than mutated, so readers holding a GetConfig() pointer
	// keep a stable snapshot. Integrations cleared so the file is authoritative.
	loaded := *m.config
	loaded.Integrations = nil
	if err := v.Unmarshal(&loaded); err != nil {
		return fmt.Errorf("error unmarshaling config: %w", err)
	}
	m.config = &loaded

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

	// Ensure compliance is a nested object for YAML output
	m.ensureComplianceNestedLocked()

	// Persist normalized config so new defaults (e.g. scan_interval) appear on disk
	if err := m.saveConfigLocked(); err != nil {
		// Non-fatal: config is correct in memory even if save fails
		_ = err
	}

	// ReportOffset can be 0 - it will be recalculated if missing
	// No need to set a default here as it's calculated dynamically

	return nil
}

// ensureComplianceNested ensures integrations.compliance is a nested map with enabled, openscap_enabled, docker_bench_enabled.
// Migrates flat keys into the nested structure for cleaner YAML output.
func (m *Manager) ensureComplianceNestedLocked() {
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}
	var nested map[string]interface{}
	if v, ok := m.config.Integrations["compliance"]; ok {
		if nm, ok := v.(map[string]interface{}); ok {
			nested = nm
		}
	}
	if nested == nil {
		nested = make(map[string]interface{})
	}
	if _, hasEnabled := nested["enabled"]; !hasEnabled {
		if v, ok := m.config.Integrations["compliance"]; ok {
			switch val := v.(type) {
			case bool:
				nested["enabled"] = val
			case string:
				if val == "disabled" || val == "false" {
					nested["enabled"] = false
				} else {
					nested["enabled"] = val
				}
			default:
				nested["enabled"] = "on-demand"
			}
		} else {
			nested["enabled"] = "on-demand"
		}
	}
	if _, has := nested["openscap_enabled"]; !has {
		if v, ok := m.config.Integrations["compliance_openscap_enabled"]; ok {
			if b, ok := v.(bool); ok {
				nested["openscap_enabled"] = b
			} else {
				nested["openscap_enabled"] = true
			}
		} else {
			nested["openscap_enabled"] = true
		}
	}
	if _, has := nested["docker_bench_enabled"]; !has {
		if v, ok := m.config.Integrations["compliance_docker_bench_enabled"]; ok {
			if b, ok := v.(bool); ok {
				nested["docker_bench_enabled"] = b
			} else {
				nested["docker_bench_enabled"] = false
			}
		} else {
			nested["docker_bench_enabled"] = false
		}
	}
	if _, has := nested["scan_interval"]; !has {
		nested["scan_interval"] = 1440
	}
	m.config.Integrations["compliance"] = nested
	delete(m.config.Integrations, "compliance_openscap_enabled")
	delete(m.config.Integrations, "compliance_docker_bench_enabled")
}

// LoadCredentials loads API credentials from file
func (m *Manager) LoadCredentials() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := os.Stat(m.config.CredentialsFile); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("credentials file not found at %s", m.config.CredentialsFile)
	}

	viper.New()
	credViper := viper.New()
	credViper.SetConfigFile(m.config.CredentialsFile)
	credViper.SetConfigType("yaml")

	if err := retryTransientFile(credViper.ReadInConfig); err != nil {
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
	m.mu.Lock()
	defer m.mu.Unlock()
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
	if err := retryTransientFile(func() error {
		return os.Rename(tmpPath, m.config.CredentialsFile)
	}); err != nil {
		return fmt.Errorf("error renaming credentials file: %w", err)
	}

	return nil
}

// SaveConfig saves configuration to file
func (m *Manager) SaveConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveConfigLocked()
}

// Caller must hold the write lock.
func (m *Manager) saveConfigLocked() error {
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
	configViper.Set("package_cache_refresh_mode", m.config.PackageCacheRefreshMode)
	configViper.Set("package_cache_refresh_max_age", m.config.PackageCacheRefreshMaxAge)

	// Always save integrations map with all available integrations
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}
	m.ensureComplianceNestedLocked()
	for _, integrationName := range AvailableIntegrations {
		if _, exists := m.config.Integrations[integrationName]; !exists {
			switch integrationName {
			case "compliance":
				m.config.Integrations[integrationName] = map[string]interface{}{
					"enabled": "on-demand", "openscap_enabled": true, "docker_bench_enabled": false,
				}
			case "ssh-proxy-enabled":
				m.config.Integrations[integrationName] = false
			case "rdp-proxy-enabled":
				m.config.Integrations[integrationName] = false
			default:
				m.config.Integrations[integrationName] = false
			}
		}
	}

	configViper.Set("integrations", m.config.Integrations)

	return writeConfigAtomically(configViper, m.configFile)
}

// writeConfigAtomically stages the config to a sibling temp file and renames it
// over the target.
//
// Viper writes in place, truncating before it writes, so anyone reading the
// file during that window sees a partial YAML document. LoadConfig ends by
// calling saveConfigLocked, and each Manager holds its own lock, so two
// Managers over the same path race: sendIntegrationData and
// runScheduledComplianceScan both reload config on their own goroutines, and
// getLatestBinaryFromServer constructs a second Manager. The observable
// failure is a "could not find expected ':'" unmarshal error on a file that is
// perfectly well-formed by the time you look at it.
//
// Rename is atomic on POSIX, so a reader sees either the whole old config or
// the whole new one. The fsync before it means a crash mid-write cannot leave
// the live config truncated either, which would stop the agent starting.
func writeConfigAtomically(v *viper.Viper, path string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.yml")
	if err != nil {
		return fmt.Errorf("error creating temp config file: %w", err)
	}
	tmpName := tmp.Name()
	// Removing after a successful rename is a no-op; this only matters on the
	// error paths below.
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("error closing temp config file: %w", err)
	}

	// The temp file becomes the config file, so it has to carry the config
	// file's mode rather than CreateTemp's. config.yml is installed 0600.
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("error setting temp config permissions: %w", err)
	}

	if err := v.WriteConfigAs(tmpName); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	f, err := os.OpenFile(tmpName, os.O_RDWR, mode)
	if err != nil {
		return fmt.Errorf("error reopening temp config file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("error syncing temp config file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing temp config file: %w", err)
	}

	// On Windows a reader holding the destination open makes this fail with
	// ERROR_ACCESS_DENIED. The rename leaves the destination untouched when it
	// fails, so retrying is safe.
	if err := retryTransientFile(func() error { return os.Rename(tmpName, path) }); err != nil {
		return fmt.Errorf("error replacing config file: %w", err)
	}

	return nil
}

// SetUpdateInterval sets the update interval and saves it to config file
func (m *Manager) SetUpdateInterval(interval int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if interval <= 0 {
		return fmt.Errorf("invalid update interval: %d (must be > 0)", interval)
	}
	m.config.UpdateInterval = interval
	return m.saveConfigLocked()
}

// SetPatchmonServer sets the server URL and saves it.
func (m *Manager) SetPatchmonServer(serverURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.PatchmonServer = serverURL
	return m.saveConfigLocked()
}

// SetReportOffset sets the report offset (in seconds) and saves it to config file
func (m *Manager) SetReportOffset(offsetSeconds int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if offsetSeconds < 0 {
		return fmt.Errorf("invalid report offset: %d (must be >= 0)", offsetSeconds)
	}
	m.config.ReportOffset = offsetSeconds
	return m.saveConfigLocked()
}

// SetPackageCacheRefresh sets the package cache refresh mode and max age, and saves to config file
func (m *Manager) SetPackageCacheRefresh(mode string, maxAge int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mode != "always" && mode != "if_stale" && mode != "never" {
		return fmt.Errorf("invalid package cache refresh mode: %s", mode)
	}
	if maxAge < 1 || maxAge > 1440 {
		return fmt.Errorf("invalid package cache refresh max age: %d (must be 1-1440)", maxAge)
	}
	m.config.PackageCacheRefreshMode = mode
	m.config.PackageCacheRefreshMaxAge = maxAge
	return m.saveConfigLocked()
}

// GetPackageCacheRefreshMode returns the package cache refresh mode, defaulting to "always"
func (m *Manager) GetPackageCacheRefreshMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.PackageCacheRefreshMode == "" {
		return "always"
	}
	return m.config.PackageCacheRefreshMode
}

// GetPackageCacheRefreshMaxAge returns the max age in minutes for stale cache checks, defaulting to 60
func (m *Manager) GetPackageCacheRefreshMaxAge() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.PackageCacheRefreshMaxAge <= 0 {
		return 60
	}
	return m.config.PackageCacheRefreshMaxAge
}

// IsIntegrationEnabled checks if an integration is enabled
// Returns false if not specified (default behavior - integrations are disabled by default)
// For compliance, returns true if enabled (true) or on-demand ("on-demand"), false if disabled
func (m *Manager) IsIntegrationEnabled(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.Integrations == nil {
		return false
	}
	val, exists := m.config.Integrations[name]
	if !exists {
		return false
	}

	// Special handling for compliance (can be false, "on-demand", or true; may be nested)
	if name == "compliance" {
		enabledVal := m.getComplianceValLocked("enabled")
		if enabledVal == nil {
			return false
		}
		switch v := enabledVal.(type) {
		case bool:
			return v
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}
	if name == "compliance" {
		m.ensureComplianceNestedLocked()
		nested := m.config.Integrations["compliance"].(map[string]interface{})
		if enabled {
			nested["enabled"] = true
		} else {
			nested["enabled"] = false
		}
	} else {
		m.config.Integrations[name] = enabled
	}
	return m.saveConfigLocked()
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getComplianceModeLocked()
}

// Caller must hold at least a read lock.
func (m *Manager) getComplianceModeLocked() ComplianceMode {
	if m.config.Integrations == nil {
		return ComplianceOnDemand
	}
	val := m.getComplianceValLocked("enabled")
	if val == nil {
		return ComplianceOnDemand
	}
	switch v := val.(type) {
	case bool:
		if v {
			return ComplianceEnabled
		}
		return ComplianceDisabled
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
		return ComplianceOnDemand
	default:
		return ComplianceOnDemand
	}
}

// getComplianceVal returns a value from the compliance nested map, or from flat keys for backward compat.
func (m *Manager) getComplianceValLocked(key string) interface{} {
	if v, ok := m.config.Integrations["compliance"]; ok {
		if nm, ok := v.(map[string]interface{}); ok {
			if val, exists := nm[key]; exists {
				return val
			}
		}
	}
	// Flat key fallback
	switch key {
	case "enabled":
		return m.config.Integrations["compliance"]
	case "openscap_enabled":
		return m.config.Integrations["compliance_openscap_enabled"]
	case "docker_bench_enabled":
		return m.config.Integrations["compliance_docker_bench_enabled"]
	}
	return nil
}

// SetComplianceMode sets the compliance integration mode
// mode can be: "disabled" (false), "on-demand" ("on-demand"), or "enabled" (true)
func (m *Manager) SetComplianceMode(mode ComplianceMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setComplianceModeLocked(mode)
}

// Caller must hold the write lock.
func (m *Manager) setComplianceModeLocked(mode ComplianceMode) error {
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}
	m.ensureComplianceNestedLocked()
	nested := m.config.Integrations["compliance"].(map[string]interface{})
	switch mode {
	case ComplianceDisabled:
		nested["enabled"] = false
	case ComplianceOnDemand:
		nested["enabled"] = "on-demand"
	case ComplianceEnabled:
		nested["enabled"] = true
	default:
		return fmt.Errorf("invalid compliance mode: %s (must be disabled, on-demand, or enabled)", mode)
	}
	return m.saveConfigLocked()
}

// IsComplianceOnDemandOnly returns true if compliance is in on-demand mode
// This is a convenience method for backward compatibility
func (m *Manager) IsComplianceOnDemandOnly() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getComplianceModeLocked() == ComplianceOnDemand
}

// SetComplianceOnDemandOnly sets compliance to on-demand mode (for backward compatibility)
// Use SetComplianceMode() for full three-state control
func (m *Manager) SetComplianceOnDemandOnly(onDemandOnly bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if onDemandOnly {
		return m.setComplianceModeLocked(ComplianceOnDemand)
	}
	// If setting to false, default to enabled (auto-scans)
	return m.setComplianceModeLocked(ComplianceEnabled)
}

// GetComplianceOpenscapEnabled returns whether OpenSCAP scans are enabled for scheduled compliance scans.
func (m *Manager) GetComplianceOpenscapEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.Integrations == nil {
		return true
	}
	val := m.getComplianceValLocked("openscap_enabled")
	if val == nil {
		return true
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return true
}

// GetComplianceDockerBenchEnabled returns whether Docker Bench scans are enabled for scheduled compliance scans.
func (m *Manager) GetComplianceDockerBenchEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.Integrations == nil {
		return false
	}
	val := m.getComplianceValLocked("docker_bench_enabled")
	if val == nil {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// SetComplianceScanners sets the OpenSCAP and Docker Bench scanner toggles for scheduled scans.
func (m *Manager) SetComplianceScanners(openscapEnabled, dockerBenchEnabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}
	m.ensureComplianceNestedLocked()
	nested := m.config.Integrations["compliance"].(map[string]interface{})
	nested["openscap_enabled"] = openscapEnabled
	nested["docker_bench_enabled"] = dockerBenchEnabled
	return m.saveConfigLocked()
}

// GetComplianceScanInterval returns the compliance scan interval in minutes (default 1440, min 60, max 10080).
func (m *Manager) GetComplianceScanInterval() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.Integrations == nil {
		return 1440
	}
	val := m.getComplianceValLocked("scan_interval")
	if val == nil {
		return 1440
	}
	var minutes int
	switch v := val.(type) {
	case int:
		minutes = v
	case float64:
		minutes = int(v)
	default:
		return 1440
	}
	if minutes < 60 {
		minutes = 60
	}
	if minutes > 10080 {
		minutes = 10080
	}
	return minutes
}

// SetComplianceScanInterval sets the compliance scan interval and saves it to config file.
func (m *Manager) SetComplianceScanInterval(minutes int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if minutes < 60 || minutes > 10080 {
		return fmt.Errorf("invalid compliance scan interval: %d (must be between 60 and 10080 minutes)", minutes)
	}
	if m.config.Integrations == nil {
		m.config.Integrations = make(map[string]interface{})
	}
	m.ensureComplianceNestedLocked()
	nested := m.config.Integrations["compliance"].(map[string]interface{})
	nested["scan_interval"] = minutes
	return m.saveConfigLocked()
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
