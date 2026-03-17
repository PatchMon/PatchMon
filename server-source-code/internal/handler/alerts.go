package handler

import (
	"log/slog"
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// AlertsHandler handles alert routes.
type AlertsHandler struct {
	alerts      *store.AlertsStore
	alertConfig *store.AlertConfigStore
	db          database.DBProvider
}

// NewAlertsHandler creates a new alerts handler.
func NewAlertsHandler(alerts *store.AlertsStore, alertConfig *store.AlertConfigStore, db database.DBProvider) *AlertsHandler {
	return &AlertsHandler{alerts: alerts, alertConfig: alertConfig, db: db}
}

// successData wraps response for Node/frontend compatibility.
func successData(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": data})
}

// List handles GET /alerts.
func (h *AlertsHandler) List(w http.ResponseWriter, r *http.Request) {
	var assignedTo *string
	if r.URL.Query().Get("assignedToMe") == "true" {
		userID, _ := r.Context().Value(middleware.UserIDKey).(string)
		if userID != "" {
			assignedTo = &userID
		}
	}
	alerts, err := h.alerts.List(r.Context(), assignedTo)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch alerts")
		return
	}
	successData(w, alerts)
}

// GetStats handles GET /alerts/stats.
func (h *AlertsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.alerts.GetStats(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch alert stats")
		return
	}
	successData(w, stats)
}

// GetAvailableActions handles GET /alerts/actions.
func (h *AlertsHandler) GetAvailableActions(w http.ResponseWriter, r *http.Request) {
	d := h.db.DB(r.Context())
	actions, err := d.Queries.ListAlertActions(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch available actions")
		return
	}
	// Convert to frontend format: id, name, display_name, description, is_state_action, severity_override
	out := make([]map[string]interface{}, len(actions))
	for i, a := range actions {
		out[i] = map[string]interface{}{
			"id":                a.ID,
			"name":              a.Name,
			"display_name":      a.DisplayName,
			"description":       a.Description,
			"is_state_action":   a.IsStateAction,
			"severity_override": a.SeverityOverride,
		}
	}
	successData(w, out)
}

// GetByID handles GET /alerts/:id.
func (h *AlertsHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Alert ID required")
		return
	}
	alert, err := h.alerts.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Alert not found")
		return
	}
	successData(w, alert)
}

// GetHistory handles GET /alerts/:id/history.
func (h *AlertsHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Alert ID required")
		return
	}
	d := h.db.DB(r.Context())
	rows, err := d.Queries.ListAlertHistoryByAlertID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch alert history")
		return
	}
	out := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		u := map[string]interface{}{}
		if row.UserIDVal != nil {
			u["id"] = *row.UserIDVal
			if row.Username != nil {
				u["username"] = *row.Username
			}
			if row.Email != nil {
				u["email"] = *row.Email
			}
		}
		out[i] = map[string]interface{}{
			"id":         row.ID,
			"alert_id":   row.AlertID,
			"user_id":    row.UserID,
			"action":     row.Action,
			"metadata":   row.Metadata,
			"created_at": row.CreatedAt.Time,
			"user":       u,
		}
	}
	successData(w, out)
}

// PerformAction handles POST /alerts/:id/action.
func (h *AlertsHandler) PerformAction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Alert ID required")
		return
	}
	var req struct {
		Action   string                 `json:"action"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Action == "" {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	d := h.db.DB(r.Context())

	action, err := d.Queries.GetAlertActionByName(r.Context(), req.Action)
	if err != nil {
		Error(w, http.StatusBadRequest, "Invalid action")
		return
	}

	_, err = h.alerts.GetByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Alert not found")
		return
	}

	// State actions (resolved, done, etc.) set is_active=false, resolved_at, resolved_by
	if action.IsStateAction {
		var uid *string
		if userID != "" {
			uid = &userID
		}
		if err := h.alerts.UpdateResolved(r.Context(), id, uid); err != nil {
			Error(w, http.StatusInternalServerError, "Failed to perform action")
			return
		}
	} else {
		// Non-state (assigned, silenced, etc.) - keep active
		if err := h.alerts.UpdateUnresolve(r.Context(), id); err != nil {
			Error(w, http.StatusInternalServerError, "Failed to perform action")
			return
		}
	}

	meta := req.Metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	var uid *string
	if userID != "" {
		uid = &userID
	}
	if err := h.alerts.RecordHistory(r.Context(), id, uid, req.Action, meta); err != nil {
		slog.Error("alerts: failed to record action history", "alert_id", id, "action", req.Action, "error", err)
	}
	if err := d.Queries.UpdateAlert(r.Context(), id); err != nil {
		slog.Error("alerts: failed to update alert timestamp", "alert_id", id, "error", err)
	}

	updated, _ := h.alerts.GetByID(r.Context(), id)
	successData(w, updated)
}

// Assign handles POST /alerts/:id/assign.
func (h *AlertsHandler) Assign(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Alert ID required")
		return
	}
	var req struct {
		UserID string `json:"userId"`
	}
	if err := decodeJSON(r, &req); err != nil || req.UserID == "" {
		Error(w, http.StatusBadRequest, "userId required")
		return
	}
	if err := h.alerts.UpdateAssignment(r.Context(), id, req.UserID); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to assign alert")
		return
	}
	d := h.db.DB(r.Context())
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	var uid *string
	if userID != "" {
		uid = &userID
	}
	if err := h.alerts.RecordHistory(r.Context(), id, uid, "assigned", map[string]interface{}{"assigned_to": req.UserID}); err != nil {
		slog.Error("alerts: failed to record assign history", "alert_id", id, "error", err)
	}
	if err := d.Queries.UpdateAlert(r.Context(), id); err != nil {
		slog.Error("alerts: failed to update alert timestamp", "alert_id", id, "error", err)
	}

	updated, _ := h.alerts.GetByID(r.Context(), id)
	successData(w, updated)
}

// Unassign handles POST /alerts/:id/unassign.
func (h *AlertsHandler) Unassign(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Alert ID required")
		return
	}
	if err := h.alerts.UpdateUnassign(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to unassign alert")
		return
	}
	d := h.db.DB(r.Context())
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	var uid *string
	if userID != "" {
		uid = &userID
	}
	if err := h.alerts.RecordHistory(r.Context(), id, uid, "unassigned", map[string]interface{}{}); err != nil {
		slog.Error("alerts: failed to record unassign history", "alert_id", id, "error", err)
	}
	if err := d.Queries.UpdateAlert(r.Context(), id); err != nil {
		slog.Error("alerts: failed to update alert timestamp", "alert_id", id, "error", err)
	}

	updated, _ := h.alerts.GetByID(r.Context(), id)
	successData(w, updated)
}

// Delete handles DELETE /alerts/:id.
func (h *AlertsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "Alert ID required")
		return
	}
	if err := h.alerts.Delete(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete alert")
		return
	}
	successData(w, map[string]interface{}{"deleted": true})
}

// BulkDelete handles POST /alerts/bulk-delete.
func (h *AlertsHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AlertIDs []string `json:"alertIds"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.AlertIDs) == 0 {
		Error(w, http.StatusBadRequest, "alertIds required")
		return
	}
	if err := h.alerts.BulkDelete(r.Context(), req.AlertIDs); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete alerts")
		return
	}
	successData(w, map[string]interface{}{"deleted": len(req.AlertIDs)})
}
