package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/sessionrecording"
	"github.com/PatchMon/PatchMon/server-source-code/internal/sshbastion"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var sshLinuxUsername = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

type SSHRecordingsHandler struct {
	sshStore   *store.SSHStore
	recordings *sessionrecording.Store
}

func NewSSHRecordingsHandler(sshStore *store.SSHStore, recordings *sessionrecording.Store) *SSHRecordingsHandler {
	return &SSHRecordingsHandler{sshStore: sshStore, recordings: recordings}
}

func (h *SSHRecordingsHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.sshStore.ListAccounts(r.Context(), chi.URLParam(r, "hostId"))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to list SSH accounts")
		return
	}
	JSON(w, http.StatusOK, accounts)
}

func (h *SSHRecordingsHandler) UpsertAccount(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "root" || !sshLinuxUsername.MatchString(username) {
		Error(w, http.StatusBadRequest, "Invalid Linux account; root is disabled by default")
		return
	}
	var request struct {
		AllowSudo bool `json:"allowSudo"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request")
		return
	}
	account, err := h.sshStore.UpsertAccount(r.Context(), chi.URLParam(r, "hostId"), username, request.AllowSudo)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to save SSH account")
		return
	}
	JSON(w, http.StatusOK, account)
}

func (h *SSHRecordingsHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := h.sshStore.DeleteAccount(r.Context(), chi.URLParam(r, "hostId"), chi.URLParam(r, "username")); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete SSH account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SSHRecordingsHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))
	params := db.ListSSHSessionRecordingsParams{
		HostID: optionalQuery(query.Get("hostId")), UserID: optionalQuery(query.Get("userId")),
		LinuxUsername: optionalQuery(query.Get("linuxUsername")), Status: optionalQuery(query.Get("status")),
		LimitCount: int32(limit), OffsetCount: int32(offset),
	}
	if value := query.Get("startedAfter"); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			params.StartedAfter = pgtype.Timestamp{Time: parsed, Valid: true}
		} else {
			Error(w, 400, "Invalid startedAfter")
			return
		}
	}
	if value := query.Get("startedBefore"); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			params.StartedBefore = pgtype.Timestamp{Time: parsed, Valid: true}
		} else {
			Error(w, 400, "Invalid startedBefore")
			return
		}
	}
	rows, err := h.sshStore.ListRecordings(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to list recordings")
		return
	}
	JSON(w, http.StatusOK, rows)
}

func (h *SSHRecordingsHandler) Events(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if _, err := h.sshStore.GetSession(r.Context(), sessionID); err != nil {
		Error(w, http.StatusNotFound, "Recording not found")
		return
	}
	tenantID := sshbastion.TenantStorageID(hostctx.TenantHostKey(r.Context()))
	events, err := h.recordings.Read(r.Context(), tenantID, sessionID)
	if err != nil {
		Error(w, http.StatusNotFound, "Recording data is unavailable")
		return
	}
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	action := "view"
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if search != "" {
		action = "search"
		needle := strings.ToLower(search)
		filtered := events[:0]
		for _, event := range events {
			if event.Type == "output" && strings.Contains(strings.ToLower(event.Data), needle) {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}
	_ = h.sshStore.AuditRecordingAccess(r.Context(), sessionID, userID, action, clientAddress(r))
	JSON(w, http.StatusOK, map[string]interface{}{"events": events})
}

func optionalQuery(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func clientAddress(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}
