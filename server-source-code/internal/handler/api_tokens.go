package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// UserApiTokenHandler handles CRUD for user-owned long-lived API tokens.
type UserApiTokenHandler struct {
	tokens *store.UserApiTokenStore
	log    *slog.Logger
}

func NewUserApiTokenHandler(tokens *store.UserApiTokenStore, log *slog.Logger) *UserApiTokenHandler {
	return &UserApiTokenHandler{tokens: tokens, log: log}
}

// generateRawToken produces a new patchmon_at_<hex> token string.
func generateRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "patchmon_at_" + hex.EncodeToString(b), nil
}

// hashToken returns the SHA-256 hex digest of the token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// List handles GET /api-tokens — returns all tokens for the authenticated user.
func (h *UserApiTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	rows, err := h.tokens.List(r.Context(), userID)
	if err != nil {
		if h.log != nil {
			h.log.Error("user_api_tokens list failed", "error", err, "user_id", userID)
		}
		Error(w, http.StatusInternalServerError, "Failed to list API tokens")
		return
	}

	items := make([]store.UserApiTokenListItem, len(rows))
	for i, row := range rows {
		items[i] = store.RowToUserApiTokenListItem(row)
	}
	JSON(w, http.StatusOK, items)
}

// Create handles POST /api-tokens — creates a new token and returns the raw value once.
func (h *UserApiTokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Name      string  `json:"name"`
		ExpiresAt *string `json:"expires_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		Error(w, http.StatusBadRequest, "name is required (max 100 chars)")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			Error(w, http.StatusBadRequest, "expires_at must be a valid RFC 3339 date")
			return
		}
		expiresAt = &t
	}

	rawToken, err := generateRawToken()
	if err != nil {
		if h.log != nil {
			h.log.Error("user_api_tokens generate token failed", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to generate API token")
		return
	}

	record, err := h.tokens.Create(r.Context(), db.CreateUserApiTokenParams{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      req.Name,
		TokenHash: hashToken(rawToken),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		if h.log != nil {
			h.log.Error("user_api_tokens create failed", "error", err, "user_id", userID)
		}
		Error(w, http.StatusInternalServerError, "Failed to create API token")
		return
	}

	// Return the raw token ONCE — it is never retrievable again
	type createResponse struct {
		store.UserApiTokenListItem
		Token string `json:"token"`
	}
	JSON(w, http.StatusCreated, createResponse{
		UserApiTokenListItem: store.RowToUserApiTokenListItem(record),
		Token:                rawToken,
	})
}

// Delete handles DELETE /api-tokens/{id} — revokes a token belonging to the caller.
func (h *UserApiTokenHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	role, _ := r.Context().Value(middleware.UserRoleKey).(string)
	tokenID := chi.URLParam(r, "id")

	// Fetch the token by hash is not available for revocation — we look it up
	// indirectly: list user's tokens and check ownership, or use admin bypass.
	// For simplicity we do an existence check via List and filter.
	rows, err := h.tokens.List(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to look up API token")
		return
	}

	ownsToken := false
	for _, row := range rows {
		if row.ID == tokenID {
			ownsToken = true
			break
		}
	}

	if !ownsToken && role != "admin" && role != "superadmin" {
		Error(w, http.StatusForbidden, "Forbidden")
		return
	}

	if err := h.tokens.Delete(r.Context(), tokenID); err != nil {
		if h.log != nil {
			h.log.Error("user_api_tokens delete failed", "error", err, "token_id", tokenID)
		}
		Error(w, http.StatusInternalServerError, "Failed to revoke API token")
		return
	}

	JSON(w, http.StatusOK, map[string]string{"message": "API token revoked"})
}
