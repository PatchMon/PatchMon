package models

// PingHashes carries the agent's per-section content hashes on a check-in.
// All fields are 64-char lowercase hex (SHA-256). Empty string == "agent has
// nothing to hash" and the server treats that the same as a mismatch (will
// ask for a full report so the column can be populated).
type PingHashes struct {
	PackagesHash   string `json:"packagesHash,omitempty"`
	ReposHash      string `json:"reposHash,omitempty"`
	InterfacesHash string `json:"interfacesHash,omitempty"`
	HostnameHash   string `json:"hostnameHash,omitempty"`
	DockerHash     string `json:"dockerHash,omitempty"`
	ComplianceHash string `json:"complianceHash,omitempty"`
}

// PingMetrics carries the volatile-but-cheap host metrics that the agent
// reports every check-in (CPU/RAM/uptime/load/reboot status). Pointers
// distinguish "agent did not collect this field" from "agent collected zero".
// Disk/load arrays are pre-marshalled JSON.
type PingMetrics struct {
	CPUCores     *int       `json:"cpuCores,omitempty"`
	CPUModel     *string    `json:"cpuModel,omitempty"`
	RAMInstalled *float64   `json:"ramInstalled,omitempty"`
	SwapSize     *float64   `json:"swapSize,omitempty"`
	DiskDetails  []DiskInfo `json:"diskDetails,omitempty"`
	SystemUptime *string    `json:"systemUptime,omitempty"`
	LoadAverage  []float64  `json:"loadAverage,omitempty"`
	NeedsReboot  *bool      `json:"needsReboot,omitempty"`
	RebootReason *string    `json:"rebootReason,omitempty"`
}

// DiskInfo mirrors the agent's DiskInfo wire shape.
type DiskInfo struct {
	Name       string `json:"name"`
	Size       string `json:"size"`
	MountPoint string `json:"mountpoint"`
}

// PingRequest is the agent's check-in body. All fields are optional — an
// empty body retains the legacy "heartbeat only" semantics so old agents
// keep working unmodified.
type PingRequest struct {
	Hashes       PingHashes  `json:"hashes,omitempty"`
	Metrics      PingMetrics `json:"metrics,omitempty"`
	AgentVersion string      `json:"agentVersion,omitempty"`
	// AgentExecutionMs is the agent-side wall-clock time in milliseconds
	// spent collecting the data shipped in this ping (mostly hashing). Used
	// by the Agent Activity UI to surface "agent took N ms" alongside the
	// server-processing duration. Optional — older agents may omit it.
	AgentExecutionMs *int `json:"agentExecutionMs,omitempty"`
}

// SectionPackages and friends are the closed set of section identifiers used
// in PingResponse.RequestFull and ReportPayload.Sections. Keep these as the
// single source of truth for valid section names so handlers and tests don't
// drift.
const (
	SectionPackages   = "packages"
	SectionRepos      = "repos"
	SectionInterfaces = "interfaces"
	SectionHostname   = "hostname"
	SectionMetrics    = "metrics"
	SectionDocker     = "docker"
	SectionCompliance = "compliance"
)

// AllSections lists every valid section identifier in the order the server
// evaluates them. Used by validation; do not reorder lightly — order is
// observable to clients in the requestFull array.
var AllSections = []string{
	SectionPackages,
	SectionRepos,
	SectionInterfaces,
	SectionHostname,
	SectionDocker,
	SectionCompliance,
}
