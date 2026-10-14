package handler

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

// UsersHandler handles user admin routes.
type UsersHandler struct {
	users          *store.UsersStore
	sessions       *store.SessionsStore
	trustedDevices *store.TrustedDevicesStore
	permissions    *store.PermissionsStore
	settings       *store.SettingsStore
	resolved       *config.ResolvedConfig
	db             database.DBProvider
	notify         *notifications.Emitter
	log            *slog.Logger
	// adminMode mirrors cfg.AdminMode at construction time. Used to drive
	// auto-newsletter-subscription for SaaS-hosted users on creation.
	adminMode bool
	// Resolves the calling context's own settings; `resolved` is startup-only.
	cfgResolver          *hostctx.ConfigResolver
	cfg                  *config.Config
	trustUnverifiedEmail func(context.Context) bool
	tfaLockout           *store.TfaLockoutStore
	audit                *store.AuditLogsStore
	tfaReplay            *store.TfaReplayStore
}

// WithConfigResolver wires the per-context config resolver.
func (h *UsersHandler) WithConfigResolver(r *hostctx.ConfigResolver) *UsersHandler {
	h.cfgResolver = r
	return h
}

func (h *UsersHandler) resolvedFor(ctx context.Context) *config.ResolvedConfig {
	if rc := h.cfgResolver.Resolve(ctx); rc != nil {
		return rc
	}
	return h.resolved
}

// NewUsersHandler creates a new users handler.
func NewUsersHandler(users *store.UsersStore, sessions *store.SessionsStore, trustedDevices *store.TrustedDevicesStore, permissions *store.PermissionsStore, settings *store.SettingsStore, resolved *config.ResolvedConfig, cfg *config.Config, db database.DBProvider, notify *notifications.Emitter, log *slog.Logger) *UsersHandler {
	adminMode := cfg != nil && cfg.AdminMode
	return &UsersHandler{users: users, sessions: sessions, trustedDevices: trustedDevices, permissions: permissions, settings: settings, resolved: resolved, db: db, notify: notify, log: log, adminMode: adminMode, cfg: cfg}
}

// canAssignRole checks whether the calling user is allowed to assign the target role.
func (h *UsersHandler) canAssignRole(r *http.Request, callerRole, targetRole string) bool {
	return canGrantRole(r.Context(), h.permissions, callerRole, targetRole)
}

// List returns paginated users.
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	returnAll := r.URL.Query().Get("all") == "true"
	page := parseIntQuery(r, "page", 1)
	pageSize := parseIntQuery(r, "pageSize", 50)
	if pageSize > 200 {
		pageSize = 200
	}
	if returnAll {
		pageSize = 10000
	}
	offset := (page - 1) * pageSize

	users, err := h.users.List(r.Context(), pageSize, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to list users")
		return
	}
	total, _ := h.users.Count(r.Context())
	totalPages := 1
	if !returnAll && pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if totalPages < 1 {
			totalPages = 1
		}
	}
	if returnAll {
		pageSize = total
	}

	callerRole, _ := r.Context().Value(middleware.UserRoleKey).(string)
	perm, _ := h.permissions.GetByRole(r.Context(), callerRole)
	canManage := perm != nil && perm.CanManageUsers

	// Build response without password_hash
	data := make([]map[string]interface{}, len(users))
	for i, u := range users {
		data[i] = userToAdminResponse(&u)
		if !canManage {
			delete(data[i], "tfa_enabled")
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"data": data,
		"pagination": map[string]interface{}{
			"total": total, "page": page, "pageSize": pageSize, "totalPages": totalPages,
		},
	})
}

