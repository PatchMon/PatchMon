package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"golang.org/x/crypto/bcrypt"
)

// HostsHandler handles hosts routes.
type HostsHandler struct {
	hosts             *store.HostsStore
	hostGroups        *store.HostGroupsStore
	settings          *store.SettingsStore
	queueClient       *asynq.Client
	registry          *agentregistry.Registry
	integrationStatus *store.IntegrationStatusStore
	pendingConfig     *store.PendingConfigStore
	db                database.DBProvider
	notify            *notifications.Emitter
}

// NewHostsHandler creates a new hosts handler.
func NewHostsHandler(hosts *store.HostsStore, hostGroups *store.HostGroupsStore, settings *store.SettingsStore, queueClient *asynq.Client, registry *agentregistry.Registry, integrationStatus *store.IntegrationStatusStore, pendingConfig *store.PendingConfigStore, db database.DBProvider, notify *notifications.Emitter) *HostsHandler {
	return &HostsHandler{
		hosts:             hosts,
		hostGroups:        hostGroups,
		settings:          settings,
		queueClient:       queueClient,
		registry:          registry,
		integrationStatus: integrationStatus,
		pendingConfig:     pendingConfig,
		db:                db,
		notify:            notify,
	}
}

// List handles GET /hosts.
func (h *HostsHandler) List(w http.ResponseWriter, r *http.Request) {
	hosts, err := h.hosts.List(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load hosts")
		return
	}
	JSON(w, http.StatusOK, hosts)
}

// AdminList handles GET /hosts/admin/list.
func (h *HostsHandler) AdminList(w http.ResponseWriter, r *http.Request) {
	returnAll := r.URL.Query().Get("all") == "true"
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "pageSize", 100)
	if pageSize > 500 {
		pageSize = 500
	}
	if returnAll {
		pageSize = 10000
	}
	offset := (page - 1) * pageSize

	hosts, err := h.hosts.ListPaginated(r.Context(), pageSize, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load hosts")
		return
	}
	total, _ := h.hosts.Count(r.Context())

	// Batch-fetch host groups for all hosts
	hostIDs := make([]string, len(hosts))
	for i := range hosts {
		hostIDs[i] = hosts[i].ID
	}
	groupsByHost, _ := h.hosts.GetHostGroupsForHosts(r.Context(), hostIDs)

	// Enrich with host groups
	data := make([]map[string]interface{}, len(hosts))
	for i, host := range hosts {
		groups := groupsByHost[host.ID]
		if groups == nil {
			groups = []models.HostGroup{}
		}
		data[i] = hostToResponse(&host, groups)
	}

	totalPages := 1
	if !returnAll && pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if totalPages < 1 {
			totalPages = 1
		}
	}
	if returnAll {
		pageSize = total
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"data": data,
		"pagination": map[string]interface{}{
			"total": total, "page": page, "pageSize": pageSize, "totalPages": totalPages,
		},
	})
}

// GetByID handles GET /hosts/:hostId.
func (h *HostsHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	groups, _ := h.hosts.GetHostGroups(r.Context(), host.ID)
	JSON(w, http.StatusOK, hostToResponse(host, groups))
}

