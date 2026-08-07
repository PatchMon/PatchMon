package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/oidc"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OidcHandler handles OIDC authentication routes.
type OidcHandler struct {
	cfg         *config.Config
	resolvedCfg *config.ResolvedConfig
	oidcStore   *store.OidcSessionStore
	users       *store.UsersStore
	auth        *AuthHandler
	settings    *store.SettingsStore
	enc         *util.Encryption
	log         *slog.Logger

	// clientMu protects entries. Each context gets its own entry: OIDC settings
	// live in each context's own database, so a single shared client would let
	// one context's settings save repoint every other context's SSO.
	clientMu sync.RWMutex
	entries  map[string]*oidcContextEntry
	// entryMu holds a per-context mutex so a slow provider discovery in one
	// context does not block logins in another. Mirrors PoolCache.hostLock.
	entryMu sync.Map
}

// oidcClientTTL bounds how long a context's resolved OIDC config is reused.
// Explicit eviction on settings save is the primary mechanism; this is a
// backstop for any invalidation path that is missed.
const oidcClientTTL = 10 * time.Minute

// oidcBuildTimeout bounds the settings read when building a context's entry.
const oidcBuildTimeout = 10 * time.Second

// oidcContextEntry is one context's resolved OIDC configuration and client.
type oidcContextEntry struct {
	client   *oidc.Client
	resolved *config.ResolvedOidcConfig
	// configuredValid is true when all required fields are present and enabled,
	// even if the provider connection later fails.
	configuredValid bool
	expiresAt       time.Time
}

// NewOidcHandler creates a new OIDC handler.
func NewOidcHandler(cfg *config.Config, resolved *config.ResolvedOidcConfig, resolvedCfg *config.ResolvedConfig, client *oidc.Client, configuredValid bool, oidcStore *store.OidcSessionStore, users *store.UsersStore, auth *AuthHandler, settings *store.SettingsStore, enc *util.Encryption, log *slog.Logger) *OidcHandler {
	h := &OidcHandler{
		cfg:         cfg,
		resolvedCfg: resolvedCfg,
		oidcStore:   oidcStore,
		users:       users,
		auth:        auth,
		settings:    settings,
		enc:         enc,
		log:         log,
		entries:     map[string]*oidcContextEntry{},
	}
	// Seed the default context with what was resolved at startup, so a
	// single-context install behaves exactly as before with no first-request
	// resolve. TenantHostKey returns "" for the default context.
	h.entries[""] = &oidcContextEntry{
		client:          client,
		resolved:        resolved,
		configuredValid: configuredValid,
		expiresAt:       time.Now().Add(oidcClientTTL),
	}
	return h
}

// entryLock returns the per-context mutex, so only requests for the same
// context block each other while an entry is being built.
func (h *OidcHandler) entryLock(key string) *sync.Mutex {
	v, _ := h.entryMu.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// entryFor returns the OIDC entry for ctx's context, building it on first use.
// Never returns nil; an unconfigured context yields a zero entry.
func (h *OidcHandler) entryFor(ctx context.Context) *oidcContextEntry {
	key := hostctx.TenantHostKey(ctx)

	h.clientMu.RLock()
	e, ok := h.entries[key]
	h.clientMu.RUnlock()
	if ok && e != nil && time.Now().Before(e.expiresAt) {
		return e
	}

	lock := h.entryLock(key)
	lock.Lock()
	defer lock.Unlock()

	// Re-check: another request for this context may have built it while we
	// waited on the lock.
	h.clientMu.RLock()
	e, ok = h.entries[key]
	h.clientMu.RUnlock()
	if ok && e != nil && time.Now().Before(e.expiresAt) {
		return e
	}

	// Build off a context that survives client disconnect. entryFor runs on the
	// request path, so a caller that hangs up mid-build must not be able to
	// cache an "unconfigured" result for the full TTL and take this context's
	// SSO offline. WithoutCancel keeps the injected per-context DB and entry.
	buildCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oidcBuildTimeout)
	defer cancel()

	built, err := h.buildEntry(buildCtx)
	if err != nil {
		// Settings unreadable. Do not cache a negative result: keep serving the
		// previous entry if there is one, otherwise retry on the next request
		// rather than in oidcClientTTL.
		if h.log != nil {
			h.log.Warn("OIDC settings read failed, keeping previous configuration", "error", err)
		}
		h.clientMu.RLock()
		prev := h.entries[key]
		h.clientMu.RUnlock()
		if prev != nil {
			return prev
		}
		return &oidcContextEntry{expiresAt: time.Now()}
	}

	h.clientMu.Lock()
	h.entries[key] = built
	h.clientMu.Unlock()
	return built
}