// ListForAssignment returns active users for assignment dropdowns.
func (h *UsersHandler) ListForAssignment(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.ListForAssignment(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch users")
		return
	}
	data := make([]map[string]interface{}, len(users))
	for i, u := range users {
		data[i] = map[string]interface{}{
			"id": u.ID, "username": u.Username, "email": u.Email,
			"first_name": u.FirstName, "last_name": u.LastName, "is_active": u.IsActive,
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{"data": data})
}

// Create creates a new user.
func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username  string  `json:"username"`
		Email     string  `json:"email"`
		Password  string  `json:"password"`
		FirstName *string `json:"first_name"`
		LastName  *string `json:"last_name"`
		Role      string  `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	if len(req.Username) < 3 {
		Error(w, http.StatusBadRequest, "Username must be at least 3 characters")
		return
	}
	if req.Email == "" {
		Error(w, http.StatusBadRequest, "Valid email is required")
		return
	}
	if req.Password == "" {
		Error(w, http.StatusBadRequest, "Password is required")
		return
	}
	if err := ValidatePasswordPolicy(h.resolvedFor(r.Context()), req.Password); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		s, _ := h.settings.GetFirst(r.Context())
		if s != nil && s.DefaultUserRole != "" {
			role = s.DefaultUserRole
		}
		if role == "" && h.resolved != nil {
			role = h.resolvedFor(r.Context()).DefaultUserRole
		}
		if role == "" && os.Getenv("DEFAULT_USER_ROLE") != "" {
			role = strings.TrimSpace(os.Getenv("DEFAULT_USER_ROLE"))
		}
		if role == "" {
			role = "user"
		}
	}

	// Enforce role escalation protection.
	callerRole, _ := r.Context().Value(middleware.UserRoleKey).(string)
	if !h.canAssignRole(r, callerRole, role) {
		Error(w, http.StatusForbidden, "You do not have permission to assign the role: "+role)
		return
	}

	exists, err := h.users.ExistsByUsernameOrEmail(r.Context(), req.Username, req.Email, "")
	if err != nil || exists {
		Error(w, http.StatusConflict, "Username or email already exists")
		return
	}

	// Enforce host user limit if a package is applied.
	if entry := hostctx.EntryFromContext(r.Context()); entry != nil && entry.MaxUsers != nil {
		count, countErr := h.users.Count(r.Context())
		if countErr == nil && count >= *entry.MaxUsers {
			Error(w, http.StatusForbidden, "User limit reached for this host's package")
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	hashStr := string(hash)
	u := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: &hashStr,
		Role:         role,
		IsActive:     true,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
	}
	if err := h.users.Create(r.Context(), u); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create user")
		return
	}
	AutoSubscribeIfHosted(r.Context(), h.adminMode, h.users, h.log, u)

	// Emit user_created event
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			callerID, _ := r.Context().Value(middleware.UserIDKey).(string)
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "user_created",
				Severity:      "informational",
				Title:         "User Created",
				Message:       "User " + u.Username + " was created with role " + u.Role + ".",
				ReferenceType: "user",
				ReferenceID:   u.ID,
				Metadata: map[string]interface{}{
					"user_id":    u.ID,
					"username":   u.Username,
					"role":       u.Role,
					"created_by": callerID,
				},
			})
		}
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"message": "User created successfully",
		"user":    userToAdminResponse(u),
	})
}

// Update updates a user.
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		Error(w, http.StatusBadRequest, "User ID required")
		return
	}

	existing, err := h.users.GetByID(r.Context(), userID)
	if err != nil || existing == nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}

	var req struct {
		Username  *string `json:"username"`
		Email     *string `json:"email"`
		FirstName *string `json:"first_name"`
		LastName  *string `json:"last_name"`
		Role      *string `json:"role"`
		IsActive  *bool   `json:"is_active"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Prevent modifying users with a higher-privilege role than the caller.
	callerRole, _ := r.Context().Value(middleware.UserRoleKey).(string)
	if !roleCovers(r.Context(), h.permissions, callerRole, existing.Role) {
		Error(w, http.StatusForbidden, "Cannot modify a user with a higher-privilege role")
		return
	}
	if existing.Role == "superadmin" && callerRole != "superadmin" {
		perm, permErr := h.permissions.GetByRole(r.Context(), callerRole)
		if permErr != nil || perm == nil || !perm.CanManageSuperusers {
			Error(w, http.StatusForbidden, "You do not have permission to modify superadmin users")
			return
		}
	}

	// Build update
	u := *existing
	if req.Username != nil {
		u.Username = *req.Username
	}
	if req.Email != nil {
		u.Email = *req.Email
	}
	if req.FirstName != nil {
		u.FirstName = req.FirstName
	}
	if req.LastName != nil {
		u.LastName = req.LastName
	}
	if req.Role != nil {
		trimmed := strings.TrimSpace(*req.Role)
		req.Role = &trimmed
		currentUserID, _ := r.Context().Value(middleware.UserIDKey).(string)
		// The token's role can be stale right after a demotion.
		assignerRole := ""
		if caller, err := h.users.GetByID(r.Context(), currentUserID); err == nil && caller != nil && caller.IsActive {
			assignerRole = caller.Role
		}
		// Enforce role escalation protection. Resending the stored role is not an assignment.
		if *req.Role != existing.Role && !h.canAssignRole(r, assignerRole, *req.Role) {
			Error(w, http.StatusForbidden, "You do not have permission to assign the role: "+*req.Role)
			return
		}
		// Prevent demoting yourself.
		if currentUserID != "" && userID == currentUserID && *req.Role != existing.Role {
			Error(w, http.StatusBadRequest, "Cannot change your own role")
			return
		}
		if *req.Role != existing.Role && !roleOutranks(r.Context(), h.permissions, assignerRole, existing.Role) {
			Error(w, http.StatusForbidden, "Cannot change the role of a user whose role is equal to or above yours")
			return
		}
		u.Role = *req.Role
	}
	if req.IsActive != nil {
		u.IsActive = *req.IsActive
	}

	// Check duplicates
	username := u.Username
	email := u.Email
	if req.Username != nil {
		username = *req.Username
	}
	if req.Email != nil {
		email = *req.Email
	}
	exists, _ := h.users.ExistsByUsernameOrEmail(r.Context(), username, email, userID)
	if exists {
		Error(w, http.StatusConflict, "Username or email already exists")
		return
	}

	if err := h.users.Update(r.Context(), &u); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	// Emit user_role_changed event when role actually changed.
	roleChanged := req.Role != nil && *req.Role != existing.Role
	if roleChanged {
		if h.notify != nil {
			if d := h.db.DB(r.Context()); d != nil {
				changedBy, _ := r.Context().Value(middleware.UserIDKey).(string)
				h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
					Type:          "user_role_changed",
					Severity:      "warning",
					Title:         "User Role Changed",
					Message:       "User " + u.Username + " role changed from " + existing.Role + " to " + *req.Role + ".",
					ReferenceType: "user",
					ReferenceID:   u.ID,
					Metadata: map[string]interface{}{
						"user_id":    u.ID,
						"username":   u.Username,
						"old_role":   existing.Role,
						"new_role":   *req.Role,
						"changed_by": changedBy,
					},
				})
			}
		}
	}

	// Revoke all sessions when role changes or account is deactivated,
	// so the old JWT privileges are immediately invalidated.
	// Deactivation also revokes MFA trust so that re-activation can't bypass MFA
	// using a trust row that predates the deactivation window.
	deactivated := req.IsActive != nil && !*req.IsActive && existing.IsActive
	if roleChanged || deactivated {
		if err := h.sessions.RevokeAllForUser(r.Context(), userID, ""); err != nil && h.log != nil {
			h.log.Error("failed to revoke sessions after user update", "user_id", userID, "error", err)
		}
	}
	if deactivated && h.trustedDevices != nil {
		if err := h.trustedDevices.RevokeAllForUser(r.Context(), userID); err != nil && h.log != nil {
			h.log.Error("failed to revoke trusted devices after deactivation", "user_id", userID, "error", err)
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "User updated successfully",
		"user":    userToAdminResponse(&u),
	})
}

