package store

import (
	"context"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

// DockerStore provides Docker inventory access.
type DockerStore struct {
	db database.DBProvider
}

// NewDockerStore creates a new Docker store.
func NewDockerStore(db database.DBProvider) *DockerStore {
	return &DockerStore{db: db}
}

// validatePagination validates and caps pagination params.
func validatePagination(page, limit, maxLimit int) (skip, take int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return (page - 1) * limit, limit
}

func dockerHostRowToHostInfo(r db.GetDockerHostsMinimalByIDsRow) *HostInfo {
	return &HostInfo{
		ID:           r.ID,
		FriendlyName: r.FriendlyName,
		Hostname:     r.Hostname,
		IP:           r.Ip,
	}
}

func dockerHostDetailRowToHostInfo(r db.GetDockerHostsByIDsRow) *HostInfo {
	return &HostInfo{
		ID:           r.ID,
		FriendlyName: r.FriendlyName,
		Hostname:     r.Hostname,
		IP:           r.Ip,
		OSType:       &r.OsType,
		OSVersion:    &r.OsVersion,
	}
}

// GetDashboard returns Docker dashboard statistics.
func (s *DockerStore) GetDashboard(ctx context.Context) (map[string]interface{}, error) {
	d := s.db.DB(ctx)
	totalHosts, err := d.Queries.CountDockerHosts(ctx)
	if err != nil {
		return nil, err
	}

	stats, err := d.Queries.GetDockerDashboardStats(ctx)
	if err != nil {
		return nil, err
	}

	containersByStatus, _ := d.Queries.GetContainersByStatus(ctx)
	imagesBySource, _ := d.Queries.GetImagesBySource(ctx)

	cbStatus := make([]map[string]interface{}, len(containersByStatus))
	for i, r := range containersByStatus {
		cbStatus[i] = map[string]interface{}{"status": r.Status, "_count": r.Count}
	}
	ibSource := make([]map[string]interface{}, len(imagesBySource))
	for i, r := range imagesBySource {
		ibSource[i] = map[string]interface{}{"source": r.Source, "_count": r.Count}
	}

	return map[string]interface{}{
		"stats": map[string]interface{}{
			"totalHostsWithDocker": totalHosts,
			"totalContainers":      stats.Column1,
			"runningContainers":    stats.Column2,
			"totalImages":          stats.Column3,
			"availableUpdates":     stats.Column4,
		},
		"containersByStatus": cbStatus,
		"imagesBySource":     ibSource,
	}, nil
}

// ContainerListParams holds filters for ListContainers.
type ContainerListParams struct {
	Status  string
	HostID  string
	ImageID string
	Search  string
	Page    int
	Limit   int
}

// ContainerWithHost extends DockerContainer with host info.
type ContainerWithHost struct {
	models.DockerContainer
	Host *HostInfo `json:"host"`
}

// HostInfo is minimal host info for Docker responses.
type HostInfo struct {
	ID           string  `db:"id" json:"id"`
	FriendlyName string  `db:"friendly_name" json:"friendly_name"`
	Hostname     *string `db:"hostname" json:"hostname"`
	IP           *string `db:"ip" json:"ip"`
	OSType       *string `db:"os_type" json:"os_type,omitempty"`
	OSVersion    *string `db:"os_version" json:"os_version,omitempty"`
}

// ListContainers returns containers with host info.
func (s *DockerStore) ListContainers(ctx context.Context, params ContainerListParams) ([]ContainerWithHost, int, error) {
	d := s.db.DB(ctx)
	skip, take := validatePagination(params.Page, params.Limit, 10000)

	arg := db.ListContainersParams{Limit: int32(take), Offset: int32(skip)}
	if params.Status != "" {
		arg.Status = &params.Status
	}
	if params.HostID != "" {
		arg.HostID = &params.HostID
	}
	if params.ImageID != "" {
		arg.ImageID = &params.ImageID
	}
	if params.Search != "" {
		arg.Search = &params.Search
	}

	countArg := db.CountContainersParams{}
	if params.Status != "" {
		countArg.Status = &params.Status
	}
	if params.HostID != "" {
		countArg.HostID = &params.HostID
	}
	if params.ImageID != "" {
		countArg.ImageID = &params.ImageID
	}
	if params.Search != "" {
		countArg.Search = &params.Search
	}

	total, err := d.Queries.CountContainers(ctx, countArg)
	if err != nil {
		return nil, 0, err
	}

	containers, err := d.Queries.ListContainers(ctx, arg)
	if err != nil {
		return nil, 0, err
	}

	if len(containers) == 0 {
		return []ContainerWithHost{}, int(total), nil
	}

	hostIDs := make(map[string]bool)
	for _, c := range containers {
		hostIDs[c.HostID] = true
	}
	ids := make([]string, 0, len(hostIDs))
	for id := range hostIDs {
		ids = append(ids, id)
	}

	hosts, err := d.Queries.GetDockerHostsMinimalByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	hostMap := make(map[string]*HostInfo)
	for i := range hosts {
		h := dockerHostRowToHostInfo(hosts[i])
		hostMap[h.ID] = h
	}

	out := make([]ContainerWithHost, len(containers))
	for i := range containers {
		out[i] = ContainerWithHost{
			DockerContainer: dbDockerContainerToModel(containers[i]),
			Host:            hostMap[containers[i].HostID],
		}
	}
	return out, int(total), nil
}

// GetContainer returns a container by ID with host and image info.
func (s *DockerStore) GetContainer(ctx context.Context, id string) (*ContainerDetail, error) {
	d := s.db.DB(ctx)
	c, err := d.Queries.GetContainerByID(ctx, id)
	if err != nil {
		return nil, err
	}

	hostRows, _ := d.Queries.GetDockerHostsByIDs(ctx, []string{c.HostID})
	var hostPtr *HostInfo
	if len(hostRows) > 0 {
		hostPtr = dockerHostDetailRowToHostInfo(hostRows[0])
	}

	var dockerImage *models.DockerImage
	var imageUpdates []models.DockerImageUpdate
	var similar []models.DockerContainer
	if c.ImageID != nil && *c.ImageID != "" {
		img, err := d.Queries.GetImageByID(ctx, *c.ImageID)
		if err == nil {
			dockerImage = ptr(dbDockerImageToModel(img))
			updates, _ := d.Queries.GetImageUpdatesByImageID(ctx, *c.ImageID)
			imageUpdates = make([]models.DockerImageUpdate, len(updates))
			for i := range updates {
				imageUpdates[i] = dbDockerImageUpdateToModel(updates[i])
			}
		}
		sim, _ := d.Queries.GetContainersByImageID(ctx, db.GetContainersByImageIDParams{ImageID: c.ImageID, ID: id})
		similar = make([]models.DockerContainer, len(sim))
		for i := range sim {
			similar[i] = dbDockerContainerToModel(sim[i])
		}
	}

	return &ContainerDetail{
		Container:          dbDockerContainerToModel(c),
		Host:               hostPtr,
		DockerImages:       dockerImage,
		DockerImageUpdates: imageUpdates,
		SimilarContainers:  similar,
	}, nil
}

func ptr[T any](v T) *T { return &v }

// ContainerDetail is container with host, image, and similar containers.
type ContainerDetail struct {
	Container          models.DockerContainer     `json:"container"`
	Host               *HostInfo                  `json:"host"`
	DockerImages       *models.DockerImage        `json:"-"`
	DockerImageUpdates []models.DockerImageUpdate `json:"-"`
	SimilarContainers  []models.DockerContainer   `json:"similarContainers"`
}

// DeleteContainer removes a container from inventory.
func (s *DockerStore) DeleteContainer(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteContainer(ctx, id)
}

// ImageListParams holds filters for ListImages.
type ImageListParams struct {
	Source string
	Search string
	Page   int
	Limit  int
}

// ImageWithCounts extends DockerImage with counts and hasUpdates.
type ImageWithCounts struct {
	models.DockerImage
	CountContainers int  `json:"-"`
	HasUpdates      bool `json:"hasUpdates"`
	CountUpdates    int  `json:"-"`
}

// ListImages returns images with container count and hasUpdates.
func (s *DockerStore) ListImages(ctx context.Context, params ImageListParams) ([]ImageWithCounts, int, error) {
	d := s.db.DB(ctx)
	skip, take := validatePagination(params.Page, params.Limit, 10000)

	arg := db.ListImagesParams{Limit: int32(take), Offset: int32(skip)}
	if params.Source != "" {
		arg.Source = &params.Source
	}
	if params.Search != "" {
		arg.Search = &params.Search
	}

	countArg := db.CountImagesParams{}
	if params.Source != "" {
		countArg.Source = &params.Source
	}
	if params.Search != "" {
		countArg.Search = &params.Search
	}

	total, err := d.Queries.CountImages(ctx, countArg)
	if err != nil {
		return nil, 0, err
	}

	images, err := d.Queries.ListImages(ctx, arg)
	if err != nil {
		return nil, 0, err
	}

	if len(images) == 0 {
		return []ImageWithCounts{}, int(total), nil
	}

	ids := make([]string, len(images))
	for i := range images {
		ids[i] = images[i].ID
	}

	containerRows, _ := d.Queries.GetContainerCountsByImageIDs(ctx, ids)
	containerCount := make(map[string]int)
	for _, r := range containerRows {
		if r.ImageID != nil {
			containerCount[*r.ImageID] = int(r.Cnt)
		}
	}

	updateRows, _ := d.Queries.GetUpdateCountsByImageIDs(ctx, ids)
	updateCount := make(map[string]int)
	for _, r := range updateRows {
		updateCount[r.ImageID] = int(r.Cnt)
	}

	out := make([]ImageWithCounts, len(images))
	for i := range images {
		out[i] = ImageWithCounts{
			DockerImage:     dbDockerImageToModel(images[i]),
			CountContainers: containerCount[images[i].ID],
			HasUpdates:      updateCount[images[i].ID] > 0,
			CountUpdates:    updateCount[images[i].ID],
		}
	}
	return out, int(total), nil
}

// GetImage returns an image by ID with containers and hosts.
func (s *DockerStore) GetImage(ctx context.Context, id string) (*ImageDetail, error) {
	d := s.db.DB(ctx)
	img, err := d.Queries.GetImageByID(ctx, id)
	if err != nil {
		return nil, err
	}

	containers, _ := d.Queries.GetContainersByImageIDAll(ctx, &id)
	hostIDs := make(map[string]bool)
	for _, c := range containers {
		hostIDs[c.HostID] = true
	}
	ids := make([]string, 0, len(hostIDs))
	for hid := range hostIDs {
		ids = append(ids, hid)
	}

	var hosts []HostInfo
	if len(ids) > 0 {
		hostRows, _ := d.Queries.GetDockerHostsMinimalByIDs(ctx, ids)
		hosts = make([]HostInfo, len(hostRows))
		for i := range hostRows {
			hosts[i] = *dockerHostRowToHostInfo(hostRows[i])
		}
	}

	return &ImageDetail{
		Image:            dbDockerImageToModel(img),
		Hosts:            hosts,
		TotalContainers:  len(containers),
		TotalHosts:       len(hosts),
		DockerContainers: convertContainers(containers),
	}, nil
}

func convertContainers(dc []db.DockerContainer) []models.DockerContainer {
	out := make([]models.DockerContainer, len(dc))
	for i := range dc {
		out[i] = dbDockerContainerToModel(dc[i])
	}
	return out
}

// ImageDetail is image with containers and hosts.
type ImageDetail struct {
	Image            models.DockerImage       `json:"image"`
	Hosts            []HostInfo               `json:"hosts"`
	TotalContainers  int                      `json:"totalContainers"`
	TotalHosts       int                      `json:"totalHosts"`
	DockerContainers []models.DockerContainer `json:"docker_containers,omitempty"`
}

// DeleteImage removes an image from inventory (must not be in use).
func (s *DockerStore) DeleteImage(ctx context.Context, id string) (int, error) {
	d := s.db.DB(ctx)
	cnt, err := d.Queries.CountContainersByImageID(ctx, &id)
	if err != nil {
		return 0, err
	}
	if cnt > 0 {
		return int(cnt), nil
	}
	_ = d.Queries.DeleteImageUpdatesByImageID(ctx, id)
	err = d.Queries.DeleteImageByID(ctx, id)
	return 0, err
}

// HostWithDockerStats is host with Docker stats.
type HostWithDockerStats struct {
	ID           string      `json:"id"`
	FriendlyName string      `json:"friendly_name"`
	Hostname     *string     `json:"hostname"`
	IP           *string     `json:"ip"`
	DockerStats  DockerStats `json:"dockerStats"`
}

// DockerStats holds container/image counts.
type DockerStats struct {
	TotalContainers   int `json:"totalContainers"`
	RunningContainers int `json:"runningContainers"`
	TotalImages       int `json:"totalImages"`
}

// ListHosts returns hosts that have Docker containers with stats.
func (s *DockerStore) ListHosts(ctx context.Context, page, limit int) ([]HostWithDockerStats, int, error) {
	d := s.db.DB(ctx)
	skip, take := validatePagination(page, limit, 10000)

	hostIDs, err := d.Queries.GetDistinctDockerHostIDs(ctx)
	if err != nil {
		return nil, 0, err
	}
	total := len(hostIDs)
	if total == 0 {
		return []HostWithDockerStats{}, 0, nil
	}

	hostRows, err := d.Queries.ListDockerHostsPaginated(ctx, db.ListDockerHostsPaginatedParams{
		Limit:  int32(take),
		Offset: int32(skip),
	})
	if err != nil {
		return nil, 0, err
	}

	out := make([]HostWithDockerStats, len(hostRows))
	for i := range hostRows {
		h := hostRows[i]
		stats, _ := d.Queries.GetHostDockerStats(ctx, h.ID)
		out[i] = HostWithDockerStats{
			ID:           h.ID,
			FriendlyName: h.FriendlyName,
			Hostname:     h.Hostname,
			IP:           h.Ip,
			DockerStats: DockerStats{
				TotalContainers:   int(stats.Column1),
				RunningContainers: int(stats.Column2),
				TotalImages:       int(stats.Column3),
			},
		}
	}
	return out, total, nil
}

// GetHostDockerDetail returns host with containers, images, volumes, networks.
func (s *DockerStore) GetHostDockerDetail(ctx context.Context, hostID string) (*HostDockerDetail, error) {
	d := s.db.DB(ctx)
	host, err := d.Queries.GetHostByID(ctx, hostID)
	if err != nil {
		return nil, err
	}

	containers, _ := d.Queries.GetContainersByHostID(ctx, hostID)
	imageIDs := make(map[string]bool)
	for _, c := range containers {
		if c.ImageID != nil && *c.ImageID != "" {
			imageIDs[*c.ImageID] = true
		}
	}
	ids := make([]string, 0, len(imageIDs))
	for id := range imageIDs {
		ids = append(ids, id)
	}

	var images []models.DockerImage
	if len(ids) > 0 {
		imgRows, _ := d.Queries.GetImagesByIDs(ctx, ids)
		images = make([]models.DockerImage, len(imgRows))
		for i := range imgRows {
			images[i] = dbDockerImageToModel(imgRows[i])
		}
	}

	volumes, _ := d.Queries.GetVolumesByHostID(ctx, hostID)
	networks, _ := d.Queries.GetNetworksByHostID(ctx, hostID)

	running, stopped := 0, 0
	for _, c := range containers {
		switch c.Status {
		case "running":
			running++
		case "exited", "stopped":
			stopped++
		}
	}

	return &HostDockerDetail{
		Host:       *dbHostToModel(host),
		Containers: convertContainers(containers),
		Images:     images,
		Volumes:    convertVolumes(volumes),
		Networks:   convertNetworks(networks),
		Stats: map[string]interface{}{
			"totalContainers":   len(containers),
			"runningContainers": running,
			"stoppedContainers": stopped,
			"totalImages":       len(images),
			"totalVolumes":      len(volumes),
			"totalNetworks":     len(networks),
		},
	}, nil
}

func convertVolumes(dv []db.DockerVolume) []models.DockerVolume {
	out := make([]models.DockerVolume, len(dv))
	for i := range dv {
		out[i] = dbDockerVolumeToModel(dv[i])
	}
	return out
}

func convertNetworks(dn []db.DockerNetwork) []models.DockerNetwork {
	out := make([]models.DockerNetwork, len(dn))
	for i := range dn {
		out[i] = dbDockerNetworkToModel(dn[i])
	}
	return out
}

// HostDockerDetail is host with full Docker inventory.
type HostDockerDetail struct {
	Host       models.Host              `json:"host"`
	Containers []models.DockerContainer `json:"containers"`
	Images     []models.DockerImage     `json:"images"`
	Volumes    []models.DockerVolume    `json:"volumes"`
	Networks   []models.DockerNetwork   `json:"networks"`
	Stats      map[string]interface{}   `json:"stats"`
}

// VolumeListParams holds filters for ListVolumes.
type VolumeListParams struct {
	Driver string
	Search string
	Page   int
	Limit  int
}

// VolumeWithHost extends DockerVolume with host info.
type VolumeWithHost struct {
	models.DockerVolume
	Hosts *HostInfo `json:"hosts"`
}

// ListVolumes returns volumes with host info.
func (s *DockerStore) ListVolumes(ctx context.Context, params VolumeListParams) ([]VolumeWithHost, int, error) {
	d := s.db.DB(ctx)
	skip, take := validatePagination(params.Page, params.Limit, 10000)

	arg := db.ListVolumesParams{Limit: int32(take), Offset: int32(skip)}
	if params.Driver != "" {
		arg.Driver = &params.Driver
	}
	if params.Search != "" {
		arg.Search = &params.Search
	}

	countArg := db.CountVolumesParams{}
	if params.Driver != "" {
		countArg.Driver = &params.Driver
	}
	if params.Search != "" {
		countArg.Search = &params.Search
	}

	total, err := d.Queries.CountVolumes(ctx, countArg)
	if err != nil {
		return nil, 0, err
	}

	volumes, err := d.Queries.ListVolumes(ctx, arg)
	if err != nil {
		return nil, 0, err
	}

	if len(volumes) == 0 {
		return []VolumeWithHost{}, int(total), nil
	}

	hostIDs := make([]string, len(volumes))
	for i := range volumes {
		hostIDs[i] = volumes[i].HostID
	}

	hosts, err := d.Queries.GetDockerHostsMinimalByIDs(ctx, hostIDs)
	if err != nil {
		return nil, 0, err
	}
	hostMap := make(map[string]*HostInfo)
	for i := range hosts {
		h := dockerHostRowToHostInfo(hosts[i])
		hostMap[h.ID] = h
	}

	out := make([]VolumeWithHost, len(volumes))
	for i := range volumes {
		out[i] = VolumeWithHost{
			DockerVolume: dbDockerVolumeToModel(volumes[i]),
			Hosts:        hostMap[volumes[i].HostID],
		}
	}
	return out, int(total), nil
}

// GetVolume returns a volume by ID with host info.
func (s *DockerStore) GetVolume(ctx context.Context, id string) (*VolumeDetail, error) {
	d := s.db.DB(ctx)
	v, err := d.Queries.GetVolumeByID(ctx, id)
	if err != nil {
		return nil, err
	}

	hostRows, _ := d.Queries.GetDockerHostsByIDs(ctx, []string{v.HostID})
	var hostPtr *HostInfo
	if len(hostRows) > 0 {
		hostPtr = dockerHostDetailRowToHostInfo(hostRows[0])
	}

	return &VolumeDetail{
		Volume: dbDockerVolumeToModel(v),
		Hosts:  hostPtr,
	}, nil
}

// VolumeDetail is volume with host.
type VolumeDetail struct {
	Volume models.DockerVolume `json:"volume"`
	Hosts  *HostInfo           `json:"hosts"`
}

// DeleteVolume removes a volume from inventory.
func (s *DockerStore) DeleteVolume(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteVolume(ctx, id)
}

// NetworkListParams holds filters for ListNetworks.
type NetworkListParams struct {
	Driver string
	Search string
	Page   int
	Limit  int
}

// NetworkWithHost extends DockerNetwork with host info.
type NetworkWithHost struct {
	models.DockerNetwork
	Hosts *HostInfo `json:"hosts"`
}

// ListNetworks returns networks with host info.
func (s *DockerStore) ListNetworks(ctx context.Context, params NetworkListParams) ([]NetworkWithHost, int, error) {
	d := s.db.DB(ctx)
	skip, take := validatePagination(params.Page, params.Limit, 10000)

	arg := db.ListNetworksParams{Limit: int32(take), Offset: int32(skip)}
	if params.Driver != "" {
		arg.Driver = &params.Driver
	}
	if params.Search != "" {
		arg.Search = &params.Search
	}

	countArg := db.CountNetworksParams{}
	if params.Driver != "" {
		countArg.Driver = &params.Driver
	}
	if params.Search != "" {
		countArg.Search = &params.Search
	}

	total, err := d.Queries.CountNetworks(ctx, countArg)
	if err != nil {
		return nil, 0, err
	}

	networks, err := d.Queries.ListNetworks(ctx, arg)
	if err != nil {
		return nil, 0, err
	}

	if len(networks) == 0 {
		return []NetworkWithHost{}, int(total), nil
	}

	hostIDs := make([]string, len(networks))
	for i := range networks {
		hostIDs[i] = networks[i].HostID
	}

	hosts, err := d.Queries.GetDockerHostsMinimalByIDs(ctx, hostIDs)
	if err != nil {
		return nil, 0, err
	}
	hostMap := make(map[string]*HostInfo)
	for i := range hosts {
		h := dockerHostRowToHostInfo(hosts[i])
		hostMap[h.ID] = h
	}

	out := make([]NetworkWithHost, len(networks))
	for i := range networks {
		out[i] = NetworkWithHost{
			DockerNetwork: dbDockerNetworkToModel(networks[i]),
			Hosts:         hostMap[networks[i].HostID],
		}
	}
	return out, int(total), nil
}

// GetNetwork returns a network by ID with host info.
func (s *DockerStore) GetNetwork(ctx context.Context, id string) (*NetworkDetail, error) {
	d := s.db.DB(ctx)
	n, err := d.Queries.GetNetworkByID(ctx, id)
	if err != nil {
		return nil, err
	}

	hostRows, _ := d.Queries.GetDockerHostsByIDs(ctx, []string{n.HostID})
	var hostPtr *HostInfo
	if len(hostRows) > 0 {
		hostPtr = dockerHostDetailRowToHostInfo(hostRows[0])
	}

	return &NetworkDetail{
		Network: dbDockerNetworkToModel(n),
		Hosts:   hostPtr,
	}, nil
}

// NetworkDetail is network with host.
type NetworkDetail struct {
	Network models.DockerNetwork `json:"network"`
	Hosts   *HostInfo            `json:"hosts"`
}

// DeleteNetwork removes a network from inventory.
func (s *DockerStore) DeleteNetwork(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteNetwork(ctx, id)
}