// buildEntry resolves this context's OIDC settings and constructs its client.
// oidc.NewClient performs no I/O; provider discovery is deferred to the first
// AuthCodeURL/Exchange on the returned client, so this stays cheap.
func (h *OidcHandler) buildEntry(ctx context.Context) (*oidcContextEntry, error) {
	empty := &oidcContextEntry{expiresAt: time.Now().Add(oidcClientTTL)}
	if h.settings == nil {
		return empty, nil
	}
	// Read the row here rather than handing GetFirst to ResolveOidcConfig, which
	// treats a read failure as "no DB settings" and resolves from env instead.
	// That makes an unreadable row indistinguishable from an unconfigured one.
	s, err := h.settings.GetFirst(ctx)
	if err != nil {
		return nil, err
	}
	oidcResolved, _ := config.ResolveOidcConfig(ctx, h.cfg, func(context.Context) (*models.Settings, error) {
		return s, nil
	})
	clientSecret := oidcResolved.ClientSecret
	if !oidcResolved.ConfiguredViaEnv && clientSecret != "" && h.enc != nil {
		dec, decErr := h.enc.Decrypt(clientSecret)
		if decErr != nil {
			// Fail closed. Keeping the ciphertext would leave the secret
			// non-empty, so OIDC would report as configured and every login
			// would post the ciphertext to the provider as client_secret.
			if h.log != nil {
				h.log.Warn("OIDC client secret could not be decrypted; treating OIDC as unconfigured")
			}
			return empty, nil
		}
		clientSecret = dec
	}
	valid := oidcResolved.Enabled && oidcResolved.IssuerURL != "" && oidcResolved.ClientID != "" && clientSecret != "" && oidcResolved.RedirectURI != ""
	if !valid {
		return empty, nil
	}
	resolvedPtr := &oidcResolved
	c, _ := oidc.NewClient(ctx, oidc.Config{
		IssuerURL:    oidcResolved.IssuerURL,
		ClientID:     oidcResolved.ClientID,
		ClientSecret: clientSecret,
		RedirectURI:  oidcResolved.RedirectURI,
		Scopes:       oidcResolved.Scopes,
	})
	return &oidcContextEntry{
		client:          c,
		resolved:        resolvedPtr,
		configuredValid: true,
		expiresAt:       time.Now().Add(oidcClientTTL),
	}, nil
}

// evictOidcClient drops this context's cached entry so the next request
// rebuilds it. Called after a settings write.
func (h *OidcHandler) evictOidcClient(ctx context.Context) {
	h.EvictHost(hostctx.TenantHostKey(ctx))
}

// EvictHost drops a named context's cached entry. Satisfies
// hostctx.HostEvictor so the provisioner's reload endpoint can invalidate this
// cache alongside the pool and Redis caches.
func (h *OidcHandler) EvictHost(host string) {
	if h == nil {
		return
	}
	// Take the per-key lock first, so an in-flight buildEntry for this context
	// finishes and stores before we delete. Without it, a build that started
	// before a settings save can write the pre-save entry back afterwards and
	// keep the old configuration live for the full TTL.
	lock := h.entryLock(host)
	lock.Lock()
	defer lock.Unlock()

	h.clientMu.Lock()
	delete(h.entries, host)
	h.clientMu.Unlock()
}

// Config handles GET /api/v1/auth/oidc/config.
func (h *OidcHandler) Config(w http.ResponseWriter, r *http.Request) {
	e := h.entryFor(r.Context())
	resolved := e.resolved
	configuredValid := e.configuredValid

	enabled := configuredValid
	var disableLocalAuth bool
	var buttonText string
	var syncRoles bool
	if resolved != nil {
		disableLocalAuth = enabled && resolved.DisableLocalAuth
		buttonText = resolved.ButtonText
		syncRoles = resolved.SyncRoles
	} else {
		disableLocalAuth = enabled && h.cfg.OidcDisableLocalAuth
		buttonText = h.cfg.OidcButtonText
		syncRoles = h.cfg.OidcSyncRoles
	}
	if buttonText == "" {
		buttonText = "Login with SSO"
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"enabled":          enabled,
		"buttonText":       buttonText,
		"disableLocalAuth": disableLocalAuth,
		"syncRoles":        syncRoles,
	})
}