// Create handles POST /hosts/create.
func (h *HostsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FriendlyName      string   `json:"friendly_name"`
		HostGroupIds      []string `json:"hostGroupIds"`
		DockerEnabled     *bool    `json:"docker_enabled"`
		ComplianceEnabled *bool    `json:"compliance_enabled"`
		ExpectedPlatform  *string  `json:"expected_platform"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.FriendlyName == "" {
		Error(w, http.StatusBadRequest, "Friendly name is required")
		return
	}

	apiID, apiKey, apiKeyHash, err := generateApiCredentials()
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to generate credentials")
		return
	}

	complianceEnabled := false
	complianceOnDemandOnly := true
	if req.ComplianceEnabled != nil {
		complianceEnabled = *req.ComplianceEnabled
		complianceOnDemandOnly = !complianceEnabled
	}
	s, _ := h.settings.GetFirst(r.Context())
	if s != nil && s.DefaultComplianceMode == "enabled" {
		complianceOnDemandOnly = false
	}

	// Enforce host limit if a package is applied.
	if entry := hostctx.EntryFromContext(r.Context()); entry != nil && entry.MaxHosts != nil {
		count, countErr := h.hosts.Count(r.Context())
		if countErr == nil && count >= *entry.MaxHosts {
			Error(w, http.StatusForbidden, "Host limit reached for this host's package")
			return
		}
	}

	machineID := "pending-" + uuid.New().String()
	host := &models.Host{
		MachineID:              &machineID,
		FriendlyName:           req.FriendlyName,
		OSType:                 "unknown",
		OSVersion:              "unknown",
		Status:                 "pending",
		ApiID:                  apiID,
		ApiKey:                 apiKeyHash,
		DockerEnabled:          req.DockerEnabled != nil && *req.DockerEnabled,
		ComplianceEnabled:      complianceEnabled,
		ComplianceOnDemandOnly: complianceOnDemandOnly,
		ExpectedPlatform:       req.ExpectedPlatform,
	}
	if err := h.hosts.Create(r.Context(), host); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create host")
		return
	}

	if len(req.HostGroupIds) > 0 {
		_ = h.hosts.SetHostGroups(r.Context(), host.ID, req.HostGroupIds)
	}

	groups, _ := h.hosts.GetHostGroups(r.Context(), host.ID)
	hostGroupsResp := make([]map[string]interface{}, len(groups))
	for i, g := range groups {
		hostGroupsResp[i] = map[string]interface{}{"id": g.ID, "name": g.Name, "color": g.Color}
	}

	// Emit host_enrolled event.
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "host_enrolled",
				Severity:      "informational",
				Title:         fmt.Sprintf("Host Enrolled - %s", host.FriendlyName),
				Message:       fmt.Sprintf("New host \"%s\" has been enrolled.", host.FriendlyName),
				ReferenceType: "host",
				ReferenceID:   host.ID,
				Metadata: map[string]interface{}{
					"host_id":   host.ID,
					"host_name": host.FriendlyName,
				},
			})
		}
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"message":      "Host created successfully",
		"hostId":       host.ID,
		"friendlyName": host.FriendlyName,
		"apiId":        apiID,
		"apiKey":       apiKey,
		"hostGroups":   hostGroupsResp,
		"instructions": "Use these credentials in your patchmon agent configuration. System information will be automatically detected when the agent connects.",
	})
}

// UpdateGroups handles PUT /hosts/:hostId/groups.
func (h *HostsHandler) UpdateGroups(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		GroupIds []string `json:"groupIds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	groupIds := req.GroupIds
	if groupIds == nil {
		groupIds = []string{}
	}

	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}

	for _, gid := range groupIds {
		_, err = h.hostGroups.GetByID(r.Context(), gid)
		if err != nil {
			Error(w, http.StatusBadRequest, "One or more host groups not found")
			return
		}
	}

	if err := h.hosts.SetHostGroups(r.Context(), hostID, groupIds); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update host groups")
		return
	}

	host, _ = h.hosts.GetByID(r.Context(), hostID)
	groups, _ := h.hosts.GetHostGroups(r.Context(), hostID)
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Host groups updated successfully",
		"host":    hostToResponse(host, groups),
	})
}

// UpdateFriendlyName handles PATCH /hosts/:hostId/friendly-name.
func (h *HostsHandler) UpdateFriendlyName(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		FriendlyName string `json:"friendly_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.FriendlyName == "" {
		Error(w, http.StatusBadRequest, "Friendly name must be between 1 and 100 characters")
		return
	}

	if err := h.hosts.UpdateFriendlyName(r.Context(), hostID, req.FriendlyName); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update friendly name")
		return
	}
	host, _ := h.hosts.GetByID(r.Context(), hostID)
	groups, _ := h.hosts.GetHostGroups(r.Context(), hostID)
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Friendly name updated successfully",
		"host":    hostToResponse(host, groups),
	})
}

// UpdateNotes handles PATCH /hosts/:hostId/notes.
func (h *HostsHandler) UpdateNotes(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		Notes *string `json:"notes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.hosts.UpdateNotes(r.Context(), hostID, req.Notes); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update notes")
		return
	}
	host, _ := h.hosts.GetByID(r.Context(), hostID)
	groups, _ := h.hosts.GetHostGroups(r.Context(), hostID)
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Notes updated successfully",
		"host":    hostToResponse(host, groups),
	})
}

