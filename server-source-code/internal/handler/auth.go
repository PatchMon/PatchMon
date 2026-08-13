package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/clientip"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Note: AuthHandler uses sessions and settings stores for profile, signup, etc.

// AuthHandler handles auth routes.
type AuthHandler struct {
	cfg                    *config.Config
	resolved               *config.ResolvedConfig
	users                  *store.UsersStore
	sessions               *store.SessionsStore
	trustedDevices         *store.TrustedDevicesStore
	settings               *store.SettingsStore
	tfaLockout             *store.TfaLockoutStore
	loginLockout           *store.LoginLockoutStore
	pendingLogin           *store.PendingLoginStore
	releaseNotesAcceptance *store.ReleaseNotesAcceptanceStore
	db                     database.DBProvider
	notify                 *notifications.Emitter
	log                    *slog.Logger
	// permissions is optional - used by MeContext to surface role permission flags
	// (such as can_manage_billing) alongside the user response. Wired via
	// WithPermissions to avoid threading it through the constructor signature.
	permissions *store.PermissionsStore
	// Resolves the calling context's own settings; `resolved` is startup-only.
	cfgResolver *hostctx.ConfigResolver
}

// WithConfigResolver wires the per-context config resolver.
func (h *AuthHandler) WithConfigResolver(r *hostctx.ConfigResolver) *AuthHandler {
	h.cfgResolver = r
	return h
}

// resolvedFor returns the effective config for ctx's context.
func (h *AuthHandler) resolvedFor(ctx context.Context) *config.ResolvedConfig {
	if rc := h.cfgResolver.Resolve(ctx); rc != nil {
		return rc
	}
	return h.resolved
}

