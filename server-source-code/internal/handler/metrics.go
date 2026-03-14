package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/google/uuid"
)

const defaultMetricsAPIURL = "https://metrics.patchmon.cloud"

// MetricsHandler handles metrics settings routes (anonymous telemetry).
type MetricsHandler struct {
	settings *store.SettingsStore
	hosts    *store.HostsStore
	cfg      *config.Config
}

// NewMetricsHandler creates a new metrics handler.
func NewMetricsHandler(settings *store.SettingsStore, hosts *store.HostsStore, cfg *config.Config) *MetricsHandler {
	return &MetricsHandler{settings: settings, hosts: hosts, cfg: cfg}
}

// Get handles GET /api/v1/metrics - returns metrics settings.
func (h *MetricsHandler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}

	// Generate anonymous ID if it doesn't exist
	if s.MetricsAnonymousID == nil || *s.MetricsAnonymousID == "" {
		anonymousID := uuid.New().String()
		s.MetricsAnonymousID = &anonymousID
		if err := h.settings.Update(r.Context(), s); err != nil {
			Error(w, http.StatusInternalServerError, "Failed to save anonymous ID")
			return
		}
	}

	var lastSent interface{}
	if s.MetricsLastSent != nil {
		lastSent = s.MetricsLastSent.Format(time.RFC3339)
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"metrics_enabled":      s.MetricsEnabled,
		"metrics_anonymous_id": s.MetricsAnonymousID,
		"metrics_last_sent":    lastSent,
	})
}

// Update handles PUT /api/v1/metrics - updates metrics_enabled.
func (h *MetricsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MetricsEnabled *bool `json:"metrics_enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.MetricsEnabled == nil {
		Error(w, http.StatusBadRequest, "metrics_enabled must be a boolean")
		return
	}

	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}

	s.MetricsEnabled = *req.MetricsEnabled
	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update metrics settings")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":         "Metrics settings updated successfully",
		"metrics_enabled": s.MetricsEnabled,
	})
}

// RegenerateID handles POST /api/v1/metrics/regenerate-id.
func (h *MetricsHandler) RegenerateID(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}

	newID := uuid.New().String()
	s.MetricsAnonymousID = &newID
	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to regenerate anonymous ID")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":              "Anonymous ID regenerated successfully",
		"metrics_anonymous_id": newID,
	})
}

// SendNow handles POST /api/v1/metrics/send-now - sends metrics to patchmon.cloud.
func (h *MetricsHandler) SendNow(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}

	if !s.MetricsEnabled {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "Metrics are disabled. Please enable metrics first.",
		})
		return
	}

	if s.MetricsAnonymousID == nil || *s.MetricsAnonymousID == "" {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "No anonymous ID found. Please regenerate your ID.",
		})
		return
	}

	hostCount, err := h.hosts.Count(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to get host count")
		return
	}

	version := h.cfg.Version
	if version == "" {
		version = "1.4.4"
	}

	metricsData := map[string]interface{}{
		"anonymous_id": *s.MetricsAnonymousID,
		"host_count":   hostCount,
		"version":      version,
	}
	body, _ := json.Marshal(metricsData)

	apiURL := os.Getenv("METRICS_API_URL")
	if apiURL == "" {
		apiURL = defaultMetricsAPIURL
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, apiURL+"/metrics/submit", bytes.NewReader(body))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to prepare metrics request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":   "Failed to send metrics",
			"details": err.Error(),
		})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		JSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":   "Failed to send metrics",
			"details": "metrics API returned non-2xx status",
		})
		return
	}

	// Update last sent timestamp
	now := time.Now()
	s.MetricsLastSent = &now
	_ = h.settings.Update(r.Context(), s)

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Metrics sent successfully",
		"data": map[string]interface{}{
			"hostCount": hostCount,
			"version":   version,
		},
	})
}
