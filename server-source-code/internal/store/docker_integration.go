package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// DockerReceivePayload holds the agent's Docker data for storage.
type DockerReceivePayload struct {
	Containers []DockerReceiveContainer
	Images     []DockerReceiveImage
	Volumes    []DockerReceiveVolume
	Networks   []DockerReceiveNetwork
	Updates    []DockerReceiveImageUpdate
}

type DockerReceiveContainer struct {
	ContainerID     string
	Name            string
	ImageName       string
	ImageTag        string
	ImageRepository string
	ImageSource     string
	ImageID         string
	Status          string
	State           string
	Ports           map[string]string
	CreatedAt       *time.Time
	StartedAt       *time.Time
	Labels          map[string]string
}

type DockerReceiveImage struct {
	Repository string
	Tag        string
	ImageID    string
	Source     string
	SizeBytes  int64
	CreatedAt  *time.Time
	Digest     string
}

type DockerReceiveVolume struct {
	VolumeID   string
	Name       string
	Driver     string
	Mountpoint string
	Renderer   string
	Scope      string
	Labels     map[string]string
	Options    map[string]string
	CreatedAt  *time.Time
	SizeBytes  *int64
	RefCount   int
}

type DockerReceiveNetwork struct {
	NetworkID      string
	Name           string
	Driver         string
	Scope          string
	IPv6Enabled    bool
	Internal       bool
	Attachable     bool
	Ingress        bool
	ConfigOnly     bool
	Labels         map[string]string
	IPAM           interface{}
	CreatedAt      *time.Time
	ContainerCount int
}

type DockerReceiveImageUpdate struct {
	Repository      string
	CurrentTag      string
	AvailableTag    string
	CurrentDigest   string
	AvailableDigest string
	ImageID         string
}

// ContainerStatusChange records a container that transitioned between running and stopped.
type ContainerStatusChange struct {
	ContainerID string // Docker container ID
	Name        string
	ImageName   string
	HostID      string
	OldStatus   string
	NewStatus   string
}

// DockerReceiveResult holds counts from ReceiveDockerData.
type DockerReceiveResult struct {
	ContainersReceived int
	ImagesReceived     int
	VolumesReceived    int
	NetworksReceived   int
	UpdatesFound       int
	StatusChanges      []ContainerStatusChange
}