// WithPermissions attaches a PermissionsStore to the handler. Returns the handler
// so it can be chained from NewAuthHandler. Safe to call with nil.
func (h *AuthHandler) WithPermissions(p *store.PermissionsStore) *AuthHandler {
	h.permissions = p
	return h
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(cfg *config.Config, resolved *config.ResolvedConfig, users *store.UsersStore, sessions *store.SessionsStore, trustedDevices *store.TrustedDevicesStore, settings *store.SettingsStore, tfaLockout *store.TfaLockoutStore, loginLockout *store.LoginLockoutStore, pendingLogin *store.PendingLoginStore, releaseNotesAcceptance *store.ReleaseNotesAcceptanceStore, db database.DBProvider, notify *notifications.Emitter, log *slog.Logger) *AuthHandler {
	return &AuthHandler{cfg: cfg, resolved: resolved, users: users, sessions: sessions, trustedDevices: trustedDevices, settings: settings, tfaLockout: tfaLockout, loginLockout: loginLockout, pendingLogin: pendingLogin, releaseNotesAcceptance: releaseNotesAcceptance, db: db, notify: notify, log: log}
}

// DeviceTrustCookieName is the HttpOnly cookie carrying the raw trust token.
// Decoupled from the session cookies: survives logout, dies only on revoke or expiry.
const DeviceTrustCookieName = "patchmon_device_trust"

// hasValidDeviceTrust looks up a trust record for the given user based on the
// inbound patchmon_device_trust cookie. Returns the matched record or nil.
// The match is keyed exclusively on (user_id, sha256(cookie_value)) — no IP,
// user-agent, or fingerprint is involved, so the trust survives network changes
// and browser updates.
func (h *AuthHandler) hasValidDeviceTrust(r *http.Request, userID string) *models.TrustedDevice {
	if h.trustedDevices == nil {
		return nil
	}
	c, err := r.Cookie(DeviceTrustCookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	hash := store.HashTrustToken(c.Value)
	if hash == "" {
		return nil
	}
	td, err := h.trustedDevices.FindValid(r.Context(), userID, hash)
	if err != nil {
		if h.log != nil {
			h.log.Debug("trusted device lookup failed", "user_id", userID, "error", err)
		}
		return nil
	}
	return td
}

// setDeviceTrustCookie writes the raw trust token to the response as an
// HttpOnly cookie with a lifetime matching the trust record's expiry.
func (h *AuthHandler) setDeviceTrustCookie(w http.ResponseWriter, r *http.Request, rawToken string, expiresAt time.Time) {
	secure := isSecureRequest(r) && h.cfg.Env == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     DeviceTrustCookieName,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearDeviceTrustCookie expires the trust cookie on the client. Called only
// on explicit revocation paths, never on ordinary logout.
func clearDeviceTrustCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     DeviceTrustCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// LoginRequest is the request body for login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the response for login (frontend expects token, not access_token).
type LoginResponse struct {
	Token        string       `json:"token"`
	AccessToken  string       `json:"access_token,omitempty"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	ExpiresAt    string       `json:"expires_at,omitempty"`
	User         UserResponse `json:"user"`
}

// UserResponse is a user in API responses.
type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.log != nil {
		h.log.Debug("auth request", "method", r.Method, "path", r.URL.Path)
	}
	if h.cfg.OidcEnabled && h.cfg.OidcDisableLocalAuth {
		h.logLoginFailure(r, "local_auth_disabled", "", "")
		Error(w, http.StatusForbidden, "Local authentication is disabled. Please use SSO.")
		return
	}
	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		h.logLoginFailure(r, "malformed_request", "", "")
		if h.log != nil {
			h.log.Debug("auth login invalid body", "error", err)
		}
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		h.logLoginFailure(r, "missing_credentials", req.Username, "")
		Error(w, http.StatusBadRequest, "username and password required")
		return
	}
	if h.log != nil {
		h.log.Debug("auth login attempt", "username", req.Username)
	}

	// Login lockout: check before password verification
	if h.loginLockout != nil {
		identifier := h.loginLockout.Identifier(h.clientIP(r), req.Username)
		if locked, remainingSec := h.loginLockout.IsLocked(r.Context(), identifier); locked {
			h.logLoginFailure(r, "locked_out", req.Username, "")
			writeLoginLockedResponse(w, remainingSec)
			return
		}
	}

	user, err := h.users.GetByUsernameOrEmail(r.Context(), req.Username)
	if err != nil {
		// A DB failure is not a wrong username. Counting it would let an outage
		// lock out the very people retrying a correct password.
		if !errors.Is(err, pgx.ErrNoRows) {
			if h.log != nil {
				h.log.Error("auth login lookup failed", "ip", h.clientIP(r), "error", err)
			}
			Error(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		// An unknown username consumes an attempt too, so that a 429 is reachable
		// for names that do not exist and stops being an "account exists" signal.
		// Deliberately no notification event: the username is unbounded attacker
		// input and each distinct guess would write a row.
		if h.loginLockout != nil {
			identifier := h.loginLockout.Identifier(h.clientIP(r), req.Username)
			if _, locked := h.loginLockout.RecordFailedAttempt(r.Context(), identifier); locked {
				h.logLoginFailure(r, "user_not_found", req.Username, "", "locked", true)
				writeLoginLockedResponse(w, h.lockoutRemaining(r.Context(), identifier))
				return
			}
		}
		h.logLoginFailure(r, "user_not_found", req.Username, "")
		Error(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	if h.log != nil {
		h.log.Debug("auth login user found", "user_id", user.ID, "username", user.Username, "is_active", user.IsActive, "has_password", user.PasswordHash != nil)
	}
	if !user.IsActive {
		h.logLoginFailure(r, "account_disabled", req.Username, user.ID)
		Error(w, http.StatusUnauthorized, "Account is disabled")
		return
	}
	if user.PasswordHash == nil {
		h.logLoginFailure(r, "no_password_set", req.Username, user.ID)
		Error(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Trim hash - database/text drivers can introduce trailing whitespace that breaks bcrypt
	hash := strings.TrimSpace(*user.PasswordHash)
	// Trim password - forms often introduce accidental leading/trailing whitespace
	password := strings.TrimSpace(req.Password)
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		if h.loginLockout != nil {
			identifier := h.loginLockout.Identifier(h.clientIP(r), req.Username)
			_, locked := h.loginLockout.RecordFailedAttempt(r.Context(), identifier)
			if locked {
				// Emit account_locked event
				if h.notify != nil {
					if d := h.db.DB(r.Context()); d != nil {
						h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
							Type:          "account_locked",
							Severity:      "error",
							Title:         "Account Locked",
							Message:       "Account locked due to too many failed login attempts for user " + user.Username + ".",
							ReferenceType: "user",
							ReferenceID:   user.ID,
							Metadata: map[string]interface{}{
								"user_id":    user.ID,
								"username":   user.Username,
								"ip_address": h.clientIP(r),
								"user_agent": r.UserAgent(),
							},
						})
					}
				}
				h.logLoginFailure(r, "invalid_password", req.Username, user.ID, "locked", true)
				writeLoginLockedResponse(w, h.lockoutRemaining(r.Context(), identifier))
				return
			}
		}
		// Emit user_login_failed event
		if h.notify != nil {
			if d := h.db.DB(r.Context()); d != nil {
				h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
					Type:          "user_login_failed",
					Severity:      "warning",
					Title:         "Failed Login Attempt",
					Message:       "Failed login attempt for user " + user.Username + ".",
					ReferenceType: "user",
					ReferenceID:   user.ID,
					Metadata: map[string]interface{}{
						"user_id":    user.ID,
						"username":   user.Username,
						"ip_address": h.clientIP(r),
						"user_agent": r.UserAgent(),
						"reason":     "invalid_password",
					},
				})
			}
		}
		h.logLoginFailure(r, "invalid_password", req.Username, user.ID)
		if h.log != nil {
			h.log.Debug("auth login bcrypt failed", "user_id", user.ID, "hash_len", len(hash), "bcrypt_err", err.Error())
		}
		Error(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Clear lockout on success
	if h.loginLockout != nil {
		identifier := h.loginLockout.Identifier(h.clientIP(r), req.Username)
		h.loginLockout.ClearFailedAttempts(r.Context(), identifier)
	}

	if h.log != nil {
		attrs := []any{"user_id", user.ID, "username", user.Username, "ip", h.clientIP(r)}
		if host := hostctx.TenantHostKey(r.Context()); host != "" {
			attrs = append(attrs, "host", host)
		}
		h.log.Info("login succeeded", attrs...)
	}

	// TFA check: if enabled, require TFA verification unless a valid device-trust
	// cookie is present. Trust is keyed on (user_id, sha256(cookie_value)) only;
	// it is independent from the user_sessions table, network, and user-agent.
	if user.TfaEnabled {
		td := h.hasValidDeviceTrust(r, user.ID)
		if td == nil {
			// Single-use proof the password was verified; VerifyTfa requires it.
			if h.pendingLogin == nil {
				if h.log != nil {
					h.log.Error("auth: pending-login store not configured, cannot start TFA")
				}
				Error(w, http.StatusInternalServerError, "Unable to start two-factor verification")
				return
			}
			ticket, err := h.pendingLogin.Create(r.Context(), user.ID)
			if err != nil {
				if h.log != nil {
					h.log.Error("auth: failed to issue pending-login ticket", "user_id", user.ID, "error", err)
				}
				Error(w, http.StatusInternalServerError, "Unable to start two-factor verification")
				return
			}
			JSON(w, http.StatusOK, map[string]interface{}{
				"message":     "TFA verification required",
				"requiresTfa": true,
				"username":    user.Username,
				"tfaTicket":   ticket,
			})
			return
		}
		// Best-effort touch of last_used_at for the audit trail.
		if err := h.trustedDevices.TouchLastUsed(r.Context(), td.ID); err != nil && h.log != nil {
			h.log.Debug("trusted device touch failed", "id", td.ID, "error", err)
		}
	}

	h.completeLogin(w, r, user, false)
}

// VerifyTfaRequest is the request body for verify-tfa.
type VerifyTfaRequest struct {
	Username   string `json:"username"`
	Token      string `json:"token"`
	RememberMe bool   `json:"remember_me"` // frontend sends snake_case
	// Single-use proof of the first factor, issued by Login.
	TfaTicket string `json:"tfa_ticket"`
}

// VerifyTfa handles POST /auth/verify-tfa.
func (h *AuthHandler) VerifyTfa(w http.ResponseWriter, r *http.Request) {
	var req VerifyTfaRequest
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Token = strings.ToUpper(strings.TrimSpace(req.Token))
	if len(req.Token) != 6 {
		Error(w, http.StatusBadRequest, "6-character token required")
		return
	}
	if !util.TokenRegex.MatchString(req.Token) {
		h.logLoginFailure(r, "tfa_token_malformed", "", "")
		Error(w, http.StatusBadRequest, "Token must be 6 alphanumeric characters")
		return
	}

	// Consumed whether or not the code is correct, so a captured ticket cannot
	// be replayed. The user comes from the ticket, never from the request body.
	if h.pendingLogin == nil {
		if h.log != nil {
			h.log.Error("auth: pending-login store not configured, refusing TFA verification")
		}
		Error(w, http.StatusInternalServerError, "Two-factor verification unavailable")
		return
	}
	userID, err := h.pendingLogin.Consume(r.Context(), req.TfaTicket)
	if err != nil {
		// The username is whatever the client typed; the trusted identity comes
		// from the ticket, which is exactly what failed here.
		h.logLoginFailure(r, "tfa_ticket_invalid", "", "", "claimed_username", truncateForLog(req.Username, 64))
		Error(w, http.StatusUnauthorized, "Login session expired, please sign in again")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || !user.IsActive || !user.TfaEnabled || user.TfaSecret == nil {
		h.logLoginFailure(r, "tfa_user_ineligible", "", userID)
		Error(w, http.StatusUnauthorized, "Invalid credentials or TFA not enabled")
		return
	}

	if h.tfaLockout != nil {
		locked, _ := h.tfaLockout.IsTFALocked(r.Context(), user.ID)
		if locked {
			h.logLoginFailure(r, "tfa_locked_out", user.Username, user.ID)
			Error(w, http.StatusTooManyRequests, "Too many failed TFA attempts. Please try again later.")
			return
		}
	}

	secret := strings.TrimSpace(*user.TfaSecret)
	verified := util.VerifyTOTP(secret, req.Token, util.TOTPWindow)

	if !verified {
		hashed := util.ParseBackupCodesJSON(user.TfaBackupCodes)
		if len(hashed) > 0 {
			valid, idx := util.VerifyBackupCode(req.Token, hashed)
			if valid {
				verified = true
				hashed = append(hashed[:idx], hashed[idx+1:]...)
				updated := util.EncodeBackupCodesJSON(hashed)
				_ = h.users.UpdateTfaBackupCodes(r.Context(), user.ID, &updated)
			}
		}
	}

	if !verified {
		if h.tfaLockout != nil {
			attempts, locked := h.tfaLockout.RecordFailedAttempt(r.Context(), user.ID)
			if locked {
				h.logLoginFailure(r, "invalid_tfa_code", user.Username, user.ID, "locked", true)
				Error(w, http.StatusTooManyRequests, "Too many failed TFA attempts. Please try again later.")
				return
			}
			h.logLoginFailure(r, "invalid_tfa_code", user.Username, user.ID)
			remaining := h.getMaxTfaAttempts(r.Context()) - attempts
			if remaining < 0 {
				remaining = 0
			}
			JSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error":             "Invalid verification code",
				"remainingAttempts": remaining,
			})
			return
		}
		h.logLoginFailure(r, "invalid_tfa_code", user.Username, user.ID)
		Error(w, http.StatusUnauthorized, "Invalid verification code")
		return
	}

	if h.tfaLockout != nil {
		h.tfaLockout.ClearFailedAttempts(r.Context(), user.ID)
	}

	h.completeLogin(w, r, user, req.RememberMe)
}

func cookieSameSite(env string, useLax, secure bool) http.SameSite {
	if !useLax && env == "production" && secure {
		return http.SameSiteStrictMode
	}
	return http.SameSiteLaxMode
}

// setAccessCookie writes the access-token cookie. Shared by login and by
// /auth/refresh. Refresh passes useLax=false: the Lax relaxation exists for the
// IdP redirect chain at login time, and a refresh is an ordinary same-origin XHR
// that has no need of it.
func setAccessCookie(w http.ResponseWriter, r *http.Request, accessToken string, tokenMaxAge int64, env string, useLax, browserSessionCookies bool) {
	secure := isSecureRequest(r)
	maxAge := int(tokenMaxAge)
	if browserSessionCookies {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    accessToken,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure && env == "production",
		SameSite: cookieSameSite(env, useLax, secure),
	})
}

// setAuthCookiesWithRemember sets cookies; rememberMe uses 30-day refresh token.
// useLax forces SameSite=Lax (required for OIDC redirects from IdP).
// browserSessionCookies: when true, both cookies use MaxAge 0 (session cookies) so they are not
// persisted to disk and are dropped when the browser session ends (close all windows / quit).
func setAuthCookiesWithRemember(w http.ResponseWriter, r *http.Request, accessToken, refreshToken string, tokenMaxAge int64, rememberMe bool, env string, useLax bool, browserSessionCookies bool) {
	secure := isSecureRequest(r)
	sameSite := cookieSameSite(env, useLax, secure)
	setAccessCookie(w, r, accessToken, tokenMaxAge, env, useLax, browserSessionCookies)
	refreshMaxAge := 7 * 24 * 3600 // 7 days
	if rememberMe {
		refreshMaxAge = 30 * 24 * 3600 // 30 days
	}
	if browserSessionCookies {
		refreshMaxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   refreshMaxAge,
		HttpOnly: true,
		Secure:   secure && env == "production",
		SameSite: sameSite,
	})
}

// completeLogin creates tokens, optionally creates session for remember-me, sets cookies, returns JSON.
func (h *AuthHandler) completeLogin(w http.ResponseWriter, r *http.Request, user *models.User, rememberMe bool) {
	expiresIn := h.getJwtExpiresInSeconds(r.Context())

	// Losing a timestamp is not a reason to fail an otherwise valid sign-in.
	if err := h.users.UpdateLastLogin(r.Context(), user.ID, time.Now()); err != nil && h.log != nil {
		h.log.Error("failed to record last login", "user_id", user.ID, "error", err)
	}

	refreshExpSec := int64(7 * 24 * 3600)
	if rememberMe {
		refreshExpSec = 30 * 24 * 3600
	}
	refreshToken, _ := h.createRefreshToken(user.ID, user.Role, refreshExpSec)

	// Always create/reuse a session so it shows in the active sessions list.
	// Session reuse is keyed on X-Device-ID (stable across IP/UA changes).
	var sessionID string
	fingerprint := util.GenerateDeviceFingerprint(r)
	deviceID := r.Header.Get("X-Device-ID")
	expiresAtSession := time.Now().Add(time.Duration(refreshExpSec) * time.Second)
	// Session rows no longer drive the TFA bypass decision — pass false/nil so the
	// legacy tfa_remember_me / tfa_bypass_until columns stay at their defaults.
	// Device trust lives in user_trusted_devices instead.
	sess, sessErr := h.sessions.CreateOrReuseSession(r.Context(), user.ID, refreshToken, "", h.clientIP(r), r.UserAgent(), fingerprint, deviceID, expiresAtSession, false, nil)
	if sessErr != nil {
		if h.log != nil {
			h.log.Error("session creation failed", "user_id", user.ID, "error", sessErr)
		}
	} else if sess != nil {
		sessionID = sess.ID
	}

	// Mint a device-trust token when the user asked to skip MFA on this device.
	var trustExpiresAt time.Time
	if rememberMe && user.TfaEnabled && h.trustedDevices != nil {
		trustDuration := parseTfaRememberDuration(h.getTfaRememberMeExpiresIn(r.Context()))
		trustExpiresAt = time.Now().Add(trustDuration)
		rawToken, tokenHash, genErr := store.GenerateTrustToken()
		if genErr != nil {
			if h.log != nil {
				h.log.Error("trust token generation failed", "user_id", user.ID, "error", genErr)
			}
		} else {
			label := buildDeviceLabel(r.UserAgent())
			if _, err := h.trustedDevices.Create(r.Context(), store.CreateTrustedDeviceParams{
				UserID:    user.ID,
				TokenHash: tokenHash,
				DeviceID:  deviceID,
				UserAgent: r.UserAgent(),
				IPAddress: h.clientIP(r),
				Label:     label,
				ExpiresAt: trustExpiresAt,
			}); err != nil {
				if h.log != nil {
					h.log.Error("trusted device create failed", "user_id", user.ID, "error", err)
				}
			} else {
				h.setDeviceTrustCookie(w, r, rawToken, trustExpiresAt)
			}
		}
	}

	accessToken, err := h.createAccessToken(user.ID, user.Role, sessionID, expiresIn)
	if err != nil {
		if h.log != nil {
			h.log.Error("auth token creation failed", "user_id", user.ID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)

	setAuthCookiesWithRemember(w, r, accessToken, refreshToken, expiresIn, rememberMe, h.cfg.Env, false, h.authBrowserSessionCookies(r.Context()))

	// Emit user_login event
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "user_login",
				Severity:      "informational",
				Title:         "User Login",
				Message:       "User " + user.Username + " logged in successfully.",
				ReferenceType: "user",
				ReferenceID:   user.ID,
				Metadata: map[string]interface{}{
					"user_id":    user.ID,
					"username":   user.Username,
					"role":       user.Role,
					"ip_address": h.clientIP(r),
					"user_agent": r.UserAgent(),
				},
			})
		}
	}

	resp := map[string]interface{}{
		"message":       "Login successful",
		"token":         accessToken,
		"refresh_token": refreshToken,
		"expires_in":    expiresIn,
		"expires_at":    expiresAt,
		"user":          h.buildUserResponse(r.Context(), user),
	}
	if rememberMe && !trustExpiresAt.IsZero() {
		resp["tfa_bypass_until"] = trustExpiresAt.Format(time.RFC3339)
	}
	JSON(w, http.StatusOK, resp)
}

// buildDeviceLabel produces a short "Browser on OS" string from a User-Agent.
// Used for display in the Trusted Devices list; never used for trust decisions.
func buildDeviceLabel(ua string) string {
	p := parseUserAgent(ua)
	browser := p["browser"]
	os := p["os"]
	if browser == "" {
		browser = "Unknown browser"
	}
	if os == "" {
		os = "Unknown OS"
	}
	return browser + " on " + os
}

// clientIP returns the IP used for login lockout and session audit records.
//
// When TRUST_PROXY is on, the RealIP middleware has already resolved
// X-Forwarded-For into RemoteAddr; when it is off, RemoteAddr is the raw peer.
// Either way RemoteAddr is the correct source, and reading the header here
// would reintroduce lockout evasion via a client-supplied leftmost entry.
func (h *AuthHandler) clientIP(r *http.Request) string {
	if ip := clientip.FromRequest(r); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

// truncateForLog bounds unauthenticated input before it reaches the log.
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.ToValidUTF8(s[:max], "") + "..."
}

// logLoginFailure records a rejected sign-in at Warn, so it is visible at the
// default log level rather than only under LOG_LEVEL=debug. One line per
// rejected request; extra carries any additional slog attributes.
func (h *AuthHandler) logLoginFailure(r *http.Request, reason, username, userID string, extra ...any) {
	if h.log == nil {
		return
	}
	attrs := []any{
		"reason", reason,
		"ip", h.clientIP(r),
		"user_agent", truncateForLog(r.UserAgent(), 200),
	}
	if username != "" {
		attrs = append(attrs, "username", truncateForLog(username, 64))
	}
	if userID != "" {
		attrs = append(attrs, "user_id", userID)
	}
	if host := hostctx.TenantHostKey(r.Context()); host != "" {
		attrs = append(attrs, "host", host)
	}
	h.log.Warn("login failed", append(attrs, extra...)...)
}

// lockoutRemaining returns seconds left on a lockout, falling back to the
// configured duration when Redis cannot supply a TTL.
func (h *AuthHandler) lockoutRemaining(ctx context.Context, identifier string) int {
	if h.loginLockout == nil {
		return 0
	}
	if _, remainingSec := h.loginLockout.IsLocked(ctx, identifier); remainingSec > 0 {
		return remainingSec
	}
	if h.resolved != nil {
		return h.resolvedFor(ctx).LockoutDurationMin * 60
	}
	return 900
}

func writeLoginLockedResponse(w http.ResponseWriter, remainingSec int) {
	w.Header().Set("Retry-After", strconv.Itoa(remainingSec))
	JSON(w, http.StatusTooManyRequests, map[string]interface{}{
		"message":           "Too many failed login attempts. Try again later.",
		"remaining_seconds": remainingSec,
	})
}

// authBrowserSessionCookies returns whether to use session-only cookies (env -> DB -> default).
func (h *AuthHandler) authBrowserSessionCookies(ctx context.Context) bool {
	if rc := h.resolvedFor(ctx); rc != nil {
		return rc.AuthBrowserSessionCookies
	}
	return h.cfg.AuthBrowserSessionCookies
}

// getJwtExpiresInSeconds returns JWT access token expiry in seconds (resolved from env -> DB -> default).
func (h *AuthHandler) getJwtExpiresInSeconds(ctx context.Context) int64 {
	s := h.cfg.JWTExpiresIn
	if rc := h.resolvedFor(ctx); rc != nil && rc.JwtExpiresIn != "" {
		s = rc.JwtExpiresIn
	}
	return parseJwtExpiresInSeconds(s)
}

func parseJwtExpiresInSeconds(s string) int64 {
	d := parseTfaRememberDuration(s)
	return int64(d.Seconds())
}

// getTfaRememberMeExpiresIn returns TFA remember-me duration string (resolved from env -> DB -> default).
func (h *AuthHandler) getTfaRememberMeExpiresIn(ctx context.Context) string {
	if rc := h.resolvedFor(ctx); rc != nil && rc.TfaRememberMeExpiresIn != "" {
		return rc.TfaRememberMeExpiresIn
	}
	return h.cfg.TfaRememberMeExpiresIn
}

// getMaxTfaAttempts returns max TFA attempts before lockout (resolved from env -> DB -> default).
func (h *AuthHandler) getMaxTfaAttempts(ctx context.Context) int {
	if rc := h.resolvedFor(ctx); rc != nil && rc.MaxTfaAttempts > 0 {
		return rc.MaxTfaAttempts
	}
	return h.cfg.MaxTfaAttempts
}

func parseTfaRememberDuration(s string) time.Duration {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) < 2 {
		return 30 * 24 * time.Hour
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return 30 * 24 * time.Hour
	}
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(n) * 24 * time.Hour
	case 'h':
		return time.Duration(n) * time.Hour
	case 'm':
		return time.Duration(n) * time.Minute
	default:
		return 30 * 24 * time.Hour
	}
}

// CompleteOidcLogin creates tokens, sets cookies (SameSite=Lax for IdP redirects), and redirects to success.
// Used by OIDC callback after successful authentication.
func (h *AuthHandler) CompleteOidcLogin(w http.ResponseWriter, r *http.Request, user *models.User) {
	expiresIn := h.getJwtExpiresInSeconds(r.Context())
	refreshExpSec := int64(7 * 24 * 3600)
	refreshToken, _ := h.createRefreshToken(user.ID, user.Role, refreshExpSec)
	var sessionID string
	fingerprint := util.GenerateDeviceFingerprint(r)
	deviceID := r.Header.Get("X-Device-ID")
	expiresAtSession := time.Now().Add(time.Duration(refreshExpSec) * time.Second)
	if sess, err := h.sessions.CreateOrReuseSession(r.Context(), user.ID, refreshToken, "", h.clientIP(r), r.UserAgent(), fingerprint, deviceID, expiresAtSession, false, nil); err != nil {
		if h.log != nil {
			h.log.Error("oidc session creation failed", "user_id", user.ID, "error", err)
		}
	} else if sess != nil {
		sessionID = sess.ID
	}
	accessToken, err := h.createAccessToken(user.ID, user.Role, sessionID, expiresIn)
	if err != nil {
		if h.log != nil {
			h.log.Error("oidc token creation failed", "user_id", user.ID, "error", err)
		}
		http.Redirect(w, r, "/login?error=Authentication+failed", http.StatusFound)
		return
	}
	setAuthCookiesWithRemember(w, r, accessToken, refreshToken, expiresIn, false, h.cfg.Env, true, h.authBrowserSessionCookies(r.Context()))

	// Emit user_login event for OIDC login
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "user_login",
				Severity:      "informational",
				Title:         "User Login",
				Message:       "User " + user.Username + " logged in via OIDC.",
				ReferenceType: "user",
				ReferenceID:   user.ID,
				Metadata: map[string]interface{}{
					"user_id":    user.ID,
					"username":   user.Username,
					"role":       user.Role,
					"ip_address": h.clientIP(r),
					"user_agent": r.UserAgent(),
					"method":     "oidc",
				},
			})
		}
	}

	// Redirect to root (not /login) so the SPA auth bootstrap validates the fresh cookie
	// via /auth/profile and renders the dashboard directly. Going to /login would force the
	// Login page to mount first and flash before a second full reload. Relative redirect
	// avoids ERR_INVALID_REDIRECT from malformed CORS_ORIGIN.
	http.Redirect(w, r, "/?oidc=success", http.StatusFound)
}

// CompleteDiscordLogin creates tokens, sets cookies, and redirects to /?discord=success.
// Used by Discord OAuth callback after successful authentication.
func (h *AuthHandler) CompleteDiscordLogin(w http.ResponseWriter, r *http.Request, user *models.User) {
	expiresIn := h.getJwtExpiresInSeconds(r.Context())
	refreshExpSec := int64(7 * 24 * 3600)
	refreshToken, _ := h.createRefreshToken(user.ID, user.Role, refreshExpSec)
	var sessionID string
	fingerprint := util.GenerateDeviceFingerprint(r)
	deviceID := r.Header.Get("X-Device-ID")
	expiresAtSession := time.Now().Add(time.Duration(refreshExpSec) * time.Second)
	if sess, err := h.sessions.CreateOrReuseSession(r.Context(), user.ID, refreshToken, "", h.clientIP(r), r.UserAgent(), fingerprint, deviceID, expiresAtSession, false, nil); err != nil {
		if h.log != nil {
			h.log.Error("discord session creation failed", "user_id", user.ID, "error", err)
		}
	} else if sess != nil {
		sessionID = sess.ID
	}
	accessToken, err := h.createAccessToken(user.ID, user.Role, sessionID, expiresIn)
	if err != nil {
		if h.log != nil {
			h.log.Error("discord token creation failed", "user_id", user.ID, "error", err)
		}
		http.Redirect(w, r, "/login?error=Authentication+failed", http.StatusFound)
		return
	}
	setAuthCookiesWithRemember(w, r, accessToken, refreshToken, expiresIn, false, h.cfg.Env, true, h.authBrowserSessionCookies(r.Context()))

	// Emit user_login event for Discord login
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "user_login",
				Severity:      "informational",
				Title:         "User Login",
				Message:       "User " + user.Username + " logged in via Discord.",
				ReferenceType: "user",
				ReferenceID:   user.ID,
				Metadata: map[string]interface{}{
					"user_id":    user.ID,
					"username":   user.Username,
					"role":       user.Role,
					"ip_address": h.clientIP(r),
					"user_agent": r.UserAgent(),
					"method":     "discord",
				},
			})
		}
	}

	// Redirect to root (not /login) so the SPA auth bootstrap validates the fresh cookie
	// via /auth/profile and renders the dashboard directly. Relative redirect avoids
	// ERR_INVALID_REDIRECT from malformed CORS_ORIGIN.
	http.Redirect(w, r, "/?discord=success", http.StatusFound)
}

// isSecureRequest returns true if the request is over HTTPS (TLS or X-Forwarded-Proto).
func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// clearAuthCookies removes auth cookies (matches Node backend).
// Secure must match the original cookie for the browser to clear it.
func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := isSecureRequest(r)
	http.SetCookie(w, &http.Cookie{Name: "token", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure})
}

// Refresh tokens are stored against the session row, never accepted as bearer.
const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

// sessionID is mandatory: the middleware gates its whole session-validity
// block on the claim being present, so a token without one is unrevocable.
func (h *AuthHandler) createAccessToken(userID, role, sessionID string, expSec int64) (string, error) {
	if sessionID == "" {
		return "", errors.New("refusing to mint a session-less access token")
	}
	return h.createToken(userID, role, expSec, sessionID, tokenTypeAccess)
}

func (h *AuthHandler) createRefreshToken(userID, role string, expSec int64) (string, error) {
	return h.createToken(userID, role, expSec, "", tokenTypeRefresh)
}

// Used by first-run admin setup and signup, which log the new user straight in.
func (h *AuthHandler) issueSessionTokens(r *http.Request, u *models.User, expiresIn, refreshExpSec int64) (accessToken, refreshToken string, err error) {
	refreshToken, err = h.createRefreshToken(u.ID, u.Role, refreshExpSec)
	if err != nil {
		return "", "", err
	}
	sess, err := h.sessions.CreateOrReuseSession(
		r.Context(), u.ID, refreshToken, "",
		h.clientIP(r), r.UserAgent(),
		util.GenerateDeviceFingerprint(r), r.Header.Get("X-Device-ID"),
		time.Now().Add(time.Duration(refreshExpSec)*time.Second), false, nil,
	)
	if err != nil {
		return "", "", err
	}
	if sess == nil {
		return "", "", errors.New("session creation returned no session")
	}
	accessToken, err = h.createAccessToken(u.ID, u.Role, sess.ID, expiresIn)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func (h *AuthHandler) createToken(userID, role string, expSec int64, sessionID, tokenType string) (string, error) {
	claims := jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"typ":  tokenType,
		"exp":  time.Now().Add(time.Duration(expSec) * time.Second).Unix(),
		"iat":  time.Now().Unix(),
	}
	if sessionID != "" {
		claims["sessionId"] = sessionID
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.cfg.JWTSecret))
}

// Profile handles GET /auth/profile.
func (h *AuthHandler) Profile(w http.ResponseWriter, r *http.Request) {
	if h.log != nil {
		h.log.Debug("auth request", "method", r.Method, "path", r.URL.Path)
	}
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"user": h.buildUserResponse(r.Context(), user),
	})
}

// MeContext handles GET /me/context.
// Returns the authenticated user plus the current multi-context ("tenant") info,
// including the enabled modules list. The frontend uses this to feature-flag
// navigation items, settings panels, and buttons so that disabled features are
// hidden rather than clicking through to a 403.
//
// Response shape:
//
//	{
//	  "user":   { ...buildUserResponse... },
//	  "tenant": {
//	    "modules":  "core,patching,..."  |  "*"  (string; "*" means all modules),
//	    "host":     "customer.patchmon.cloud" | "" (empty in single-context mode),
//	    "slug":     "customer" | "" (empty in single-context mode),
//	    "multi_context": true|false
//	  }
//	}
//
// In single-context (self-hosted) mode, "modules" is "*" so every feature is allowed.
func (h *AuthHandler) MeContext(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}

	// Resolve the multi-context entry (if any). Nil entry = single-context mode.
	entry := hostctx.EntryFromContext(r.Context())
	tenant := map[string]interface{}{
		"multi_context": entry != nil,
		"host":          "",
		"slug":          "",
		"modules":       "*", // default: single-context mode allows everything
	}
	if entry != nil {
		tenant["host"] = entry.Host
		tenant["slug"] = entry.Slug
		if entry.Modules != nil {
			mods := strings.TrimSpace(*entry.Modules)
			if mods == "" {
				mods = "*"
			}
			tenant["modules"] = mods
		}
	}

	// Surface the role permission flags that the frontend needs to feature-flag
	// nav items and buttons. Currently only can_manage_billing is exposed this way;
	// the broader permissions list is available at /permissions/user-permissions.
	// Keeping this a small, fixed set avoids accidentally leaking sensitive flags.
	resp := map[string]interface{}{
		"user":   h.buildUserResponse(r.Context(), user),
		"tenant": tenant,
	}
	if h.permissions != nil {
		canManageBilling := false
		if p, err := h.permissions.GetByRole(r.Context(), user.Role); err == nil && p != nil {
			canManageBilling = p.CanManageBilling
		} else if user.Role == "admin" || user.Role == "superadmin" {
			// Built-in admin roles default to full access even if the row is missing.
			canManageBilling = true
		}
		resp["permissions"] = map[string]bool{
			"can_manage_billing": canManageBilling,
		}
	}
	// Surface admin_mode so the frontend can double-gate the Billing nav item
	// alongside the can_manage_billing permission flag.
	resp["admin_mode"] = h.cfg != nil && h.cfg.AdminMode

	JSON(w, http.StatusOK, resp)
}

// UpdateProfile handles PUT /auth/profile (update own profile: username, email, first_name, last_name).
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
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
	// OIDC users cannot modify profile fields managed by IdP
	if user.OidcSub != nil || user.OidcProvider != nil {
		Error(w, http.StatusForbidden, "Profile information is managed by your OIDC provider and cannot be modified here")
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	extractStr := func(keys ...string) *string {
		for _, k := range keys {
			if v, ok := req[k]; ok && v != nil {
				if s, ok := v.(string); ok {
					t := strings.TrimSpace(s)
					return &t
				}
			}
		}
		return nil
	}
	// For first_name/last_name: key present with empty string means clear
	extractName := func(keys ...string) *string {
		for _, k := range keys {
			if _, has := req[k]; has {
				if v, ok := req[k].(string); ok {
					t := strings.TrimSpace(v)
					if t == "" {
						return nil // explicit clear
					}
					return &t
				}
				return nil // null
			}
		}
		return nil
	}
	username := extractStr("username")
	email := extractStr("email")
	firstName := extractName("first_name", "firstName")
	lastName := extractName("last_name", "lastName")
	u := *user
	if username != nil {
		if len(*username) < 3 {
			Error(w, http.StatusBadRequest, "Username must be at least 3 characters")
			return
		}
		u.Username = *username
	}
	if email != nil {
		lower := strings.ToLower(*email)
		if lower == "" {
			Error(w, http.StatusBadRequest, "Valid email is required")
			return
		}
		u.Email = lower
	}
	// firstName/lastName: extractName returns nil if key present but empty (clear), or key not present (no change)
	if _, hasFirst := req["first_name"]; hasFirst {
		u.FirstName = firstName
	} else if _, hasFirst := req["firstName"]; hasFirst {
		u.FirstName = firstName
	}
	if _, hasLast := req["last_name"]; hasLast {
		u.LastName = lastName
	} else if _, hasLast := req["lastName"]; hasLast {
		u.LastName = lastName
	}
	// Check username/email uniqueness excluding current user
	checkUsername := u.Username
	checkEmail := u.Email
	if username != nil {
		checkUsername = *username
	}
	if email != nil {
		checkEmail = strings.ToLower(*email)
	}
	exists, _ := h.users.ExistsByUsernameOrEmail(r.Context(), checkUsername, checkEmail, userID)
	if exists {
		Error(w, http.StatusConflict, "Username or email already exists")
		return
	}
	if err := h.users.Update(r.Context(), &u); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update profile")
		return
	}
	// Fetch fresh user for response
	updated, _ := h.users.GetByID(r.Context(), userID)
	if updated == nil {
		updated = &u
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Profile updated successfully",
		"user":    h.buildUserResponse(r.Context(), updated),
	})
}

// newsletterDisplayName builds the name passed to Listmonk: trimmed
// "first last" if either is set, otherwise the local-part of the email.
func newsletterDisplayName(u *models.User) string {
	first := ""
	last := ""
	if u.FirstName != nil {
		first = strings.TrimSpace(*u.FirstName)
	}
	if u.LastName != nil {
		last = strings.TrimSpace(*u.LastName)
	}
	combined := strings.TrimSpace(first + " " + last)
	if combined != "" {
		return combined
	}
	if at := strings.IndexByte(u.Email, '@'); at > 0 {
		return u.Email[:at]
	}
	return u.Email
}

// SubscribeNewsletter handles POST /auth/subscribe-newsletter.
// Subscribes the authenticated user to the marketing newsletter via the public
// Listmonk list, then stamps the local newsletter_subscribed flag on success.
func (h *AuthHandler) SubscribeNewsletter(w http.ResponseWriter, r *http.Request) {
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

	name := newsletterDisplayName(user)
	if err := SubscribeToList(r.Context(), name, user.Email); err != nil {
		if h.log != nil {
			h.log.Error("newsletter subscribe failed", "user_id", userID, "error", err)
		}
		Error(w, http.StatusBadGateway, "Failed to subscribe to newsletter")
		return
	}

	if err := h.users.SetNewsletterSubscribed(r.Context(), userID); err != nil {
		if h.log != nil {
			h.log.Error("newsletter flag update failed", "user_id", userID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to record newsletter subscription")
		return
	}

	updated, err := h.users.GetByID(r.Context(), userID)
	if err != nil || updated == nil {
		updated = user
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"user": h.buildUserResponse(r.Context(), updated),
	})
}

// ChangePassword handles PUT /auth/change-password.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.CurrentPassword == "" {
		Error(w, http.StatusBadRequest, "Current password is required")
		return
	}
	if err := ValidatePasswordPolicy(h.resolvedFor(r.Context()), req.NewPassword); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		Error(w, http.StatusNotFound, "User not found")
		return
	}
	if user.PasswordHash == nil {
		Error(w, http.StatusBadRequest, "Cannot change password for OIDC-only accounts")
		return
	}
	hash := strings.TrimSpace(*user.PasswordHash)
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.CurrentPassword)); err != nil {
		Error(w, http.StatusUnauthorized, "Current password is incorrect")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}
	if err := h.users.UpdatePassword(r.Context(), userID, string(newHash)); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to change password")
		return
	}
	// Security baseline: invalidate everything that grants access without
	// re-entering the new password on another browser.
	//   * Trusted-device rows → attacker with a trust cookie cannot skip MFA.
	//   * Other sessions     → attacker with a refresh_token cannot keep a live session.
	// We keep the caller's own session alive so the UI doesn't log them out
	// mid-action after a successful password change.
	if h.trustedDevices != nil {
		if err := h.trustedDevices.RevokeAllForUser(r.Context(), userID); err != nil && h.log != nil {
			h.log.Error("change password revoke trusted devices failed", "user_id", userID, "error", err)
		}
	}
	currentSessionID, _ := r.Context().Value(middleware.SessionIDKey).(string)
	if h.sessions != nil {
		if err := h.sessions.RevokeAllForUser(r.Context(), userID, currentSessionID); err != nil && h.log != nil {
			h.log.Error("change password revoke sessions failed", "user_id", userID, "error", err)
		}
	}
	clearDeviceTrustCookie(w, r)
	JSON(w, http.StatusOK, map[string]string{"message": "Password changed successfully"})
}

// Logout handles POST /auth/logout.
// Revokes the current session server-side and clears auth cookies on the client.
// The patchmon_device_trust cookie is intentionally preserved — "remember this device"
// must survive logout (that is the whole point of the feature). Trust is killed only
// by explicit revocation, password change, TFA disable, or natural expiry.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	sessionID, _ := r.Context().Value(middleware.SessionIDKey).(string)
	if h.log != nil {
		h.log.Debug("auth request", "method", r.Method, "path", r.URL.Path, "user_id", userID, "session_id", sessionID)
	}
	if sessionID != "" && userID != "" && h.sessions != nil {
		if err := h.sessions.RevokeByID(r.Context(), sessionID, userID); err != nil && h.log != nil {
			h.log.Error("logout revoke session failed", "user_id", userID, "session_id", sessionID, "error", err)
		}
	}
	clearAuthCookies(w, r)
	JSON(w, http.StatusOK, map[string]string{"message": "Logged out"})
}

// Heartbeat handles POST /auth/heartbeat. It exists purely so the UI has
// something cheap to call that carries X-User-Activity: the auth middleware does
// the session lookup, the inactivity check and the last_activity write, so this
// body has nothing left to do. Pages differ in what they poll, and some poll
// nothing at all, so without a dedicated beat a user reading a screen would be
// logged out mid-read on those pages.
func (h *AuthHandler) Heartbeat(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// Refresh handles POST /auth/refresh. It swaps the refresh_token cookie for a
// fresh access token on the same session.
//
// Without this the access token's lifetime was the real session length: it is
// absolute from login, no amount of use extended it, and any
// SESSION_INACTIVITY_TIMEOUT_MINUTES longer than JWT_EXPIRES_IN was unreachable.
//
// The refresh token is deliberately not rotated. It is bound to the session row,
// which several tabs share, and rotating it would make whichever tab lost the
// race log the user out.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if h.log != nil {
		h.log.Debug("auth request", "method", r.Method, "path", r.URL.Path)
	}
	deny := func(msg string) {
		clearAuthCookies(w, r)
		Error(w, http.StatusUnauthorized, msg)
	}

	c, err := r.Cookie("refresh_token")
	if err != nil || c.Value == "" {
		deny("Missing refresh token")
		return
	}

	claims := jwt.MapClaims{}
	t, err := jwt.ParseWithClaims(c.Value, &claims, func(_ *jwt.Token) (interface{}, error) {
		return []byte(h.cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !t.Valid {
		deny("Invalid refresh token")
		return
	}
	// An access token presented here would otherwise be accepted as a refresh
	// token, letting a leaked one outlive its own expiry.
	if typ, _ := claims["typ"].(string); typ != tokenTypeRefresh {
		deny("Invalid refresh token")
		return
	}
	userID, _ := claims["sub"].(string)
	if userID == "" {
		deny("Invalid refresh token")
		return
	}

	// GetByRefreshToken already filters on is_revoked and expires_at, so a logged
	// out or reaped session cannot be refreshed.
	sess, err := h.sessions.GetByRefreshToken(r.Context(), c.Value)
	if err != nil || sess == nil || sess.UserID != userID {
		deny("Session expired")
		return
	}

	// Same rule the middleware applies, for the same reason: a refresh is not user
	// activity, so it must not resurrect a session that has already gone idle.
	rc := h.resolvedFor(r.Context())
	if rc != nil && rc.SessionInactivityTimeoutMin > 0 &&
		time.Since(sess.LastActivity) > time.Duration(rc.SessionInactivityTimeoutMin)*time.Minute {
		if err := h.sessions.RevokeByID(r.Context(), sess.ID, sess.UserID); err != nil && h.log != nil {
			h.log.Error("refresh: failed to revoke inactive session", "session_id", sess.ID, "error", err)
		}
		deny("Session expired due to inactivity")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || !user.IsActive {
		deny("Session expired")
		return
	}

	expiresIn := h.getJwtExpiresInSeconds(r.Context())
	// Role comes from the database, not the refresh token, so a demotion takes
	// effect on the next refresh rather than at the end of the refresh window.
	accessToken, err := h.createAccessToken(user.ID, user.Role, sess.ID, expiresIn)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to refresh session")
		return
	}

	setAccessCookie(w, r, accessToken, expiresIn, h.cfg.Env, false, h.authBrowserSessionCookies(r.Context()))
	JSON(w, http.StatusOK, map[string]interface{}{
		"expires_at": time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339),
	})
}

// parseUserAgent extracts browser, OS, and device from user agent string.
func parseUserAgent(ua string) map[string]string {
	out := map[string]string{"browser": "Unknown", "os": "Unknown", "device": "Desktop"}
	if ua == "" {
		return out
	}
	lower := strings.ToLower(ua)
	if strings.Contains(lower, "chrome") && !strings.Contains(lower, "edg") {
		out["browser"] = "Chrome"
	} else if strings.Contains(lower, "firefox") {
		out["browser"] = "Firefox"
	} else if strings.Contains(lower, "safari") && !strings.Contains(lower, "chrome") {
		out["browser"] = "Safari"
	} else if strings.Contains(lower, "edg") {
		out["browser"] = "Edge"
	} else if strings.Contains(lower, "opera") {
		out["browser"] = "Opera"
	}
	if strings.Contains(lower, "windows") {
		out["os"] = "Windows"
	} else if strings.Contains(lower, "macintosh") || strings.Contains(lower, "mac os") {
		out["os"] = "macOS"
	} else if strings.Contains(lower, "linux") {
		out["os"] = "Linux"
	} else if strings.Contains(lower, "android") {
		out["os"] = "Android"
	} else if strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad") {
		out["os"] = "iOS"
	}
	if strings.Contains(lower, "mobile") {
		out["device"] = "Mobile"
	} else if strings.Contains(lower, "tablet") || strings.Contains(lower, "ipad") {
		out["device"] = "Tablet"
	}
	return out
}

// getLocationFromIP returns basic location info (simplified; for local/private IPs returns "Local").
func getLocationFromIP(ip string) map[string]string {
	out := map[string]string{"country": "Unknown", "city": "Unknown"}
	if ip == "" {
		return out
	}
	if ip == "127.0.0.1" || ip == "::1" || strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") {
		out["country"] = "Local"
		out["city"] = "Local Network"
	}
	return out
}

// GetSessions handles GET /auth/sessions.
func (h *AuthHandler) GetSessions(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	sessionID, _ := r.Context().Value(middleware.SessionIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	sessions, err := h.sessions.ListByUserID(r.Context(), userID)
	if err != nil {
		if h.log != nil {
			h.log.Error("get sessions failed", "user_id", userID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to fetch sessions")
		return
	}
	enhanced := make([]map[string]interface{}, 0, len(sessions))
	for _, s := range sessions {
		ua := ""
		if s.UserAgent != nil {
			ua = *s.UserAgent
		}
		ip := ""
		if s.IPAddress != nil {
			ip = *s.IPAddress
		}
		enhanced = append(enhanced, map[string]interface{}{
			"id":                 s.ID,
			"ip_address":         ip,
			"user_agent":         ua,
			"device_fingerprint": s.DeviceFingerprint,
			"last_activity":      s.LastActivity.Format(time.RFC3339),
			"created_at":         s.CreatedAt.Format(time.RFC3339),
			"expires_at":         s.ExpiresAt.Format(time.RFC3339),
			"tfa_remember_me":    s.TfaRememberMe,
			"tfa_bypass_until":   tfaBypassUntilToStr(s.TfaBypassUntil),
			"login_count":        s.LoginCount,
			"last_login_ip":      s.LastLoginIP,
			"is_current_session": s.ID == sessionID,
			"device_info":        parseUserAgent(ua),
			"location_info":      getLocationFromIP(ip),
		})
	}
	JSON(w, http.StatusOK, map[string]interface{}{"sessions": enhanced})
}

func tfaBypassUntilToStr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

// RevokeSession handles DELETE /auth/sessions/{sessionId}.
func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	currentSessionID, _ := r.Context().Value(middleware.SessionIDKey).(string)
	sessionIDParam := chi.URLParam(r, "sessionId")
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if sessionIDParam == "" {
		Error(w, http.StatusBadRequest, "Session ID required")
		return
	}
	if sessionIDParam == currentSessionID {
		Error(w, http.StatusBadRequest, "Cannot revoke current session")
		return
	}
	if err := h.sessions.RevokeByID(r.Context(), sessionIDParam, userID); err != nil {
		Error(w, http.StatusNotFound, "Session not found")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"message": "Session revoked successfully"})
}

// RevokeAllSessions handles DELETE /auth/sessions (revoke all except current).
func (h *AuthHandler) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	currentSessionID, _ := r.Context().Value(middleware.SessionIDKey).(string)
	if userID == "" {
		Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.sessions.RevokeAllForUser(r.Context(), userID, currentSessionID); err != nil {
		if h.log != nil {
			h.log.Error("revoke all sessions failed", "user_id", userID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to revoke sessions")
		return
	}
	JSON(w, http.StatusOK, map[string]string{"message": "All other sessions revoked successfully"})
}

// SignupEnabled handles GET /auth/signup-enabled.
func (h *AuthHandler) SignupEnabled(w http.ResponseWriter, r *http.Request) {
	if h.log != nil {
		h.log.Debug("auth request", "method", r.Method, "path", r.URL.Path)
	}
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		JSON(w, http.StatusOK, map[string]bool{"signupEnabled": false})
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"signupEnabled": s.SignupEnabled})
}

// SetupAdmin handles POST /auth/setup-admin (first-time admin creation).
func (h *AuthHandler) SetupAdmin(w http.ResponseWriter, r *http.Request) {
	if h.log != nil {
		h.log.Debug("auth request", "method", r.Method, "path", r.URL.Path)
	}
	var req struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		Password  string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.FirstName == "" || req.LastName == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		Error(w, http.StatusBadRequest, "All fields are required")
		return
	}
	if err := ValidatePasswordPolicy(h.resolvedFor(r.Context()), req.Password); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	count, err := h.users.CountAdmins(r.Context())
	if err != nil || count > 0 {
		Error(w, http.StatusBadRequest, "Admin users already exist. This endpoint is only for first-time setup.")
		return
	}

	exists, _ := h.users.ExistsByUsernameOrEmail(r.Context(), req.Username, req.Email, "")
	if exists {
		Error(w, http.StatusBadRequest, "Username or email already exists")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create admin")
		return
	}
	hashStr := string(hash)

	u := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: &hashStr,
		Role:         "superadmin",
		IsActive:     true,
		FirstName:    &req.FirstName,
		LastName:     &req.LastName,
	}
	if err := h.users.Create(r.Context(), u); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create admin")
		return
	}
	AutoSubscribeIfHosted(h.cfg != nil && h.cfg.AdminMode, h.users, h.log, u)

	expiresIn := int64(3600)
	accessToken, refreshToken, err := h.issueSessionTokens(r, u, expiresIn, 7*24*3600)
	if err != nil {
		if h.log != nil {
			h.log.Error("admin setup session creation failed", "user_id", u.ID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to create session")
		return
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)

	setAuthCookiesWithRemember(w, r, accessToken, refreshToken, expiresIn, false, h.cfg.Env, false, h.authBrowserSessionCookies(r.Context()))

	JSON(w, http.StatusCreated, map[string]interface{}{
		"message":       "Admin user created successfully",
		"token":         accessToken,
		"refresh_token": refreshToken,
		"expires_at":    expiresAt,
		"user":          h.buildUserResponse(r.Context(), u),
	})
}

// Signup handles POST /auth/signup (public, when signup enabled).
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	if h.log != nil {
		h.log.Debug("auth request", "method", r.Method, "path", r.URL.Path)
	}
	s, err := h.settings.GetFirst(r.Context())
	if err != nil || s == nil || !s.SignupEnabled {
		Error(w, http.StatusForbidden, "User signup is currently disabled")
		return
	}

	var req struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		Password  string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.FirstName == "" || req.LastName == "" || req.Username == "" || req.Email == "" || req.Password == "" {
		Error(w, http.StatusBadRequest, "All fields are required")
		return
	}
	if len(req.Username) < 3 {
		Error(w, http.StatusBadRequest, "Username must be at least 3 characters")
		return
	}
	if err := ValidatePasswordPolicy(h.resolvedFor(r.Context()), req.Password); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	exists, _ := h.users.ExistsByUsernameOrEmail(r.Context(), req.Username, req.Email, "")
	if exists {
		Error(w, http.StatusConflict, "Username or email already exists")
		return
	}

	role := s.DefaultUserRole
	if role == "" && h.resolved != nil {
		role = h.resolvedFor(r.Context()).DefaultUserRole
	}
	if role == "" {
		role = "user"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create account")
		return
	}
	hashStr := string(hash)

	u := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: &hashStr,
		Role:         role,
		IsActive:     true,
		FirstName:    &req.FirstName,
		LastName:     &req.LastName,
	}
	if err := h.users.Create(r.Context(), u); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create account")
		return
	}
	AutoSubscribeIfHosted(h.cfg != nil && h.cfg.AdminMode, h.users, h.log, u)

	accessToken, _, err := h.issueSessionTokens(r, u, 3600, 7*24*3600)
	if err != nil {
		if h.log != nil {
			h.log.Error("signup session creation failed", "user_id", u.ID, "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Account created successfully",
		"token":   accessToken,
		"user":    h.buildUserResponse(r.Context(), u),
	})
}

func userToResponse(u *models.User, acceptedVersions []string) map[string]interface{} {
	res := map[string]interface{}{
		"id": u.ID, "username": u.Username, "email": u.Email, "role": u.Role,
		"is_active": u.IsActive, "theme_preference": strVal(u.ThemePreference, "dark"),
		"color_theme":  strVal(u.ColorTheme, "cyber_blue"),
		"updated_at":   u.UpdatedAt,
		"has_password": u.PasswordHash != nil && *u.PasswordHash != "",
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
	if u.AvatarURL != nil {
		res["avatar_url"] = *u.AvatarURL
	}
	if u.OidcSub != nil {
		res["oidc_sub"] = *u.OidcSub
	}
	if u.OidcProvider != nil {
		res["oidc_provider"] = *u.OidcProvider
	}
	if u.DiscordID != nil {
		res["discord_id"] = *u.DiscordID
	}
	if u.DiscordUsername != nil {
		res["discord_username"] = *u.DiscordUsername
	}
	res["newsletter_subscribed"] = u.NewsletterSubscribed
	if u.NewsletterSubscribedAt != nil {
		res["newsletter_subscribed_at"] = u.NewsletterSubscribedAt.Format(time.RFC3339)
	}
	if acceptedVersions != nil {
		res["accepted_release_notes_versions"] = acceptedVersions
	}
	return res
}

func strVal(s *string, def string) string {
	if s != nil && *s != "" {
		return *s
	}
	return def
}

// buildUserResponse returns the user map for API responses, including accepted_release_notes_versions.
func (h *AuthHandler) buildUserResponse(ctx context.Context, u *models.User) map[string]interface{} {
	var accepted []string
	if h.releaseNotesAcceptance != nil {
		accepted, _ = h.releaseNotesAcceptance.GetAcceptedVersions(ctx, u.ID)
	}
	return userToResponse(u, accepted)
}
