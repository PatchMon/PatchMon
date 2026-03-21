package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// NotificationsHandler manages notification destinations, routes, logs, and scheduled reports.
type NotificationsHandler struct {
	db       database.DBProvider
	enc      *util.Encryption
	emit     *notifications.Emitter
	resolved *config.ResolvedConfig
}

// NewNotificationsHandler creates the handler.
func NewNotificationsHandler(db database.DBProvider, enc *util.Encryption, emit *notifications.Emitter, resolved *config.ResolvedConfig) *NotificationsHandler {
	return &NotificationsHandler{db: db, enc: enc, emit: emit, resolved: resolved}
}

func (h *NotificationsHandler) q(ctx context.Context) *db.Queries {
	return h.db.DB(ctx).Queries
}

func (h *NotificationsHandler) timezone() string {
	if h.resolved != nil && h.resolved.Timezone != "" {
		return h.resolved.Timezone
	}
	return "UTC"
}

// ListDestinations GET /notifications/destinations
func (h *NotificationsHandler) ListDestinations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q(r.Context()).ListNotificationDestinations(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to list destinations")
		return
	}
	out := make([]map[string]interface{}, len(rows))
	for i, d := range rows {
		out[i] = map[string]interface{}{
			"id":           d.ID,
			"channel_type": d.ChannelType,
			"display_name": d.DisplayName,
			"enabled":      d.Enabled,
			"has_secret":   d.ConfigEncrypted != "",
			"created_at":   pgTime(d.CreatedAt),
			"updated_at":   pgTime(d.UpdatedAt),
		}
	}
	JSON(w, http.StatusOK, out)
}

// CreateDestination POST /notifications/destinations
func (h *NotificationsHandler) CreateDestination(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelType string                 `json:"channel_type"`
		DisplayName string                 `json:"display_name"`
		Config      map[string]interface{} `json:"config"`
		Enabled     *bool                  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil || req.ChannelType == "" || req.DisplayName == "" {
		Error(w, http.StatusBadRequest, "channel_type, display_name, and config required")
		return
	}
	cfgJSON, _ := json.Marshal(req.Config)
	encStr := string(cfgJSON)
	if h.enc != nil {
		if e, err := h.enc.Encrypt(encStr); err == nil {
			encStr = e
		}
	}
	en := true
	if req.Enabled != nil {
		en = *req.Enabled
	}
	row, err := h.q(r.Context()).CreateNotificationDestination(r.Context(), db.CreateNotificationDestinationParams{
		ID:              uuid.New().String(),
		ChannelType:     req.ChannelType,
		DisplayName:     req.DisplayName,
		ConfigEncrypted: encStr,
		Enabled:         en,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create destination")
		return
	}
	JSON(w, http.StatusCreated, map[string]interface{}{
		"id": row.ID, "channel_type": row.ChannelType, "display_name": row.DisplayName, "enabled": row.Enabled,
	})
}

// UpdateDestination PUT /notifications/destinations/{id}
func (h *NotificationsHandler) UpdateDestination(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "id required")
		return
	}
	var req struct {
		DisplayName string                 `json:"display_name"`
		Config      map[string]interface{} `json:"config"`
		Enabled     *bool                  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	existing, err := h.q(r.Context()).GetNotificationDestinationByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Not found")
		return
	}
	encStr := existing.ConfigEncrypted
	if req.Config != nil {
		b, _ := json.Marshal(req.Config)
		encStr = string(b)
		if h.enc != nil {
			if e, err := h.enc.Encrypt(encStr); err == nil {
				encStr = e
			}
		}
	}
	en := existing.Enabled
	if req.Enabled != nil {
		en = *req.Enabled
	}
	dn := existing.DisplayName
	if req.DisplayName != "" {
		dn = req.DisplayName
	}
	row, err := h.q(r.Context()).UpdateNotificationDestination(r.Context(), db.UpdateNotificationDestinationParams{
		ID:              id,
		DisplayName:     dn,
		ConfigEncrypted: encStr,
		Enabled:         en,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"id": row.ID, "display_name": row.DisplayName, "enabled": row.Enabled})
}

// GetDestinationConfig GET /notifications/destinations/{id}/config
// Returns the decrypted config for editing. Protected by can_manage_notifications.
func (h *NotificationsHandler) GetDestinationConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "id required")
		return
	}
	dest, err := h.q(r.Context()).GetNotificationDestinationByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Not found")
		return
	}
	plain := "{}"
	if dest.ConfigEncrypted != "" && h.enc != nil {
		if d, err := h.enc.Decrypt(dest.ConfigEncrypted); err == nil {
			plain = d
		}
	} else if dest.ConfigEncrypted != "" {
		plain = dest.ConfigEncrypted
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
		cfg = map[string]interface{}{}
	}
	JSON(w, http.StatusOK, cfg)
}

