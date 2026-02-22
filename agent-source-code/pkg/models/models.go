package models

// Package represents a software package
type Package struct {
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	CurrentVersion   string `json:"currentVersion"`
	AvailableVersion string `json:"availableVersion,omitempty"`
	NeedsUpdate      bool   `json:"needsUpdate"`
	IsSecurityUpdate bool   `json:"isSecurityUpdate"`
}

// Repository represents a software repository
type Repository struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Distribution string `json:"distribution"`
	Components   string `json:"components"`
	RepoType     string `json:"repoType"`
	IsEnabled    bool   `json:"isEnabled"`
	IsSecure     bool   `json:"isSecure"`
}

// SystemInfo represents system information
type SystemInfo struct {
	KernelVersion string    `json:"kernelVersion"`
	SELinuxStatus string    `json:"selinuxStatus"`
	SystemUptime  string    `json:"systemUptime"`
	LoadAverage   []float64 `json:"loadAverage"`
}

// HardwareInfo represents hardware information
type HardwareInfo struct {
	CPUModel     string     `json:"cpuModel"`
	CPUCores     int        `json:"cpuCores"`
	RAMInstalled float64    `json:"ramInstalled"` // GB
	SwapSize     float64    `json:"swapSize"`     // GB
	DiskDetails  []DiskInfo `json:"diskDetails"`
}

// DiskInfo represents disk information
type DiskInfo struct {
	Name       string `json:"name"`
	Size       string `json:"size"`
	MountPoint string `json:"mountpoint"`
}

// NetworkInfo represents network information
type NetworkInfo struct {
	GatewayIP         string             `json:"gatewayIp"`
	DNSServers        []string           `json:"dnsServers"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces"`
}

// NetworkInterface represents a network interface
type NetworkInterface struct {
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	MACAddress string           `json:"macAddress,omitempty"`
	MTU        int              `json:"mtu,omitempty"`
	Status     string           `json:"status,omitempty"`    // "up" or "down"
	LinkSpeed  int              `json:"linkSpeed,omitempty"` // Speed in Mbps, -1 if unknown
	Duplex     string           `json:"duplex,omitempty"`    // "full", "half", or ""
	Addresses  []NetworkAddress `json:"addresses"`
}

// NetworkAddress represents an IP address
type NetworkAddress struct {
	Address string `json:"address"`
	Family  string `json:"family"`            // "inet" or "inet6"
	Netmask string `json:"netmask,omitempty"` // CIDR notation (e.g., "/24" or "/64")
	Gateway string `json:"gateway,omitempty"` // Gateway for this specific address/interface
}

// ReportPayload represents the data sent to the server
type ReportPayload struct {
	Packages               []Package          `json:"packages"`
	Repositories           []Repository       `json:"repositories"`
	OSType                 string             `json:"osType"`
	OSVersion              string             `json:"osVersion"`
	Hostname               string             `json:"hostname"`
	IP                     string             `json:"ip"`
	Architecture           string             `json:"architecture"`
	AgentVersion           string             `json:"agentVersion"`
	MachineID              string             `json:"machineId"`
	KernelVersion          string             `json:"kernelVersion"`
	InstalledKernelVersion string             `json:"installedKernelVersion,omitempty"`
	SELinuxStatus          string             `json:"selinuxStatus"`
	SystemUptime           string             `json:"systemUptime"`
	LoadAverage            []float64          `json:"loadAverage"`
	CPUModel               string             `json:"cpuModel"`
	CPUCores               int                `json:"cpuCores"`
	RAMInstalled           float64            `json:"ramInstalled"`
	SwapSize               float64            `json:"swapSize"`
	DiskDetails            []DiskInfo         `json:"diskDetails"`
	GatewayIP              string             `json:"gatewayIp"`
	DNSServers             []string           `json:"dnsServers"`
	NetworkInterfaces      []NetworkInterface `json:"networkInterfaces"`
	ExecutionTime          float64            `json:"executionTime"` // Collection time in seconds
	NeedsReboot            bool               `json:"needsReboot"`
	RebootReason           string             `json:"rebootReason,omitempty"`
}

