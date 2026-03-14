package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/go-chi/chi/v5"
)

// WSStatusHandler serves WebSocket connection status for the frontend.
type WSStatusHandler struct {
	registry *agentregistry.Registry
}

// NewWSStatusHandler creates a new ws status handler.
func NewWSStatusHandler(registry *agentregistry.Registry) *WSStatusHandler {
	return &WSStatusHandler{registry: registry}
}

// ServeStatusBulk handles GET /api/v1/ws/status?apiIds=id1,id2,id3
func (h *WSStatusHandler) ServeStatusBulk(w http.ResponseWriter, r *http.Request) {
	apiIdsParam := r.URL.Query().Get("apiIds")
	apiIds := []string{}
	if apiIdsParam != "" {
		for _, id := range strings.Split(apiIdsParam, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				apiIds = append(apiIds, trimmed)
			}
		}
	}

	statusMap := h.registry.GetBulk(apiIds)

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

	info := h.registry.Get(apiID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    info,
	})
}