func timeToPg(t *time.Time) pgtype.Timestamp {
	if t == nil || t.IsZero() {
		return pgtype.Timestamp{Time: time.Now().UTC(), Valid: true}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}

func mapToJSON(m interface{}) []byte {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

// ReceiveDockerData processes and stores Docker data from an agent.
func (s *DockerStore) ReceiveDockerData(ctx context.Context, hostID string, payload *DockerReceivePayload) (*DockerReceiveResult, error) {
	d := s.db.DB(ctx)
	now := time.Now().UTC()
	nowPg := pgtype.Timestamp{Time: now, Valid: true}

	result := &DockerReceiveResult{}

	// Map: "repository|tag|image_id" -> docker_images.id (UUID)
	imageIDMap := make(map[string]string)

	// 1. Process images from containers first
	for _, c := range payload.Containers {
		if c.ImageRepository == "" || c.ImageTag == "" {
			continue
		}
		imgID := c.ImageID
		if imgID == "" {
			imgID = "unknown"
		}
		key := c.ImageRepository + "|" + c.ImageTag + "|" + imgID
		if _, ok := imageIDMap[key]; ok {
			continue
		}
		source := c.ImageSource
		if source == "" {
			source = "docker-hub"
		}
		createdAt := timeToPg(c.CreatedAt)
		id := uuid.New().String()
		retID, err := d.Queries.UpsertDockerImage(ctx, db.UpsertDockerImageParams{
			ID:          id,
			Repository:  c.ImageRepository,
			Tag:         c.ImageTag,
			ImageID:     imgID,
			Digest:      nil,
			SizeBytes:   nil,
			Source:      source,
			CreatedAt:   createdAt,
			LastChecked: nowPg,
			UpdatedAt:   nowPg,
		})
		if err != nil {
			return nil, err
		}
		imageIDMap[key] = retID
	}

	// 2. Process standalone images
	for _, img := range payload.Images {
		key := img.Repository + "|" + img.Tag + "|" + img.ImageID
		if _, ok := imageIDMap[key]; ok {
			continue
		}
		source := img.Source
		if source == "" {
			source = "docker-hub"
		}
		var digest *string
		if img.Digest != "" {
			digest = &img.Digest
		}
		var sizeBytes *int64
		if img.SizeBytes > 0 {
			sizeBytes = &img.SizeBytes
		}
		createdAt := timeToPg(img.CreatedAt)
		id := uuid.New().String()
		retID, err := d.Queries.UpsertDockerImage(ctx, db.UpsertDockerImageParams{
			ID:          id,
			Repository:  img.Repository,
			Tag:         img.Tag,
			ImageID:     img.ImageID,
			Digest:      digest,
			SizeBytes:   sizeBytes,
			Source:      source,
			CreatedAt:   createdAt,
			LastChecked: nowPg,
			UpdatedAt:   nowPg,
		})
		if err != nil {
			return nil, err
		}
		imageIDMap[key] = retID
	}
	result.ImagesReceived = len(payload.Images)

	// 3. Process containers — snapshot existing statuses for change detection.
	oldContainers, _ := d.Queries.GetContainersByHostID(ctx, hostID)
	oldStatusMap := make(map[string]string, len(oldContainers)) // container_id -> status
	oldNameMap := make(map[string]string, len(oldContainers))
	for _, oc := range oldContainers {
		oldStatusMap[oc.ContainerID] = oc.Status
		oldNameMap[oc.ContainerID] = oc.Name
	}
	for _, c := range payload.Containers {
		var imgUUID *string
		if c.ImageRepository != "" && c.ImageTag != "" {
			imgID := c.ImageID
			if imgID == "" {
				imgID = "unknown"
			}
			key := c.ImageRepository + "|" + c.ImageTag + "|" + imgID
			if id, ok := imageIDMap[key]; ok {
				imgUUID = &id
			}
		}
		state := c.State
		if state == "" {
			state = c.Status
		}
		tag := c.ImageTag
		if tag == "" {
			tag = "latest"
		}
		createdAt := timeToPg(c.CreatedAt)
		startedAt := pgtype.Timestamp{}
		if c.StartedAt != nil {
			startedAt = pgtype.Timestamp{Time: *c.StartedAt, Valid: true}
		}
		err := d.Queries.UpsertDockerContainer(ctx, db.UpsertDockerContainerParams{
			ID:          uuid.New().String(),
			HostID:      hostID,
			ContainerID: c.ContainerID,
			Name:        c.Name,
			ImageID:     imgUUID,
			ImageName:   c.ImageName,
			ImageTag:    tag,
			Status:      c.Status,
			State:       &state,
			Ports:       mapToJSON(c.Ports),
			Labels:      mapToJSON(c.Labels),
			CreatedAt:   createdAt,
			StartedAt:   startedAt,
			UpdatedAt:   nowPg,
			LastChecked: nowPg,
		})
		if err != nil {
			return nil, err
		}
	}
	result.ContainersReceived = len(payload.Containers)

	// Detect container status transitions (running ↔ stopped/exited).
	for _, c := range payload.Containers {
		oldStatus, existed := oldStatusMap[c.ContainerID]
		if !existed {
			continue // new container, no transition
		}
		newStatus := c.Status
		if oldStatus == newStatus {
			continue
		}
		wasRunning := isRunningStatus(oldStatus)
		nowRunning := isRunningStatus(newStatus)
		if wasRunning != nowRunning {
			name := c.Name
			if name == "" {
				name = oldNameMap[c.ContainerID]
			}
			result.StatusChanges = append(result.StatusChanges, ContainerStatusChange{
				ContainerID: c.ContainerID,
				Name:        name,
				ImageName:   c.ImageName,
				HostID:      hostID,
				OldStatus:   oldStatus,
				NewStatus:   newStatus,
			})
		}
	}

	// 4. Process volumes
	for _, v := range payload.Volumes {
		scope := v.Scope
		if scope == "" {
			scope = "local"
		}
		driver := v.Driver
		if driver == "" {
			driver = "local"
		}
		var mountpoint, renderer *string
		if v.Mountpoint != "" {
			mountpoint = &v.Mountpoint
		}
		if v.Renderer != "" {
			renderer = &v.Renderer
		}
		createdAt := timeToPg(v.CreatedAt)
		err := d.Queries.UpsertDockerVolume(ctx, db.UpsertDockerVolumeParams{
			ID:          uuid.New().String(),
			HostID:      hostID,
			VolumeID:    v.VolumeID,
			Name:        v.Name,
			Driver:      driver,
			Mountpoint:  mountpoint,
			Renderer:    renderer,
			Scope:       scope,
			Labels:      mapToJSON(v.Labels),
			Options:     mapToJSON(v.Options),
			SizeBytes:   v.SizeBytes,
			RefCount:    int32(v.RefCount),
			CreatedAt:   createdAt,
			UpdatedAt:   nowPg,
			LastChecked: nowPg,
		})
		if err != nil {
			return nil, err
		}
	}
	result.VolumesReceived = len(payload.Volumes)

	// 5. Process networks
	for _, n := range payload.Networks {
		scope := n.Scope
		if scope == "" {
			scope = "local"
		}
		attachable := n.Attachable
		createdAt := pgtype.Timestamp{}
		if n.CreatedAt != nil {
			createdAt = pgtype.Timestamp{Time: *n.CreatedAt, Valid: true}
		}
		err := d.Queries.UpsertDockerNetwork(ctx, db.UpsertDockerNetworkParams{
			ID:             uuid.New().String(),
			HostID:         hostID,
			NetworkID:      n.NetworkID,
			Name:           n.Name,
			Driver:         n.Driver,
			Scope:          scope,
			Ipv6Enabled:    n.IPv6Enabled,
			Internal:       n.Internal,
			Attachable:     attachable,
			Ingress:        n.Ingress,
			ConfigOnly:     n.ConfigOnly,
			Labels:         mapToJSON(n.Labels),
			Ipam:           mapToJSON(n.IPAM),
			ContainerCount: int32(n.ContainerCount),
			CreatedAt:      createdAt,
			UpdatedAt:      nowPg,
			LastChecked:    nowPg,
		})
		if err != nil {
			return nil, err
		}
	}
	result.NetworksReceived = len(payload.Networks)

	// 6. Process updates (need to resolve image_id UUID from repository+current_tag+image_id)
	for _, u := range payload.Updates {
		imgUUID, err := d.Queries.GetImageIDByRepositoryTagImageID(ctx, db.GetImageIDByRepositoryTagImageIDParams{
			Repository: u.Repository,
			Tag:        u.CurrentTag,
			ImageID:    u.ImageID,
		})
		if err != nil {
			continue // image may not exist yet, skip
		}
		digestInfo := ""
		if u.CurrentDigest != "" || u.AvailableDigest != "" {
			digestJSON, _ := json.Marshal(map[string]string{
				"method":           "digest_comparison",
				"current_digest":   u.CurrentDigest,
				"available_digest": u.AvailableDigest,
			})
			digestInfo = string(digestJSON)
		}
		severity := "digest_changed"
		err = d.Queries.UpsertDockerImageUpdate(ctx, db.UpsertDockerImageUpdateParams{
			ID:               uuid.New().String(),
			ImageID:          imgUUID,
			CurrentTag:       u.CurrentTag,
			AvailableTag:     u.AvailableTag,
			IsSecurityUpdate: false,
			Severity:         &severity,
			ChangelogUrl:     &digestInfo,
			CreatedAt:        nowPg,
			UpdatedAt:        nowPg,
		})
		if err != nil {
			return nil, err
		}
		result.UpdatesFound++
	}

	return result, nil
}

// isRunningStatus returns true for Docker container statuses that indicate a running container.
func isRunningStatus(status string) bool {
	switch status {
	case "running", "restarting":
		return true
	}
	return false
}
