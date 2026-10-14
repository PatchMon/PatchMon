package handler

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// DockerHandler handles Docker inventory routes.
type DockerHandler struct {
	docker   *store.DockerStore
	registry *agentregistry.Registry
}

// NewDockerHandler creates a new Docker handler.
func NewDockerHandler(docker *store.DockerStore, registry *agentregistry.Registry) *DockerHandler {
	return &DockerHandler{docker: docker, registry: registry}
}

var imageUpdateStatusFilters = map[string]bool{
	store.ImageStatusUpdateAvailable: true,
	store.ImageStatusRecreateNeeded:  true,
	store.ImageStatusUpToDate:        true,
	store.ImageStatusUnknown:         true,
	store.ImageStatusNotChecked:      true,
	store.ImageStatusLocalBuild:      true,
	store.ImageStatusPinned:          true,
	store.ImageStatusSkipped:         true,
}

var dockerImageCheckCommand = []byte(`{"type":"docker_image_check"}`)

// Dashboard handles GET /docker/dashboard.
func (h *DockerHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	data, err := h.docker.GetDashboard(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch Docker dashboard")
		return
	}
	JSON(w, http.StatusOK, data)
}

// ListContainers handles GET /docker/containers.
func (h *DockerHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parseIntQuery(r, "page", 1)
	limit := parseIntQuery(r, "limit", 50)
	search := q.Get("search")
	if len(search) > 200 {
		search = search[:200]
	}
	params := store.ContainerListParams{
		Status:  q.Get("status"),
		HostID:  q.Get("hostId"),
		ImageID: q.Get("imageId"),
		Search:  search,
		Page:    page,
		Limit:   limit,
	}
	containers, total, err := h.docker.ListContainers(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch containers")
		return
	}
	pages := (total + params.Limit - 1) / params.Limit
	if pages < 1 {
		pages = 1
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"containers": containers,
		"pagination": map[string]interface{}{
			"page":       page,
			"limit":      params.Limit,
			"total":      total,
			"totalPages": pages,
		},
	})
}

// GetContainer handles GET /docker/containers/:id.
func (h *DockerHandler) GetContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Container ID is required")
		return
	}
	detail, err := h.docker.GetContainer(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			Error(w, http.StatusNotFound, "Container not found")
			return
		}
		Error(w, http.StatusInternalServerError, "Failed to fetch container")
		return
	}
	// Match Node response: container with host and docker_images embedded, similarContainers
	containerResp := map[string]interface{}{
		"id":            detail.Container.ID,
		"host_id":       detail.Container.HostID,
		"container_id":  detail.Container.ContainerID,
		"name":          detail.Container.Name,
		"image_id":      detail.Container.ImageID,
		"image_name":    detail.Container.ImageName,
		"image_tag":     detail.Container.ImageTag,
		"status":        detail.Container.Status,
		"state":         detail.Container.State,
		"ports":         detail.Container.Ports,
		"labels":        detail.Container.Labels,
		"created_at":    detail.Container.CreatedAt,
		"started_at":    detail.Container.StartedAt,
		"updated_at":    detail.Container.UpdatedAt,
		"last_checked":  detail.Container.LastChecked,
		"image_stale":   detail.Container.ImageStale,
		"restart_count": detail.Container.RestartCount,
		"image_digest":  detail.Container.ImageDigest,
		"update_status": detail.Container.UpdateStatus,
		"host":          detail.Host,
	}
	if detail.DockerImages != nil {
		dockerImages := map[string]interface{}{
			"id":           detail.DockerImages.ID,
			"repository":   detail.DockerImages.Repository,
			"tag":          detail.DockerImages.Tag,
			"image_id":     detail.DockerImages.ImageID,
			"digest":       detail.DockerImages.Digest,
			"size_bytes":   detail.DockerImages.SizeBytes,
			"source":       detail.DockerImages.Source,
			"created_at":   detail.DockerImages.CreatedAt,
			"last_pulled":  detail.DockerImages.LastPulled,
			"last_checked": detail.DockerImages.LastChecked,
			"updated_at":   detail.DockerImages.UpdatedAt,

			"update_status":               detail.DockerImages.UpdateStatus,
			"update_check":                detail.DockerImages.UpdateCheck,
			"hasUpdates":                  detail.DockerImages.HasUpdates,
			"containers_needing_recreate": detail.DockerImages.ContainersNeedingRecreate,
		}
		containerResp["docker_images"] = dockerImages
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"container":         containerResp,
		"similarContainers": detail.SimilarContainers,
	})
}