// Login handles GET /api/v1/auth/oidc/login.
func (h *OidcHandler) Login(w http.ResponseWriter, r *http.Request) {
	client := h.entryFor(r.Context()).client

	if client == nil {
		Error(w, http.StatusBadRequest, "OIDC authentication is not configured")
		return
	}
	if h.requireHTTPS(w, r) {
		return
	}
	state := generateState()
	authURL, session, err := client.AuthCodeURL(r.Context(), state)
	if err != nil {
		if h.log != nil {
			h.log.Error("oidc auth url failed", "error", err)
		}
		Error(w, http.StatusServiceUnavailable, "Failed to reach the OIDC provider; please try again shortly")
		return
	}
	ttl := time.Duration(h.cfg.OidcSessionTTL) * time.Second
	if ttl <= 0 {
		ttl = 600 * time.Second
	}
	sessionData := &store.OidcSessionData{
		CodeVerifier: session.CodeVerifier,
		Nonce:        session.Nonce,
		State:        state,
		CreatedAt:    time.Now().UnixMilli(),
	}
	if err := h.oidcStore.Store(r.Context(), state, sessionData, ttl); err != nil {
		if h.log != nil {
			h.log.Error("oidc store session failed", "error", err)
		}
		Error(w, http.StatusInternalServerError, "Failed to initiate OIDC login")
		return
	}
	// Narrow Path to the OIDC routes so this cookie isn't transmitted on every
	// request to the app (defense-in-depth; it carries no credential, only a
	// random 32-byte state value, but least-privilege scoping is the right default).
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/api/v1/auth/oidc",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.Env == "production" && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback handles GET /api/v1/auth/oidc/callback.
func (h *OidcHandler) Callback(w http.ResponseWriter, r *http.Request) {
	client := h.entryFor(r.Context()).client

	if client == nil {
		http.Redirect(w, r, "/login?error=OIDC+not+enabled", http.StatusFound)
		return
	}
	q := r.URL.Query()
	code := q.Get("code")
	state := q.Get("state")
	errParam := q.Get("error")
	if errParam != "" {
		if h.log != nil {
			h.log.Error("oidc idp error", "error", errParam, "description", q.Get("error_description"))
		}
		http.Redirect(w, r, "/login?error=Authentication+failed", http.StatusFound)
		return
	}
	if state == "" {
		if h.log != nil {
			h.log.Error("oidc callback missing state")
		}
		http.Redirect(w, r, "/login?error=Invalid+authentication+response", http.StatusFound)
		return
	}
	if code == "" {
		if h.log != nil {
			h.log.Error("oidc callback missing code")
		}
		http.Redirect(w, r, "/login?error=Invalid+authentication+response", http.StatusFound)
		return
	}
	// Double-submit cookie check: the oidc_state cookie MUST be present and match.
	// A missing cookie used to be accepted (only mismatches were rejected), which
	// allowed a login-CSRF / forced-login attack where an attacker who captured a
	// valid state+code pair could trick a victim (whose browser has no oidc_state
	// cookie for this flow) into completing the callback and getting auth cookies
	// for the attacker's account. We now require cookie presence AND a match, in
	// addition to the server-side state store GetAndDelete below.
	cookieState, _ := r.Cookie("oidc_state")
	if cookieState == nil {
		if h.log != nil {
			h.log.Error("oidc state cookie missing", "state_present", state != "")
		}
		http.Redirect(w, r, "/login?error=Invalid+authentication+response", http.StatusFound)
		return
	}
	if cookieState.Value != state {
		if h.log != nil {
			h.log.Error("oidc state mismatch")
		}
		http.Redirect(w, r, "/login?error=Invalid+authentication+response", http.StatusFound)
		return
	}
	sessionData, err := h.oidcStore.GetAndDelete(r.Context(), state)
	if err != nil || sessionData == nil {
		if h.log != nil {
			h.log.Error("oidc session not found or expired", "error", err)
		}
		http.Redirect(w, r, "/login?error=Session+expired", http.StatusFound)
		return
	}
	// Path must match how the cookie was set (see Login handler) for the browser to clear it.
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", Path: "/api/v1/auth/oidc", MaxAge: -1, HttpOnly: true, Secure: isSecureRequest(r)})
	userInfo, err := client.Exchange(r.Context(), code, sessionData.CodeVerifier, state, sessionData.Nonce, q)
	if err != nil {
		if h.log != nil {
			h.log.Error("oidc exchange failed", "error", err)
		}
		http.Redirect(w, r, "/login?error=Authentication+failed", http.StatusFound)
		return
	}
	user, err := h.users.GetByOidcSubOrEmail(r.Context(), userInfo.Sub, userInfo.Email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		if h.log != nil {
			h.log.Error("oidc user lookup failed", "error", err)
		}
		http.Redirect(w, r, "/login?error=Authentication+failed", http.StatusFound)
		return
	}
	// The lookup matches on oidc_sub OR email; establish which before trusting it.
	matchedBySub := user != nil && user.OidcSub != nil && *user.OidcSub == userInfo.Sub

	// An email match, or auto-create, means the email claim decides who this is,
	// so it has to be verified.
	if !matchedBySub && !userInfo.EmailVerified {
		if h.log != nil {
			h.log.Warn("oidc login rejected: unverified email claim",
				"email", userInfo.Email, "sub", userInfo.Sub)
		}
		http.Redirect(w, r, "/login?error=Unable+to+sign+in+with+this+account", http.StatusFound)
		return
	}

	if user == nil && h.oidcAutoCreateUsers(r.Context()) {
		user = h.createOidcUser(r.Context(), userInfo)
	}
	if user == nil {
		if h.log != nil {
			h.log.Error("oidc user not found and auto-create disabled", "email", userInfo.Email)
		}
		http.Redirect(w, r, "/login?error=Unable+to+sign+in+with+this+account", http.StatusFound)
		return
	}
	if !user.IsActive {
		if h.log != nil {
			h.log.Error("oidc login inactive user", "email", userInfo.Email)
		}
		http.Redirect(w, r, "/login?error=Account+disabled", http.StatusFound)
		return
	}
	if !matchedBySub {
		// An email match must not override an established identity binding.
		if user.OidcSub != nil {
			if h.log != nil {
				h.log.Error("oidc login rejected: account is linked to a different subject",
					"email", userInfo.Email, "sub", userInfo.Sub)
			}
			http.Redirect(w, r, "/login?error=Unable+to+sign+in+with+this+account", http.StatusFound)
			return
		}
		existing, _ := h.users.GetByOidcSub(r.Context(), userInfo.Sub)
		if existing != nil && existing.ID != user.ID {
			if h.log != nil {
				h.log.Error("oidc sub already linked", "email", existing.Email)
			}
			http.Redirect(w, r, "/login?error=Unable+to+sign+in+with+this+account", http.StatusFound)
			return
		}
		issuerHost := extractHost(h.oidcIssuerURL(r.Context()))
		_ = h.users.UpdateOidcLink(r.Context(), user.ID, userInfo.Sub, issuerHost, strPtr(userInfo.Picture))
		user.OidcSub = &userInfo.Sub
		user.OidcProvider = &issuerHost
		user.AvatarURL = strPtr(userInfo.Picture)
	}
	effectiveRole := user.Role // preserve existing role by default
	if h.oidcSyncRoles(r.Context()) {
		mappedRole := h.mapGroupsToRole(r.Context(), userInfo.Groups)
		if h.log != nil && len(userInfo.Groups) == 0 {
			h.log.Warn("oidc no groups in token", "email", userInfo.Email, "hint", "Create a Scope Mapping in Authentik to add 'groups' claim")
		}
		// Lockout guardrail: if the user is already a superadmin in the DB and
		// no OIDC superadmin group is configured, there is no OIDC path that
		// can grant superadmin. Silently demoting them here would lock the
		// instance out of superadmin management. Preserve the existing role
		// and log a warning so operators notice the misconfiguration.
		if user.Role == "superadmin" && mappedRole != "superadmin" && h.oidcSuperadminGroup(r.Context()) == "" {
			if h.log != nil {
				h.log.Warn("oidc sync skip demote superadmin",
					"email", userInfo.Email,
					"mapped_role", mappedRole,
					"hint", "set oidc_superadmin_group to manage superadmin role via OIDC")
			}
		} else {
			effectiveRole = mappedRole
		}
	}
	now := time.Now()
	_ = h.users.UpdateOidcProfile(r.Context(), user.ID, now, strPtr(userInfo.Picture), strPtr(userInfo.GivenName), strPtr(userInfo.FamilyName), effectiveRole)
	user.LastLogin = &now
	user.AvatarURL = strPtr(userInfo.Picture)
	user.FirstName = strPtr(userInfo.GivenName)
	user.LastName = strPtr(userInfo.FamilyName)
	user.Role = effectiveRole
	if userInfo.IDToken != "" {
		_ = h.oidcStore.StoreIDToken(r.Context(), user.ID, userInfo.IDToken)
	}
	h.auth.CompleteOidcLogin(w, r, user)
}

// GetSettings handles GET /api/v1/auth/oidc/settings.
func (h *OidcHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		JSON(w, http.StatusOK, oidcSettingsResponse(nil, nil, true, nil))
		return
	}
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		JSON(w, http.StatusOK, oidcSettingsResponse(nil, nil, config.ConfiguredViaEnv(h.cfg), config.EnvPreview(h.cfg)))
		return
	}
	secretSet := false
	if s.OidcClientSecret != nil && *s.OidcClientSecret != "" && h.enc != nil {
		_, err := h.enc.Decrypt(*s.OidcClientSecret)
		secretSet = err == nil
	}
	callbackURL := buildOidcCallbackURL(h.cfg, s)
	JSON(w, http.StatusOK, oidcSettingsResponse(s, &secretSet, config.ConfiguredViaEnv(h.cfg), config.EnvPreview(h.cfg), callbackURL))
}

