package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// ReleaseNotesAcceptanceHandler handles marking release notes as accepted.
type ReleaseNotesAcceptanceHandler struct {
	store *store.ReleaseNotesAcceptanceStore
	log   *slog.Logger
}

// NewReleaseNotesAcceptanceHandler creates a new handler.
func NewReleaseNotesAcceptanceHandler(store *store.ReleaseNotesAcceptanceStore, log *slog.Logger) *ReleaseNotesAcceptanceHandler {
	return &ReleaseNotesAcceptanceHandler{store: store, log: log}
}

// AcceptRequest is the request body for POST /release-notes-acceptance/accept.
type AcceptRequest struct {
	Version string `json:"version"`
}

// Accept handles POST /api/v1/release-notes-acceptance/accept.
func (h *ReleaseNotesAcceptanceHandler) Accept(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req AcceptRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Version == "" {
		Error(w, http.StatusBadRequest, "Version is required")
		return
	}

	if err := h.store.Upsert(r.Context(), userID, req.Version); err != nil {
		if h.log != nil {
			h.log.Error("release notes acceptance upsert failed", "error", err, "user_id", userID, "version", req.Version)
		}
		if errors.Is(err, store.ErrReleaseNotesFKViolation) {
			Error(w, http.StatusUnauthorized, "Session may have expired. Please log in again.")
			return
		}
		if errors.Is(err, store.ErrReleaseNotesTableMissing) {
			Error(w, http.StatusServiceUnavailable, "Database migration required. Please restart the application.")
			return
		}
		Error(w, http.StatusInternalServerError, "Failed to accept release notes")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{"success": true})
}
