package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const auditEventUserTfaReset = "user_tfa_reset"

// canResetTfaOf: superadmin may act on anyone else; everyone else only on a role it outranks.
func (h *UsersHandler) canResetTfaOf(ctx context.Context, actor, target *models.User) bool {
	if actor.ID == target.ID {
		return false
	}
	return roleOutranks(ctx, h.permissions, actor.Role, target.Role)
}

// WithTfaReset wires the stores used by ResetTfa.
func (h *UsersHandler) WithTfaReset(tfaLockout *store.TfaLockoutStore, audit *store.AuditLogsStore, replay *store.TfaReplayStore) *UsersHandler {
	h.tfaLockout = tfaLockout
	h.audit = audit
	h.tfaReplay = replay
	return h
}

type resetTfaRequest struct {
	Password    string `json:"password"`
	TfaToken    string `json:"tfaToken"`
	NewPassword string `json:"newPassword"`
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
	req.Password = strings.TrimSpace(req.Password)
	req.NewPassword = strings.TrimSpace(req.NewPassword)
	if req.Password == "" {
		Error(w, http.StatusBadRequest, "Your password is required to confirm this action")
		return
	}

	actor, err := h.users.GetByID(ctx, actorID)
	if err != nil || actor == nil {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if !actor.IsActive {
		h.denyTfaReset(r, "actor_inactive", actorID, targetID)
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if actor.PasswordHash == nil {
		h.denyTfaReset(r, "actor_no_password", actorID, targetID)
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
		h.denyTfaReset(r, "target_not_found", actorID, targetID)
		Error(w, http.StatusNotFound, "User not found")
		return
	}
	if !h.canResetTfaOf(ctx, actor, target) {
		h.denyTfaReset(r, "insufficient_role", actorID, targetID)
		Error(w, http.StatusForbidden, "Cannot reset two-factor authentication for a user with an equal or higher-privilege role")
		return
	}
	if !target.TfaEnabled {
		h.denyTfaReset(r, "target_tfa_not_enabled", actorID, target.ID)
		Error(w, http.StatusBadRequest, "Two-factor authentication is not enabled for this user")
		return
	}

	var tempHash *string
	if req.NewPassword != "" {
		if target.PasswordHash == nil {
			h.denyTfaReset(r, "temp_password_sso_target", actorID, target.ID)
			Error(w, http.StatusBadRequest, "This account has no local password because it signs in through single sign-on")
			return
		}
		if err := ValidatePasswordPolicy(h.resolvedFor(ctx), req.NewPassword); err != nil {
			h.denyTfaReset(r, "temp_password_policy", actorID, target.ID)
			Error(w, http.StatusBadRequest, err.Error())
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
		if err != nil {
			Error(w, http.StatusInternalServerError, "Failed to reset two-factor authentication")
			return
		}
		hs := string(hash)
		tempHash = &hs
	}

	reset, enrolmentRequired, err := h.users.AdminResetTfa(ctx, target.ID, tempHash, newAuditEntry(r, auditEventUserTfaReset, actorID, target.ID, true, nil))
	if err != nil {
		if h.log != nil {
			h.log.Error("admin tfa reset failed", "actor_id", actorID, "target_user_id", target.ID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to reset two-factor authentication")
		return
	}
	if !reset {
		h.denyTfaReset(r, "target_tfa_not_enabled", actorID, target.ID)
		Error(w, http.StatusBadRequest, "Two-factor authentication is not enabled for this user")
		return
	}
	if h.tfaLockout != nil {
		h.tfaLockout.ClearLockout(ctx, target.ID)
	}

	if h.log != nil {
		attrs := []any{"actor_id", actorID, "actor_username", actor.Username, "target_user_id", target.ID, "target_username", target.Username, "enrolment_required", enrolmentRequired, "temporary_password", tempHash != nil, "ip", requestClientIP(r)}
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
		"message":                "Two-factor authentication has been reset for " + target.Username,
		"user":                   map[string]string{"id": target.ID, "username": target.Username},
		"enrolmentRequired":      enrolmentRequired,
		"passwordChangeRequired": tempHash != nil,
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
	if strings.TrimSpace(raw) == "" {
		return http.StatusForbidden, "Your two-factor code is required to confirm this action"
	}
	verified, err := verifyTfaCode(ctx, h.users, h.tfaReplay, actor, raw)
	if err != nil {
		if h.log != nil {
			h.log.Error("admin tfa reset: verify actor code failed", "actor_id", actor.ID, "error", err)
		}
		return http.StatusInternalServerError, "Failed to verify two-factor code"
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
