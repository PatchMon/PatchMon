package handler

import (
	"encoding/base64"
	"net/http"
	"strings"

	"log/slog"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
)

// TfaHandler handles TFA management routes (setup, verify-setup, disable, status, regenerate).
type TfaHandler struct {
	users    *store.UsersStore
	sessions *store.SessionsStore
	db       database.DBProvider
	notify   *notifications.Emitter
	log      *slog.Logger
}

// NewTfaHandler creates a new TFA handler.
func NewTfaHandler(users *store.UsersStore, sessions *store.SessionsStore, db database.DBProvider, notify *notifications.Emitter, log *slog.Logger) *TfaHandler {
	return &TfaHandler{users: users, sessions: sessions, db: db, notify: notify, log: log}
}

// Setup handles GET /tfa/setup.
func (h *TfaHandler) Setup(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}
	if user.TfaEnabled {
		Error(w, http.StatusBadRequest, "Two-factor authentication is already enabled for this account")
		return
	}
	if user.OidcSub != nil || user.OidcProvider != nil {
		Error(w, http.StatusBadRequest, "MFA is managed by your OIDC provider")
		return
	}

	secret, otpauthURL, err := util.GenerateTOTPSecret("PatchMon", "PatchMon ("+user.Username+")")
	if err != nil {
		if h.log != nil {
			h.log.Error("tfa setup generate secret", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to setup two-factor authentication")
		return
	}

	if err := h.users.UpdateTfaSecret(r.Context(), userID, &secret); err != nil {
		if h.log != nil {
			h.log.Error("tfa setup save secret", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to setup two-factor authentication")
		return
	}

	qrPNG, err := qrcode.Encode(otpauthURL, qrcode.Medium, 256)
	if err != nil {
		if h.log != nil {
			h.log.Error("tfa setup qr encode", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to generate QR code")
		return
	}
	qrDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPNG)

	JSON(w, http.StatusOK, map[string]interface{}{
		"secret":         secret,
		"qrCode":         qrDataURL,
		"manualEntryKey": secret,
	})
}

// VerifySetupRequest is the request body for verify-setup.
type VerifySetupRequest struct {
	Token string `json:"token"`
}

// VerifySetup handles POST /tfa/verify-setup.
func (h *TfaHandler) VerifySetup(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req VerifySetupRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if len(req.Token) != 6 || !util.TokenRegex.MatchString(strings.ToUpper(req.Token)) {
		Error(w, http.StatusBadRequest, "Token must be exactly 6 digits")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || user.TfaSecret == nil || user.TfaEnabled {
		Error(w, http.StatusBadRequest, "No TFA secret found. Please start the setup process first.")
		return
	}

	secret := strings.TrimSpace(*user.TfaSecret)
	if !util.VerifyTOTP(secret, req.Token, util.TOTPWindow) {
		Error(w, http.StatusBadRequest, "Invalid verification code. Please try again.")
		return
	}

	codes := util.GenerateBackupCodes(util.BackupCodeCount)
	hashed, err := util.HashBackupCodes(codes)
	if err != nil {
		if h.log != nil {
			h.log.Error("tfa verify-setup hash codes", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to enable two-factor authentication")
		return
	}
	jsonStr := util.EncodeBackupCodesJSON(hashed)
	if err := h.users.UpdateTfaEnabled(r.Context(), userID, &jsonStr); err != nil {
		if h.log != nil {
			h.log.Error("tfa verify-setup enable", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to enable two-factor authentication")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":     "Two-factor authentication has been enabled successfully",
		"backupCodes": codes,
	})
}

// DisableRequest is the request body for disable.
type DisableRequest struct {
	Password string `json:"password"`
}

// Disable handles POST /tfa/disable.
func (h *TfaHandler) Disable(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req DisableRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Password == "" {
		Error(w, http.StatusBadRequest, "Password is required to disable TFA")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}
	if !user.TfaEnabled {
		Error(w, http.StatusBadRequest, "Two-factor authentication is not enabled for this account")
		return
	}
	if user.PasswordHash == nil {
		Error(w, http.StatusBadRequest, "Cannot disable TFA for accounts without a password (e.g., OIDC-only accounts)")
		return
	}

	hash := strings.TrimSpace(*user.PasswordHash)
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		Error(w, http.StatusUnauthorized, "Invalid password")
		return
	}

	if err := h.users.DisableTfa(r.Context(), userID); err != nil {
		if h.log != nil {
			h.log.Error("tfa disable", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to disable two-factor authentication")
		return
	}

	// Emit user_tfa_disabled event
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "user_tfa_disabled",
				Severity:      "warning",
				Title:         "Two-Factor Authentication Disabled",
				Message:       "Two-factor authentication was disabled for user " + user.Username + ".",
				ReferenceType: "user",
				ReferenceID:   user.ID,
				Metadata: map[string]interface{}{
					"user_id":    user.ID,
					"username":   user.Username,
					"ip_address": r.RemoteAddr,
					"user_agent": r.UserAgent(),
				},
			})
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Two-factor authentication has been disabled successfully",
	})
}

// Status handles GET /tfa/status.
func (h *TfaHandler) Status(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}

	hasBackupCodes := user.TfaBackupCodes != nil && *user.TfaBackupCodes != "" && *user.TfaBackupCodes != "[]"
	JSON(w, http.StatusOK, map[string]interface{}{
		"enabled":        user.TfaEnabled,
		"hasBackupCodes": hasBackupCodes,
	})
}

// RegenerateBackupCodes handles POST /tfa/regenerate-backup-codes.
func (h *TfaHandler) RegenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}
	if !user.TfaEnabled {
		Error(w, http.StatusBadRequest, "Two-factor authentication is not enabled for this account")
		return
	}

	codes := util.GenerateBackupCodes(util.BackupCodeCount)
	hashed, err := util.HashBackupCodes(codes)
	if err != nil {
		if h.log != nil {
			h.log.Error("tfa regenerate hash codes", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to regenerate backup codes")
		return
	}
	jsonStr := util.EncodeBackupCodesJSON(hashed)
	if err := h.users.UpdateTfaBackupCodes(r.Context(), userID, &jsonStr); err != nil {
		if h.log != nil {
			h.log.Error("tfa regenerate save", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to regenerate backup codes")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":     "Backup codes have been regenerated successfully",
		"backupCodes": codes,
	})
}
