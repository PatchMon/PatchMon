package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

// IntegrationsHandler handles agent integration endpoints (Docker, etc.)
type IntegrationsHandler struct {
	hosts             *store.HostsStore
	docker            *store.DockerStore
	integrationStatus *store.IntegrationStatusStore
	db                database.DBProvider
	notify            *notifications.Emitter
}

// NewIntegrationsHandler creates a new integrations handler.
func NewIntegrationsHandler(hosts *store.HostsStore, docker *store.DockerStore, integrationStatus *store.IntegrationStatusStore, db database.DBProvider, notify *notifications.Emitter) *IntegrationsHandler {
	return &IntegrationsHandler{
		hosts:             hosts,
		docker:            docker,
		integrationStatus: integrationStatus,
		db:                db,
		notify:            notify,
	}
}

// Docker payload structs - match agent JSON (snake_case)
type dockerContainerReq struct {
	ContainerID     string            `json:"container_id"`
	Name            string            `json:"name"`
	ImageName       string            `json:"image_name"`
	ImageTag        string            `json:"image_tag"`
	ImageRepository string            `json:"image_repository"`
	ImageSource     string            `json:"image_source"`
	ImageID         string            `json:"image_id"`
	Status          string            `json:"status"`
	State           string            `json:"state"`
	Ports           map[string]string `json:"ports,omitempty"`
	CreatedAt       *time.Time        `json:"created_at,omitempty"`
	StartedAt       *time.Time        `json:"started_at,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type dockerImageReq struct {
	Repository string     `json:"repository"`
	Tag        string     `json:"tag"`
	ImageID    string     `json:"image_id"`
	Source     string     `json:"source"`
	SizeBytes  int64      `json:"size_bytes"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	Digest     string     `json:"digest,omitempty"`
}

