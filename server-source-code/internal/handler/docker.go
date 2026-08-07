package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// DockerHandler handles Docker inventory routes.
type DockerHandler struct {
	docker *store.DockerStore
}

// NewDockerHandler creates a new Docker handler.
func NewDockerHandler(docker *store.DockerStore) *DockerHandler {
	return &DockerHandler{docker: docker}
}

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
		"id":           detail.Container.ID,
		"host_id":      detail.Container.HostID,
		"container_id": detail.Container.ContainerID,
		"name":         detail.Container.Name,
		"image_id":     detail.Container.ImageID,
		"image_name":   detail.Container.ImageName,
		"image_tag":    detail.Container.ImageTag,
		"status":       detail.Container.Status,
		"state":        detail.Container.State,
		"ports":        detail.Container.Ports,
		"labels":       detail.Container.Labels,
		"created_at":   detail.Container.CreatedAt,
		"started_at":   detail.Container.StartedAt,
		"updated_at":   detail.Container.UpdatedAt,
		"last_checked": detail.Container.LastChecked,
		"host":         detail.Host,
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
		}
		if len(detail.DockerImageUpdates) > 0 {
			dockerImages["docker_image_updates"] = detail.DockerImageUpdates
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
	params := store.ImageListParams{
		Source: q.Get("source"),
		Search: search,
		Page:   page,
		Limit:  limit,
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
	JSON(w, http.StatusOK, map[string]interface{}{
		"image":           detail.Image,
		"hosts":           detail.Hosts,
		"totalContainers": detail.TotalContainers,
		"totalHosts":      detail.TotalHosts,
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
