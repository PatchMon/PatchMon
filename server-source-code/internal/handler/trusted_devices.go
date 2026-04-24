package handler

import (
	"net/http"
	"time"

	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// TrustedDevicesHandler exposes the authenticated user's device-trust records.
// Each record represents a browser that may skip MFA until expiry or revocation.
type TrustedDevicesHandler struct {
	trustedDevices *store.TrustedDevicesStore
	log            *slog.Logger
}

// NewTrustedDevicesHandler creates a new trusted devices handler.
func NewTrustedDevicesHandler(trustedDevices *store.TrustedDevicesStore, log *slog.Logger) *TrustedDevicesHandler {
	return &TrustedDevicesHandler{trustedDevices: trustedDevices, log: log}
}

// List handles GET /auth/trusted-devices.
// Returns non-revoked, non-expired records for the current user. The token
// hash is deliberately not returned — it is server-only.
func (h *TrustedDevicesHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	devices, err := h.trustedDevices.ListForUser(r.Context(), userID)
	if err != nil {
		if h.log != nil {
			h.log.Error("trusted devices list failed", "user_id", userID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to load trusted devices")
		return
	}
	// Surface the caller's own trust cookie so the UI can badge "This device".
	var currentHash string
	if c, err := r.Cookie(DeviceTrustCookieName); err == nil && c.Value != "" {
		currentHash = store.HashTrustToken(c.Value)
	}
	out := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		out = append(out, map[string]any{
			"id":           d.ID,
			"label":        strDeref(d.Label),
			"user_agent":   strDeref(d.UserAgent),
			"ip_address":   strDeref(d.IPAddress),
			"created_at":   d.CreatedAt.Format(time.RFC3339),
			"last_used_at": d.LastUsedAt.Format(time.RFC3339),
			"expires_at":   d.ExpiresAt.Format(time.RFC3339),
			"is_current":   currentHash != "" && currentHash == d.TokenHash,
		})
	}
	JSON(w, http.StatusOK, map[string]any{"trusted_devices": out})
}

// Revoke handles DELETE /auth/trusted-devices/{id}.
func (h *TrustedDevicesHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		Error(w, http.StatusBadRequest, "id is required")
		return
	}
	// Determine if the caller is revoking their own current device so we can
	// tell the browser to drop the cookie at the same time.
	revokingCurrent := false
	if c, err := r.Cookie(DeviceTrustCookieName); err == nil && c.Value != "" {
		hash := store.HashTrustToken(c.Value)
		if hash != "" {
			if td, err := h.trustedDevices.FindValid(r.Context(), userID, hash); err == nil && td != nil && td.ID == id {
				revokingCurrent = true
			}
		}
	}
	if err := h.trustedDevices.RevokeByID(r.Context(), id, userID); err != nil {
		if h.log != nil {
			h.log.Error("trusted device revoke failed", "user_id", userID, "id", id, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to revoke trusted device")
		return
	}
	if revokingCurrent {
		clearDeviceTrustCookie(w, r)
	}
	JSON(w, http.StatusOK, map[string]string{"message": "Trusted device revoked"})
}

// RevokeAll handles DELETE /auth/trusted-devices.
func (h *TrustedDevicesHandler) RevokeAll(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.trustedDevices.RevokeAllForUser(r.Context(), userID); err != nil {
		if h.log != nil {
			h.log.Error("trusted devices revoke all failed", "user_id", userID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to revoke trusted devices")
		return
	}
	clearDeviceTrustCookie(w, r)
	JSON(w, http.StatusOK, map[string]string{"message": "All trusted devices revoked"})
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
