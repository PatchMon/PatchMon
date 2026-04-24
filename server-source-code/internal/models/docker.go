package models

import "time"

// DockerContainer matches docker_containers table.
// JSON tags use snake_case to match Node/Prisma API for frontend compatibility.
type DockerContainer struct {
	ID          string     `db:"id" json:"id"`
	HostID      string     `db:"host_id" json:"host_id"`
	ContainerID string     `db:"container_id" json:"container_id"`
	Name        string     `db:"name" json:"name"`
	ImageID     *string    `db:"image_id" json:"image_id"`
	ImageName   string     `db:"image_name" json:"image_name"`
	ImageTag    string     `db:"image_tag" json:"image_tag"`
	Status      string     `db:"status" json:"status"`
	State       *string    `db:"state" json:"state"`
	Ports       JSON       `db:"ports" json:"ports"`
	Labels      JSON       `db:"labels" json:"labels"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	StartedAt   *time.Time `db:"started_at" json:"started_at"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
	LastChecked time.Time  `db:"last_checked" json:"last_checked"`
}

// DockerImage matches docker_images table.
type DockerImage struct {
	ID          string     `db:"id" json:"id"`
	Repository  string     `db:"repository" json:"repository"`
	Tag         string     `db:"tag" json:"tag"`
	ImageID     string     `db:"image_id" json:"image_id"`
	Digest      *string    `db:"digest" json:"digest"`
	SizeBytes   *int64     `db:"size_bytes" json:"size_bytes"`
	Source      string     `db:"source" json:"source"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	LastPulled  *time.Time `db:"last_pulled" json:"last_pulled"`
	LastChecked time.Time  `db:"last_checked" json:"last_checked"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
}

// DockerImageUpdate matches docker_image_updates table.
type DockerImageUpdate struct {
	ID               string    `db:"id" json:"id"`
	ImageID          string    `db:"image_id" json:"image_id"`
	CurrentTag       string    `db:"current_tag" json:"current_tag"`
	AvailableTag     string    `db:"available_tag" json:"available_tag"`
	IsSecurityUpdate bool      `db:"is_security_update" json:"is_security_update"`
	Severity         *string   `db:"severity" json:"severity"`
	ChangelogURL     *string   `db:"changelog_url" json:"changelog_url"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

// DockerVolume matches docker_volumes table.
type DockerVolume struct {
	ID          string    `db:"id" json:"id"`
	HostID      string    `db:"host_id" json:"host_id"`
	VolumeID    string    `db:"volume_id" json:"volume_id"`
	Name        string    `db:"name" json:"name"`
	Driver      string    `db:"driver" json:"driver"`
	Mountpoint  *string   `db:"mountpoint" json:"mountpoint"`
	Renderer    *string   `db:"renderer" json:"renderer"`
	Scope       string    `db:"scope" json:"scope"`
	Labels      JSON      `db:"labels" json:"labels"`
	Options     JSON      `db:"options" json:"options"`
	SizeBytes   *int64    `db:"size_bytes" json:"size_bytes"`
	RefCount    int       `db:"ref_count" json:"ref_count"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
	LastChecked time.Time `db:"last_checked" json:"last_checked"`
}

// DockerNetwork matches docker_networks table.
type DockerNetwork struct {
	ID             string     `db:"id" json:"id"`
	HostID         string     `db:"host_id" json:"host_id"`
	NetworkID      string     `db:"network_id" json:"network_id"`
	Name           string     `db:"name" json:"name"`
	Driver         string     `db:"driver" json:"driver"`
	Scope          string     `db:"scope" json:"scope"`
	IPv6Enabled    bool       `db:"ipv6_enabled" json:"ipv6_enabled"`
	Internal       bool       `db:"internal" json:"internal"`
	Attachable     bool       `db:"attachable" json:"attachable"`
	Ingress        bool       `db:"ingress" json:"ingress"`
	ConfigOnly     bool       `db:"config_only" json:"config_only"`
	Labels         JSON       `db:"labels" json:"labels"`
	IPAM           JSON       `db:"ipam" json:"ipam"`
	ContainerCount int        `db:"container_count" json:"container_count"`
	CreatedAt      *time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updated_at"`
	LastChecked    time.Time  `db:"last_checked" json:"last_checked"`
}
