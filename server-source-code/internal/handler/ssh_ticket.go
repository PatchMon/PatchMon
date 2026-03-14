package handler

import (
	"encoding/json"
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// SshTicketHandler handles SSH terminal ticket creation.
type SshTicketHandler struct {
	tickets *store.SshTicketStore
}

// NewSshTicketHandler creates a new SSH ticket handler.
func NewSshTicketHandler(tickets *store.SshTicketStore) *SshTicketHandler {
	return &SshTicketHandler{tickets: tickets}
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

	JSON(w, http.StatusOK, map[string]interface{}{
		"ticket":    ticket,
		"expiresIn": 30,
	})
}
