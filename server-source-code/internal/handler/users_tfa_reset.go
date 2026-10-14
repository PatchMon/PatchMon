package handler

import (
	"errors"
	"net/http"
	"strings"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const auditEventUserTfaReset = "user_tfa_reset"

// WithTfaReset wires the stores used by ResetTfa.
func (h *UsersHandler) WithTfaReset(tfaLockout *store.TfaLockoutStore, audit *store.AuditLogsStore) *UsersHandler {
	h.tfaLockout = tfaLockout
	h.audit = audit
	return h
}

type resetTfaRequest struct {
	Password string `json:"password"`
	TfaToken string `json:"tfaToken"`
}

// ResetTfa handles POST /auth/admin/users/{userId}/reset-tfa.
func (h *UsersHandler) ResetTfa(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targetID := chi.URLParam(r, "userId")
	actorID, _ := ctx.Value(middleware.UserIDKey).(string)
	if actorID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if targetID == "" {
		Error(w, http.StatusBadRequest, "User ID required")
		return
	}
	if targetID == actorID {
		h.logTfaReset(r, "self", actorID, targetID)
		Error(w, http.StatusBadRequest, "Use your profile to manage your own two-factor authentication")
		return
	}

	var req resetTfaRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Password == "" {
		Error(w, http.StatusBadRequest, "Your password is required to confirm this action")
		return
	}

	actor, err := h.users.GetByID(ctx, actorID)
	if err != nil || actor == nil || !actor.IsActive {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if actor.PasswordHash == nil {
		h.logTfaReset(r, "actor_no_password", actorID, targetID)
		Error(w, http.StatusForbidden, "Resetting two-factor authentication requires an account with a local password")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(strings.TrimSpace(*actor.PasswordHash)), []byte(req.Password)); err != nil {
		h.denyTfaReset(r, "invalid_password", actorID, targetID)
		Error(w, http.StatusForbidden, "Your password is incorrect")
		return
	}
	if actor.TfaEnabled {
		if status, msg := h.verifyActorTfa(r, actor, req.TfaToken); status != 0 {
			switch status {
			case http.StatusTooManyRequests:
				h.denyTfaReset(r, "tfa_locked_out", actorID, targetID)
			case http.StatusForbidden:
				h.denyTfaReset(r, "invalid_tfa_code", actorID, targetID)
			}
			Error(w, status, msg)
			return
		}
	}

	target, err := h.users.GetByID(ctx, targetID)
	if err != nil || target == nil {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			if h.log != nil {
				h.log.Error("admin tfa reset: target lookup failed", "actor_id", actorID, "error", err)
			}
			Error(w, http.StatusInternalServerError, "Failed to reset two-factor authentication")
			return
		}
		h.logTfaReset(r, "target_not_found", actorID, targetID)
		Error(w, http.StatusNotFound, "User not found")
		return
	}
	if actor.Role != "superadmin" && roleRank(target.Role) >= roleRank(actor.Role) {
		h.denyTfaReset(r, "insufficient_role", actorID, targetID)
		Error(w, http.StatusForbidden, "Cannot reset two-factor authentication for a user with an equal or higher-privilege role")
		return
	}
	if !target.TfaEnabled {
		Error(w, http.StatusBadRequest, "Two-factor authentication is not enabled for this user")
		return
	}

	reset, enrolmentRequired, err := h.users.AdminResetTfa(ctx, target.ID, newAuditEntry(r, auditEventUserTfaReset, actorID, target.ID, true, nil))
	if err != nil {
		if h.log != nil {
			h.log.Error("admin tfa reset failed", "actor_id", actorID, "target_user_id", target.ID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to reset two-factor authentication")
		return
	}
	if !reset {
		Error(w, http.StatusBadRequest, "Two-factor authentication is not enabled for this user")
		return
	}
	if h.tfaLockout != nil {
		h.tfaLockout.ClearLockout(ctx, target.ID)
	}

	if h.log != nil {
		attrs := []any{"actor_id", actorID, "actor_username", actor.Username, "target_user_id", target.ID, "target_username", target.Username, "enrolment_required", enrolmentRequired, "ip", requestClientIP(r)}
		if host := hostctx.TenantHostKey(ctx); host != "" {
			attrs = append(attrs, "host", host)
		}
		h.log.Info("admin reset user two-factor authentication", attrs...)
	}

	if h.notify != nil {
		if d := h.db.DB(ctx); d != nil {
			h.notify.EmitEvent(ctx, d, hostctx.TenantHostKey(ctx), notifications.Event{
				Type:          "user_tfa_disabled",
				Severity:      "warning",
				Title:         "Two-Factor Authentication Reset",
				Message:       "Two-factor authentication for user " + target.Username + " was reset by " + actor.Username + ".",
				ReferenceType: "user",
				ReferenceID:   target.ID,
				Metadata: map[string]interface{}{
					"user_id":           target.ID,
					"username":          target.Username,
					"reset_by":          actor.ID,
					"reset_by_username": actor.Username,
					"ip_address":        requestClientIP(r),
					"user_agent":        r.UserAgent(),
				},
			})
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":           "Two-factor authentication has been reset for " + target.Username,
		"user":              map[string]string{"id": target.ID, "username": target.Username},
		"enrolmentRequired": enrolmentRequired,
	})
}

// verifyActorTfa returns a non-zero status and message when the code is not accepted.
// Re-authentication failures are 403, never 401: the frontend treats a 401 as an expired session.
func (h *UsersHandler) verifyActorTfa(r *http.Request, actor *models.User, raw string) (int, string) {
	ctx := r.Context()
	if h.tfaLockout != nil {
		if locked, _ := h.tfaLockout.IsTFALocked(ctx, actor.ID); locked {
			return http.StatusTooManyRequests, "Too many failed two-factor attempts. Please try again later."
		}
	}
	token := strings.ToUpper(strings.TrimSpace(raw))
	if token == "" {
		return http.StatusForbidden, "Your two-factor code is required to confirm this action"
	}
	verified := false
	if len(token) == 6 && util.TokenRegex.MatchString(token) && actor.TfaSecret != nil {
		verified = util.VerifyTOTP(strings.TrimSpace(*actor.TfaSecret), token, util.TOTPWindow)
		if !verified {
			hashed := util.ParseBackupCodesJSON(actor.TfaBackupCodes)
			if ok, idx := util.VerifyBackupCode(token, hashed); ok {
				hashed = append(hashed[:idx], hashed[idx+1:]...)
				updated := util.EncodeBackupCodesJSON(hashed)
				if err := h.users.UpdateTfaBackupCodes(ctx, actor.ID, &updated); err != nil {
					if h.log != nil {
						h.log.Error("admin tfa reset: consume backup code failed", "actor_id", actor.ID, "error", err)
					}
					return http.StatusInternalServerError, "Failed to verify two-factor code"
				}
				verified = true
			}
		}
	}
	if !verified {
		if h.tfaLockout != nil {
			if _, locked := h.tfaLockout.RecordFailedAttempt(ctx, actor.ID); locked {
				return http.StatusTooManyRequests, "Too many failed two-factor attempts. Please try again later."
			}
		}
		return http.StatusForbidden, "Your two-factor code is incorrect"
	}
	if h.tfaLockout != nil {
		h.tfaLockout.ClearFailedAttempts(ctx, actor.ID)
	}
	return 0, ""
}

func (h *UsersHandler) logTfaReset(r *http.Request, reason, actorID, targetID string) {
	if h.log == nil {
		return
	}
	attrs := []any{"reason", reason, "actor_id", actorID, "target_user_id", truncateForLog(targetID, 64), "ip", requestClientIP(r)}
	if host := hostctx.TenantHostKey(r.Context()); host != "" {
		attrs = append(attrs, "host", host)
	}
	h.log.Warn("admin tfa reset denied", attrs...)
}

func (h *UsersHandler) denyTfaReset(r *http.Request, reason, actorID, targetID string) {
	h.logTfaReset(r, reason, actorID, targetID)
	if h.audit == nil {
		return
	}
	if err := h.audit.Record(r.Context(), newAuditEntry(r, auditEventUserTfaReset, actorID, targetID, false, map[string]any{"reason": reason})); err != nil && h.log != nil {
		h.log.Error("admin tfa reset: audit write failed", "actor_id", actorID, "error", err)
	}
}