type dockerVolumeReq struct {
	VolumeID   string            `json:"volume_id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint,omitempty"`
	Renderer   string            `json:"renderer,omitempty"`
	Scope      string            `json:"scope"`
	Labels     map[string]string `json:"labels,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
	CreatedAt  *time.Time        `json:"created_at,omitempty"`
	SizeBytes  *int64            `json:"size_bytes,omitempty"`
	RefCount   int               `json:"ref_count,omitempty"`
}

type dockerNetworkReq struct {
	NetworkID      string            `json:"network_id"`
	Name           string            `json:"name"`
	Driver         string            `json:"driver"`
	Scope          string            `json:"scope"`
	IPv6Enabled    bool              `json:"ipv6_enabled"`
	Internal       bool              `json:"internal"`
	Attachable     bool              `json:"attachable"`
	Ingress        bool              `json:"ingress"`
	ConfigOnly     bool              `json:"config_only"`
	Labels         map[string]string `json:"labels,omitempty"`
	IPAM           interface{}       `json:"ipam,omitempty"`
	CreatedAt      *time.Time        `json:"created_at,omitempty"`
	ContainerCount int               `json:"container_count,omitempty"`
}

type dockerImageUpdateReq struct {
	Repository      string `json:"repository"`
	CurrentTag      string `json:"current_tag"`
	AvailableTag    string `json:"available_tag"`
	CurrentDigest   string `json:"current_digest"`
	AvailableDigest string `json:"available_digest"`
	ImageID         string `json:"image_id"`
}

type dockerPayloadReq struct {
	Containers []dockerContainerReq   `json:"containers"`
	Images     []dockerImageReq       `json:"images"`
	Volumes    []dockerVolumeReq      `json:"volumes"`
	Networks   []dockerNetworkReq     `json:"networks"`
	Updates    []dockerImageUpdateReq `json:"updates"`
	Hostname   string                 `json:"hostname"`
	MachineID  string                 `json:"machine_id"`
	// DockerHash is the agent-computed canonical hash for the docker
	// payload. Optional — when present, server validates and persists it
	// so the next ping can hash-gate the docker section.
	DockerHash string `json:"docker_hash,omitempty"`
}

// canonicalDockerHashFromReq derives the canonical hash inputs from the
// agent's wire payload by mapping each top-level slice through the wire-shape
// types accepted by CanonicalDockerHash.
func canonicalDockerHashFromReq(p *dockerPayloadReq) (string, error) {
	in := DockerHashInput{
		Containers: make([]DockerWireContainer, len(p.Containers)),
		Images:     make([]DockerWireImage, len(p.Images)),
		Volumes:    make([]DockerWireVolume, len(p.Volumes)),
		Networks:   make([]DockerWireNetwork, len(p.Networks)),
		Updates:    make([]DockerWireImageUpdate, len(p.Updates)),
	}
	for i, c := range p.Containers {
		in.Containers[i] = DockerWireContainer{
			ContainerID:     c.ContainerID,
			Name:            c.Name,
			ImageRepository: c.ImageRepository,
			ImageTag:        c.ImageTag,
			ImageID:         c.ImageID,
			Status:          c.Status,
			State:           c.State,
		}
	}
	for i, img := range p.Images {
		in.Images[i] = DockerWireImage{
			Repository: img.Repository,
			Tag:        img.Tag,
			ImageID:    img.ImageID,
			Digest:     img.Digest,
			SizeBytes:  img.SizeBytes,
		}
	}
	for i, v := range p.Volumes {
		in.Volumes[i] = DockerWireVolume{
			VolumeID: v.VolumeID,
			Name:     v.Name,
			Driver:   v.Driver,
			Scope:    v.Scope,
		}
	}
	for i, n := range p.Networks {
		in.Networks[i] = DockerWireNetwork{
			NetworkID: n.NetworkID,
			Name:      n.Name,
			Driver:    n.Driver,
			Scope:     n.Scope,
		}
	}
	for i, u := range p.Updates {
		in.Updates[i] = DockerWireImageUpdate(u)
	}
	return CanonicalDockerHash(in)
}

type dockerResponse struct {
	Message            string `json:"message"`
	ContainersReceived int    `json:"containers_received"`
	ImagesReceived     int    `json:"images_received"`
	VolumesReceived    int    `json:"volumes_received"`
	NetworksReceived   int    `json:"networks_received"`
	UpdatesFound       int    `json:"updates_found"`
}

func convertDockerPayloadToStore(p *dockerPayloadReq) *store.DockerReceivePayload {
	out := &store.DockerReceivePayload{
		Containers: make([]store.DockerReceiveContainer, len(p.Containers)),
		Images:     make([]store.DockerReceiveImage, len(p.Images)),
		Volumes:    make([]store.DockerReceiveVolume, len(p.Volumes)),
		Networks:   make([]store.DockerReceiveNetwork, len(p.Networks)),
		Updates:    make([]store.DockerReceiveImageUpdate, len(p.Updates)),
	}
	for i, c := range p.Containers {
		out.Containers[i] = store.DockerReceiveContainer{
			ContainerID:     c.ContainerID,
			Name:            c.Name,
			ImageName:       c.ImageName,
			ImageTag:        c.ImageTag,
			ImageRepository: c.ImageRepository,
			ImageSource:     c.ImageSource,
			ImageID:         c.ImageID,
			Status:          c.Status,
			State:           c.State,
			Ports:           c.Ports,
			CreatedAt:       c.CreatedAt,
			StartedAt:       c.StartedAt,
			Labels:          c.Labels,
		}
	}
	for i, img := range p.Images {
		out.Images[i] = store.DockerReceiveImage{
			Repository: img.Repository,
			Tag:        img.Tag,
			ImageID:    img.ImageID,
			Source:     img.Source,
			SizeBytes:  img.SizeBytes,
			CreatedAt:  img.CreatedAt,
			Digest:     img.Digest,
		}
	}
	for i, v := range p.Volumes {
		out.Volumes[i] = store.DockerReceiveVolume{
			VolumeID:   v.VolumeID,
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			Renderer:   v.Renderer,
			Scope:      v.Scope,
			Labels:     v.Labels,
			Options:    v.Options,
			CreatedAt:  v.CreatedAt,
			SizeBytes:  v.SizeBytes,
			RefCount:   v.RefCount,
		}
	}
	for i, n := range p.Networks {
		out.Networks[i] = store.DockerReceiveNetwork{
			NetworkID:      n.NetworkID,
			Name:           n.Name,
			Driver:         n.Driver,
			Scope:          n.Scope,
			IPv6Enabled:    n.IPv6Enabled,
			Internal:       n.Internal,
			Attachable:     n.Attachable,
			Ingress:        n.Ingress,
			ConfigOnly:     n.ConfigOnly,
			Labels:         n.Labels,
			IPAM:           n.IPAM,
			CreatedAt:      n.CreatedAt,
			ContainerCount: n.ContainerCount,
		}
	}
	for i, u := range p.Updates {
		out.Updates[i] = store.DockerReceiveImageUpdate{
			Repository:      u.Repository,
			CurrentTag:      u.CurrentTag,
			AvailableTag:    u.AvailableTag,
			CurrentDigest:   u.CurrentDigest,
			AvailableDigest: u.AvailableDigest,
			ImageID:         u.ImageID,
		}
	}
	return out
}

// ReceiveDockerData handles POST /api/v1/integrations/docker (agent endpoint, API key auth).
func (h *IntegrationsHandler) ReceiveDockerData(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("integrations docker handler panic", "error", err, "stack", string(debug.Stack()))
			JSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal Server Error"})
		}
	}()

	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	var payload dockerPayloadReq
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	slog.Info("received docker data from agent", "host", host.FriendlyName, "host_id", host.ID,
		"containers", len(payload.Containers), "images", len(payload.Images),
		"volumes", len(payload.Volumes), "networks", len(payload.Networks), "updates", len(payload.Updates))

	// Verify the agent's canonical docker hash if present. Mismatch means
	// the agent's canonicalisation diverged from ours — surface immediately
	// rather than silently storing a hash the next ping will reject.
	computedHash, hashErr := canonicalDockerHashFromReq(&payload)
	if hashErr != nil {
		slog.Error("failed to compute canonical docker hash", "error", hashErr, "host_id", host.ID)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate Docker payload"})
		return
	}
	if payload.DockerHash != "" && computedHash != payload.DockerHash {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "docker hash mismatch"})
		return
	}

	storePayload := convertDockerPayloadToStore(&payload)
	result, err := h.docker.ReceiveDockerData(r.Context(), host.ID, storePayload)
	if err != nil {
		slog.Error("failed to process docker data", "error", err, "host_id", host.ID)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to process Docker data"})
		return
	}

	// Persist the canonical hash so the next ping can hash-gate the docker
	// section. If the call fails we log but do not fail the request — data
	// landed; the worst case is an extra docker upload next cycle.
	if err := h.hosts.UpdateDockerHash(r.Context(), host.ID, computedHash); err != nil {
		slog.Error("failed to persist docker hash", "error", err, "host_id", host.ID)
	}

	// Emit notification events for container status transitions.
	if h.notify != nil && len(result.StatusChanges) > 0 {
		d := h.db.DB(r.Context())
		tenantHost := hostctx.TenantHostKey(r.Context())
		hostName := host.FriendlyName
		if hostName == "" && host.Hostname != nil {
			hostName = *host.Hostname
		}
		for _, sc := range result.StatusChanges {
			eventType := "container_stopped"
			if isContainerRunning(sc.NewStatus) {
				eventType = "container_started"
			}
			containerLabel := sc.Name
			if containerLabel == "" {
				containerLabel = sc.ContainerID[:12]
			}
			title := fmt.Sprintf("Container %s %s", containerLabel, eventType[len("container_"):])
			msg := fmt.Sprintf("Container \"%s\" (%s) on host \"%s\" changed from %s to %s.",
				containerLabel, sc.ImageName, hostName, sc.OldStatus, sc.NewStatus)

			h.notify.EmitEvent(r.Context(), d, tenantHost, notifications.Event{
				Type:          eventType,
				Severity:      "informational",
				Title:         title,
				Message:       msg,
				ReferenceType: "host",
				ReferenceID:   host.ID,
				Metadata: map[string]interface{}{
					"host_id":        host.ID,
					"host_name":      hostName,
					"container_id":   sc.ContainerID,
					"container_name": sc.Name,
					"image_name":     sc.ImageName,
					"old_status":     sc.OldStatus,
					"new_status":     sc.NewStatus,
				},
			})
		}
	}

	// Emit container_image_update_available when new image updates are detected.
	if h.notify != nil && result.UpdatesFound > 0 {
		d := h.db.DB(r.Context())
		tenantHost := hostctx.TenantHostKey(r.Context())
		hostName := host.FriendlyName
		if hostName == "" && host.Hostname != nil {
			hostName = *host.Hostname
		}
		h.notify.EmitEvent(r.Context(), d, tenantHost, notifications.Event{
			Type:          "container_image_update_available",
			Severity:      "informational",
			Title:         fmt.Sprintf("Docker Image Updates - %s", hostName),
			Message:       fmt.Sprintf("%d Docker image update(s) detected on host \"%s\".", result.UpdatesFound, hostName),
			ReferenceType: "host",
			ReferenceID:   host.ID,
			Metadata: map[string]interface{}{
				"host_id":       host.ID,
				"host_name":     hostName,
				"updates_found": result.UpdatesFound,
			},
		})
	}

	JSON(w, http.StatusOK, dockerResponse{
		Message:            "Docker data collected successfully",
		ContainersReceived: result.ContainersReceived,
		ImagesReceived:     result.ImagesReceived,
		VolumesReceived:    result.VolumesReceived,
		NetworksReceived:   result.NetworksReceived,
		UpdatesFound:       result.UpdatesFound,
	})
}

func isContainerRunning(status string) bool {
	return status == "running" || status == "restarting"
}

// integrationStatusReq matches agent IntegrationSetupStatus payload.
type integrationStatusReq struct {
	Integration   string                   `json:"integration"`
	Enabled       bool                     `json:"enabled"`
	Status        string                   `json:"status"`
	Message       string                   `json:"message"`
	Components    map[string]interface{}   `json:"components"`
	ScannerInfo   interface{}              `json:"scanner_info"`
	InstallEvents []map[string]interface{} `json:"install_events"`
}

// ReceiveIntegrationStatus handles POST /hosts/integration-status (agent reports integration setup status).
func (h *IntegrationsHandler) ReceiveIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	var payload integrationStatusReq
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	statusData := map[string]interface{}{
		"integration":    payload.Integration,
		"enabled":        payload.Enabled,
		"status":         payload.Status,
		"message":        payload.Message,
		"components":     payload.Components,
		"scanner_info":   payload.ScannerInfo,
		"install_events": payload.InstallEvents,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	}
	if statusData["components"] == nil {
		statusData["components"] = map[string]interface{}{}
	}
	if statusData["install_events"] == nil {
		statusData["install_events"] = []interface{}{}
	}

	if h.integrationStatus != nil {
		_ = h.integrationStatus.Set(r.Context(), apiID, payload.Integration, statusData)
	}

	if payload.Integration == "compliance" {
		statusJSON, _ := json.Marshal(statusData)
		_ = h.hosts.UpdateComplianceScannerStatus(r.Context(), host.ID, statusJSON, time.Now())
		if payload.Status == "ready" {
			_ = h.hosts.UpdateComplianceEnabled(r.Context(), host.ID, payload.Enabled)
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Integration status received",
	})
}

// AgentGetIntegrationStatus handles GET /hosts/integrations (agent endpoint, API key auth).
// Returns integration status for the agent to sync on startup.
func (h *IntegrationsHandler) AgentGetIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API credentials required"})
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	complianceMode := "disabled"
	if host.ComplianceEnabled {
		if host.ComplianceOnDemandOnly {
			complianceMode = "on-demand"
		} else {
			complianceMode = "enabled"
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success":                         true,
		"integrations":                    map[string]bool{"docker": host.DockerEnabled, "compliance": host.ComplianceEnabled},
		"compliance_mode":                 complianceMode,
		"compliance_openscap_enabled":     host.ComplianceOpenscapEnabled,
		"compliance_docker_bench_enabled": host.ComplianceDockerBenchEnabled,
	})
}
