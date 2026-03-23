package handler

import (
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// PermissionsHandler handles permissions routes.
type PermissionsHandler struct {
	permissions *store.PermissionsStore
}

// NewPermissionsHandler creates a new permissions handler.
func NewPermissionsHandler(permissions *store.PermissionsStore) *PermissionsHandler {
	return &PermissionsHandler{permissions: permissions}
}

// GetRoles returns all role permissions (for settings users/roles tabs).
func (h *PermissionsHandler) GetRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.permissions.ListRoles(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch role permissions")
		return
	}
	data := make([]map[string]interface{}, len(roles))
	for i, p := range roles {
		data[i] = map[string]interface{}{
			"id":                         p.ID,
			"role":                       p.Role,
			"can_view_dashboard":         p.CanViewDashboard,
			"can_view_hosts":             p.CanViewHosts,
			"can_manage_hosts":           p.CanManageHosts,
			"can_view_packages":          p.CanViewPackages,
			"can_manage_packages":        p.CanManagePackages,
			"can_view_users":             p.CanViewUsers,
			"can_manage_users":           p.CanManageUsers,
			"can_manage_superusers":      p.CanManageSuperusers,
			"can_view_reports":           p.CanViewReports,
			"can_export_data":            p.CanExportData,
			"can_manage_settings":        p.CanManageSettings,
			"can_manage_notifications":   p.CanManageNotifications,
			"can_view_notification_logs": p.CanViewNotificationLogs,
			"can_manage_patching":        p.CanManagePatching,
			"can_manage_compliance":      p.CanManageCompliance,
			"can_manage_docker":          p.CanManageDocker,
			"can_manage_alerts":          p.CanManageAlerts,
			"can_manage_automation":      p.CanManageAutomation,
			"can_use_remote_access":      p.CanUseRemoteAccess,
			"created_at":                 p.CreatedAt,
			"updated_at":                 p.UpdatedAt,
		}
	}
	JSON(w, http.StatusOK, data)
}

// GetRole handles GET /permissions/roles/:role.
func (h *PermissionsHandler) GetRole(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")
	if role == "" {
		Error(w, http.StatusBadRequest, "Role is required")
		return
	}
	p, err := h.permissions.GetByRole(r.Context(), role)
	if err != nil || p == nil {
		Error(w, http.StatusNotFound, "Role not found")
		return
	}
	JSON(w, http.StatusOK, roleToResponse(p))
}