// Delete deletes a user.
func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	currentUserID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if currentUserID != "" && userID == currentUserID {
		Error(w, http.StatusBadRequest, "Cannot delete your own account")
		return
	}

	existing, err := h.users.GetByID(r.Context(), userID)
	if err != nil || existing == nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}

	// Prevent deleting users with a higher-privilege role.
	callerRole, _ := r.Context().Value(middleware.UserRoleKey).(string)
	if !roleCovers(r.Context(), h.permissions, callerRole, existing.Role) {
		Error(w, http.StatusForbidden, "Cannot delete a user with a higher-privilege role")
		return
	}
	// Deleting a superadmin requires being superadmin or having can_manage_superusers.
	if existing.Role == "superadmin" && callerRole != "superadmin" {
		perm, permErr := h.permissions.GetByRole(r.Context(), callerRole)
		if permErr != nil || perm == nil || !perm.CanManageSuperusers {
			Error(w, http.StatusForbidden, "You do not have permission to delete superadmin users")
			return
		}
	}

	var superCount, adminCount int
	if existing.Role == "superadmin" {
		superCount, _ = h.users.CountSuperadmins(r.Context())
		if superCount <= 1 {
			Error(w, http.StatusBadRequest, "Cannot delete the last superadmin user")
			return
		}
	}
	if existing.Role == "admin" {
		superCount, _ = h.users.CountSuperadmins(r.Context())
		if superCount == 0 {
			adminCount, _ = h.users.CountActiveAdmins(r.Context())
			if adminCount <= 1 {
				Error(w, http.StatusBadRequest, "Cannot delete the last admin user")
				return
			}
		}
	}

	if err := h.users.Delete(r.Context(), userID); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	JSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
}

