package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// AlertConfigHandler handles alert config routes.
type AlertConfigHandler struct {
	alertConfig *store.AlertConfigStore
}

// NewAlertConfigHandler creates a new alert config handler.
func NewAlertConfigHandler(alertConfig *store.AlertConfigStore) *AlertConfigHandler {
	return &AlertConfigHandler{alertConfig: alertConfig}
}

func alertConfigSuccessData(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": data})
}

// GetAll handles GET /alerts/config.
func (h *AlertConfigHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	configs, err := h.alertConfig.GetAll(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch alert config")
		return
	}
	// Convert to frontend format (snake_case for API consistency with Node)
	out := make([]map[string]interface{}, len(configs))
	for i, c := range configs {
		out[i] = alertConfigToMap(&c.AlertConfig, c.AutoAssignUser)
	}
	alertConfigSuccessData(w, out)
}

// GetByType handles GET /alerts/config/:alertType.
func (h *AlertConfigHandler) GetByType(w http.ResponseWriter, r *http.Request) {
	alertType := chi.URLParam(r, "alertType")
	if alertType == "" {
		Error(w, http.StatusBadRequest, "Alert type required")
		return
	}
	cfg, err := h.alertConfig.GetByType(r.Context(), alertType)
	if err != nil {
		Error(w, http.StatusNotFound, "Alert config not found")
		return
	}
	alertConfigSuccessData(w, alertConfigToMap(&cfg.AlertConfig, cfg.AutoAssignUser))
}

// Update handles PUT /alerts/config/:alertType.
func (h *AlertConfigHandler) Update(w http.ResponseWriter, r *http.Request) {
	alertType := chi.URLParam(r, "alertType")
	if alertType == "" {
		Error(w, http.StatusBadRequest, "Alert type required")
		return
	}
	existing, err := h.alertConfig.GetByType(r.Context(), alertType)
	if err != nil {
		existing = nil
	}

	var req map[string]interface{}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cfg := &models.AlertConfig{AlertType: alertType}
	if existing != nil {
		cfg = &existing.AlertConfig
	}
	applyAlertConfigUpdate(cfg, req)

	if err := h.alertConfig.Upsert(r.Context(), cfg); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update alert config")
		return
	}

	updated, _ := h.alertConfig.GetByType(r.Context(), alertType)
	if updated != nil {
		alertConfigSuccessData(w, alertConfigToMap(&updated.AlertConfig, updated.AutoAssignUser))
	} else {
		alertConfigSuccessData(w, alertConfigToMap(cfg, nil))
	}
}