// PingResponse represents server ping response
type PingResponse struct {
	Message       string             `json:"message"`
	Timestamp     string             `json:"timestamp"`
	FriendlyName  string             `json:"friendlyName"`
	AgentStartup  bool               `json:"agentStartup,omitempty"`
	Integrations  map[string]bool    `json:"integrations,omitempty"` // Server-side integration enable states
	CrontabUpdate *CrontabUpdateInfo `json:"crontabUpdate,omitempty"`
}

// UpdateResponse represents server update response
type UpdateResponse struct {
	Message           string             `json:"message"`
	PackagesProcessed int                `json:"packagesProcessed"`
	UpdatesAvailable  int                `json:"updatesAvailable,omitempty"`
	SecurityUpdates   int                `json:"securityUpdates,omitempty"`
	AutoUpdate        *AutoUpdateInfo    `json:"autoUpdate,omitempty"`
	CrontabUpdate     *CrontabUpdateInfo `json:"crontabUpdate,omitempty"`
}

// AutoUpdateInfo represents agent auto-update information
type AutoUpdateInfo struct {
	ShouldUpdate   bool   `json:"shouldUpdate"`
	LatestVersion  string `json:"latestVersion"`
	CurrentVersion string `json:"currentVersion"`
	Message        string `json:"message"`
}

// CrontabUpdateInfo represents crontab update information
type CrontabUpdateInfo struct {
	ShouldUpdate bool   `json:"shouldUpdate"`
	Message      string `json:"message"`
	Command      string `json:"command"`
}

// VersionResponse represents version check response
type VersionResponse struct {
	CurrentVersion string `json:"currentVersion"`
	DownloadURL    string `json:"downloadUrl"`
	ReleaseNotes   string `json:"releaseNotes"`
}

// UpdateIntervalResponse represents update interval response
type UpdateIntervalResponse struct {
	UpdateInterval int `json:"updateInterval"`
}

// AgentTimestampResponse represents agent timestamp response
type AgentTimestampResponse struct {
	Version   string `json:"version"`
	Timestamp int64  `json:"timestamp"`
	Exists    bool   `json:"exists"`
}

// HostSettingsResponse represents host settings response
type HostSettingsResponse struct {
	AutoUpdate     bool `json:"auto_update"`
	HostAutoUpdate bool `json:"host_auto_update"`
}

// IntegrationStatusResponse represents integration status response from server
type IntegrationStatusResponse struct {
	Success      bool            `json:"success"`
	Integrations map[string]bool `json:"integrations"`
}

