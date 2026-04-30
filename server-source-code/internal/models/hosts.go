package models

import "time"

// Host matches hosts table.
type Host struct {
	ID                           string     `db:"id"`
	MachineID                    *string    `db:"machine_id"`
	FriendlyName                 string     `db:"friendly_name"`
	IP                           *string    `db:"ip"`
	OSType                       string     `db:"os_type"`
	OSVersion                    string     `db:"os_version"`
	Architecture                 *string    `db:"architecture"`
	LastUpdate                   time.Time  `db:"last_update"`
	Status                       string     `db:"status"`
	CreatedAt                    time.Time  `db:"created_at"`
	UpdatedAt                    time.Time  `db:"updated_at"`
	ApiID                        string     `db:"api_id"`
	ApiKey                       string     `db:"api_key"`
	AgentVersion                 *string    `db:"agent_version"`
	AutoUpdate                   bool       `db:"auto_update"`
	CPUCores                     *int       `db:"cpu_cores"`
	CPUModel                     *string    `db:"cpu_model"`
	DiskDetails                  JSON       `db:"disk_details"`
	DNSServers                   JSON       `db:"dns_servers"`
	GatewayIP                    *string    `db:"gateway_ip"`
	Hostname                     *string    `db:"hostname"`
	KernelVersion                *string    `db:"kernel_version"`
	InstalledKernelVersion       *string    `db:"installed_kernel_version"`
	LoadAverage                  JSON       `db:"load_average"`
	NetworkInterfaces            JSON       `db:"network_interfaces"`
	RamInstalled                 *float64   `db:"ram_installed"`
	SelinuxStatus                *string    `db:"selinux_status"`
	SwapSize                     *float64   `db:"swap_size"`
	SystemUptime                 *string    `db:"system_uptime"`
	Notes                        *string    `db:"notes"`
	NeedsReboot                  *bool      `db:"needs_reboot"`
	RebootReason                 *string    `db:"reboot_reason"`
	DockerEnabled                bool       `db:"docker_enabled"`
	ComplianceEnabled            bool       `db:"compliance_enabled"`
	ComplianceOnDemandOnly       bool       `db:"compliance_on_demand_only"`
	ComplianceOpenscapEnabled    bool       `db:"compliance_openscap_enabled"`
	ComplianceDockerBenchEnabled bool       `db:"compliance_docker_bench_enabled"`
	ComplianceScannerStatus      JSON       `db:"compliance_scanner_status"`
	ComplianceScannerUpdatedAt   *time.Time `db:"compliance_scanner_updated_at"`
	ComplianceDefaultProfileID   *string    `db:"compliance_default_profile_id"`
	HostDownAlertsEnabled        *bool      `db:"host_down_alerts_enabled"`
	ExpectedPlatform             *string    `db:"expected_platform"`
	PackageManager               *string    `db:"package_manager"`
	PrimaryInterface             *string    `db:"primary_interface"`
	AwaitingPostPatchReportRunID *string    `db:"awaiting_post_patch_report_run_id"`
	PackagesHash                 *string    `db:"packages_hash"`
	ReposHash                    *string    `db:"repos_hash"`
	InterfacesHash               *string    `db:"interfaces_hash"`
	HostnameHash                 *string    `db:"hostname_hash"`
	DockerHash                   *string    `db:"docker_hash"`
	ComplianceHash               *string    `db:"compliance_hash"`
	LastFullReportAt             *time.Time `db:"last_full_report_at"`
}

// NetworkInterface mirrors the agent's outbound NetworkInterface struct for
// canonical hashing on the server side. Stored on hosts.network_interfaces
// as opaque JSONB; this type exists so the hash-gating handler can decode +
// re-hash the agent's payload without forcing other call sites to parse it.
type NetworkInterface struct {
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	MACAddress string           `json:"macAddress,omitempty"`
	MTU        int              `json:"mtu,omitempty"`
	Status     string           `json:"status,omitempty"`
	LinkSpeed  int              `json:"linkSpeed,omitempty"`
	Duplex     string           `json:"duplex,omitempty"`
	Addresses  []NetworkAddress `json:"addresses"`
}

// NetworkAddress is one IP address bound to a NetworkInterface.
type NetworkAddress struct {
	Address string `json:"address"`
	Family  string `json:"family"`
	Netmask string `json:"netmask,omitempty"`
	Gateway string `json:"gateway,omitempty"`
}

// HostGroup matches host_groups table.
type HostGroup struct {
	ID          string    `db:"id"`
	Name        string    `db:"name"`
	Description *string   `db:"description"`
	Color       *string   `db:"color"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// HostGroupMembership matches host_group_memberships table.
type HostGroupMembership struct {
	ID          string    `db:"id"`
	HostID      string    `db:"host_id"`
	HostGroupID string    `db:"host_group_id"`
	CreatedAt   time.Time `db:"created_at"`
}

// HostPackage matches host_packages table.
type HostPackage struct {
	ID               string    `db:"id"`
	HostID           string    `db:"host_id"`
	PackageID        string    `db:"package_id"`
	CurrentVersion   string    `db:"current_version"`
	AvailableVersion *string   `db:"available_version"`
	NeedsUpdate      bool      `db:"needs_update"`
	IsSecurityUpdate bool      `db:"is_security_update"`
	LastChecked      time.Time `db:"last_checked"`
}

// HostRepository matches host_repositories table.
type HostRepository struct {
	ID           string    `db:"id"`
	HostID       string    `db:"host_id"`
	RepositoryID string    `db:"repository_id"`
	IsEnabled    bool      `db:"is_enabled"`
	LastChecked  time.Time `db:"last_checked"`
}