// UpdateRole handles PUT /permissions/roles/:role (create or update role).
func (h *PermissionsHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")
	if role == "" {
		Error(w, http.StatusBadRequest, "Role is required")
		return
	}
	builtInRoles := map[string]bool{
		"superadmin": true, "admin": true, "host_manager": true, "readonly": true, "user": true,
	}
	if builtInRoles[role] {
		Error(w, http.StatusBadRequest, "Cannot modify built-in role permissions")
		return
	}
	var req struct {
		CanViewDashboard        *bool `json:"can_view_dashboard"`
		CanViewHosts            *bool `json:"can_view_hosts"`
		CanManageHosts          *bool `json:"can_manage_hosts"`
		CanViewPackages         *bool `json:"can_view_packages"`
		CanManagePackages       *bool `json:"can_manage_packages"`
		CanViewUsers            *bool `json:"can_view_users"`
		CanManageUsers          *bool `json:"can_manage_users"`
		CanManageSuperusers     *bool `json:"can_manage_superusers"`
		CanViewReports          *bool `json:"can_view_reports"`
		CanExportData           *bool `json:"can_export_data"`
		CanManageSettings       *bool `json:"can_manage_settings"`
		CanManageNotifications  *bool `json:"can_manage_notifications"`
		CanViewNotificationLogs *bool `json:"can_view_notification_logs"`
		CanManagePatching       *bool `json:"can_manage_patching"`
		CanManageCompliance     *bool `json:"can_manage_compliance"`
		CanManageDocker         *bool `json:"can_manage_docker"`
		CanManageAlerts         *bool `json:"can_manage_alerts"`
		CanManageAutomation     *bool `json:"can_manage_automation"`
		CanUseRemoteAccess      *bool `json:"can_use_remote_access"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	boolVal := func(b *bool) bool {
		if b != nil {
			return *b
		}
		return false
	}
	p := &models.RolePermission{
		Role:                    role,
		CanViewDashboard:        boolVal(req.CanViewDashboard),
		CanViewHosts:            boolVal(req.CanViewHosts),
		CanManageHosts:          boolVal(req.CanManageHosts),
		CanViewPackages:         boolVal(req.CanViewPackages),
		CanManagePackages:       boolVal(req.CanManagePackages),
		CanViewUsers:            boolVal(req.CanViewUsers),
		CanManageUsers:          boolVal(req.CanManageUsers),
		CanManageSuperusers:     boolVal(req.CanManageSuperusers),
		CanViewReports:          boolVal(req.CanViewReports),
		CanExportData:           boolVal(req.CanExportData),
		CanManageSettings:       boolVal(req.CanManageSettings),
		CanManageNotifications:  boolVal(req.CanManageNotifications),
		CanViewNotificationLogs: boolVal(req.CanViewNotificationLogs),
		CanManagePatching:       boolVal(req.CanManagePatching),
		CanManageCompliance:     boolVal(req.CanManageCompliance),
		CanManageDocker:         boolVal(req.CanManageDocker),
		CanManageAlerts:         boolVal(req.CanManageAlerts),
		CanManageAutomation:     boolVal(req.CanManageAutomation),
		CanUseRemoteAccess:      boolVal(req.CanUseRemoteAccess),
	}
	if err := h.permissions.UpsertRole(r.Context(), p); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update role permissions")
		return
	}
	updated, _ := h.permissions.GetByRole(r.Context(), role)
	if updated != nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"message":     "Role permissions updated successfully",
			"permissions": roleToResponse(updated),
		})
	} else {
		JSON(w, http.StatusOK, map[string]interface{}{"message": "Role permissions updated successfully"})
	}
}

// DeleteRole handles DELETE /permissions/roles/:role.
func (h *PermissionsHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	role := chi.URLParam(r, "role")
	if role == "" {
		Error(w, http.StatusBadRequest, "Role is required")
		return
	}
	builtInRoles := map[string]bool{
		"superadmin": true, "admin": true, "host_manager": true, "readonly": true, "user": true,
	}
	if builtInRoles[role] {
		Error(w, http.StatusBadRequest, "Cannot delete built-in role")
		return
	}
	count, _ := h.permissions.CountUsersByRole(r.Context(), role)
	if count > 0 {
		Error(w, http.StatusBadRequest, "Cannot delete role: users are assigned to it")
		return
	}
	if err := h.permissions.DeleteRole(r.Context(), role); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete role")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"message": "Role deleted successfully"})
}

func roleToResponse(p *models.RolePermission) map[string]interface{} {
	return map[string]interface{}{
		"id": p.ID, "role": p.Role,
		"can_view_dashboard": p.CanViewDashboard, "can_view_hosts": p.CanViewHosts,
		"can_manage_hosts": p.CanManageHosts, "can_view_packages": p.CanViewPackages,
		"can_manage_packages": p.CanManagePackages, "can_view_users": p.CanViewUsers,
		"can_manage_users": p.CanManageUsers, "can_manage_superusers": p.CanManageSuperusers,
		"can_view_reports": p.CanViewReports, "can_export_data": p.CanExportData,
		"can_manage_settings":        p.CanManageSettings,
		"can_manage_notifications":   p.CanManageNotifications,
		"can_view_notification_logs": p.CanViewNotificationLogs,
		"can_manage_patching":        p.CanManagePatching,
		"can_manage_compliance":      p.CanManageCompliance,
		"can_manage_docker":          p.CanManageDocker,
		"can_manage_alerts":          p.CanManageAlerts,
		"can_manage_automation":      p.CanManageAutomation,
		"can_use_remote_access":      p.CanUseRemoteAccess,
		"created_at":                 p.CreatedAt, "updated_at": p.UpdatedAt,
	}
}

// UserPermissions returns the current user's permissions based on their role.
func (h *PermissionsHandler) UserPermissions(w http.ResponseWriter, r *http.Request) {
	role, _ := r.Context().Value(middleware.UserRoleKey).(string)
	if role == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	p, err := h.permissions.GetByRole(r.Context(), role)
	if err != nil || p == nil {
		// Admin/superadmin without explicit permissions - return full access
		if role == "admin" || role == "superadmin" {
			JSON(w, http.StatusOK, fullPermissions())
			return
		}
		Error(w, http.StatusForbidden, "No permissions found")
		return
	}
	JSON(w, http.StatusOK, map[string]bool{
		"can_view_dashboard":         p.CanViewDashboard,
		"can_view_hosts":             p.CanViewHosts,
		"can_manage_hosts":           p.CanManageHosts,
		"can_view_packages":          p.CanViewPackages,
		"can_manage_packages":        p.CanManagePackages,
		"can_view_users":             p.CanViewUsers,
		"can_manage_users":           p.CanManageUsers,
		"can_view_reports":           p.CanViewReports,
		"can_export_data":            p.CanExportData,
		"can_manage_settings":        p.CanManageSettings,
		"can_manage_superusers":      p.CanManageSuperusers,
		"can_manage_notifications":   p.CanManageNotifications,
		"can_view_notification_logs": p.CanViewNotificationLogs,
		"can_manage_patching":        p.CanManagePatching,
		"can_manage_compliance":      p.CanManageCompliance,
		"can_manage_docker":          p.CanManageDocker,
		"can_manage_alerts":          p.CanManageAlerts,
		"can_manage_automation":      p.CanManageAutomation,
		"can_use_remote_access":      p.CanUseRemoteAccess,
	})
}

func fullPermissions() map[string]bool {
	return map[string]bool{
		"can_view_dashboard": true, "can_view_hosts": true, "can_manage_hosts": true,
		"can_view_packages": true, "can_manage_packages": true, "can_view_users": true,
		"can_manage_users": true, "can_view_reports": true, "can_export_data": true,
		"can_manage_settings": true, "can_manage_superusers": true,
		"can_manage_notifications": true, "can_view_notification_logs": true,
		"can_manage_patching": true, "can_manage_compliance": true,
		"can_manage_docker": true, "can_manage_alerts": true,
		"can_manage_automation": true, "can_use_remote_access": true,
	}
}