// InstallEvent represents a single notable event during scanner installation
type InstallEvent struct {
	Step      string `json:"step"`
	Status    string `json:"status"` // "in_progress", "done", "failed", "skipped"
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// IntegrationSetupStatus represents the setup status of an integration
type IntegrationSetupStatus struct {
	Integration   string                    `json:"integration"`
	Enabled       bool                      `json:"enabled"`
	Status        string                    `json:"status"` // "ready", "installing", "removing", "error"
	Message       string                    `json:"message"`
	Components    map[string]string         `json:"components,omitempty"` // Component name -> status
	ScannerInfo   *ComplianceScannerDetails `json:"scanner_info,omitempty"`
	InstallEvents []InstallEvent            `json:"install_events,omitempty"`
}

// ComplianceScannerDetails contains detailed OpenSCAP scanner information
type ComplianceScannerDetails struct {
	// OpenSCAP info
	OpenSCAPVersion   string `json:"openscap_version,omitempty"`
	OpenSCAPAvailable bool   `json:"openscap_available"`

	// SCAP Content info
	ContentFile       string `json:"content_file,omitempty"`
	ContentPackage    string `json:"content_package,omitempty"`     // e.g., "ssg-base 0.1.76"
	SSGVersion        string `json:"ssg_version,omitempty"`         // Just the version number (e.g., "0.1.76")
	SSGMinVersion     string `json:"ssg_min_version,omitempty"`     // Minimum required version for this OS
	SSGNeedsUpgrade   bool   `json:"ssg_needs_upgrade,omitempty"`   // True if upgrade is recommended
	SSGUpgradeMessage string `json:"ssg_upgrade_message,omitempty"` // Message explaining why upgrade is needed

	// Available scan profiles
	AvailableProfiles []ScanProfileInfo `json:"available_profiles,omitempty"`

	// Docker Bench info
	DockerBenchAvailable bool   `json:"docker_bench_available"`
	DockerBenchVersion   string `json:"docker_bench_version,omitempty"`

	// oscap-docker info (for Docker image CVE scanning)
	OscapDockerAvailable bool `json:"oscap_docker_available"`

	// OS info for content matching
	OSName    string `json:"os_name,omitempty"`
	OSVersion string `json:"os_version,omitempty"`
	OSFamily  string `json:"os_family,omitempty"`

	// Content compatibility
	ContentMismatch bool   `json:"content_mismatch,omitempty"`
	MismatchWarning string `json:"mismatch_warning,omitempty"`
}

// ScanProfileInfo describes an available scan profile
type ScanProfileInfo struct {
	ID          string `json:"id"`                    // Internal ID (e.g., "level1_server") or full XCCDF ID
	Name        string `json:"name"`                  // Display name (e.g., "CIS Level 1 Server")
	Description string `json:"description,omitempty"` // Brief description
	Type        string `json:"type"`                  // "openscap" or "docker-bench"
	XCCDFId     string `json:"xccdf_id,omitempty"`    // Full XCCDF profile ID
	Category    string `json:"category,omitempty"`    // Category: "cis", "stig", "pci-dss", "hipaa", etc.
}

// ComplianceScanOptions represents configurable scan options
type ComplianceScanOptions struct {
	ProfileID            string `json:"profile_id"`
	RuleID               string `json:"rule_id,omitempty"`
	EnableRemediation    bool   `json:"enable_remediation,omitempty"`
	RemediationType      string `json:"remediation_type,omitempty"`
	FetchRemoteResources bool   `json:"fetch_remote_resources,omitempty"`
	TailoringFile        string `json:"tailoring_file,omitempty"`
	OutputFormat         string `json:"output_format,omitempty"`
	Timeout              int    `json:"timeout,omitempty"`
	OpenSCAPEnabled      *bool  `json:"openscap_enabled,omitempty"`     // Per-host toggle: run OpenSCAP scans
	DockerBenchEnabled   *bool  `json:"docker_bench_enabled,omitempty"` // Per-host toggle: run Docker Bench scans
}

// Credentials holds API authentication information
type Credentials struct {
	APIID  string `yaml:"api_id" mapstructure:"api_id"`
	APIKey string `yaml:"api_key" mapstructure:"api_key"`
}

// Config represents agent configuration
type Config struct {
	PatchmonServer  string                 `yaml:"patchmon_server" mapstructure:"patchmon_server"`
	APIVersion      string                 `yaml:"api_version" mapstructure:"api_version"`
	CredentialsFile string                 `yaml:"credentials_file" mapstructure:"credentials_file"`
	LogFile         string                 `yaml:"log_file" mapstructure:"log_file"`
	LogLevel        string                 `yaml:"log_level" mapstructure:"log_level"`
	SkipSSLVerify   bool                   `yaml:"skip_ssl_verify" mapstructure:"skip_ssl_verify"`
	UpdateInterval  int                    `yaml:"update_interval" mapstructure:"update_interval"` // Interval in minutes
	ReportOffset    int                    `yaml:"report_offset" mapstructure:"report_offset"`     // Offset in seconds
	Integrations    map[string]interface{} `yaml:"integrations" mapstructure:"integrations"`       // Supports bool for simple integrations, string for compliance mode
}