// UpdateConnection handles PATCH /hosts/:hostId/connection.
func (h *HostsHandler) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		IP       *string `json:"ip"`
		Hostname *string `json:"hostname"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.hosts.UpdateConnection(r.Context(), hostID, req.IP, req.Hostname); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update connection")
		return
	}
	host, _ := h.hosts.GetByID(r.Context(), hostID)
	groups, _ := h.hosts.GetHostGroups(r.Context(), hostID)
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Host connection information updated successfully",
		"host":    hostToResponse(host, groups),
	})
}

// SetPrimaryInterface handles PATCH /hosts/:hostId/primary-interface.
func (h *HostsHandler) SetPrimaryInterface(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		InterfaceName *string `json:"interface_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}

	if err := h.hosts.SetPrimaryInterface(r.Context(), hostID, req.InterfaceName); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to set primary interface")
		return
	}

	// Re-derive hosts.ip from the primary interface when one is set
	if req.InterfaceName != nil && *req.InterfaceName != "" && len(host.NetworkInterfaces) > 0 {
		derivedIP := store.ExtractIPFromInterface(host.NetworkInterfaces, *req.InterfaceName)
		if derivedIP != "" {
			_ = h.hosts.UpdateConnection(r.Context(), hostID, &derivedIP, nil)
		}
	}

	host, _ = h.hosts.GetByID(r.Context(), hostID)
	groups, _ := h.hosts.GetHostGroups(r.Context(), hostID)
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Primary interface updated successfully",
		"host":    hostToResponse(host, groups),
	})
}

// UpdateHostDownAlerts handles PATCH /hosts/:hostId/host-down-alerts.
func (h *HostsHandler) UpdateHostDownAlerts(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		HostDownAlertsEnabled *bool `json:"host_down_alerts_enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.hosts.UpdateHostDownAlerts(r.Context(), hostID, req.HostDownAlertsEnabled); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update host down alerts")
		return
	}
	host, _ := h.hosts.GetByID(r.Context(), hostID)
	statusMsg := "inherit from global settings"
	if host != nil && host.HostDownAlertsEnabled != nil {
		if *host.HostDownAlertsEnabled {
			statusMsg = "enabled"
		} else {
			statusMsg = "disabled"
		}
	}
	hostResp := map[string]interface{}{"id": hostID, "hostDownAlertsEnabled": req.HostDownAlertsEnabled}
	if host != nil {
		hostResp["friendlyName"] = host.FriendlyName
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Host down alerts " + statusMsg + " successfully",
		"host":    hostResp,
	})
}

// UpdateAutoUpdate handles PATCH /hosts/:hostId/auto-update.
func (h *HostsHandler) UpdateAutoUpdate(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		AutoUpdate bool `json:"auto_update"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.hosts.UpdateAutoUpdate(r.Context(), hostID, req.AutoUpdate); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update auto-update")
		return
	}
	host, _ := h.hosts.GetByID(r.Context(), hostID)
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Agent auto-update " + map[bool]string{true: "enabled", false: "disabled"}[req.AutoUpdate] + " successfully",
		"host":    map[string]interface{}{"id": host.ID, "friendlyName": host.FriendlyName, "autoUpdate": req.AutoUpdate},
	})
}

// RegenerateCredentials handles POST /hosts/:hostId/regenerate-credentials.
func (h *HostsHandler) RegenerateCredentials(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}

	apiID, apiKey, apiKeyHash, err := generateApiCredentials()
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to regenerate credentials")
		return
	}

	if err := h.hosts.UpdateApiCredentials(r.Context(), hostID, apiID, apiKeyHash); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to regenerate credentials")
		return
	}

	hostname := ""
	if host.Hostname != nil {
		hostname = *host.Hostname
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":  "API credentials regenerated successfully",
		"hostname": hostname,
		"apiId":    apiID,
		"apiKey":   apiKey,
		"warning":  "Previous credentials are now invalid. Update your agent configuration.",
	})
}

// FetchReport handles POST /hosts/:hostId/fetch-report.
func (h *HostsHandler) FetchReport(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	task, err := queue.NewReportNowTask(host.ApiID, hostFromRequest(r))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create fetch report task")
		return
	}
	if _, err := h.queueClient.Enqueue(task); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to enqueue fetch report")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Fetch report requested",
		"success": true,
	})
}

