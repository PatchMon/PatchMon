package handler

import (
	"net/http"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// HostGroupsHandler handles host group routes.
type HostGroupsHandler struct {
	hostGroups *store.HostGroupsStore
	hosts      *store.HostsStore
}

// NewHostGroupsHandler creates a new host groups handler.
func NewHostGroupsHandler(hostGroups *store.HostGroupsStore, hosts *store.HostsStore) *HostGroupsHandler {
	return &HostGroupsHandler{hostGroups: hostGroups, hosts: hosts}
}

// List handles GET /host-groups.
func (h *HostGroupsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.hostGroups.ListWithHostCount(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load host groups")
		return
	}
	// Build response with _count.hosts for frontend compatibility
	data := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		color := "#3B82F6"
		if row.Color != nil {
			color = *row.Color
		}
		createdAt, updatedAt := time.Time{}, time.Time{}
		if row.CreatedAt.Valid {
			createdAt = row.CreatedAt.Time
		}
		if row.UpdatedAt.Valid {
			updatedAt = row.UpdatedAt.Time
		}
		data[i] = map[string]interface{}{
			"id":          row.ID,
			"name":        row.Name,
			"description": row.Description,
			"color":       color,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
			"_count":      map[string]interface{}{"hosts": int(row.HostCount)},
		}
	}
	JSON(w, http.StatusOK, data)
}

// GetByID handles GET /host-groups/:id.
func (h *HostGroupsHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	group, err := h.hostGroups.GetByID(r.Context(), id)
	if err != nil || group == nil {
		Error(w, http.StatusNotFound, "Host group not found")
		return
	}
	JSON(w, http.StatusOK, group)
}

// Create handles POST /host-groups.
func (h *HostGroupsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Name == "" {
		Error(w, http.StatusBadRequest, "Name is required")
		return
	}
	color := "#3B82F6"
	if req.Color != nil {
		color = *req.Color
	}
	g := &models.HostGroup{
		Name:        req.Name,
		Description: req.Description,
		Color:       &color,
	}
	if err := h.hostGroups.Create(r.Context(), g); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create host group")
		return
	}
	JSON(w, http.StatusCreated, g)
}

// Update handles PUT /host-groups/:id.
func (h *HostGroupsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	group, err := h.hostGroups.GetByID(r.Context(), id)
	if err != nil || group == nil {
		Error(w, http.StatusNotFound, "Host group not found")
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Color       *string `json:"color"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Name != nil {
		group.Name = *req.Name
	}
	if req.Description != nil {
		group.Description = req.Description
	}
	if req.Color != nil {
		group.Color = req.Color
	}
	if err := h.hostGroups.Update(r.Context(), group); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update host group")
		return
	}
	JSON(w, http.StatusOK, group)
}

// Delete handles DELETE /host-groups/:id.
func (h *HostGroupsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.hostGroups.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Host group not found")
		return
	}
	if err := h.hostGroups.Delete(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete host group")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"message": "Host group deleted successfully"})
}

// GetHosts handles GET /host-groups/:id/hosts.
func (h *HostGroupsHandler) GetHosts(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := h.hostGroups.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Host group not found")
		return
	}
	hostIDs, err := h.hostGroups.GetHostIDs(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load hosts")
		return
	}
	if len(hostIDs) == 0 {
		JSON(w, http.StatusOK, []map[string]interface{}{})
		return
	}
	hostsList, _ := h.hosts.GetByIDs(r.Context(), hostIDs)
	groupsByHost, _ := h.hosts.GetHostGroupsForHosts(r.Context(), hostIDs)
	hosts := make([]map[string]interface{}, 0, len(hostsList))
	for _, host := range hostsList {
		groups := groupsByHost[host.ID]
		if groups == nil {
			groups = []models.HostGroup{}
		}
		hosts = append(hosts, hostToResponse(&host, groups))
	}
	JSON(w, http.StatusOK, hosts)
}
