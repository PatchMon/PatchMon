package store

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/PatchMon/PatchMon/server-source-code/internal/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// DockerReceivePayload holds the agent's Docker data for storage.
type DockerReceivePayload struct {
	Containers []DockerReceiveContainer
	Images     []DockerReceiveImage
	Volumes    []DockerReceiveVolume
	Networks   []DockerReceiveNetwork
	Collected  []string
}

// An empty list from an agent that does not send Collected may be a failed collection, so it must not prune.
func (p *DockerReceivePayload) reconciles(kind string, reported int) bool {
	return reported > 0 || slices.Contains(p.Collected, kind)
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
	ImageStale      bool
	RestartCount    int
	ImageDigest     string
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
	StatusChanges      []ContainerStatusChange
}

func timeToPg(t *time.Time) pgtype.Timestamp {
	if t == nil || t.IsZero() {
		return pgtime.Now()
	}
	return pgtime.From(*t)
}

func mapToJSON(m interface{}) []byte {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

// ReceiveDockerData processes and stores Docker data from an agent in one transaction.
func (s *DockerStore) ReceiveDockerData(ctx context.Context, hostID string, payload *DockerReceivePayload) (*DockerReceiveResult, error) {
	// Strip NUL before anything touches a query parameter. The handler has
	// already run the canonical docker-hash check by this point, so cleaning
	// here cannot cause a hash mismatch.
	sanitizeDockerPayload(payload)

	d := s.db.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()
	result, err := receiveDockerData(ctx, d.Queries.WithTx(tx), hostID, payload, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func dockerImageKey(repository, tag, imageID string) string {
	return repository + "|" + tag + "|" + imageID
}

func containerImageKey(c *DockerReceiveContainer) (string, bool) {
	if c.ImageRepository == "" || c.ImageTag == "" {
		return "", false
	}
	imgID := c.ImageID
	if imgID == "" {
		imgID = "unknown"
	}
	return dockerImageKey(c.ImageRepository, c.ImageTag, imgID), true
}

func receiveDockerData(ctx context.Context, q *db.Queries, hostID string, payload *DockerReceivePayload, now time.Time) (*DockerReceiveResult, error) {
	nowPg := pgtime.From(now)
	result := &DockerReceiveResult{}

	type imageSource struct {
		reported *DockerReceiveImage
		derived  *DockerReceiveContainer
	}
	sources := make(map[string]imageSource, len(payload.Images)+len(payload.Containers))
	for i := range payload.Images {
		img := &payload.Images[i]
		key := dockerImageKey(img.Repository, img.Tag, img.ImageID)
		if _, ok := sources[key]; !ok {
			sources[key] = imageSource{reported: img}
		}
	}
	for i := range payload.Containers {
		c := &payload.Containers[i]
		if key, ok := containerImageKey(c); ok {
			if _, known := sources[key]; !known {
				sources[key] = imageSource{derived: c}
			}
		}
	}
	keys := make([]string, 0, len(sources))
	for key := range sources {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	imageIDMap := make(map[string]string, len(keys))
	for _, key := range keys {
		var retID string
		var err error
		if img := sources[key].reported; img != nil {
			source := img.Source
			if source == "" {
				source = "docker-hub"
			}
			var sizeBytes *int64
			if img.SizeBytes > 0 {
				sizeBytes = &img.SizeBytes
			}
			retID, err = q.UpsertDockerImage(ctx, db.UpsertDockerImageParams{
				ID:          uuid.New().String(),
				Repository:  img.Repository,
				Tag:         img.Tag,
				ImageID:     img.ImageID,
				Digest:      optionalText(img.Digest),
				SizeBytes:   sizeBytes,
				Source:      source,
				CreatedAt:   pgtime.FromPtr(nonZeroTime(img.CreatedAt)),
				LastChecked: nowPg,
				UpdatedAt:   nowPg,
			})
		} else {
			c := sources[key].derived
			imgID := c.ImageID
			if imgID == "" {
				imgID = "unknown"
			}
			source := c.ImageSource
			if source == "" {
				source = "docker-hub"
			}
			retID, err = q.InsertDockerImageFromContainer(ctx, db.InsertDockerImageFromContainerParams{
				ID:         uuid.New().String(),
				Repository: c.ImageRepository,
				Tag:        c.ImageTag,
				ImageID:    imgID,
				Source:     source,
				CreatedAt:  nowPg,
			})
		}
		if err != nil {
			return nil, err
		}
		imageIDMap[key] = retID
	}
	result.ImagesReceived = len(payload.Images)

	linkedImageIDs := make([]string, 0, len(imageIDMap))
	for _, id := range imageIDMap {
		linkedImageIDs = append(linkedImageIDs, id)
	}
	slices.Sort(linkedImageIDs)
	linkedImageIDs = slices.Compact(linkedImageIDs)
	for _, id := range linkedImageIDs {
		if err := q.UpsertDockerHostImage(ctx, db.UpsertDockerHostImageParams{
			HostID:   hostID,
			ImageID:  id,
			LastSeen: nowPg,
		}); err != nil {
			return nil, err
		}
	}
	if payload.reconciles("images", len(payload.Images)) {
		if err := q.DeleteStaleDockerHostImages(ctx, db.DeleteStaleDockerHostImagesParams{
			HostID:   hostID,
			ImageIds: linkedImageIDs,
		}); err != nil {
			return nil, err
		}
	}

	oldContainers, err := q.GetContainersByHostID(ctx, hostID)
	if err != nil {
		return nil, err
	}
	oldStatusMap := make(map[string]string, len(oldContainers))
	oldNameMap := make(map[string]string, len(oldContainers))
	for _, oc := range oldContainers {
		oldStatusMap[oc.ContainerID] = oc.Status
		oldNameMap[oc.ContainerID] = oc.Name
	}
	for i := range payload.Containers {
		c := &payload.Containers[i]
		var imgUUID *string
		if key, ok := containerImageKey(c); ok {
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
		err := q.UpsertDockerContainer(ctx, db.UpsertDockerContainerParams{
			ID:           uuid.New().String(),
			HostID:       hostID,
			ContainerID:  c.ContainerID,
			Name:         c.Name,
			ImageID:      imgUUID,
			ImageName:    c.ImageName,
			ImageTag:     tag,
			Status:       c.Status,
			State:        &state,
			Ports:        mapToJSON(c.Ports),
			Labels:       mapToJSON(c.Labels),
			CreatedAt:    timeToPg(c.CreatedAt),
			StartedAt:    pgtime.FromPtr(nonZeroTime(c.StartedAt)),
			UpdatedAt:    nowPg,
			LastChecked:  nowPg,
			ImageStale:   c.ImageStale,
			RestartCount: safeconv.ClampToInt32(c.RestartCount),
			ImageDigest:  optionalText(c.ImageDigest),
		})
		if err != nil {
			return nil, err
		}
	}
	result.ContainersReceived = len(payload.Containers)

	for _, c := range payload.Containers {
		oldStatus, existed := oldStatusMap[c.ContainerID]
		if !existed || oldStatus == c.Status {
			continue
		}
		if isRunningStatus(oldStatus) != isRunningStatus(c.Status) {
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
				NewStatus:   c.Status,
			})
		}
	}

	if payload.reconciles("containers", len(payload.Containers)) {
		reportedContainerIDs := make([]string, 0, len(payload.Containers))
		for _, c := range payload.Containers {
			reportedContainerIDs = append(reportedContainerIDs, c.ContainerID)
		}
		if err := q.DeleteStaleContainers(ctx, db.DeleteStaleContainersParams{
			HostID:       hostID,
			ContainerIds: reportedContainerIDs,
		}); err != nil {
			return nil, err
		}
	}

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
		err := q.UpsertDockerVolume(ctx, db.UpsertDockerVolumeParams{
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
			RefCount:    safeconv.ClampToInt32(v.RefCount),
			CreatedAt:   timeToPg(v.CreatedAt),
			UpdatedAt:   nowPg,
			LastChecked: nowPg,
		})
		if err != nil {
			return nil, err
		}
	}
	result.VolumesReceived = len(payload.Volumes)

	if payload.reconciles("volumes", len(payload.Volumes)) {
		reportedVolumeIDs := make([]string, 0, len(payload.Volumes))
		for _, v := range payload.Volumes {
			reportedVolumeIDs = append(reportedVolumeIDs, v.VolumeID)
		}
		if err := q.DeleteStaleVolumes(ctx, db.DeleteStaleVolumesParams{
			HostID:    hostID,
			VolumeIds: reportedVolumeIDs,
		}); err != nil {
			return nil, err
		}
	}

	for _, n := range payload.Networks {
		scope := n.Scope
		if scope == "" {
			scope = "local"
		}
		err := q.UpsertDockerNetwork(ctx, db.UpsertDockerNetworkParams{
			ID:             uuid.New().String(),
			HostID:         hostID,
			NetworkID:      n.NetworkID,
			Name:           n.Name,
			Driver:         n.Driver,
			Scope:          scope,
			Ipv6Enabled:    n.IPv6Enabled,
			Internal:       n.Internal,
			Attachable:     n.Attachable,
			Ingress:        n.Ingress,
			ConfigOnly:     n.ConfigOnly,
			Labels:         mapToJSON(n.Labels),
			Ipam:           mapToJSON(n.IPAM),
			ContainerCount: safeconv.ClampToInt32(n.ContainerCount),
			CreatedAt:      pgtime.FromPtr(n.CreatedAt),
			UpdatedAt:      nowPg,
			LastChecked:    nowPg,
		})
		if err != nil {
			return nil, err
		}
	}
	result.NetworksReceived = len(payload.Networks)

	if payload.reconciles("networks", len(payload.Networks)) {
		reportedNetworkIDs := make([]string, 0, len(payload.Networks))
		for _, n := range payload.Networks {
			reportedNetworkIDs = append(reportedNetworkIDs, n.NetworkID)
		}
		if err := q.DeleteStaleNetworks(ctx, db.DeleteStaleNetworksParams{
			HostID:     hostID,
			NetworkIds: reportedNetworkIDs,
		}); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func nonZeroTime(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}

// isRunningStatus returns true for Docker container statuses that indicate a running container.
func isRunningStatus(status string) bool {
	switch status {
	case "running", "restarting":
		return true
	}
	return false
}