// FetchReportBulk handles POST /hosts/bulk/fetch-report.
func (h *HostsHandler) FetchReportBulk(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	var req struct {
		HostIDs []string `json:"hostIds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.HostIDs) == 0 {
		Error(w, http.StatusBadRequest, "hostIds required")
		return
	}
	enqueued := 0
	for _, hostID := range req.HostIDs {
		host, err := h.hosts.GetByID(r.Context(), hostID)
		if err != nil || host == nil {
			continue
		}
		task, err := queue.NewReportNowTask(host.ApiID, hostFromRequest(r))
		if err != nil {
			continue
		}
		if _, err := h.queueClient.Enqueue(task); err != nil {
			continue
		}
		enqueued++
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Fetch report requested",
		"success":  true,
		"enqueued": enqueued,
	})
}

// RefreshIntegrationStatus handles POST /hosts/:hostId/refresh-integration-status.
func (h *HostsHandler) RefreshIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	task, err := queue.NewRefreshIntegrationStatusTask(host.ApiID, hostFromRequest(r))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create refresh integration status task")
		return
	}
	info, err := h.queueClient.Enqueue(task)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to refresh integration status")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Integration status refresh queued",
		"jobId":   info.ID,
		"host": map[string]interface{}{
			"id":           host.ID,
			"friendlyName": host.FriendlyName,
			"apiId":        host.ApiID,
		},
	})
}

// RefreshDocker handles POST /hosts/:hostId/refresh-docker.
func (h *HostsHandler) RefreshDocker(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	task, err := queue.NewDockerInventoryRefreshTask(host.ApiID, hostFromRequest(r))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create Docker refresh task")
		return
	}
	info, err := h.queueClient.Enqueue(task)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to refresh Docker inventory")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Docker inventory refresh queued",
		"jobId":   info.ID,
		"host": map[string]interface{}{
			"id":           host.ID,
			"friendlyName": host.FriendlyName,
			"apiId":        host.ApiID,
		},
	})
}

// ForceAgentUpdate handles POST /hosts/:hostId/force-agent-update.
func (h *HostsHandler) ForceAgentUpdate(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	task, err := queue.NewUpdateAgentTask(host.ApiID, hostFromRequest(r), true) // bypass_settings=true for force update
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create agent update task")
		return
	}
	info, err := h.queueClient.Enqueue(task)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to queue agent update")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Agent update queued successfully",
		"jobId":   info.ID,
		"host": map[string]interface{}{
			"id":           host.ID,
			"friendlyName": host.FriendlyName,
			"apiId":        host.ApiID,
		},
	})
}

// Delete handles DELETE /hosts/:hostId.
func (h *HostsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}

	if err := h.hosts.Delete(r.Context(), hostID); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete host")
		return
	}

	// Emit host_deleted event.
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			hostName := host.FriendlyName
			if hostName == "" && host.Hostname != nil {
				hostName = *host.Hostname
			}
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "host_deleted",
				Severity:      "warning",
				Title:         fmt.Sprintf("Host Deleted - %s", hostName),
				Message:       fmt.Sprintf("Host \"%s\" has been removed from inventory.", hostName),
				ReferenceType: "host",
				ReferenceID:   host.ID,
				Metadata: map[string]interface{}{
					"host_id":   host.ID,
					"host_name": hostName,
				},
			})
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":     "Host deleted successfully",
		"deletedHost": map[string]interface{}{"id": host.ID, "friendly_name": host.FriendlyName},
	})
}

// BulkDelete handles DELETE /hosts/bulk.
func (h *HostsHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostIds []string `json:"hostIds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.HostIds) == 0 {
		Error(w, http.StatusBadRequest, "At least one host ID is required")
		return
	}

	// Verify all exist
	for _, id := range req.HostIds {
		_, err := h.hosts.GetByID(r.Context(), id)
		if err != nil {
			Error(w, http.StatusNotFound, "Some hosts not found")
			return
		}
	}

	n, err := h.hosts.DeleteMany(r.Context(), req.HostIds)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete hosts")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":        fmt.Sprintf("%d host(s) deleted successfully", n),
		"deletedCount":   n,
		"requestedCount": len(req.HostIds),
	})
}