const auditEventUserOidcUnlink = "user_oidc_unlink"

// UnlinkOidc handles POST /auth/admin/users/{userId}/unlink-oidc. The user's
// next SSO login links again through the verified-email path.
func (h *UsersHandler) UnlinkOidc(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	callerID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if callerID != "" && userID == callerID {
		h.recordUnlinkAudit(r, callerID, userID, false, map[string]any{"reason": "self"})
		Error(w, http.StatusBadRequest, "Cannot unlink your own SSO identity")
		return
	}
	existing, err := h.users.GetByID(r.Context(), userID)
	if err != nil || existing == nil {
		h.recordUnlinkAudit(r, callerID, userID, false, map[string]any{"reason": "target_not_found"})
		Error(w, http.StatusNotFound, "User not found")
		return
	}

	// Strictly outrank: with SSO as the only login, unlink plus an email
	// change hands the account to whoever owns the new address.
	callerRole, _ := r.Context().Value(middleware.UserRoleKey).(string)
	if !roleOutranks(r.Context(), h.permissions, callerRole, existing.Role) {
		h.recordUnlinkAudit(r, callerID, existing.ID, false, map[string]any{"reason": "insufficient_role"})
		Error(w, http.StatusForbidden, "Cannot unlink the SSO identity of a user with an equal or higher-privilege role")
		return
	}

	if existing.OidcSub == nil && existing.OidcProvider == nil {
		h.recordUnlinkAudit(r, callerID, existing.ID, false, map[string]any{"reason": "not_linked"})
		Error(w, http.StatusBadRequest, "User is not linked to an SSO identity")
		return
	}
	if existing.Role == "superadmin" && h.oidcTrustUnverifiedEmail(r.Context()) {
		h.recordUnlinkAudit(r, callerID, existing.ID, false, map[string]any{"reason": "trust_unverified_email"})
		Error(w, http.StatusBadRequest, "Cannot unlink a superadmin while SSO trusts unverified email addresses. Turn off Trust unverified email (OIDC_TRUST_UNVERIFIED_EMAIL) first")
		return
	}
	if err := h.users.ClearOidcLink(r.Context(), userID); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to unlink SSO identity")
		return
	}

	if h.log != nil {
		h.log.Warn("oidc identity unlinked by admin",
			"actor_id", callerID, "user_id", existing.ID, "email", existing.Email,
			"previous_sub", strVal(existing.OidcSub, ""), "previous_provider", strVal(existing.OidcProvider, ""))
	}
	h.recordUnlinkAudit(r, callerID, existing.ID, true, map[string]any{
		"previous_sub":      strVal(existing.OidcSub, ""),
		"previous_provider": strVal(existing.OidcProvider, ""),
	})
	JSON(w, http.StatusOK, map[string]interface{}{"message": "SSO identity unlinked"})
}

func (h *UsersHandler) recordUnlinkAudit(r *http.Request, actorID, targetID string, success bool, metadata map[string]any) {
	if h.audit == nil {
		return
	}
	if err := h.audit.Record(r.Context(), newAuditEntry(r, auditEventUserOidcUnlink, actorID, targetID, success, metadata)); err != nil && h.log != nil {
		h.log.Error("oidc unlink: audit write failed", "actor_id", actorID, "error", err)
	}
}

