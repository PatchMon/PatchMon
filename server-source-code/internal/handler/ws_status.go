package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

const (
	maxWSStatusAPIIDs      = 1000
	maxWSStatusQueryLength = 20000
)

// WSStatusHandler serves WebSocket connection status for the frontend.
type WSStatusHandler struct {
	registry *agentregistry.Registry
	hosts    *store.HostsStore
}

// NewWSStatusHandler creates a new ws status handler.
func NewWSStatusHandler(registry *agentregistry.Registry, hosts *store.HostsStore) *WSStatusHandler {
	return &WSStatusHandler{registry: registry, hosts: hosts}
}

// ServeStatusBulk handles GET /api/v1/ws/status?apiIds=id1,id2,id3
func (h *WSStatusHandler) ServeStatusBulk(w http.ResponseWriter, r *http.Request) {
	apiIdsParam := r.URL.Query().Get("apiIds")
	apiIds := []string{}
	seen := make(map[string]struct{})
	if len(apiIdsParam) > maxWSStatusQueryLength {
		Error(w, http.StatusBadRequest, "apiIds query is too large")
		return
	}
	if apiIdsParam != "" {
		for _, id := range strings.Split(apiIdsParam, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				if _, ok := seen[trimmed]; ok {
					continue
				}
				if len(apiIds) >= maxWSStatusAPIIDs {
					Error(w, http.StatusBadRequest, "too many apiIds requested")
					return
				}
				seen[trimmed] = struct{}{}
				apiIds = append(apiIds, trimmed)
			}
		}
	}

	statusMap := h.authorizedBulkStatus(w, r, apiIds)
	if statusMap == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    statusMap,
	})
}

// ServeStatusSingle handles GET /api/v1/ws/status/:apiId
func (h *WSStatusHandler) ServeStatusSingle(w http.ResponseWriter, r *http.Request) {
	apiID := chi.URLParam(r, "apiId")
	if apiID == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "apiId required"})
		return
	}

	statusMap := h.authorizedBulkStatus(w, r, []string{apiID})
	if statusMap == nil {
		return
	}
	info := statusMap[apiID]

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    info,
	})
}

// ServeSummary handles GET /api/v1/ws/status/summary.
func (h *WSStatusHandler) ServeSummary(w http.ResponseWriter, r *http.Request) {
	apiIDs, err := h.hosts.ListApiIDs(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to load host status summary")
		return
	}
	statuses := h.registry.GetBulk(apiIDs)
	connected := 0
	for _, status := range statuses {
		if status.Connected {
			connected++
		}
	}
	JSON(w, http.StatusOK, map[string]int{
		"connected": connected,
	})
}

func (h *WSStatusHandler) authorizedBulkStatus(w http.ResponseWriter, r *http.Request, apiIDs []string) map[string]agentregistry.ConnectionInfo {
	statusMap := h.registry.GetBulk(apiIDs)
	allowed, err := h.hosts.ListExistingApiIDs(r.Context(), apiIDs)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to load host status")
		return nil
	}
	for apiID := range statusMap {
		if _, ok := allowed[apiID]; !ok {
			statusMap[apiID] = agentregistry.ConnectionInfo{Connected: false, Secure: false}
		}
	}
	return statusMap
}