// BulkUpdateGroups handles PUT /hosts/bulk/groups.
func (h *HostsHandler) BulkUpdateGroups(w http.ResponseWriter, r *http.Request) {
	var req struct {
		HostIds  []string `json:"hostIds"`
		GroupIds []string `json:"groupIds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.HostIds) == 0 {
		Error(w, http.StatusBadRequest, "Host IDs must be an array")
		return
	}
	groupIds := req.GroupIds
	if groupIds == nil {
		groupIds = []string{}
	}

	for _, gid := range groupIds {
		_, err := h.hostGroups.GetByID(r.Context(), gid)
		if err != nil {
			Error(w, http.StatusBadRequest, "One or more host groups not found")
			return
		}
	}

	if err := h.hosts.SetHostGroupsBulk(r.Context(), req.HostIds, groupIds); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update host groups")
		return
	}

	hostsList, _ := h.hosts.GetByIDs(r.Context(), req.HostIds)
	groupsByHost, _ := h.hosts.GetHostGroupsForHosts(r.Context(), req.HostIds)
	hosts := make([]map[string]interface{}, len(req.HostIds))
	hostByID := make(map[string]*models.Host)
	for i := range hostsList {
		hostByID[hostsList[i].ID] = &hostsList[i]
	}
	for i, hid := range req.HostIds {
		if host := hostByID[hid]; host != nil {
			groups := groupsByHost[hid]
			if groups == nil {
				groups = []models.HostGroup{}
			}
			hosts[i] = hostToResponse(host, groups)
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":      fmt.Sprintf("Successfully updated %d host(s)", len(req.HostIds)),
		"updatedCount": len(req.HostIds),
		"hosts":        hosts,
	})
}

// GetIntegrations handles GET /hosts/:hostId/integrations.
func (h *HostsHandler) GetIntegrations(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	connected := h.registry != nil && h.registry.GetConnection(host.ApiID) != nil

	// Merge pending config with host for effective display values
	dockerEnabled := host.DockerEnabled
	complianceEnabled := host.ComplianceEnabled
	complianceOnDemandOnly := host.ComplianceOnDemandOnly
	openscapEnabled := host.ComplianceOpenscapEnabled
	dockerBenchEnabled := host.ComplianceDockerBenchEnabled

	pending, _ := h.pendingConfig.GetPendingConfig(r.Context(), hostID)
	pendingConfigMap := make(map[string]interface{})
	if pending != nil {
		if pending.DockerEnabled != nil {
			dockerEnabled = *pending.DockerEnabled
			pendingConfigMap["docker_enabled"] = *pending.DockerEnabled
		}
		if pending.ComplianceEnabled != nil {
			complianceEnabled = *pending.ComplianceEnabled
			pendingConfigMap["compliance_enabled"] = *pending.ComplianceEnabled
		}
		if pending.ComplianceOnDemandOnly != nil {
			complianceOnDemandOnly = *pending.ComplianceOnDemandOnly
			pendingConfigMap["compliance_on_demand_only"] = *pending.ComplianceOnDemandOnly
		}
		if pending.ComplianceOpenscapEnabled != nil {
			openscapEnabled = *pending.ComplianceOpenscapEnabled
			pendingConfigMap["compliance_openscap_enabled"] = *pending.ComplianceOpenscapEnabled
		}
		if pending.ComplianceDockerBenchEnabled != nil {
			dockerBenchEnabled = *pending.ComplianceDockerBenchEnabled
			pendingConfigMap["compliance_docker_bench_enabled"] = *pending.ComplianceDockerBenchEnabled
		}
	}

	complianceMode := "disabled"
	if complianceEnabled {
		if complianceOnDemandOnly {
			complianceMode = "on-demand"
		} else {
			complianceMode = "enabled"
		}
	}
	if pending != nil && (pending.ComplianceEnabled != nil || pending.ComplianceOnDemandOnly != nil) {
		pendingConfigMap["compliance_mode"] = complianceMode
	}
	if pending != nil && (pending.DockerEnabled != nil) {
		pendingConfigMap["docker_enabled"] = dockerEnabled
	}

	if complianceEnabled {
		if complianceOnDemandOnly {
			complianceMode = "on-demand"
		} else {
			complianceMode = "enabled"
		}
	}

	resp := map[string]interface{}{
		"success":                         true,
		"compliance_mode":                 complianceMode,
		"compliance_on_demand_only":       complianceOnDemandOnly,
		"compliance_openscap_enabled":     openscapEnabled,
		"compliance_docker_bench_enabled": dockerBenchEnabled,
		"compliance_default_profile_id":   host.ComplianceDefaultProfileID,
		"pending_config_exists":           pending != nil,
		"pending_config":                  pendingConfigMap,
		"data": map[string]interface{}{
			"integrations": map[string]interface{}{
				"docker":     dockerEnabled,
				"compliance": complianceEnabled,
			},
			"connected":                       connected,
			"compliance_mode":                 complianceMode,
			"compliance_openscap_enabled":     openscapEnabled,
			"compliance_docker_bench_enabled": dockerBenchEnabled,
			"compliance_default_profile_id":   host.ComplianceDefaultProfileID,
			"host": map[string]interface{}{
				"id":           host.ID,
				"friendlyName": host.FriendlyName,
				"apiId":        host.ApiID,
			},
		},
	}
	JSON(w, http.StatusOK, resp)
}

// GetIntegrationStatus handles GET /hosts/:hostId/integrations/:integrationName/status.
// For compliance: returns Redis if present, else fallback to host.compliance_scanner_status.
func (h *HostsHandler) GetIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	integrationName := chi.URLParam(r, "integrationName")
	if integrationName != "compliance" && integrationName != "docker" {
		Error(w, http.StatusBadRequest, "Invalid integration name")
		return
	}
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	if h.integrationStatus != nil {
		status, err := h.integrationStatus.Get(r.Context(), host.ApiID, integrationName)
		if err == nil && status != nil {
			JSON(w, http.StatusOK, map[string]interface{}{
				"success": true,
				"status":  status,
				"source":  "live",
			})
			return
		}
	}
	if integrationName == "compliance" && host.ComplianceScannerStatus != nil && len(host.ComplianceScannerStatus) > 0 {
		var status map[string]interface{}
		if err := json.Unmarshal(host.ComplianceScannerStatus, &status); err == nil {
			resp := map[string]interface{}{
				"success": true,
				"status":  status,
				"source":  "cached",
			}
			if host.ComplianceScannerUpdatedAt != nil {
				resp["cached_at"] = host.ComplianceScannerUpdatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			JSON(w, http.StatusOK, resp)
			return
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"status":  nil,
		"message": "No status available",
	})
}

// RequestComplianceStatus handles POST /hosts/:hostId/integrations/compliance/request-status.
func (h *HostsHandler) RequestComplianceStatus(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	task, err := queue.NewRefreshIntegrationStatusTask(host.ApiID, hostFromRequest(r))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create refresh integration status task")
		return
	}
	if _, err := h.queueClient.Enqueue(task); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to request compliance status")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Compliance status refresh requested",
	})
}

// SetComplianceMode handles POST /hosts/:hostId/integrations/compliance/mode.
// Stores change as pending; user must apply via ApplyPendingConfig.
func (h *HostsHandler) SetComplianceMode(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	validModes := map[string]bool{"disabled": true, "on-demand": true, "enabled": true}
	if !validModes[req.Mode] {
		Error(w, http.StatusBadRequest, "mode must be one of: disabled, on-demand, enabled")
		return
	}
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	complianceEnabled := req.Mode != "disabled"
	complianceOnDemandOnly := req.Mode == "on-demand"
	if err := h.pendingConfig.SetPendingConfig(r.Context(), hostID, store.PendingConfigFields{
		ComplianceEnabled:      &complianceEnabled,
		ComplianceOnDemandOnly: &complianceOnDemandOnly,
	}); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to store pending compliance mode")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Compliance mode set to %s (pending apply)", req.Mode),
		"data": map[string]interface{}{
			"mode": req.Mode,
			"host": map[string]interface{}{
				"id":           host.ID,
				"friendlyName": host.FriendlyName,
				"apiId":        host.ApiID,
			},
		},
	})
}

// SetComplianceScanners handles POST /hosts/:hostId/integrations/compliance/scanners.
// Stores change as pending; user must apply via ApplyPendingConfig.
func (h *HostsHandler) SetComplianceScanners(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		OpenscapEnabled    *bool `json:"openscap_enabled"`
		DockerBenchEnabled *bool `json:"docker_bench_enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.OpenscapEnabled == nil && req.DockerBenchEnabled == nil {
		Error(w, http.StatusBadRequest, "At least one of openscap_enabled or docker_bench_enabled must be provided")
		return
	}
	_, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	fields := store.PendingConfigFields{}
	if req.OpenscapEnabled != nil {
		fields.ComplianceOpenscapEnabled = req.OpenscapEnabled
	}
	if req.DockerBenchEnabled != nil {
		fields.ComplianceDockerBenchEnabled = req.DockerBenchEnabled
	}
	if err := h.pendingConfig.SetPendingConfig(r.Context(), hostID, fields); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to store pending scanner settings")
		return
	}
	resp := map[string]interface{}{}
	if req.OpenscapEnabled != nil {
		resp["openscap_enabled"] = *req.OpenscapEnabled
	}
	if req.DockerBenchEnabled != nil {
		resp["docker_bench_enabled"] = *req.DockerBenchEnabled
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Scanner settings updated (pending apply)",
		"data":    resp,
	})
}