// ImportFromEnv handles POST /api/v1/auth/oidc/settings/import-from-env.
// Copies OIDC config from env vars to the database.
func (h *OidcHandler) ImportFromEnv(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		Error(w, http.StatusServiceUnavailable, "Settings not available")
		return
	}
	if !config.ConfiguredViaEnv(h.cfg) {
		Error(w, http.StatusBadRequest, "OIDC is not configured via .env")
		return
	}
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	// Copy env values to settings
	s.OidcEnabled = h.cfg.OidcEnabled
	s.OidcIssuerURL = strOrNil(strings.TrimSpace(h.cfg.OidcIssuerURL))
	s.OidcClientID = strOrNil(strings.TrimSpace(h.cfg.OidcClientID))
	s.OidcRedirectURI = strOrNil(strings.TrimSpace(h.cfg.OidcRedirectURI))
	scopes := strings.TrimSpace(h.cfg.OidcScopes)
	if scopes == "" {
		scopes = "openid email profile groups"
	}
	s.OidcScopes = &scopes
	s.OidcButtonText = strPtr(strings.TrimSpace(h.cfg.OidcButtonText))
	if s.OidcButtonText == nil || *s.OidcButtonText == "" {
		t := "Login with SSO"
		s.OidcButtonText = &t
	}
	s.OidcAutoCreateUsers = h.cfg.OidcAutoCreateUsers
	defRole := strings.TrimSpace(h.cfg.OidcDefaultRole)
	if defRole == "" {
		defRole = "user"
	}
	s.OidcDefaultRole = &defRole
	s.OidcDisableLocalAuth = h.cfg.OidcDisableLocalAuth
	s.OidcSyncRoles = h.cfg.OidcSyncRoles
	s.OidcAdminGroup = strOrNil(strings.TrimSpace(h.cfg.OidcAdminGroup))
	s.OidcSuperadminGroup = strOrNil(strings.TrimSpace(h.cfg.OidcSuperadminGroup))
	s.OidcHostManagerGroup = strOrNil(strings.TrimSpace(h.cfg.OidcHostManagerGroup))
	s.OidcReadonlyGroup = strOrNil(strings.TrimSpace(h.cfg.OidcReadonlyGroup))
	s.OidcUserGroup = strOrNil(strings.TrimSpace(h.cfg.OidcUserGroup))
	s.OidcEnforceHTTPS = h.cfg.OidcEnforceHTTPS
	if secret := strings.TrimSpace(h.cfg.OidcClientSecret); secret != "" && h.enc != nil {
		encrypted, err := h.enc.Encrypt(secret)
		if err == nil {
			s.OidcClientSecret = &encrypted
		}
	}
	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to import OIDC settings")
		return
	}
	h.evictOidcClient(r.Context())
	secretSet := false
	if s.OidcClientSecret != nil && *s.OidcClientSecret != "" && h.enc != nil {
		_, err := h.enc.Decrypt(*s.OidcClientSecret)
		secretSet = err == nil
	}
	callbackURL := buildOidcCallbackURL(h.cfg, s)
	JSON(w, http.StatusOK, oidcSettingsResponse(s, &secretSet, config.ConfiguredViaEnv(h.cfg), config.EnvPreview(h.cfg), callbackURL))
}