func (h *UsersHandler) oidcTrustUnverifiedEmail(ctx context.Context) bool {
	if h.trustUnverifiedEmail != nil {
		return h.trustUnverifiedEmail(ctx)
	}
	if h.cfg == nil || h.settings == nil {
		return false
	}
	resolved, _ := config.ResolveOidcConfig(ctx, h.cfg, h.settings.GetFirst)
	return resolved.TrustUnverifiedEmail
}

// ResetPassword resets a user's password.
func (h *UsersHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	var req struct {
		NewPassword string `json:"newPassword"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.NewPassword = strings.TrimSpace(req.NewPassword)
	if req.NewPassword == "" {
		Error(w, http.StatusBadRequest, "New password is required")
		return
	}

	existing, err := h.users.GetByID(r.Context(), userID)
	if err != nil || existing == nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}
	if !existing.IsActive {
		Error(w, http.StatusBadRequest, "Cannot reset password for inactive user")
		return
	}

	// Prevent resetting the password of users with a higher-privilege role.
	callerRole, _ := r.Context().Value(middleware.UserRoleKey).(string)
	if !roleCovers(r.Context(), h.permissions, callerRole, existing.Role) {
		Error(w, http.StatusForbidden, "Cannot reset password for a user with a higher-privilege role")
		return
	}
	if existing.Role == "superadmin" && callerRole != "superadmin" {
		perm, permErr := h.permissions.GetByRole(r.Context(), callerRole)
		if permErr != nil || perm == nil || !perm.CanManageSuperusers {
			Error(w, http.StatusForbidden, "You do not have permission to reset superadmin passwords")
			return
		}
	}
	if existing.TfaEnrolmentRequired || existing.PasswordChangeRequired {
		callerID, _ := r.Context().Value(middleware.UserIDKey).(string)
		caller, err := h.users.GetByID(r.Context(), callerID)
		if err != nil || caller == nil || !h.canResetTfaOf(r.Context(), caller, existing) {
			Error(w, http.StatusForbidden, "This user is part-way through a two-factor reset. Only a superadmin or a user with a higher role can reset their password now.")
			return
		}
	}

	if err := ValidatePasswordPolicy(h.resolvedFor(r.Context()), req.NewPassword); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), userID, string(hash)); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to reset password")
		return
	}

	// Revoke all sessions and trusted devices for the target user after password
	// reset. An admin reset is the canonical post-compromise action; leaving
	// either surface alive would let an attacker retain access despite the reset.
	if err := h.sessions.RevokeAllForUser(r.Context(), userID, ""); err != nil && h.log != nil {
		h.log.Error("failed to revoke sessions after password reset", "user_id", userID, "error", err)
	}
	if h.trustedDevices != nil {
		if err := h.trustedDevices.RevokeAllForUser(r.Context(), userID); err != nil && h.log != nil {
			h.log.Error("failed to revoke trusted devices after password reset", "user_id", userID, "error", err)
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Password reset successfully",
		"user":    map[string]string{"id": existing.ID, "username": existing.Username, "email": existing.Email},
	})
}

func userToAdminResponse(u *models.User) map[string]interface{} {
	res := map[string]interface{}{
		"id": u.ID, "username": u.Username, "email": u.Email, "role": u.Role,
		"is_active": u.IsActive, "created_at": u.CreatedAt, "updated_at": u.UpdatedAt,
		"tfa_enabled": u.TfaEnabled,
	}
	if u.FirstName != nil {
		res["first_name"] = *u.FirstName
	}
	if u.LastName != nil {
		res["last_name"] = *u.LastName
	}
	if u.LastLogin != nil {
		res["last_login"] = u.LastLogin.Format(time.RFC3339)
	}
	res["avatar_url"] = u.AvatarURL
	res["oidc_linked"] = u.OidcSub != nil || u.OidcProvider != nil
	if u.OidcProvider != nil {
		res["oidc_provider"] = *u.OidcProvider
	}
	return res
}