// DeleteContainer handles DELETE /docker/containers/:id.
func (h *DockerHandler) DeleteContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Container ID is required")
		return
	}
	if err := h.docker.DeleteContainer(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete container")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Container deleted successfully",
	})
}

// ListImages handles GET /docker/images.
func (h *DockerHandler) ListImages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parseIntQuery(r, "page", 1)
	limit := parseIntQuery(r, "limit", 50)
	search := q.Get("search")
	if len(search) > 200 {
		search = search[:200]
	}
	updateStatus := q.Get("update_status")
	if updateStatus == "" && q.Get("hasUpdates") == "true" {
		updateStatus = store.ImageStatusUpdateAvailable
	}
	if updateStatus != "" && !imageUpdateStatusFilters[updateStatus] {
		Error(w, http.StatusBadRequest, "Invalid update_status filter")
		return
	}
	params := store.ImageListParams{
		Source:       q.Get("source"),
		Search:       search,
		UpdateStatus: updateStatus,
		Page:         page,
		Limit:        limit,
	}
	images, total, err := h.docker.ListImages(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch images")
		return
	}
	pages := (total + params.Limit - 1) / params.Limit
	if pages < 1 {
		pages = 1
	}
	// Build response with _count for frontend compatibility
	imgResp := make([]map[string]interface{}, len(images))
	for i, img := range images {
		imgResp[i] = map[string]interface{}{
			"id":           img.ID,
			"repository":   img.Repository,
			"tag":          img.Tag,
			"image_id":     img.ImageID,
			"digest":       img.Digest,
			"size_bytes":   img.SizeBytes,
			"source":       img.Source,
			"created_at":   img.CreatedAt,
			"last_pulled":  img.LastPulled,
			"last_checked": img.LastChecked,
			"updated_at":   img.UpdatedAt,
			"hasUpdates":   img.HasUpdates,
			"_count":       map[string]interface{}{"docker_containers": img.CountContainers},

			"update_status":               img.UpdateStatus,
			"update_check":                img.UpdateCheck,
			"containers_needing_recreate": img.ContainersNeedingRecreate,
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"images": imgResp,
		"pagination": map[string]interface{}{
			"page":       page,
			"limit":      params.Limit,
			"total":      total,
			"totalPages": pages,
		},
	})
}

// GetImage handles GET /docker/images/:id.
func (h *DockerHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Image ID is required")
		return
	}
	detail, err := h.docker.GetImage(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			Error(w, http.StatusNotFound, "Image not found")
			return
		}
		Error(w, http.StatusInternalServerError, "Failed to fetch image")
		return
	}
	JSON(w, http.StatusOK, detail)
}

// RunImageChecks handles POST /docker/image-checks/run.
func (h *DockerHandler) RunImageChecks(w http.ResponseWriter, r *http.Request) {
	hosts, err := h.docker.ListDockerHostAPIIDs(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to list Docker hosts")
		return
	}
	connected := map[string]bool{}
	if h.registry != nil {
		for _, apiID := range h.registry.GetConnectedApiIDs(hostctx.TenantHostKey(r.Context())) {
			connected[apiID] = true
		}
	}
	queued, skipped := 0, 0
	for _, host := range hosts {
		if !connected[host.ApiID] {
			skipped++
			continue
		}
		if err := h.registry.SendMessage(host.ApiID, websocket.TextMessage, dockerImageCheckCommand); err != nil {
			slog.Warn("docker image check: send failed", "host_id", host.HostID, "error", err)
			skipped++
			continue
		}
		queued++
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"queued":          queued,
		"skipped_offline": skipped,
	})
}