// SetComplianceDefaultProfile handles POST /hosts/:hostId/integrations/compliance/default-profile.
func (h *HostsHandler) SetComplianceDefaultProfile(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	var req struct {
		ProfileID *string `json:"profile_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	_, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	if err := h.hosts.UpdateComplianceDefaultProfile(r.Context(), hostID, req.ProfileID); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to save default profile")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message":    "Default compliance profile updated",
		"profile_id": req.ProfileID,
	})
}

// ToggleIntegrationRequest is the request body for POST /hosts/:hostId/integrations/:integrationName/toggle.
type ToggleIntegrationRequest struct {
	Enabled bool `json:"enabled"`
}

// ToggleIntegration handles POST /hosts/:hostId/integrations/:integrationName/toggle.
// Stores change as pending; user must apply via ApplyPendingConfig.
func (h *HostsHandler) ToggleIntegration(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	integrationName := chi.URLParam(r, "integrationName")
	if integrationName != "docker" && integrationName != "compliance" {
		Error(w, http.StatusBadRequest, "Invalid integration name")
		return
	}
	var req ToggleIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	fields := store.PendingConfigFields{}
	switch integrationName {
	case "docker":
		fields.DockerEnabled = &req.Enabled
	case "compliance":
		fields.ComplianceEnabled = &req.Enabled
		// Preserve on-demand when enabling; when disabling, on-demand is irrelevant
		if req.Enabled {
			fields.ComplianceOnDemandOnly = &host.ComplianceOnDemandOnly
		}
	}
	if err := h.pendingConfig.SetPendingConfig(r.Context(), hostID, fields); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to store pending integration toggle")
		return
	}
	mode := "disabled"
	if integrationName == "compliance" && req.Enabled {
		mode = "enabled"
		if host.ComplianceOnDemandOnly {
			mode = "on-demand"
		}
	}
	resp := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Integration %s %s (pending apply)", integrationName, map[bool]string{true: "enabled", false: "disabled"}[req.Enabled]),
		"data": map[string]interface{}{
			"integration": integrationName,
			"enabled":     req.Enabled,
			"mode":        mode,
			"host": map[string]interface{}{
				"id":           host.ID,
				"friendlyName": host.FriendlyName,
				"apiId":        host.ApiID,
			},
		},
	}
	JSON(w, http.StatusOK, resp)
}

// ApplyPendingConfig handles POST /hosts/:hostId/integrations/apply-pending-config.
// Sends all pending config to the agent via WebSocket, applies to hosts table, and clears pending.
func (h *HostsHandler) ApplyPendingConfig(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		slog.Warn("apply-pending-config: host not found", "host_id", hostID, "error", err)
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	conn := h.registry.GetConnection(host.ApiID)
	if conn == nil {
		slog.Info("apply-pending-config: agent not connected", "host_id", hostID, "api_id", host.ApiID)
		Error(w, http.StatusServiceUnavailable, "Agent is not connected. Ensure the agent's server_url in config.yml points to this server.")
		return
	}
	pending, err := h.pendingConfig.GetPendingConfig(r.Context(), hostID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load pending config")
		return
	}
	if pending == nil {
		Error(w, http.StatusBadRequest, "No pending configuration changes")
		return
	}

	// Merge pending with host to get full config to apply
	dockerEnabled := host.DockerEnabled
	complianceEnabled := host.ComplianceEnabled
	complianceOnDemandOnly := host.ComplianceOnDemandOnly
	openscapEnabled := host.ComplianceOpenscapEnabled
	dockerBenchEnabled := host.ComplianceDockerBenchEnabled
	if pending.DockerEnabled != nil {
		dockerEnabled = *pending.DockerEnabled
	}
	if pending.ComplianceEnabled != nil {
		complianceEnabled = *pending.ComplianceEnabled
	}
	if pending.ComplianceOnDemandOnly != nil {
		complianceOnDemandOnly = *pending.ComplianceOnDemandOnly
	}
	if pending.ComplianceOpenscapEnabled != nil {
		openscapEnabled = *pending.ComplianceOpenscapEnabled
	}
	if pending.ComplianceDockerBenchEnabled != nil {
		dockerBenchEnabled = *pending.ComplianceDockerBenchEnabled
	}

	// Build compliance value for agent: "on-demand", true, or false
	var complianceVal interface{}
	if !complianceEnabled {
		complianceVal = false
	} else if complianceOnDemandOnly {
		complianceVal = "on-demand"
	} else {
		complianceVal = true
	}

	msg := map[string]interface{}{
		"type": "apply_config",
		"config": map[string]interface{}{
			"docker": dockerEnabled,
			"compliance": map[string]interface{}{
				"enabled":              complianceVal,
				"openscap_enabled":     openscapEnabled,
				"docker_bench_enabled": dockerBenchEnabled,
			},
		},
	}
	if err := conn.WriteJSON(msg); err != nil {
		slog.Error("apply-pending-config: failed to send to agent", "host_id", hostID, "api_id", host.ApiID, "error", err)
		Error(w, http.StatusServiceUnavailable, "Failed to send config to agent")
		return
	}
	slog.Info("apply-pending-config: sent apply_config to agent", "host_id", hostID, "api_id", host.ApiID)

	// Apply to hosts table
	if err := h.hosts.UpdateDockerEnabled(r.Context(), hostID, dockerEnabled); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update host")
		return
	}
	if err := h.hosts.UpdateComplianceMode(r.Context(), hostID, complianceEnabled, complianceOnDemandOnly); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update compliance mode")
		return
	}
	if err := h.hosts.UpdateComplianceScanners(r.Context(), hostID, openscapEnabled, dockerBenchEnabled); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update scanner settings")
		return
	}
	if err := h.pendingConfig.ClearPendingConfig(r.Context(), hostID); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to clear pending config")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Configuration applied successfully",
	})
}

func hostToResponse(h *models.Host, groups []models.HostGroup) map[string]interface{} {
	res := map[string]interface{}{
		"id": h.ID, "friendly_name": h.FriendlyName, "hostname": h.Hostname, "ip": h.IP,
		"os_type": h.OSType, "os_version": h.OSVersion, "architecture": h.Architecture,
		"last_update": h.LastUpdate, "status": h.Status, "api_id": h.ApiID, "agent_version": h.AgentVersion,
		"auto_update": h.AutoUpdate, "created_at": h.CreatedAt, "notes": h.Notes,
		"system_uptime": h.SystemUptime, "needs_reboot": h.NeedsReboot,
		"docker_enabled": h.DockerEnabled, "compliance_enabled": h.ComplianceEnabled,
		"package_manager": h.PackageManager, "primary_interface": h.PrimaryInterface,
	}
	hg := make([]map[string]interface{}, len(groups))
	for i, g := range groups {
		hg[i] = map[string]interface{}{"id": g.ID, "name": g.Name, "color": g.Color}
	}
	res["host_group_memberships"] = hg
	return res
}

func generateApiCredentials() (apiID, apiKey, apiKeyHash string, err error) {
	b := make([]byte, 8)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	apiID = "patchmon_" + hex.EncodeToString(b)
	b = make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	apiKey = hex.EncodeToString(b)
	hash, err := bcrypt.GenerateFromPassword([]byte(apiKey), 10)
	if err != nil {
		return "", "", "", err
	}
	return apiID, apiKey, string(hash), nil
}