// UpdateSettings handles PUT /api/v1/auth/oidc/settings.
func (h *OidcHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		Error(w, http.StatusServiceUnavailable, "Settings not available")
		return
	}
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	applyOidcSettingsUpdate(s, req, h.enc)
	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update OIDC settings")
		return
	}
	secretSet := false
	if s.OidcClientSecret != nil && *s.OidcClientSecret != "" && h.enc != nil {
		_, err := h.enc.Decrypt(*s.OidcClientSecret)
		secretSet = err == nil
	}
	h.evictOidcClient(r.Context())
	callbackURL := buildOidcCallbackURL(h.cfg, s)
	JSON(w, http.StatusOK, oidcSettingsResponse(s, &secretSet, config.ConfiguredViaEnv(h.cfg), config.EnvPreview(h.cfg), callbackURL))
}

func oidcSettingsResponse(s *models.Settings, secretSet *bool, configuredViaEnv bool, envPreview map[string]string, callbackURL ...string) map[string]interface{} {
	res := map[string]interface{}{
		"configured_via_env":      configuredViaEnv,
		"env_preview":             envPreview,
		"oidc_enabled":            false,
		"oidc_issuer_url":         nil,
		"oidc_client_id":          nil,
		"oidc_client_secret_set":  false,
		"oidc_redirect_uri":       nil,
		"oidc_scopes":             "openid email profile groups",
		"oidc_auto_create_users":  false,
		"oidc_default_role":       "user",
		"oidc_disable_local_auth": false,
		"oidc_button_text":        "Login with SSO",
		"oidc_sync_roles":         false,
		"oidc_admin_group":        nil,
		"oidc_superadmin_group":   nil,
		"oidc_host_manager_group": nil,
		"oidc_readonly_group":     nil,
		"oidc_user_group":         nil,
		"oidc_enforce_https":      true,
		"callback_url":            "",
	}
	if s != nil {
		res["oidc_enabled"] = s.OidcEnabled
		res["oidc_issuer_url"] = s.OidcIssuerURL
		res["oidc_client_id"] = s.OidcClientID
		res["oidc_redirect_uri"] = s.OidcRedirectURI
		res["oidc_scopes"] = ptrOrDefault(s.OidcScopes, "openid email profile groups")
		res["oidc_auto_create_users"] = s.OidcAutoCreateUsers
		res["oidc_default_role"] = ptrOrDefault(s.OidcDefaultRole, "user")
		res["oidc_disable_local_auth"] = s.OidcDisableLocalAuth
		res["oidc_button_text"] = ptrOrDefault(s.OidcButtonText, "Login with SSO")
		res["oidc_sync_roles"] = s.OidcSyncRoles
		res["oidc_admin_group"] = s.OidcAdminGroup
		res["oidc_superadmin_group"] = s.OidcSuperadminGroup
		res["oidc_host_manager_group"] = s.OidcHostManagerGroup
		res["oidc_readonly_group"] = s.OidcReadonlyGroup
		res["oidc_user_group"] = s.OidcUserGroup
		res["oidc_enforce_https"] = s.OidcEnforceHTTPS
	}
	if secretSet != nil {
		res["oidc_client_secret_set"] = *secretSet
	}
	if len(callbackURL) > 0 {
		res["callback_url"] = callbackURL[0]
	}
	return res
}