// BulkUpdate handles POST /alerts/config/bulk-update.
func (h *AlertConfigHandler) BulkUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Configs []map[string]interface{} `json:"configs"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	var failed []string
	for _, c := range req.Configs {
		alertType, _ := c["alert_type"].(string)
		if alertType == "" {
			continue
		}
		existing, _ := h.alertConfig.GetByType(r.Context(), alertType)
		cfg := &models.AlertConfig{AlertType: alertType}
		if existing != nil {
			cfg = &existing.AlertConfig
		}
		applyAlertConfigUpdate(cfg, c)
		if err := h.alertConfig.Upsert(r.Context(), cfg); err != nil {
			slog.Error("alert config bulk update: upsert failed", "alert_type", alertType, "error", err)
			failed = append(failed, alertType)
		}
	}
	configs, err := h.alertConfig.GetAll(r.Context())
	if err != nil {
		slog.Error("alert config bulk update: failed to reload configs", "error", err)
		Error(w, http.StatusInternalServerError, "Failed to reload alert configs after update")
		return
	}
	out := make([]map[string]interface{}, len(configs))
	for i, c := range configs {
		out[i] = alertConfigToMap(&c.AlertConfig, c.AutoAssignUser)
	}
	result := map[string]interface{}{"success": true, "data": out}
	if len(failed) > 0 {
		result["partial_failures"] = failed
	}
	JSON(w, http.StatusOK, result)
}

// PreviewCleanup handles GET /alerts/cleanup/preview.
func (h *AlertConfigHandler) PreviewCleanup(w http.ResponseWriter, r *http.Request) {
	toClean, err := h.alertConfig.GetAlertsToCleanup(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to preview cleanup")
		return
	}
	alertConfigSuccessData(w, map[string]interface{}{
		"alerts":  toClean,
		"count":   len(toClean),
		"preview": true,
	})
}

// TriggerCleanup handles POST /alerts/cleanup.
func (h *AlertConfigHandler) TriggerCleanup(w http.ResponseWriter, r *http.Request) {
	deleted, err := h.alertConfig.CleanupOldAlerts(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to run cleanup")
		return
	}
	alertConfigSuccessData(w, map[string]interface{}{
		"deleted": deleted,
		"success": true,
	})
}

func alertConfigToMap(c *models.AlertConfig, autoAssignUser *store.UserRef) map[string]interface{} {
	m := map[string]interface{}{
		"id":                      c.ID,
		"alert_type":              c.AlertType,
		"is_enabled":              c.IsEnabled,
		"default_severity":        c.DefaultSeverity,
		"auto_assign_enabled":     c.AutoAssignEnabled,
		"auto_assign_user_id":     c.AutoAssignUserID,
		"auto_assign_rule":        c.AutoAssignRule,
		"retention_days":          c.RetentionDays,
		"auto_resolve_after_days": c.AutoResolveAfterDays,
		"cleanup_resolved_only":   c.CleanupResolvedOnly,
		"notification_enabled":    c.NotificationEnabled,
		"escalation_enabled":      c.EscalationEnabled,
		"escalation_after_hours":  c.EscalationAfterHours,
		"alert_delay_seconds":     c.AlertDelaySeconds,
		"category":                c.Category,
		"check_interval_minutes":  c.CheckIntervalMinutes,
		"reminder_interval_hours": c.ReminderIntervalHours,
		"created_at":              c.CreatedAt,
		"updated_at":              c.UpdatedAt,
	}
	if c.AutoAssignConditions != nil {
		m["auto_assign_conditions"] = c.AutoAssignConditions
	} else {
		m["auto_assign_conditions"] = map[string]interface{}{}
	}
	if c.Metadata != nil {
		m["metadata"] = c.Metadata
	} else {
		m["metadata"] = map[string]interface{}{}
	}
	if autoAssignUser != nil {
		m["users_auto_assign"] = map[string]interface{}{
			"id":         autoAssignUser.ID,
			"username":   autoAssignUser.Username,
			"email":      autoAssignUser.Email,
			"first_name": autoAssignUser.FirstName,
			"last_name":  autoAssignUser.LastName,
		}
	}
	return m
}

func applyNullableInt(dst **int, req map[string]interface{}, key string) {
	v, present := req[key]
	if !present {
		return
	}
	switch n := v.(type) {
	case nil:
		*dst = nil
	case float64:
		i := int(n)
		*dst = &i
	case int:
		*dst = &n
	}
}

func applyNullableString(dst **string, req map[string]interface{}, key string) {
	v, present := req[key]
	if !present {
		return
	}
	switch s := v.(type) {
	case nil:
		*dst = nil
	case string:
		if s == "" {
			*dst = nil
		} else {
			*dst = &s
		}
	}
}

func applyAlertConfigUpdate(cfg *models.AlertConfig, req map[string]interface{}) {
	if v, ok := req["is_enabled"].(bool); ok {
		cfg.IsEnabled = v
	}
	if v, ok := req["default_severity"].(string); ok {
		cfg.DefaultSeverity = v
	}
	if v, ok := req["auto_assign_enabled"].(bool); ok {
		cfg.AutoAssignEnabled = v
	}
	applyNullableString(&cfg.AutoAssignUserID, req, "auto_assign_user_id")
	applyNullableString(&cfg.AutoAssignRule, req, "auto_assign_rule")
	applyNullableInt(&cfg.RetentionDays, req, "retention_days")
	applyNullableInt(&cfg.AutoResolveAfterDays, req, "auto_resolve_after_days")
	if v, ok := req["cleanup_resolved_only"].(bool); ok {
		cfg.CleanupResolvedOnly = v
	}
	if v, ok := req["notification_enabled"].(bool); ok {
		cfg.NotificationEnabled = v
	}
	if v, ok := req["escalation_enabled"].(bool); ok {
		cfg.EscalationEnabled = v
	}
	applyNullableInt(&cfg.EscalationAfterHours, req, "escalation_after_hours")
	applyNullableInt(&cfg.AlertDelaySeconds, req, "alert_delay_seconds")
	applyNullableInt(&cfg.CheckIntervalMinutes, req, "check_interval_minutes")
	applyNullableInt(&cfg.ReminderIntervalHours, req, "reminder_interval_hours")
	if v, ok := req["auto_assign_conditions"].(map[string]interface{}); ok {
		b, _ := json.Marshal(v)
		cfg.AutoAssignConditions = b
	}
	if v, ok := req["metadata"].(map[string]interface{}); ok {
		b, _ := json.Marshal(v)
		cfg.Metadata = b
	}
	if v, ok := req["category"].(string); ok {
		cfg.Category = v
	}
}
