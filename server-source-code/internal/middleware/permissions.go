package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// RequirePermission returns middleware that checks the user has the given permission.
func RequirePermission(perm string, permissions *store.PermissionsStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, _ := r.Context().Value(UserRoleKey).(string)
			if role == "" {
				writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
			p, err := permissions.GetByRole(r.Context(), role)
			if err != nil || p == nil {
				writeJSONError(w, http.StatusForbidden, "Access denied")
				return
			}
			if !hasPermission(p, perm) {
				writeJSONError(w, http.StatusForbidden, "Insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func hasPermission(p *models.RolePermission, perm string) bool {
	switch perm {
	case "can_view_dashboard":
		return p.CanViewDashboard
	case "can_view_hosts":
		return p.CanViewHosts
	case "can_manage_hosts":
		return p.CanManageHosts
	case "can_view_packages":
		return p.CanViewPackages
	case "can_manage_packages":
		return p.CanManagePackages
	case "can_view_users":
		return p.CanViewUsers
	case "can_manage_users":
		return p.CanManageUsers
	case "can_manage_superusers":
		return p.CanManageSuperusers
	case "can_view_reports":
		return p.CanViewReports
	case "can_export_data":
		return p.CanExportData
	case "can_manage_settings":
		return p.CanManageSettings
	case "can_manage_notifications":
		return p.CanManageNotifications
	case "can_view_notification_logs":
		return p.CanViewNotificationLogs
	case "can_manage_patching":
		return p.CanManagePatching
	case "can_manage_compliance":
		return p.CanManageCompliance
	case "can_manage_docker":
		return p.CanManageDocker
	case "can_manage_alerts":
		return p.CanManageAlerts
	case "can_manage_automation":
		return p.CanManageAutomation
	case "can_use_remote_access":
		return p.CanUseRemoteAccess
	default:
		return false
	}
}
