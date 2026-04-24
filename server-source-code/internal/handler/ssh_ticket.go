package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// SshTicketHandler handles SSH terminal ticket creation.
type SshTicketHandler struct {
	tickets *store.SshTicketStore
	hosts   *store.HostsStore
	db      database.DBProvider
	notify  *notifications.Emitter
}

// NewSshTicketHandler creates a new SSH ticket handler.
func NewSshTicketHandler(tickets *store.SshTicketStore, hosts *store.HostsStore, db database.DBProvider, notify *notifications.Emitter) *SshTicketHandler {
	return &SshTicketHandler{tickets: tickets, hosts: hosts, db: db, notify: notify}
}

// ServeCreate handles POST /auth/ssh-ticket.
func (h *SshTicketHandler) ServeCreate(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var req struct {
		HostID string `json:"hostId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.HostID == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Host ID is required"})
		return
	}

	ticket, err := h.tickets.CreateTicket(r.Context(), userID, req.HostID)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate SSH ticket"})
		return
	}

	// Emit ssh_session_started event.
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			hostName := req.HostID
			if host, err := h.hosts.GetByID(r.Context(), req.HostID); err == nil && host != nil {
				if host.FriendlyName != "" {
					hostName = host.FriendlyName
				} else if host.Hostname != nil && *host.Hostname != "" {
					hostName = *host.Hostname
				}
			}
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "ssh_session_started",
				Severity:      "informational",
				Title:         fmt.Sprintf("SSH Session - %s", hostName),
				Message:       fmt.Sprintf("SSH session initiated to host \"%s\".", hostName),
				ReferenceType: "host",
				ReferenceID:   req.HostID,
				Metadata: map[string]interface{}{
					"host_id":   req.HostID,
					"host_name": hostName,
					"user_id":   userID,
				},
			})
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"ticket":    ticket,
		"expiresIn": 30,
	})
}
