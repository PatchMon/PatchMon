package models

import "time"

// IntegrationData represents data collected from an integration
type IntegrationData struct {
	Name          string      `json:"name"`
	Enabled       bool        `json:"enabled"`
	Data          interface{} `json:"data"`
	CollectedAt   time.Time   `json:"collected_at"`
	ExecutionTime float64     `json:"execution_time"` // seconds
	Error         string      `json:"error,omitempty"`
}

// DockerContainer represents a Docker container
type DockerContainer struct {
	ContainerID     string            `json:"container_id"`
	Name            string            `json:"name"`
	ImageName       string            `json:"image_name"`
	ImageTag        string            `json:"image_tag"`
	ImageRepository string            `json:"image_repository"`
	ImageSource     string            `json:"image_source"` // docker-hub, github, gitlab, private
	ImageID         string            `json:"image_id"`
	Status          string            `json:"status"` // running, exited, created, restarting, paused, dead
	State           string            `json:"state"`
	Ports           map[string]string `json:"ports,omitempty"`
	CreatedAt       *time.Time        `json:"created_at,omitempty"`
	StartedAt       *time.Time        `json:"started_at,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	NetworkMode     string            `json:"network_mode,omitempty"`
	RestartCount    int               `json:"restart_count,omitempty"`
}

// DockerImage represents a Docker image
type DockerImage struct {
	Repository string            `json:"repository"`
	Tag        string            `json:"tag"`
	ImageID    string            `json:"image_id"`
	Source     string            `json:"source"` // docker-hub, github, gitlab, private
	SizeBytes  int64             `json:"size_bytes"`
	CreatedAt  *time.Time        `json:"created_at,omitempty"`
	Digest     string            `json:"digest,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// DockerImageUpdate represents an available update for a Docker image
type DockerImageUpdate struct {
	Repository      string `json:"repository"`
	CurrentTag      string `json:"current_tag"`
	AvailableTag    string `json:"available_tag"`
	CurrentDigest   string `json:"current_digest"`
	AvailableDigest string `json:"available_digest"`
	ImageID         string `json:"image_id"`
}

// DockerVolume represents a Docker volume
type DockerVolume struct {
	VolumeID   string            `json:"volume_id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint,omitempty"`
	Renderer   string            `json:"renderer,omitempty"` // For overlay2, etc.
	Scope      string            `json:"scope"`              // local, global
	Labels     map[string]string `json:"labels,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
	CreatedAt  *time.Time        `json:"created_at,omitempty"`
	SizeBytes  *int64            `json:"size_bytes,omitempty"` // Usage size if available
	RefCount   int               `json:"ref_count,omitempty"`  // Number of containers using this volume
}

// DockerNetwork represents a Docker network
type DockerNetwork struct {
	NetworkID      string            `json:"network_id"`
	Name           string            `json:"name"`
	Driver         string            `json:"driver"` // bridge, host, overlay, macvlan, etc.
	Scope          string            `json:"scope"`  // local, swarm, global
	IPv6Enabled    bool              `json:"ipv6_enabled"`
	Internal       bool              `json:"internal"`
	Attachable     bool              `json:"attachable"`
	Ingress        bool              `json:"ingress"` // Swarm ingress network
	ConfigOnly     bool              `json:"config_only"`
	Labels         map[string]string `json:"labels,omitempty"`
	IPAM           *DockerIPAM       `json:"ipam,omitempty"` // IP Address Management config
	CreatedAt      *time.Time        `json:"created_at,omitempty"`
	ContainerCount int               `json:"container_count,omitempty"` // Number of containers attached
}

// DockerIPAM represents IP Address Management configuration
type DockerIPAM struct {
	Driver  string             `json:"driver,omitempty"`
	Config  []DockerIPAMConfig `json:"config,omitempty"`
	Options map[string]string  `json:"options,omitempty"`
}

// DockerIPAMConfig represents IPAM configuration subnet/gateway
type DockerIPAMConfig struct {
	Subnet       string            `json:"subnet,omitempty"`
	Gateway      string            `json:"gateway,omitempty"`
	IPRange      string            `json:"ip_range,omitempty"`
	AuxAddresses map[string]string `json:"aux_addresses,omitempty"`
}

// DockerData represents all Docker-related data
type DockerData struct {
	Containers []DockerContainer   `json:"containers"`
	Images     []DockerImage       `json:"images"`
	Volumes    []DockerVolume      `json:"volumes,omitempty"`
	Networks   []DockerNetwork     `json:"networks,omitempty"`
	Updates    []DockerImageUpdate `json:"updates"`
	DaemonInfo *DockerDaemonInfo   `json:"daemon_info,omitempty"`
}

// DockerDaemonInfo represents Docker daemon information
type DockerDaemonInfo struct {
	Version       string `json:"version"`
	APIVersion    string `json:"api_version"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	KernelVersion string `json:"kernel_version"`
	TotalMemory   int64  `json:"total_memory"`
	NCPU          int    `json:"ncpu"`
}

// DockerStatusEvent represents a real-time container status change
type DockerStatusEvent struct {
	Type        string    `json:"type"` // container_start, container_stop, container_die, container_pause, container_unpause
	ContainerID string    `json:"container_id"`
	Name        string    `json:"name"`
	Image       string    `json:"image"`
	Status      string    `json:"status"`
	Timestamp   time.Time `json:"timestamp"`
}

// DockerPayload represents the payload sent to the Docker endpoint
type DockerPayload struct {
	DockerData
	APIID        string `json:"-"` // Sent via header
	APIKey       string `json:"-"` // Sent via header
	Hostname     string `json:"hostname"`
	MachineID    string `json:"machine_id"`
	AgentVersion string `json:"agent_version"`
}

// DockerResponse represents the response from the Docker collection endpoint
type DockerResponse struct {
	Message            string `json:"message"`
	ContainersReceived int    `json:"containers_received"`
	ImagesReceived     int    `json:"images_received"`
	VolumesReceived    int    `json:"volumes_received"`
	NetworksReceived   int    `json:"networks_received"`
	UpdatesFound       int    `json:"updates_found"`
}