// DeleteImage handles DELETE /docker/images/:id.
func (h *DockerHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Image ID is required")
		return
	}
	inUse, err := h.docker.DeleteImage(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete image")
		return
	}
	if inUse > 0 {
		Error(w, http.StatusBadRequest, "Cannot delete image: "+strconv.Itoa(inUse)+" container(s) are using this image")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Image deleted successfully",
	})
}

// ListHosts handles GET /docker/hosts.
func (h *DockerHandler) ListHosts(w http.ResponseWriter, r *http.Request) {
	page := parseIntQuery(r, "page", 1)
	limit := parseIntQuery(r, "limit", 50)
	hosts, total, err := h.docker.ListHosts(r.Context(), page, limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch Docker hosts")
		return
	}
	pages := (total + limit - 1) / limit
	if pages < 1 {
		pages = 1
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
		"pagination": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": pages,
		},
	})
}

// GetHostDockerDetail handles GET /docker/hosts/:id.
func (h *DockerHandler) GetHostDockerDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Host ID is required")
		return
	}
	detail, err := h.docker.GetHostDockerDetail(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			Error(w, http.StatusNotFound, "Host not found")
			return
		}
		Error(w, http.StatusInternalServerError, "Failed to fetch host Docker detail")
		return
	}
	JSON(w, http.StatusOK, detail)
}

// ListVolumes handles GET /docker/volumes.
func (h *DockerHandler) ListVolumes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parseIntQuery(r, "page", 1)
	limit := parseIntQuery(r, "limit", 50)
	search := q.Get("search")
	if len(search) > 200 {
		search = search[:200]
	}
	params := store.VolumeListParams{
		Driver: q.Get("driver"),
		Search: search,
		Page:   page,
		Limit:  limit,
	}
	volumes, total, err := h.docker.ListVolumes(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch volumes")
		return
	}
	pages := (total + params.Limit - 1) / params.Limit
	if pages < 1 {
		pages = 1
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"volumes": volumes,
		"pagination": map[string]interface{}{
			"page":       page,
			"limit":      params.Limit,
			"total":      total,
			"totalPages": pages,
		},
	})
}

// GetVolume handles GET /docker/volumes/:id.
func (h *DockerHandler) GetVolume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Volume ID is required")
		return
	}
	detail, err := h.docker.GetVolume(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			Error(w, http.StatusNotFound, "Volume not found")
			return
		}
		Error(w, http.StatusInternalServerError, "Failed to fetch volume")
		return
	}
	JSON(w, http.StatusOK, detail)
}

// DeleteVolume handles DELETE /docker/volumes/:id.
func (h *DockerHandler) DeleteVolume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Volume ID is required")
		return
	}
	if err := h.docker.DeleteVolume(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete volume")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Volume deleted successfully",
	})
}

// ListNetworks handles GET /docker/networks.
func (h *DockerHandler) ListNetworks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := parseIntQuery(r, "page", 1)
	limit := parseIntQuery(r, "limit", 50)
	search := q.Get("search")
	if len(search) > 200 {
		search = search[:200]
	}
	params := store.NetworkListParams{
		Driver: q.Get("driver"),
		Search: search,
		Page:   page,
		Limit:  limit,
	}
	networks, total, err := h.docker.ListNetworks(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch networks")
		return
	}
	pages := (total + params.Limit - 1) / params.Limit
	if pages < 1 {
		pages = 1
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"networks": networks,
		"pagination": map[string]interface{}{
			"page":       page,
			"limit":      params.Limit,
			"total":      total,
			"totalPages": pages,
		},
	})
}

// GetNetwork handles GET /docker/networks/:id.
func (h *DockerHandler) GetNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Network ID is required")
		return
	}
	detail, err := h.docker.GetNetwork(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			Error(w, http.StatusNotFound, "Network not found")
			return
		}
		Error(w, http.StatusInternalServerError, "Failed to fetch network")
		return
	}
	JSON(w, http.StatusOK, detail)
}

// DeleteNetwork handles DELETE /docker/networks/:id.
func (h *DockerHandler) DeleteNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Network ID is required")
		return
	}
	if err := h.docker.DeleteNetwork(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete network")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Network deleted successfully",
	})
}
