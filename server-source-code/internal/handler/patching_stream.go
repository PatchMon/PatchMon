package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/patchstream"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// patchStreamUpgrader is the WebSocket upgrader used for patch-run live streams.
// CheckOrigin returns true because CORS enforcement is handled by the router
// middleware before this handler is reached.
var patchStreamUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// patchStreamWriteTimeout bounds any single write to a connected browser so a
// stuck client can't pin a goroutine indefinitely.
const patchStreamWriteTimeout = 10 * time.Second

// patchStreamPingInterval is how often we send a ping frame to keep the
// connection alive through intermediaries.
const patchStreamPingInterval = 30 * time.Second

// ServeRunStream handles GET /patching/runs/{id}/stream.
// It upgrades to a WebSocket, sends an initial snapshot of the current run
// (stage + persisted shell_output), then forwards every live event published
// to the in-process hub for this patch run.
//
// Authentication is handled by the outer Auth middleware (JWT cookie / bearer).
// RBAC is enforced by RequirePermission("can_view_hosts") on the route.
// Because runs belong to a tenant-scoped host, the tenant middleware that
// precedes this handler already limits the patchRuns store to the caller's
// tenant, so looking up by ID will fail for cross-tenant access.
func (h *PatchingHandler) ServeRunStream(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil {
		http.Error(w, "Streaming not available", http.StatusServiceUnavailable)
		return
	}
	patchRunID := chi.URLParam(r, "id")
	if !isValidPatchUUID(patchRunID) {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	// Tenant-scoped lookup. If the run doesn't exist in this tenant's DB we
	// reject the upgrade instead of completing it.
	run, err := h.patchRuns.GetByID(r.Context(), patchRunID)
	if err != nil || run == nil {
		http.Error(w, "Patch run not found", http.StatusNotFound)
		return
	}

	// Subscribe before upgrading so we never miss an event that fires
	// between snapshot-send and the first read of the hub channel.
	events, unsubscribe := h.hub.Subscribe(patchRunID)
	defer unsubscribe()

	conn, err := patchStreamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Debug("patching stream upgrade failed", "patch_run_id", patchRunID, "error", err)
		return
	}
	defer func() { _ = conn.Close() }()

	// Close the connection as soon as the client goes away. We don't parse
	// incoming frames (the client is read-only for live output today), but
	// we still read so gorilla/websocket can process control frames and
	// detect disconnect promptly.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		conn.SetReadLimit(1024)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Send the initial snapshot: current stage + whatever output is already
	// persisted in the database. The frontend uses this to prime the terminal
	// buffer before the first live chunk arrives.
	snapshot := map[string]any{
		"type":          "snapshot",
		"patch_run_id":  patchRunID,
		"stage":         run.Status,
		"shell_output":  run.ShellOutput,
		"error_message": stringPtrValue(run.ErrorMessage),
	}
	if err := writeJSONWithDeadline(conn, snapshot); err != nil {
		return
	}

	// If the run is already in a terminal state when the client connects,
	// there is nothing live to wait for; emit a synthetic done and return.
	if isTerminalPatchStatus(run.Status) {
		_ = writeJSONWithDeadline(conn, map[string]any{
			"type":         "done",
			"patch_run_id": patchRunID,
			"stage":        run.Status,
		})
		return
	}

	pingTicker := time.NewTicker(patchStreamPingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-readDone:
			return
		case <-pingTicker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(patchStreamWriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := writeJSONWithDeadline(conn, ev); err != nil {
				return
			}
			if ev.Type == patchstream.EventDone {
				return
			}
		}
	}
}

// StopRun handles POST /patching/runs/{id}/stop.
// It looks up the run's agent via the agent WebSocket registry and sends a
// patch_run_stop command. The agent is responsible for sending SIGINT to the
// subprocess and eventually reporting a terminal "cancelled" stage.
func (h *PatchingHandler) StopRun(w http.ResponseWriter, r *http.Request) {
	patchRunID := chi.URLParam(r, "id")
	if !isValidPatchUUID(patchRunID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid run ID"})
		return
	}
	if h.registry == nil {
		JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Agent registry unavailable"})
		return
	}

	run, err := h.patchRuns.GetByID(r.Context(), patchRunID)
	if err != nil || run == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Patch run not found"})
		return
	}
	if isTerminalPatchStatus(run.Status) {
		JSON(w, http.StatusConflict, map[string]string{"error": "Run already finished"})
		return
	}

	host, err := h.hosts.GetByID(r.Context(), run.HostID)
	if err != nil || host == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Host not found"})
		return
	}

	if !h.registry.IsConnected(host.ApiID) {
		JSON(w, http.StatusConflict, map[string]string{"error": "Agent is not currently connected"})
		return
	}

	msg, err := json.Marshal(map[string]any{
		"type":         "patch_run_stop",
		"patch_run_id": patchRunID,
	})
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to encode stop message"})
		return
	}
	// Serialised write with a bounded deadline that is reset automatically
	// so we don't poison subsequent writers on this shared agent connection.
	sendErr := h.registry.SendMessageWithTimeout(host.ApiID, websocket.TextMessage, msg, patchStreamWriteTimeout)
	if sendErr != nil {
		h.log.Warn("patching: failed to send patch_run_stop to agent", "api_id", host.ApiID, "patch_run_id", patchRunID, "error", sendErr)
		JSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to reach agent"})
		return
	}

	h.log.Info("patching: patch_run_stop sent", "patch_run_id", patchRunID, "api_id", host.ApiID)
	JSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// writeJSONWithDeadline marshals v and writes it as a single text frame under
// the configured write deadline.
func writeJSONWithDeadline(conn *websocket.Conn, v any) error {
	_ = conn.SetWriteDeadline(time.Now().Add(patchStreamWriteTimeout))
	return conn.WriteJSON(v)
}

// isTerminalPatchStatus reports whether a run has already reached a final state.
func isTerminalPatchStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "validated", "dry_run_completed":
		return true
	default:
		return false
	}
}

// stringPtrValue dereferences a nullable string from the DB layer.
func stringPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SetStreamDependencies attaches stream and agent-registry collaborators to
// the handler. Kept as a setter so the existing constructor signature doesn't
// churn across every call-site.
func (h *PatchingHandler) SetStreamDependencies(hub *patchstream.Hub, registry *agentregistry.Registry) {
	h.hub = hub
	h.registry = registry
}