func buildOidcCallbackURL(cfg *config.Config, s *models.Settings) string {
	base := strings.TrimSuffix(cfg.CORSOrigin, "/")
	if s != nil && strings.TrimSpace(s.ServerURL) != "" {
		base = strings.TrimSuffix(strings.TrimSpace(s.ServerURL), "/")
	}
	return base + "/api/v1/auth/oidc/callback"
}

func applyOidcSettingsUpdate(s *models.Settings, req map[string]interface{}, enc *util.Encryption) {
	if v, ok := getReqBool(req, "oidc_enabled", "oidcEnabled"); ok {
		s.OidcEnabled = v
	}
	if v, ok := getReqString(req, "oidc_issuer_url", "oidcIssuerUrl"); ok {
		s.OidcIssuerURL = &v
		if v == "" {
			s.OidcIssuerURL = nil
		}
	}
	if v, ok := getReqString(req, "oidc_client_id", "oidcClientId"); ok {
		s.OidcClientID = &v
		if v == "" {
			s.OidcClientID = nil
		}
	}
	if v, ok := getReqStringOrEmpty(req, "oidc_client_secret", "oidcClientSecret"); ok {
		if v == "" {
			s.OidcClientSecret = nil
		} else if util.IsEncrypted(v) {
			s.OidcClientSecret = &v
		} else if enc != nil {
			encrypted, err := enc.Encrypt(v)
			if err == nil {
				s.OidcClientSecret = &encrypted
			}
		} else {
			s.OidcClientSecret = &v
		}
	}
	if v, ok := getReqString(req, "oidc_redirect_uri", "oidcRedirectUri"); ok {
		s.OidcRedirectURI = &v
		if v == "" {
			s.OidcRedirectURI = nil
		}
	}
	if v, ok := getReqString(req, "oidc_scopes", "oidcScopes"); ok {
		t := v
		if t == "" {
			t = "openid email profile groups"
		}
		s.OidcScopes = &t
	}
	if v, ok := getReqBool(req, "oidc_auto_create_users", "oidcAutoCreateUsers"); ok {
		s.OidcAutoCreateUsers = v
	}
	if v, ok := getReqString(req, "oidc_default_role", "oidcDefaultRole"); ok {
		t := v
		if t == "" {
			t = "user"
		}
		s.OidcDefaultRole = &t
	}
	if v, ok := getReqBool(req, "oidc_disable_local_auth", "oidcDisableLocalAuth"); ok {
		s.OidcDisableLocalAuth = v
	}
	if v, ok := getReqString(req, "oidc_button_text", "oidcButtonText"); ok {
		t := v
		if t == "" {
			t = "Login with SSO"
		}
		s.OidcButtonText = &t
	}
	if v, ok := getReqBool(req, "oidc_sync_roles", "oidcSyncRoles"); ok {
		s.OidcSyncRoles = v
	}
	if v, ok := getReqString(req, "oidc_admin_group", "oidcAdminGroup"); ok {
		s.OidcAdminGroup = strOrNil(v)
	}
	if v, ok := getReqString(req, "oidc_superadmin_group", "oidcSuperadminGroup"); ok {
		s.OidcSuperadminGroup = strOrNil(v)
	}
	if v, ok := getReqString(req, "oidc_host_manager_group", "oidcHostManagerGroup"); ok {
		s.OidcHostManagerGroup = strOrNil(v)
	}
	if v, ok := getReqString(req, "oidc_readonly_group", "oidcReadonlyGroup"); ok {
		s.OidcReadonlyGroup = strOrNil(v)
	}
	if v, ok := getReqString(req, "oidc_user_group", "oidcUserGroup"); ok {
		s.OidcUserGroup = strOrNil(v)
	}
	if v, ok := getReqBool(req, "oidc_enforce_https", "oidcEnforceHttps"); ok {
		s.OidcEnforceHTTPS = v
	}
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Logout handles GET /api/v1/auth/oidc/logout.
func (h *OidcHandler) Logout(w http.ResponseWriter, r *http.Request) {
	client := h.entryFor(r.Context()).client

	if client == nil {
		http.Redirect(w, r, h.cfg.OidcPostLogoutURI, http.StatusFound)
		return
	}
	var idTokenHint string
	if userID, _ := r.Context().Value(middleware.UserIDKey).(string); userID != "" {
		idTokenHint, _ = h.oidcStore.GetAndDeleteIDToken(r.Context(), userID)
	}
	clearAuthCookies(w, r)
	// Path must match how the cookie was set in Login handler.
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", Value: "", Path: "/api/v1/auth/oidc", MaxAge: -1, HttpOnly: true, Secure: isSecureRequest(r)})
	logoutURL := client.LogoutURL(h.cfg.OidcPostLogoutURI, idTokenHint, h.oidcClientID(r.Context()))
	if logoutURL != "" {
		http.Redirect(w, r, logoutURL, http.StatusFound)
	} else {
		http.Redirect(w, r, h.cfg.OidcPostLogoutURI, http.StatusFound)
	}
}

func (h *OidcHandler) requireHTTPS(w http.ResponseWriter, r *http.Request) bool {
	resolved := h.entryFor(r.Context()).resolved
	if resolved != nil && !resolved.EnforceHTTPS {
		return false
	}
	if h.cfg.Env != "production" {
		return false
	}
	secure := r.TLS != nil
	if !secure && h.resolvedCfg != nil && h.resolvedCfg.TrustProxy {
		secure = r.Header.Get("X-Forwarded-Proto") == "https"
	}
	if !secure {
		if h.log != nil {
			h.log.Error("oidc rejected: HTTPS required")
		}
		Error(w, http.StatusForbidden, "HTTPS required for authentication")
		return true
	}
	return false
}

func generateState() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return uuid.New().String()
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func extractHost(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	return parsed.Hostname()
}

// resolvedSnapshot returns this context's resolved OIDC config, or nil when the
// context has none configured. Accessors below must go through this rather than
// touching the cache directly, so a settings save racing a login stays safe.
func (h *OidcHandler) resolvedSnapshot(ctx context.Context) *config.ResolvedOidcConfig {
	return h.entryFor(ctx).resolved
}

func (h *OidcHandler) oidcIssuerURL(ctx context.Context) string {
	if r := h.resolvedSnapshot(ctx); r != nil {
		return r.IssuerURL
	}
	return h.cfg.OidcIssuerURL
}

func (h *OidcHandler) oidcClientID(ctx context.Context) string {
	if r := h.resolvedSnapshot(ctx); r != nil {
		return r.ClientID
	}
	return h.cfg.OidcClientID
}

func (h *OidcHandler) oidcAutoCreateUsers(ctx context.Context) bool {
	if r := h.resolvedSnapshot(ctx); r != nil {
		return r.AutoCreateUsers
	}
	return h.cfg.OidcAutoCreateUsers
}

func (h *OidcHandler) oidcSyncRoles(ctx context.Context) bool {
	if r := h.resolvedSnapshot(ctx); r != nil {
		return r.SyncRoles
	}
	return h.cfg.OidcSyncRoles
}

// oidcSuperadminGroup returns the effective OIDC superadmin group mapping.
// Empty string means no group is configured (superadmin cannot be granted via OIDC).
func (h *OidcHandler) oidcSuperadminGroup(ctx context.Context) string {
	if r := h.resolvedSnapshot(ctx); r != nil {
		return strings.TrimSpace(r.SuperadminGroup)
	}
	return strings.TrimSpace(h.cfg.OidcSuperadminGroup)
}

func (h *OidcHandler) mapGroupsToRole(ctx context.Context, groups []string) string {
	if r := h.resolvedSnapshot(ctx); r != nil {
		return mapGroupsToRoleFromResolved(groups, r)
	}
	return mapGroupsToRoleFromConfig(groups, h.cfg)
}

func mapGroupsToRoleFromResolved(groups []string, r *config.ResolvedOidcConfig) string {
	if len(groups) == 0 {
		return r.DefaultRole
	}
	lower := make([]string, len(groups))
	for i, g := range groups {
		lower[i] = strings.ToLower(g)
	}
	superadminGroup := strings.ToLower(r.SuperadminGroup)
	adminGroup := strings.ToLower(r.AdminGroup)
	hostManagerGroup := strings.ToLower(r.HostManagerGroup)
	readonlyGroup := strings.ToLower(r.ReadonlyGroup)
	userGroup := strings.ToLower(r.UserGroup)
	// Superadmin: in superadmin group (optionally also admin for backward compat)
	if superadminGroup != "" {
		for _, g := range lower {
			if g == superadminGroup {
				return "superadmin"
			}
		}
	}
	if adminGroup != "" {
		for _, g := range lower {
			if g == adminGroup {
				return "admin"
			}
		}
	}
	if hostManagerGroup != "" {
		for _, g := range lower {
			if g == hostManagerGroup {
				return "host_manager"
			}
		}
	}
	if readonlyGroup != "" {
		for _, g := range lower {
			if g == readonlyGroup {
				return "readonly"
			}
		}
	}
	if userGroup != "" {
		for _, g := range lower {
			if g == userGroup {
				return "user"
			}
		}
	}
	return r.DefaultRole
}

func mapGroupsToRoleFromConfig(groups []string, cfg *config.Config) string {
	if len(groups) == 0 {
		return cfg.OidcDefaultRole
	}
	lower := make([]string, len(groups))
	for i, g := range groups {
		lower[i] = strings.ToLower(g)
	}
	superadminGroup := strings.ToLower(cfg.OidcSuperadminGroup)
	adminGroup := strings.ToLower(cfg.OidcAdminGroup)
	hostManagerGroup := strings.ToLower(cfg.OidcHostManagerGroup)
	readonlyGroup := strings.ToLower(cfg.OidcReadonlyGroup)
	userGroup := strings.ToLower(cfg.OidcUserGroup)
	// Superadmin: in superadmin group alone
	if superadminGroup != "" {
		for _, g := range lower {
			if g == superadminGroup {
				return "superadmin"
			}
		}
	}
	if adminGroup != "" {
		for _, g := range lower {
			if g == adminGroup {
				return "admin"
			}
		}
	}
	if hostManagerGroup != "" {
		for _, g := range lower {
			if g == hostManagerGroup {
				return "host_manager"
			}
		}
	}
	if readonlyGroup != "" {
		for _, g := range lower {
			if g == readonlyGroup {
				return "readonly"
			}
		}
	}
	if userGroup != "" {
		for _, g := range lower {
			if g == userGroup {
				return "user"
			}
		}
	}
	return cfg.OidcDefaultRole
}

var usernameSanitize = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func (h *OidcHandler) createOidcUser(ctx context.Context, info *oidc.UserInfo) *models.User {
	baseUsername := usernameSanitize.ReplaceAllString(strings.Split(info.Email, "@")[0], "")
	if len(baseUsername) > 32 {
		baseUsername = baseUsername[:32]
	}
	username := baseUsername
	counter := 1
	for {
		_, err := h.users.GetByUsername(ctx, username)
		if err != nil && errors.Is(err, pgx.ErrNoRows) {
			break
		}
		username = baseUsername + fmt.Sprintf("%d", counter)
		counter++
		if counter > 1000 {
			username = baseUsername + uuid.New().String()[:8]
			break
		}
	}
	role := h.mapGroupsToRole(ctx, info.Groups)
	if h.log != nil && len(info.Groups) == 0 && h.oidcSyncRoles(ctx) {
		h.log.Warn("oidc no groups in token for new user", "email", info.Email, "hint", "Create a Scope Mapping in Authentik to add 'groups' claim")
	}
	// If no admin/superadmin exists yet, promote the first auto-created user to superadmin.
	//
	// Must check the error: (0, err) on a transient failure would fail open and
	// promote an ordinary user to superadmin.
	adminCount, adminCountErr := h.users.CountAdmins(ctx)
	if adminCountErr != nil {
		if h.log != nil {
			h.log.Error("oidc: cannot determine admin count, refusing to auto-create",
				"error", adminCountErr, "email", info.Email)
		}
		return nil
	}
	if adminCount == 0 {
		role = "superadmin"
		if h.log != nil {
			h.log.Info("oidc first user promoted to superadmin", "email", info.Email)
		}
	}
	issuerHost := extractHost(h.oidcIssuerURL(ctx))
	u := &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        strings.ToLower(info.Email),
		Role:         role,
		IsActive:     true,
		FirstName:    strPtr(info.GivenName),
		LastName:     strPtr(info.FamilyName),
		OidcSub:      &info.Sub,
		OidcProvider: &issuerHost,
		AvatarURL:    strPtr(info.Picture),
	}
	if err := h.users.CreateOidcUser(ctx, u, info.Sub, issuerHost, strPtr(info.Picture)); err != nil {
		if h.log != nil {
			h.log.Error("oidc create user failed", "error", err, "email", info.Email)
		}
		return nil
	}
	AutoSubscribeIfHosted(h.cfg != nil && h.cfg.AdminMode, h.users, h.log, u)
	return u
}
