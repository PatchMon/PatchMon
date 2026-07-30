package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// wsStatusResponse is the per-host wire shape returned by the ws-status
// endpoint. Existing fields (`connected`, `secure`) stay byte-identical for
// back-compat; `disconnected_seconds_ago` is a new optional field used by the
// host-status pill redesign so the WS pill can render "disconnected Xs ago"
// without conflating WS state with host-report freshness.
//
// `disconnected_seconds_ago` is `nil` (JSON null) when the agent is currently
// connected OR when the registry has no recorded disconnect timestamp (e.g.
// the agent has never connected since this server started).
type wsStatusResponse struct {
	Connected              bool `json:"connected"`
	Secure                 bool `json:"secure"`
	DisconnectedSecondsAgo *int `json:"disconnected_seconds_ago"`
}

// toWSStatusResponse converts a raw ConnectionInfo into the wire shape.
func toWSStatusResponse(info agentregistry.ConnectionInfo) wsStatusResponse {
	resp := wsStatusResponse{Connected: info.Connected, Secure: info.Secure}
	if !info.Connected && info.DisconnectedAt != nil {
		secs := time.Since(*info.DisconnectedAt).Seconds()
		if secs < 0 {
			secs = 0
		}
		// Cap at a sane upper bound so a clock-skew event can't produce
		// astronomically large numbers in the JSON payload.
		if secs > math.MaxInt32 {
			secs = math.MaxInt32
		}
		v := int(secs)
		resp.DisconnectedSecondsAgo = &v
	}
	return resp
}

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
	wireMap := make(map[string]wsStatusResponse, len(statusMap))
	for id, info := range statusMap {
		wireMap[id] = toWSStatusResponse(info)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    wireMap,
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
	info := toWSStatusResponse(statusMap[apiID])

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