// DeleteDestination DELETE /notifications/destinations/{id}
func (h *NotificationsHandler) DeleteDestination(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.q(r.Context()).DeleteNotificationDestination(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// ListRoutes GET /notifications/routes
func (h *NotificationsHandler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q(r.Context()).ListNotificationRoutes(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to list routes")
		return
	}
	out := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		out[i] = map[string]interface{}{
			"id":                       row.ID,
			"destination_id":           row.DestinationID,
			"event_types":              jsonOrEmpty(row.EventTypes),
			"min_severity":             row.MinSeverity,
			"host_group_ids":           jsonOrEmpty(row.HostGroupIds),
			"host_ids":                 jsonOrEmpty(row.HostIds),
			"match_rules":              json.RawMessage(row.MatchRules),
			"enabled":                  row.RouteEnabled,
			"channel_type":             row.ChannelType,
			"destination_display_name": row.DestinationDisplayName,
			"created_at":               pgTime(row.CreatedAt),
			"updated_at":               pgTime(row.UpdatedAt),
		}
	}
	JSON(w, http.StatusOK, out)
}

func jsonOrEmpty(b []byte) json.RawMessage {
	if len(b) == 0 || string(b) == "null" {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}

// CreateRoute POST /notifications/routes
func (h *NotificationsHandler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DestinationID string                 `json:"destination_id"`
		EventTypes    []string               `json:"event_types"`
		MinSeverity   string                 `json:"min_severity"`
		HostGroupIDs  []string               `json:"host_group_ids"`
		HostIDs       []string               `json:"host_ids"`
		MatchRules    map[string]interface{} `json:"match_rules"`
		Enabled       *bool                  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil || req.DestinationID == "" {
		Error(w, http.StatusBadRequest, "destination_id required")
		return
	}
	if len(req.EventTypes) == 0 {
		req.EventTypes = []string{"*"}
	}
	ms := req.MinSeverity
	if ms == "" {
		ms = "informational"
	}
	eventTypes, _ := json.Marshal(req.EventTypes)
	hostGroupIDs, _ := json.Marshal(req.HostGroupIDs)
	if hostGroupIDs == nil {
		hostGroupIDs = []byte("[]")
	}
	hostIDs, _ := json.Marshal(req.HostIDs)
	if hostIDs == nil {
		hostIDs = []byte("[]")
	}
	var rules []byte
	if req.MatchRules != nil {
		rules, _ = json.Marshal(req.MatchRules)
	}
	en := true
	if req.Enabled != nil {
		en = *req.Enabled
	}
	row, err := h.q(r.Context()).CreateNotificationRoute(r.Context(), db.CreateNotificationRouteParams{
		ID:            uuid.New().String(),
		DestinationID: req.DestinationID,
		EventTypes:    eventTypes,
		MinSeverity:   ms,
		HostGroupIds:  hostGroupIDs,
		HostIds:       hostIDs,
		MatchRules:    rules,
		Enabled:       en,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create route")
		return
	}
	JSON(w, http.StatusCreated, map[string]interface{}{"id": row.ID})
}

// UpdateRoute PUT /notifications/routes/{id}
func (h *NotificationsHandler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		DestinationID string                 `json:"destination_id"`
		EventTypes    []string               `json:"event_types"`
		MinSeverity   string                 `json:"min_severity"`
		HostGroupIDs  []string               `json:"host_group_ids"`
		HostIDs       []string               `json:"host_ids"`
		MatchRules    map[string]interface{} `json:"match_rules"`
		Enabled       *bool                  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	existing, err := h.q(r.Context()).GetNotificationRouteByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Not found")
		return
	}
	did := existing.DestinationID
	if req.DestinationID != "" {
		did = req.DestinationID
	}
	ms := existing.MinSeverity
	if req.MinSeverity != "" {
		ms = req.MinSeverity
	}
	eventTypes := existing.EventTypes
	if req.EventTypes != nil {
		eventTypes, _ = json.Marshal(req.EventTypes)
	}
	hostGroupIDs := existing.HostGroupIds
	if req.HostGroupIDs != nil {
		hostGroupIDs, _ = json.Marshal(req.HostGroupIDs)
	}
	hostIDs := existing.HostIds
	if req.HostIDs != nil {
		hostIDs, _ = json.Marshal(req.HostIDs)
	}
	rules := existing.MatchRules
	if req.MatchRules != nil {
		rules, _ = json.Marshal(req.MatchRules)
	}
	en := existing.Enabled
	if req.Enabled != nil {
		en = *req.Enabled
	}
	row, err := h.q(r.Context()).UpdateNotificationRoute(r.Context(), db.UpdateNotificationRouteParams{
		ID:            id,
		DestinationID: did,
		EventTypes:    eventTypes,
		MinSeverity:   ms,
		HostGroupIds:  hostGroupIDs,
		HostIds:       hostIDs,
		MatchRules:    rules,
		Enabled:       en,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update route")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"id": row.ID})
}

// DeleteRoute DELETE /notifications/routes/{id}
func (h *NotificationsHandler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.q(r.Context()).DeleteNotificationRoute(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// ListDeliveryLog GET /notifications/delivery-log
func (h *NotificationsHandler) ListDeliveryLog(w http.ResponseWriter, r *http.Request) {
	limit := int32(50)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = int32(n)
		}
	}
	offset := int32(0)
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}
	rows, err := h.q(r.Context()).ListNotificationDeliveryLog(r.Context(), db.ListNotificationDeliveryLogParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to list log")
		return
	}
	out := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		out[i] = map[string]interface{}{
			"id":                  row.ID,
			"event_fingerprint":   row.EventFingerprint,
			"reference_type":      row.ReferenceType,
			"reference_id":        row.ReferenceID,
			"destination_id":      row.DestinationID,
			"event_type":          row.EventType,
			"status":              row.Status,
			"error_message":       row.ErrorMessage,
			"attempt_count":       row.AttemptCount,
			"provider_message_id": row.ProviderMessageID,
			"created_at":          pgTime(row.CreatedAt),
			"updated_at":          pgTime(row.UpdatedAt),
		}
	}
	JSON(w, http.StatusOK, out)
}

// TestDestination POST /notifications/test
func (h *NotificationsHandler) TestDestination(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DestinationID string `json:"destination_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.DestinationID == "" {
		Error(w, http.StatusBadRequest, "destination_id required")
		return
	}
	d := h.db.DB(r.Context())
	if d == nil {
		Error(w, http.StatusInternalServerError, "No database available")
		return
	}
	if h.emit == nil {
		Error(w, http.StatusServiceUnavailable, "Notifications not configured")
		return
	}
	th := hostctx.TenantHostKey(r.Context())
	err := h.emit.EnqueueToDestination(r.Context(), d, th, req.DestinationID, notifications.Event{
		Type:          "test",
		Severity:      "informational",
		Title:         "PatchMon test notification",
		Message:       "This is a test message from PatchMon notification settings.",
		ReferenceType: "test",
		ReferenceID:   uuid.New().String(),
		Metadata:      map[string]interface{}{"source": "manual_test"},
	})
	if err != nil {
		switch {
		case errors.Is(err, notifications.ErrDestinationNotFound):
			Error(w, http.StatusNotFound, "Destination not found")
		case errors.Is(err, notifications.ErrDestinationDisabled):
			Error(w, http.StatusBadRequest, "Destination is disabled")
		case errors.Is(err, notifications.ErrRateLimited):
			Error(w, http.StatusTooManyRequests, "Too many notifications; try again shortly")
		case errors.Is(err, notifications.ErrNotificationsDisabled):
			Error(w, http.StatusServiceUnavailable, "Notifications not configured")
		default:
			Error(w, http.StatusInternalServerError, "Failed to enqueue test")
		}
		return
	}
	JSON(w, http.StatusOK, map[string]string{"status": "enqueued"})
}

func (h *NotificationsHandler) scheduledReportToMap(row db.ScheduledReport) map[string]interface{} {
	var def interface{}
	if len(row.Definition) > 0 {
		_ = json.Unmarshal(row.Definition, &def)
	}
	if def == nil {
		def = map[string]interface{}{}
	}
	var destIDs interface{}
	if len(row.DestinationIds) > 0 {
		_ = json.Unmarshal(row.DestinationIds, &destIDs)
	}
	if destIDs == nil {
		destIDs = []interface{}{}
	}
	return map[string]interface{}{
		"id":              row.ID,
		"name":            row.Name,
		"cron_expr":       row.CronExpr,
		"enabled":         row.Enabled,
		"definition":      def,
		"destination_ids": destIDs,
		"next_run_at":     pgTime(row.NextRunAt),
		"last_run_at":     pgTime(row.LastRunAt),
		"created_at":      pgTime(row.CreatedAt),
		"updated_at":      pgTime(row.UpdatedAt),
	}
}

// ListScheduledReports GET /notifications/scheduled-reports
func (h *NotificationsHandler) ListScheduledReports(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q(r.Context()).ListScheduledReports(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to list")
		return
	}
	out := make([]map[string]interface{}, len(rows))
	for i := range rows {
		out[i] = h.scheduledReportToMap(rows[i])
	}
	JSON(w, http.StatusOK, out)
}

// CreateScheduledReport POST /notifications/scheduled-reports
func (h *NotificationsHandler) CreateScheduledReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string                 `json:"name"`
		CronExpr       string                 `json:"cron_expr"`
		Enabled        *bool                  `json:"enabled"`
		Definition     map[string]interface{} `json:"definition"`
		DestinationIDs []string               `json:"destination_ids"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Name == "" {
		Error(w, http.StatusBadRequest, "name required")
		return
	}
	cron := req.CronExpr
	if cron == "" {
		cron = "0 8 * * *"
	}
	tz := h.timezone()
	def, _ := json.Marshal(req.Definition)
	if len(def) == 0 || string(def) == "null" {
		def = []byte("{}")
	}
	dest, _ := json.Marshal(req.DestinationIDs)
	en := true
	if req.Enabled != nil {
		en = *req.Enabled
	}
	next, err := notifications.NextCronRun(cron, tz, time.Now())
	if err != nil {
		Error(w, http.StatusBadRequest, "Invalid cron_expr")
		return
	}
	row, err := h.q(r.Context()).CreateScheduledReport(r.Context(), db.CreateScheduledReportParams{
		ID:             uuid.New().String(),
		Name:           req.Name,
		CronExpr:       cron,
		Enabled:        en,
		Definition:     def,
		DestinationIds: dest,
		NextRunAt:      pgtype.Timestamp{Time: next, Valid: true},
		LastRunAt:      pgtype.Timestamp{Valid: false},
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create")
		return
	}
	JSON(w, http.StatusCreated, h.scheduledReportToMap(row))
}

// UpdateScheduledReport PUT /notifications/scheduled-reports/{id}
func (h *NotificationsHandler) UpdateScheduledReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name           string                 `json:"name"`
		CronExpr       string                 `json:"cron_expr"`
		Enabled        *bool                  `json:"enabled"`
		Definition     map[string]interface{} `json:"definition"`
		DestinationIDs []string               `json:"destination_ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	ex, err := h.q(r.Context()).GetScheduledReportByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Not found")
		return
	}
	name := ex.Name
	if req.Name != "" {
		name = req.Name
	}
	cron := ex.CronExpr
	if req.CronExpr != "" {
		cron = req.CronExpr
	}
	tz := h.timezone()
	en := ex.Enabled
	if req.Enabled != nil {
		en = *req.Enabled
	}
	def := ex.Definition
	if req.Definition != nil {
		def, _ = json.Marshal(req.Definition)
	}
	dest := ex.DestinationIds
	if req.DestinationIDs != nil {
		dest, _ = json.Marshal(req.DestinationIDs)
	}
	nextAt := ex.NextRunAt
	if req.CronExpr != "" {
		if n, err := notifications.NextCronRun(cron, tz, time.Now()); err == nil {
			nextAt = pgtype.Timestamp{Time: n, Valid: true}
		}
	}
	row, err := h.q(r.Context()).UpdateScheduledReport(r.Context(), db.UpdateScheduledReportParams{
		ID:             id,
		Name:           name,
		CronExpr:       cron,
		Enabled:        en,
		Definition:     def,
		DestinationIds: dest,
		NextRunAt:      nextAt,
		LastRunAt:      ex.LastRunAt,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update")
		return
	}
	JSON(w, http.StatusOK, h.scheduledReportToMap(row))
}

// RunScheduledReportNow POST /notifications/scheduled-reports/{id}/run-now
func (h *NotificationsHandler) RunScheduledReportNow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "id required")
		return
	}
	ex, err := h.q(r.Context()).GetScheduledReportByID(r.Context(), id)
	if err != nil {
		Error(w, http.StatusNotFound, "Not found")
		return
	}
	if !ex.Enabled {
		Error(w, http.StatusBadRequest, "Report is disabled")
		return
	}
	if err := h.q(r.Context()).UpdateScheduledReportRunTimes(r.Context(), db.UpdateScheduledReportRunTimesParams{
		ID:        id,
		LastRunAt: ex.LastRunAt,
		NextRunAt: pgtype.Timestamp{Time: time.Now().Add(-1 * time.Minute), Valid: true},
	}); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to schedule report")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"status": "scheduled"})
}

// DeleteScheduledReport DELETE /notifications/scheduled-reports/{id}
func (h *NotificationsHandler) DeleteScheduledReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.q(r.Context()).DeleteScheduledReport(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func pgTime(t pgtype.Timestamp) interface{} {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format(time.RFC3339)
}
