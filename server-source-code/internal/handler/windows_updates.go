package handler

import (
	"net/http"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WindowsUpdatesHandler handles the UI-facing Windows Update endpoint.
// These work entirely through the existing host_packages + packages tables - WUA-specific
// fields (wua_guid, wua_kb, wua_severity, etc.) extend those rows transparently.
type WindowsUpdatesHandler struct {
	hosts *store.HostsStore
	db    database.DBProvider
}

// NewWindowsUpdatesHandler creates a new Windows updates handler.
func NewWindowsUpdatesHandler(hosts *store.HostsStore, dbProvider database.DBProvider) *WindowsUpdatesHandler {
	return &WindowsUpdatesHandler{hosts: hosts, db: dbProvider}
}

// ListForHost handles GET /patching/windows-updates/{hostId} (UI-facing).
// Returns all Windows Update entries for a host with full WUA metadata.
func (h *WindowsUpdatesHandler) ListForHost(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" {
		Error(w, http.StatusBadRequest, "hostId is required")
		return
	}

	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}

	d := h.db.DB(r.Context())
	rows, err := d.Queries.GetHostWindowsUpdates(r.Context(), hostID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch Windows updates")
		return
	}

	updates := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		u := map[string]interface{}{
			"id":                 row.ID,
			"package_id":         row.PackageID,
			"name":               row.PkgName,
			"current_version":    row.CurrentVersion,
			"available_version":  row.AvailableVersion,
			"needs_update":       row.NeedsUpdate,
			"is_security_update": row.IsSecurityUpdate,
			"last_checked":       pgTimestampToString(row.LastChecked),
		}
		if row.WuaGuid != nil {
			u["guid"] = *row.WuaGuid
		}
		if row.WuaKb != nil {
			u["kb"] = *row.WuaKb
		}
		if row.WuaSeverity != nil {
			u["severity"] = *row.WuaSeverity
		}
		if len(row.WuaCategories) > 0 {
			u["categories"] = row.WuaCategories
		}
		if row.WuaDescription != nil {
			u["description"] = *row.WuaDescription
		}
		if row.WuaSupportUrl != nil {
			u["support_url"] = *row.WuaSupportUrl
		}
		if row.WuaRevisionNumber != nil {
			u["revision_number"] = *row.WuaRevisionNumber
		}
		if row.WuaDateInstalled.Valid {
			u["date_installed"] = row.WuaDateInstalled.Time.Format(time.RFC3339)
		}
		if row.PkgDescription != nil {
			u["pkg_description"] = *row.PkgDescription
		}
		updates = append(updates, u)
	}

	stats, _ := d.Queries.CountWindowsUpdatesByHostID(r.Context(), hostID)

	JSON(w, http.StatusOK, map[string]interface{}{
		"host_id":         hostID,
		"updates":         updates,
		"total":           len(updates),
		"pending_count":   stats.PendingCount,
		"security_count":  stats.SecurityCount,
		"installed_count": stats.InstalledCount,
	})
}

// pgTimestampToString converts a pgtype.Timestamp to RFC3339 string.
func pgTimestampToString(t pgtype.Timestamp) string {
	if t.Valid {
		return t.Time.Format(time.RFC3339)
	}
	return ""
}
