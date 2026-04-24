package handler

import (
	"net/http"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// WindowsUpdatesHandler handles agent-facing and UI-facing Windows Update endpoints.
// These work entirely through the existing host_packages + packages tables - WUA-specific
// fields (wua_guid, wua_kb, wua_severity, etc.) extend those rows transparently.
type WindowsUpdatesHandler struct {
	hosts  *store.HostsStore
	db     database.DBProvider
	notify *notifications.Emitter
}

// NewWindowsUpdatesHandler creates a new Windows updates handler.
func NewWindowsUpdatesHandler(hosts *store.HostsStore, dbProvider database.DBProvider, notify *notifications.Emitter) *WindowsUpdatesHandler {
	return &WindowsUpdatesHandler{hosts: hosts, db: dbProvider, notify: notify}
}

// resolveAgentHost authenticates the agent request via X-API-ID / X-API-KEY headers.
func (h *WindowsUpdatesHandler) resolveAgentHost(r *http.Request) (*models.Host, bool) {
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		return nil, false
	}
	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		return nil, false
	}
	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		return nil, false
	}
	return host, true
}

// RecordInstallResult handles POST /patching/windows-updates/result (agent-facing).
// The agent calls this once per update GUID after attempting installation.
func (h *WindowsUpdatesHandler) RecordInstallResult(w http.ResponseWriter, r *http.Request) {
	host, ok := h.resolveAgentHost(r)
	if !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	var body struct {
		PatchRunID string `json:"patch_run_id"`
		GUID       string `json:"guid"`
		Success    bool   `json:"success"`
		Error      string `json:"error"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if body.GUID == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "guid is required"})
		return
	}

	result := "failed"
	if body.Success {
		result = "success"
	}

	d := h.db.DB(r.Context())
	guid := body.GUID
	_ = d.Queries.UpdateHostPackageWUAInstallResult(r.Context(), db.UpdateHostPackageWUAInstallResultParams{
		HostID:           host.ID,
		WuaGuid:          &guid,
		WuaInstallResult: &result,
	})

	JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RecordRebootStatus handles POST /patching/windows-updates/reboot (agent-facing).
// The agent reports whether a reboot is required after installation.
func (h *WindowsUpdatesHandler) RecordRebootStatus(w http.ResponseWriter, r *http.Request) {
	host, ok := h.resolveAgentHost(r)
	if !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	var body struct {
		PatchRunID  string `json:"patch_run_id"`
		NeedsReboot bool   `json:"needs_reboot"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}

	d := h.db.DB(r.Context())
	reboot := body.NeedsReboot
	var reason *string
	if reboot {
		s := "Windows Update installation requires a restart"
		reason = &s
	}
	_ = d.Queries.UpdateHostRebootStatus(r.Context(), db.UpdateHostRebootStatusParams{
		ID:           host.ID,
		NeedsReboot:  &reboot,
		RebootReason: reason,
	})

	// Emit patch_reboot_required when a reboot is needed.
	if reboot && h.notify != nil {
		hostName := host.FriendlyName
		if hostName == "" && host.Hostname != nil {
			hostName = *host.Hostname
		}
		h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
			Type:          "patch_reboot_required",
			Severity:      "warning",
			Title:         "Reboot Required - " + hostName,
			Message:       "Host \"" + hostName + "\" requires a reboot after Windows Update installation.",
			ReferenceType: "host",
			ReferenceID:   host.ID,
			Metadata: map[string]interface{}{
				"host_id":   host.ID,
				"host_name": hostName,
			},
		})
	}

	JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RemoveSuperseded handles POST /patching/windows-updates/superseded (agent-facing).
// The agent reports GUIDs that WUA no longer returns - they have been superseded by newer updates.
func (h *WindowsUpdatesHandler) RemoveSuperseded(w http.ResponseWriter, r *http.Request) {
	host, ok := h.resolveAgentHost(r)
	if !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	var body struct {
		GUIDs []string `json:"guids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if len(body.GUIDs) == 0 {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	d := h.db.DB(r.Context())
	_ = d.Queries.DeleteHostPackagesByWUAGUIDs(r.Context(), db.DeleteHostPackagesByWUAGUIDsParams{
		HostID:  host.ID,
		Column2: body.GUIDs,
	})

	JSON(w, http.StatusOK, map[string]string{"removed": "ok"})
}

// GetApprovedGUIDs handles GET /patching/windows-updates/approved (agent-facing).
// Returns the GUIDs of pending updates for this host that the agent should install.
func (h *WindowsUpdatesHandler) GetApprovedGUIDs(w http.ResponseWriter, r *http.Request) {
	host, ok := h.resolveAgentHost(r)
	if !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	d := h.db.DB(r.Context())
	rows, err := d.Queries.GetPendingWindowsUpdateGUIDs(r.Context(), host.ID)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch pending updates"})
		return
	}

	guids := make([]string, 0, len(rows))
	for _, g := range rows {
		if g != nil && *g != "" {
			guids = append(guids, *g)
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{"guids": guids})
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
		if row.WuaInstallResult != nil {
			u["install_result"] = *row.WuaInstallResult
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
