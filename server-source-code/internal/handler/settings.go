package handler

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
)

// SettingsHandler handles settings routes.
type SettingsHandler struct {
	settings  *store.SettingsStore
	users     *store.UsersStore
	enc       *util.Encryption
	registry  *agentregistry.Registry
	assetsDir string
	cfg       *config.Config
	resolved  *config.ResolvedConfig
}

// NewSettingsHandler creates a new settings handler.
// enc is optional; when set, used for AI API key encryption and AIStatus decrypt check.
// registry is optional; when set, used to push settings_update to connected agents when interval changes.
// assetsDir is deprecated (unused). Custom logos are stored in the database.
func NewSettingsHandler(settings *store.SettingsStore, users *store.UsersStore, enc *util.Encryption, registry *agentregistry.Registry, assetsDir string) *SettingsHandler {
	return &SettingsHandler{settings: settings, users: users, enc: enc, registry: registry, assetsDir: assetsDir}
}

// NewSettingsHandlerWithConfig creates a settings handler with config for environment endpoints.
func NewSettingsHandlerWithConfig(settings *store.SettingsStore, users *store.UsersStore, enc *util.Encryption, registry *agentregistry.Registry, assetsDir string, cfg *config.Config, resolved *config.ResolvedConfig) *SettingsHandler {
	return &SettingsHandler{settings: settings, users: users, enc: enc, registry: registry, assetsDir: assetsDir, cfg: cfg, resolved: resolved}
}

// Get handles GET /settings.
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	JSON(w, http.StatusOK, settingsToResponse(s, h.enc))
}

// GetServerURL handles GET /settings/server-url (public, used by install commands and Add Host wizard).
func (h *SettingsHandler) GetServerURL(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		JSON(w, http.StatusOK, map[string]string{"server_url": "http://localhost:3001"})
		return
	}
	url := s.ServerURL
	if url == "" {
		url = "http://localhost:3001"
	}
	JSON(w, http.StatusOK, map[string]string{"server_url": url})
}

// GetCurrentURL handles GET /settings/current-url (public). Returns the effective URL from X-Forwarded-* or Host.
func (h *SettingsHandler) GetCurrentURL(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.ToLower(proto)
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host == "" {
		host = "localhost:3000"
	}
	url := scheme + "://" + host
	JSON(w, http.StatusOK, map[string]string{"url": url})
}

// VersionCurrent handles GET /version/current (matches Node backend for frontend compatibility).
func (h *SettingsHandler) VersionCurrent(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := h.settings.GetFirst(r.Context())
		if err != nil {
			JSON(w, http.StatusOK, map[string]interface{}{
				"version": version, "latest_version": nil, "is_update_available": false,
				"last_update_check": nil, "buildDate": time.Now().UTC().Format(time.RFC3339),
				"environment": "production",
			})
			return
		}
		var lastCheck interface{}
		if s.LastUpdateCheck != nil {
			lastCheck = s.LastUpdateCheck.Format(time.RFC3339)
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"version":             version,
			"latest_version":      s.LatestVersion,
			"is_update_available": s.UpdateAvailable,
			"last_update_check":   lastCheck,
			"buildDate":           time.Now().UTC().Format(time.RFC3339),
			"environment":         "production",
		})
	}
}

const serverVersionDNS = "server.vcheck.patchmon.net"

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// VersionCheckUpdates handles GET /version/check-updates (matches Node backend).
// Fetches latest server version from DNS (server.vcheck.patchmon.net) and returns update info.
func (h *SettingsHandler) VersionCheckUpdates(currentVersion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		s, err := h.settings.GetFirst(ctx)
		if err != nil {
			Error(w, http.StatusBadRequest, "Settings not found")
			return
		}

		// Fetch fresh version from DNS (same as Node and agent version detection)
		latestVersion, err := util.LookupVersionFromDNS(serverVersionDNS)
		if err != nil {
			// Fall back to cached data from settings
			if s.LatestVersion != nil && *s.LatestVersion != "" {
				latestVersion = *s.LatestVersion
			} else {
				latestVersion = currentVersion
			}
		} else {
			latestVersion = strings.TrimSpace(strings.Trim(latestVersion, "\"'"))
			if !semverRe.MatchString(latestVersion) {
				latestVersion = ""
				if s.LatestVersion != nil {
					latestVersion = *s.LatestVersion
				} else {
					latestVersion = currentVersion
				}
			}
		}

		// Determine if update is available
		isUpdateAvailable := false
		if latestVersion != "" && currentVersion != "" {
			isUpdateAvailable = util.CompareVersions(latestVersion, currentVersion) > 0
		} else if s.LatestVersion != nil {
			isUpdateAvailable = s.UpdateAvailable
		}

		// Build latestRelease object (matches Node response format)
		var latestRelease map[string]interface{}
		if latestVersion != "" {
			latestRelease = map[string]interface{}{
				"version":     latestVersion,
				"tagName":     "v" + latestVersion,
				"publishedAt": nil,
				"htmlUrl":     "https://github.com/PatchMon/PatchMon/releases/tag/v" + latestVersion,
			}

			// Persist to settings so /version/current shows fresh data (like Node versionUpdateCheck job)
			now := time.Now()
			s.LatestVersion = &latestVersion
			s.UpdateAvailable = isUpdateAvailable
			s.LastUpdateCheck = &now
			_ = h.settings.Update(ctx, s)
		}

		var lastCheck interface{}
		if s.LastUpdateCheck != nil {
			lastCheck = s.LastUpdateCheck.Format(time.RFC3339)
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"currentVersion":    currentVersion,
			"latestVersion":     latestVersion,
			"isUpdateAvailable": isUpdateAvailable,
			"lastUpdateCheck":   lastCheck,
			"latestRelease":     latestRelease,
		})
	}
}

// AIStatus handles GET /ai/status (matches Node backend for frontend compatibility).
func (h *SettingsHandler) AIStatus(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"ai_enabled":     false,
			"ai_api_key_set": false,
		})
		return
	}
	apiKeySet := false
	if s.AiAPIKey != nil && *s.AiAPIKey != "" {
		if h.enc != nil {
			_, err := h.enc.Decrypt(*s.AiAPIKey)
			apiKeySet = err == nil
		} else {
			apiKeySet = true
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"ai_enabled":     s.AiEnabled,
		"ai_api_key_set": apiKeySet,
	})
}

// isDiscordProperlyConfigured returns true only when Discord OAuth is enabled and has valid Client ID + decryptable secret.
func (h *SettingsHandler) isDiscordProperlyConfigured(s *models.Settings) bool {
	if s == nil || !s.DiscordOAuthEnabled {
		return false
	}
	if s.DiscordClientID == nil || *s.DiscordClientID == "" {
		return false
	}
	if s.DiscordClientSecret == nil || *s.DiscordClientSecret == "" {
		return false
	}
	if h.enc == nil {
		return false
	}
	_, err := h.enc.Decrypt(*s.DiscordClientSecret)
	return err == nil
}

// GetLoginSettings handles GET /settings/login-settings (public, used by login screen and first-time admin setup).
// Includes has_admin_users and oidc fields for first-time setup flow (replaces /auth/check-admin-users).
func (h *SettingsHandler) GetLoginSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		// Settings table may be empty (e.g. fresh Docker deploy without default row).
		// Still check users table for admin existence - do not assume hasAdminUsers=false.
		hasAdminUsers := false
		if h.users != nil {
			if count, countErr := h.users.CountAdmins(r.Context()); countErr == nil {
				hasAdminUsers = count > 0
			}
		}
		currentVersion := ""
		if h.cfg != nil {
			currentVersion = h.cfg.Version
		}
		// In admin/multi-context mode, always hide GitHub version info on login.
		showGithubVersionFallback := true
		if h.cfg != nil && (h.cfg.AdminMode || h.cfg.RegistryDatabaseURL != "") {
			showGithubVersionFallback = false
		}
		showNewsletter := h.cfg == nil || !h.cfg.AdminMode
		adminMode := h.cfg != nil && h.cfg.AdminMode
		JSON(w, http.StatusOK, map[string]interface{}{
			"signup_enabled":               false,
			"show_github_version_on_login": showGithubVersionFallback,
			"current_version":              currentVersion,
			"show_newsletter":              showNewsletter,
			"admin_mode":                   adminMode,
			"discord": map[string]interface{}{
				"enabled":    false,
				"buttonText": "Login with Discord",
			},
			"hasAdminUsers": hasAdminUsers,
			"oidc": map[string]interface{}{
				"enabled":          false,
				"autoCreateUsers":  false,
				"canBypassWelcome": false,
			},
		})
		return
	}
	discordButtonText := "Login with Discord"
	if s.DiscordButtonText != nil && *s.DiscordButtonText != "" {
		discordButtonText = *s.DiscordButtonText
	}

	hasAdminUsers := false
	canBypassWelcome := false
	if h.users != nil {
		count, err := h.users.CountAdmins(r.Context())
		if err == nil {
			hasAdminUsers = count > 0
			canBypassWelcome = s.OidcEnabled && s.OidcAutoCreateUsers && count == 0
		}
	}

	currentVersion := ""
	if h.cfg != nil {
		currentVersion = h.cfg.Version
	}

	// In admin/multi-context mode, always hide GitHub version info on the login page —
	// managed contexts should not see the upstream repo's release panel.
	showGithubVersion := s.ShowGithubVersionOnLogin
	if h.cfg != nil && (h.cfg.AdminMode || h.cfg.RegistryDatabaseURL != "") {
		showGithubVersion = false
	}

	showNewsletter := h.cfg == nil || !h.cfg.AdminMode
	adminMode := h.cfg != nil && h.cfg.AdminMode
	signupEnabled := s.SignupEnabled
	if h.cfg != nil && h.cfg.AdminMode {
		signupEnabled = false
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"signup_enabled":               signupEnabled,
		"show_github_version_on_login": showGithubVersion,
		"current_version":              currentVersion,
		"show_newsletter":              showNewsletter,
		"admin_mode":                   adminMode,
		"discord": map[string]interface{}{
			"enabled":    h.isDiscordProperlyConfigured(s),
			"buttonText": discordButtonText,
		},
		"logo_dark":     s.LogoDark,
		"logo_light":    s.LogoLight,
		"favicon":       s.Favicon,
		"updated_at":    s.UpdatedAt,
		"hasAdminUsers": hasAdminUsers,
		"oidc": map[string]interface{}{
			"enabled":          s.OidcEnabled,
			"autoCreateUsers":  s.OidcAutoCreateUsers,
			"canBypassWelcome": canBypassWelcome,
		},
	})
}

// GetEnvConfig handles GET /settings/env-config (environment config for settings page display).
// Requires can_manage_settings permission. Matches Node backend for frontend compatibility.
func (h *SettingsHandler) GetEnvConfig(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	env := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	envEffective := env("APP_ENV", env("NODE_ENV", "production"))
	envConfig := map[string]interface{}{
		"server": map[string]string{
			"CORS_ORIGIN":         env("CORS_ORIGIN", "Not set"),
			"CORS_ORIGINS":        env("CORS_ORIGINS", "Not set"),
			"PORT":                env("PORT", "3001"),
			"APP_ENV":             envEffective,
			"ENV":                 envEffective,
			"DB_SERVER_PROTOCOL":  orEmpty(s.ServerProtocol, "Not set"),
			"DB_SERVER_HOST":      orEmpty(s.ServerHost, "Not set"),
			"DB_SERVER_PORT":      strconv.Itoa(s.ServerPort),
			"DB_SERVER_URL":       orEmpty(s.ServerURL, "Not set"),
			"ENV_SERVER_PROTOCOL": env("SERVER_PROTOCOL", "Not set (DB value takes precedence)"),
			"ENV_SERVER_HOST":     env("SERVER_HOST", "Not set (DB value takes precedence)"),
			"ENV_SERVER_PORT":     env("SERVER_PORT", "Not set (DB value takes precedence)"),
		},
		"frontend": map[string]string{
			"VITE_API_URL": env("VITE_API_URL", "Check frontend/.env file"),
		},
	}
	JSON(w, http.StatusOK, envConfig)
}

// envVarMeta describes a configurable environment variable.
type envVarMeta struct {
	Category        string `json:"category"`
	Key             string `json:"key"`
	EffectiveValue  string `json:"effectiveValue"`
	EffectiveSource string `json:"effectiveSource"`
	EnvValue        string `json:"envValue,omitempty"`
	DBValue         string `json:"dbValue,omitempty"`
	DefaultValue    string `json:"defaultValue"`
	Editable        bool   `json:"editable"`
	Conflict        bool   `json:"conflict"`
	Description     string `json:"description"`
}

// GetEnvironmentConfig handles GET /settings/environment.
func (h *SettingsHandler) GetEnvironmentConfig(w http.ResponseWriter, r *http.Request) {
	if h.cfg == nil || h.resolved == nil {
		JSON(w, http.StatusOK, map[string]interface{}{"variables": []envVarMeta{}})
		return
	}
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	// Re-resolve config from fresh settings so EffectiveValue reflects current DB state
	// (h.resolved is from server startup and would show stale values after DB updates)
	resolved := config.ResolveConfig(r.Context(), h.cfg, s)
	dbTz := ""
	if s.Timezone != nil {
		dbTz = *s.Timezone
	}
	slog.Debug("env config get timezone", "db_raw", dbTz, "resolved", resolved.Timezone, "env_TZ", os.Getenv("TZ"), "env_TIMEZONE", os.Getenv("TIMEZONE"))
	variables := buildEnvironmentVariables(h.cfg, resolved, s)
	if h.cfg.AdminMode {
		filtered := make([]envVarMeta, 0, 1)
		for _, v := range variables {
			if v.Key == "TIMEZONE" {
				filtered = append(filtered, v)
			}
		}
		variables = filtered
	}
	JSON(w, http.StatusOK, map[string]interface{}{"variables": variables})
}

func buildEnvironmentVariables(cfg *config.Config, resolved *config.ResolvedConfig, s *models.Settings) []envVarMeta {
	env := func(k string) string { return os.Getenv(k) }
	envDefault := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	dbInt := func(p *int) string {
		if p == nil {
			return ""
		}
		return strconv.Itoa(*p)
	}
	dbBool := func(p *bool) string {
		if p == nil {
			return ""
		}
		if *p {
			return "true"
		}
		return "false"
	}
	dbStr := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	corsEffective := resolved.CORSOrigin
	corsSource := source(env("CORS_ORIGIN"), dbStr(s.CorsOrigin), "http://localhost:3000")
	enableLoggingStr := boolStr(resolved.EnableLogging)
	logLevelEffective := resolved.LogLevel
	trustProxyStr := boolStr(resolved.TrustProxy)
	out := []envVarMeta{
		// Database - env-only, startup/deployment; configure via .env
		{Category: "Database", Key: "DATABASE_URL", EffectiveValue: maskDatabaseURL(cfg.DatabaseURL), EffectiveSource: source(env("DATABASE_URL"), "", ""), EnvValue: maskEnvIfSet("DATABASE_URL"), DBValue: "", DefaultValue: "(required)", Editable: false, Conflict: false, Description: "Connection string; startup/deployment only - configure via .env"},
		{Category: "Database", Key: "DB_CONNECTION_LIMIT", EffectiveValue: strconv.Itoa(cfg.DBConnectionLimit), EffectiveSource: source(env("DB_CONNECTION_LIMIT"), "", "30"), EnvValue: env("DB_CONNECTION_LIMIT"), DBValue: "", DefaultValue: "30", Editable: false, Conflict: false, Description: "Max connections in pool; startup only - configure via .env"},
		{Category: "Database", Key: "DB_CONNECT_TIMEOUT", EffectiveValue: strconv.Itoa(cfg.DBConnectTimeout), EffectiveSource: source(env("DB_CONNECT_TIMEOUT"), "", "10"), EnvValue: env("DB_CONNECT_TIMEOUT"), DBValue: "", DefaultValue: "10", Editable: false, Conflict: false, Description: "Connect timeout (seconds); startup only - configure via .env"},
		{Category: "Database", Key: "PM_DB_CONN_MAX_ATTEMPTS", EffectiveValue: strconv.Itoa(cfg.DBConnMaxAttempts), EffectiveSource: source(env("PM_DB_CONN_MAX_ATTEMPTS"), "", "30"), EnvValue: env("PM_DB_CONN_MAX_ATTEMPTS"), DBValue: "", DefaultValue: "30", Editable: false, Conflict: false, Description: "Retry attempts at startup; configure via .env"},
		{Category: "Database", Key: "PM_DB_CONN_WAIT_INTERVAL", EffectiveValue: strconv.Itoa(cfg.DBConnWaitInterval), EffectiveSource: source(env("PM_DB_CONN_WAIT_INTERVAL"), "", "2"), EnvValue: env("PM_DB_CONN_WAIT_INTERVAL"), DBValue: "", DefaultValue: "2", Editable: false, Conflict: false, Description: "Seconds between connection retries; configure via .env"},
		{Category: "Server", Key: "CORS_ORIGIN", EffectiveValue: corsEffective, EffectiveSource: corsSource, EnvValue: env("CORS_ORIGIN"), DBValue: dbStr(s.CorsOrigin), DefaultValue: "http://localhost:3000", Editable: true, Conflict: env("CORS_ORIGIN") != "" && s.CorsOrigin != nil && *s.CorsOrigin != "", Description: "Allowed origin for CORS (frontend URL). Comma-separated for multiple origins. Requires a server restart to take effect."},
		{Category: "Server", Key: "ENABLE_HSTS", EffectiveValue: boolStr(resolved.EnableHSTS), EffectiveSource: source(env("ENABLE_HSTS"), dbBool(s.EnableHSTS), "false"), EnvValue: env("ENABLE_HSTS"), DBValue: dbBool(s.EnableHSTS), DefaultValue: "false", Editable: true, Conflict: env("ENABLE_HSTS") != "" && s.EnableHSTS != nil, Description: "Enable HSTS header for HTTPS"},
		{Category: "Server", Key: "TRUST_PROXY", EffectiveValue: trustProxyStr, EffectiveSource: source(env("TRUST_PROXY"), dbBool(s.TrustProxy), "false"), EnvValue: env("TRUST_PROXY"), DBValue: dbBool(s.TrustProxy), DefaultValue: "false", Editable: true, Conflict: env("TRUST_PROXY") != "" && s.TrustProxy != nil, Description: "Trust proxy headers when behind nginx/Caddy"},
		{Category: "Server", Key: "PORT", EffectiveValue: strconv.Itoa(cfg.Port), EffectiveSource: source(env("PORT"), "", "3001"), EnvValue: env("PORT"), DBValue: "", DefaultValue: "3001", Editable: false, Conflict: false, Description: "Backend API port; configure via .env"},
		{Category: "Server", Key: "APP_ENV", EffectiveValue: cfg.Env, EffectiveSource: source(env("APP_ENV"), "", envDefault("NODE_ENV", "production")), EnvValue: env("APP_ENV"), DBValue: "", DefaultValue: "production", Editable: false, Conflict: false, Description: "Environment mode (production/development)"},
		{Category: "Server", Key: "TIMEZONE", EffectiveValue: resolved.Timezone, EffectiveSource: source(envTzOrTimezone(), dbStr(s.Timezone), "UTC"), EnvValue: envTzOrTimezone(), DBValue: dbStr(s.Timezone), DefaultValue: "UTC", Editable: true, Conflict: envTzOrTimezone() != "" && s.Timezone != nil && *s.Timezone != "", Description: "IANA timezone (e.g. America/New_York, Europe/London)"},
		{Category: "Logging", Key: "ENABLE_LOGGING", EffectiveValue: enableLoggingStr, EffectiveSource: source(env("ENABLE_LOGGING"), dbBool(s.EnableLogging), "false"), EnvValue: env("ENABLE_LOGGING"), DBValue: dbBool(s.EnableLogging), DefaultValue: "false", Editable: true, Conflict: env("ENABLE_LOGGING") != "" && s.EnableLogging != nil, Description: "Enable backend logging to stdout"},
		{Category: "Logging", Key: "LOG_LEVEL", EffectiveValue: logLevelEffective, EffectiveSource: source(env("LOG_LEVEL"), dbStr(s.LogLevel), "info"), EnvValue: env("LOG_LEVEL"), DBValue: dbStr(s.LogLevel), DefaultValue: "info", Editable: true, Conflict: env("LOG_LEVEL") != "" && s.LogLevel != nil && *s.LogLevel != "", Description: "Log level: debug, info, warn, error"},
		{Category: "Authentication", Key: "MAX_LOGIN_ATTEMPTS", EffectiveValue: strconv.Itoa(resolved.MaxLoginAttempts), EffectiveSource: source(env("MAX_LOGIN_ATTEMPTS"), dbInt(s.MaxLoginAttempts), strconv.Itoa(cfg.MaxLoginAttempts)), EnvValue: env("MAX_LOGIN_ATTEMPTS"), DBValue: dbInt(s.MaxLoginAttempts), DefaultValue: "5", Editable: true, Conflict: env("MAX_LOGIN_ATTEMPTS") != "" && s.MaxLoginAttempts != nil, Description: "Max failed login attempts before lockout"},
		{Category: "Authentication", Key: "LOCKOUT_DURATION_MINUTES", EffectiveValue: strconv.Itoa(resolved.LockoutDurationMin), EffectiveSource: source(env("LOCKOUT_DURATION_MINUTES"), dbInt(s.LockoutDurationMinutes), "15"), EnvValue: env("LOCKOUT_DURATION_MINUTES"), DBValue: dbInt(s.LockoutDurationMinutes), DefaultValue: "15", Editable: true, Conflict: env("LOCKOUT_DURATION_MINUTES") != "" && s.LockoutDurationMinutes != nil, Description: "Lockout duration in minutes"},
		{Category: "Authentication", Key: "SESSION_INACTIVITY_TIMEOUT_MINUTES", EffectiveValue: strconv.Itoa(resolved.SessionInactivityTimeoutMin), EffectiveSource: source(env("SESSION_INACTIVITY_TIMEOUT_MINUTES"), dbInt(s.SessionInactivityTimeoutMinutes), "30"), EnvValue: env("SESSION_INACTIVITY_TIMEOUT_MINUTES"), DBValue: dbInt(s.SessionInactivityTimeoutMinutes), DefaultValue: "30", Editable: true, Conflict: env("SESSION_INACTIVITY_TIMEOUT_MINUTES") != "" && s.SessionInactivityTimeoutMinutes != nil, Description: "Session inactivity timeout in minutes"},
		{Category: "Authentication", Key: "TFA_MAX_REMEMBER_SESSIONS", EffectiveValue: strconv.Itoa(resolved.TfaMaxRememberSessions), EffectiveSource: source(env("TFA_MAX_REMEMBER_SESSIONS"), dbInt(s.TfaMaxRememberSessions), "5"), EnvValue: env("TFA_MAX_REMEMBER_SESSIONS"), DBValue: dbInt(s.TfaMaxRememberSessions), DefaultValue: "5", Editable: true, Conflict: env("TFA_MAX_REMEMBER_SESSIONS") != "" && s.TfaMaxRememberSessions != nil, Description: "Max TFA remember-me sessions per user"},
		{Category: "Password policy", Key: "PASSWORD_MIN_LENGTH", EffectiveValue: strconv.Itoa(resolved.PasswordMinLength), EffectiveSource: source(env("PASSWORD_MIN_LENGTH"), dbInt(s.PasswordMinLength), "8"), EnvValue: env("PASSWORD_MIN_LENGTH"), DBValue: dbInt(s.PasswordMinLength), DefaultValue: "8", Editable: true, Conflict: env("PASSWORD_MIN_LENGTH") != "" && s.PasswordMinLength != nil, Description: "Minimum password length"},
		{Category: "Password policy", Key: "PASSWORD_REQUIRE_UPPERCASE", EffectiveValue: boolStr(resolved.PasswordRequireUppercase), EffectiveSource: source(env("PASSWORD_REQUIRE_UPPERCASE"), dbBool(s.PasswordRequireUppercase), "true"), EnvValue: env("PASSWORD_REQUIRE_UPPERCASE"), DBValue: dbBool(s.PasswordRequireUppercase), DefaultValue: "true", Editable: true, Conflict: env("PASSWORD_REQUIRE_UPPERCASE") != "" && s.PasswordRequireUppercase != nil, Description: "Require uppercase in password"},
		{Category: "Password policy", Key: "PASSWORD_REQUIRE_LOWERCASE", EffectiveValue: boolStr(resolved.PasswordRequireLowercase), EffectiveSource: source(env("PASSWORD_REQUIRE_LOWERCASE"), dbBool(s.PasswordRequireLowercase), "true"), EnvValue: env("PASSWORD_REQUIRE_LOWERCASE"), DBValue: dbBool(s.PasswordRequireLowercase), DefaultValue: "true", Editable: true, Conflict: env("PASSWORD_REQUIRE_LOWERCASE") != "" && s.PasswordRequireLowercase != nil, Description: "Require lowercase in password"},
		{Category: "Password policy", Key: "PASSWORD_REQUIRE_NUMBER", EffectiveValue: boolStr(resolved.PasswordRequireNumber), EffectiveSource: source(env("PASSWORD_REQUIRE_NUMBER"), dbBool(s.PasswordRequireNumber), "true"), EnvValue: env("PASSWORD_REQUIRE_NUMBER"), DBValue: dbBool(s.PasswordRequireNumber), DefaultValue: "true", Editable: true, Conflict: env("PASSWORD_REQUIRE_NUMBER") != "" && s.PasswordRequireNumber != nil, Description: "Require number in password"},
		{Category: "Password policy", Key: "PASSWORD_REQUIRE_SPECIAL", EffectiveValue: boolStr(resolved.PasswordRequireSpecial), EffectiveSource: source(env("PASSWORD_REQUIRE_SPECIAL"), dbBool(s.PasswordRequireSpecial), "true"), EnvValue: env("PASSWORD_REQUIRE_SPECIAL"), DBValue: dbBool(s.PasswordRequireSpecial), DefaultValue: "true", Editable: true, Conflict: env("PASSWORD_REQUIRE_SPECIAL") != "" && s.PasswordRequireSpecial != nil, Description: "Require special char in password"},
		{Category: "Server performance", Key: "JSON_BODY_LIMIT", EffectiveValue: formatBytesEnv(resolved.JSONBodyLimitBytes), EffectiveSource: source(env("JSON_BODY_LIMIT"), dbStr(s.JSONBodyLimit), "5mb"), EnvValue: env("JSON_BODY_LIMIT"), DBValue: dbStr(s.JSONBodyLimit), DefaultValue: "5mb", Editable: true, Conflict: env("JSON_BODY_LIMIT") != "" && s.JSONBodyLimit != nil, Description: "Max JSON body size (e.g. 5mb)"},
		{Category: "Server performance", Key: "AGENT_UPDATE_BODY_LIMIT", EffectiveValue: formatBytesEnv(resolved.AgentUpdateBodyLimitBytes), EffectiveSource: source(env("AGENT_UPDATE_BODY_LIMIT"), dbStr(s.AgentUpdateBodyLimit), "2mb"), EnvValue: env("AGENT_UPDATE_BODY_LIMIT"), DBValue: dbStr(s.AgentUpdateBodyLimit), DefaultValue: "2mb", Editable: true, Conflict: env("AGENT_UPDATE_BODY_LIMIT") != "" && s.AgentUpdateBodyLimit != nil, Description: "Max agent report body size"},
		{Category: "Server performance", Key: "DB_TRANSACTION_LONG_TIMEOUT", EffectiveValue: strconv.Itoa(resolved.DBTransactionLongTimeout), EffectiveSource: source(env("DB_TRANSACTION_LONG_TIMEOUT"), dbInt(s.DBTransactionLongTimeout), "60000"), EnvValue: env("DB_TRANSACTION_LONG_TIMEOUT"), DBValue: dbInt(s.DBTransactionLongTimeout), DefaultValue: "60000", Editable: true, Conflict: env("DB_TRANSACTION_LONG_TIMEOUT") != "" && s.DBTransactionLongTimeout != nil, Description: "Long transaction timeout (ms)"},
		{Category: "Authentication", Key: "JWT_EXPIRES_IN", EffectiveValue: resolved.JwtExpiresIn, EffectiveSource: source(env("JWT_EXPIRES_IN"), dbStr(s.JwtExpiresIn), "1h"), EnvValue: env("JWT_EXPIRES_IN"), DBValue: dbStr(s.JwtExpiresIn), DefaultValue: "1h", Editable: true, Conflict: env("JWT_EXPIRES_IN") != "" && s.JwtExpiresIn != nil && *s.JwtExpiresIn != "", Description: "JWT access token expiration (e.g. 1h, 30m)"},
		{Category: "Authentication", Key: "AUTH_BROWSER_SESSION_COOKIES", EffectiveValue: boolStr(resolved.AuthBrowserSessionCookies), EffectiveSource: source(env("AUTH_BROWSER_SESSION_COOKIES"), dbBool(s.AuthBrowserSessionCookies), "false"), EnvValue: env("AUTH_BROWSER_SESSION_COOKIES"), DBValue: dbBool(s.AuthBrowserSessionCookies), DefaultValue: "false", Editable: true, Conflict: env("AUTH_BROWSER_SESSION_COOKIES") != "" && s.AuthBrowserSessionCookies != nil, Description: "Use browser session cookies for auth (cleared when the browser session ends; not persisted across browser restarts)"},
		{Category: "Authentication", Key: "MAX_TFA_ATTEMPTS", EffectiveValue: strconv.Itoa(resolved.MaxTfaAttempts), EffectiveSource: source(env("MAX_TFA_ATTEMPTS"), dbInt(s.MaxTfaAttempts), "5"), EnvValue: env("MAX_TFA_ATTEMPTS"), DBValue: dbInt(s.MaxTfaAttempts), DefaultValue: "5", Editable: true, Conflict: env("MAX_TFA_ATTEMPTS") != "" && s.MaxTfaAttempts != nil, Description: "Failed TFA code attempts before lockout"},
		{Category: "Authentication", Key: "TFA_LOCKOUT_DURATION_MINUTES", EffectiveValue: strconv.Itoa(resolved.TfaLockoutDurationMin), EffectiveSource: source(env("TFA_LOCKOUT_DURATION_MINUTES"), dbInt(s.TfaLockoutDurationMinutes), "30"), EnvValue: env("TFA_LOCKOUT_DURATION_MINUTES"), DBValue: dbInt(s.TfaLockoutDurationMinutes), DefaultValue: "30", Editable: true, Conflict: env("TFA_LOCKOUT_DURATION_MINUTES") != "" && s.TfaLockoutDurationMinutes != nil, Description: "Minutes locked out after exceeding TFA attempts"},
		{Category: "Authentication", Key: "TFA_REMEMBER_ME_EXPIRES_IN", EffectiveValue: resolved.TfaRememberMeExpiresIn, EffectiveSource: source(env("TFA_REMEMBER_ME_EXPIRES_IN"), dbStr(s.TfaRememberMeExpiresIn), "30d"), EnvValue: env("TFA_REMEMBER_ME_EXPIRES_IN"), DBValue: dbStr(s.TfaRememberMeExpiresIn), DefaultValue: "30d", Editable: true, Conflict: env("TFA_REMEMBER_ME_EXPIRES_IN") != "" && s.TfaRememberMeExpiresIn != nil && *s.TfaRememberMeExpiresIn != "", Description: "Remember-me token expiration (e.g. 30d, 7d)"},
		{Category: "Authentication", Key: "JWT_SECRET", EffectiveValue: maskDatabaseURL(cfg.JWTSecret), EffectiveSource: source(env("JWT_SECRET"), "", ""), EnvValue: maskEnvIfSet("JWT_SECRET"), DBValue: "", DefaultValue: "(required)", Editable: false, Conflict: false, Description: "Secret for signing JWT tokens; configure via .env only"},
		{Category: "Authentication", Key: "DEFAULT_USER_ROLE", EffectiveValue: resolved.DefaultUserRole, EffectiveSource: source(env("DEFAULT_USER_ROLE"), s.DefaultUserRole, cfg.DefaultUserRole), EnvValue: env("DEFAULT_USER_ROLE"), DBValue: s.DefaultUserRole, DefaultValue: "user", Editable: true, Conflict: env("DEFAULT_USER_ROLE") != "" && s.DefaultUserRole != "", Description: "Default role for new users"},
		{Category: "Rate limits", Key: "RATE_LIMIT_WINDOW_MS", EffectiveValue: strconv.Itoa(resolved.RateLimitWindowMs), EffectiveSource: source(env("RATE_LIMIT_WINDOW_MS"), dbInt(s.RateLimitWindowMs), "900000"), EnvValue: env("RATE_LIMIT_WINDOW_MS"), DBValue: dbInt(s.RateLimitWindowMs), DefaultValue: "900000", Editable: true, Conflict: env("RATE_LIMIT_WINDOW_MS") != "" && s.RateLimitWindowMs != nil, Description: "General rate limit window (ms)"},
		{Category: "Rate limits", Key: "RATE_LIMIT_MAX", EffectiveValue: strconv.Itoa(resolved.RateLimitMax), EffectiveSource: source(env("RATE_LIMIT_MAX"), dbInt(s.RateLimitMax), "5000"), EnvValue: env("RATE_LIMIT_MAX"), DBValue: dbInt(s.RateLimitMax), DefaultValue: "5000", Editable: true, Conflict: env("RATE_LIMIT_MAX") != "" && s.RateLimitMax != nil, Description: "Max requests per window"},
		{Category: "Rate limits", Key: "AUTH_RATE_LIMIT_WINDOW_MS", EffectiveValue: strconv.Itoa(resolved.AuthRateLimitWindowMs), EffectiveSource: source(env("AUTH_RATE_LIMIT_WINDOW_MS"), dbInt(s.AuthRateLimitWindowMs), "600000"), EnvValue: env("AUTH_RATE_LIMIT_WINDOW_MS"), DBValue: dbInt(s.AuthRateLimitWindowMs), DefaultValue: "600000", Editable: true, Conflict: env("AUTH_RATE_LIMIT_WINDOW_MS") != "" && s.AuthRateLimitWindowMs != nil, Description: "Auth rate limit window (ms)"},
		{Category: "Rate limits", Key: "AUTH_RATE_LIMIT_MAX", EffectiveValue: strconv.Itoa(resolved.AuthRateLimitMax), EffectiveSource: source(env("AUTH_RATE_LIMIT_MAX"), dbInt(s.AuthRateLimitMax), "500"), EnvValue: env("AUTH_RATE_LIMIT_MAX"), DBValue: dbInt(s.AuthRateLimitMax), DefaultValue: "500", Editable: true, Conflict: env("AUTH_RATE_LIMIT_MAX") != "" && s.AuthRateLimitMax != nil, Description: "Max auth requests per window"},
		{Category: "Rate limits", Key: "AGENT_RATE_LIMIT_WINDOW_MS", EffectiveValue: strconv.Itoa(resolved.AgentRateLimitWindowMs), EffectiveSource: source(env("AGENT_RATE_LIMIT_WINDOW_MS"), dbInt(s.AgentRateLimitWindowMs), "60000"), EnvValue: env("AGENT_RATE_LIMIT_WINDOW_MS"), DBValue: dbInt(s.AgentRateLimitWindowMs), DefaultValue: "60000", Editable: true, Conflict: env("AGENT_RATE_LIMIT_WINDOW_MS") != "" && s.AgentRateLimitWindowMs != nil, Description: "Agent API rate limit window (ms)"},
		{Category: "Rate limits", Key: "AGENT_RATE_LIMIT_MAX", EffectiveValue: strconv.Itoa(resolved.AgentRateLimitMax), EffectiveSource: source(env("AGENT_RATE_LIMIT_MAX"), dbInt(s.AgentRateLimitMax), "1000"), EnvValue: env("AGENT_RATE_LIMIT_MAX"), DBValue: dbInt(s.AgentRateLimitMax), DefaultValue: "1000", Editable: true, Conflict: env("AGENT_RATE_LIMIT_MAX") != "" && s.AgentRateLimitMax != nil, Description: "Max agent requests per window"},
		{Category: "Rate limits", Key: "PASSWORD_RATE_LIMIT_WINDOW_MS", EffectiveValue: strconv.Itoa(resolved.PasswordRateLimitWindowMs), EffectiveSource: source(env("PASSWORD_RATE_LIMIT_WINDOW_MS"), dbInt(s.PasswordRateLimitWindowMs), "900000"), EnvValue: env("PASSWORD_RATE_LIMIT_WINDOW_MS"), DBValue: dbInt(s.PasswordRateLimitWindowMs), DefaultValue: "900000", Editable: true, Conflict: env("PASSWORD_RATE_LIMIT_WINDOW_MS") != "" && s.PasswordRateLimitWindowMs != nil, Description: "Password change rate limit window (ms)"},
		{Category: "Rate limits", Key: "PASSWORD_RATE_LIMIT_MAX", EffectiveValue: strconv.Itoa(resolved.PasswordRateLimitMax), EffectiveSource: source(env("PASSWORD_RATE_LIMIT_MAX"), dbInt(s.PasswordRateLimitMax), "5"), EnvValue: env("PASSWORD_RATE_LIMIT_MAX"), DBValue: dbInt(s.PasswordRateLimitMax), DefaultValue: "5", Editable: true, Conflict: env("PASSWORD_RATE_LIMIT_MAX") != "" && s.PasswordRateLimitMax != nil, Description: "Max password changes per window"},
		{Category: "Redis", Key: "REDIS_HOST", EffectiveValue: cfg.RedisHost, EffectiveSource: source(env("REDIS_HOST"), "", "localhost"), EnvValue: env("REDIS_HOST"), DBValue: "", DefaultValue: "localhost", Editable: false, Conflict: false, Description: "Redis host; configure via .env"},
		{Category: "Redis", Key: "REDIS_PORT", EffectiveValue: strconv.Itoa(cfg.RedisPort), EffectiveSource: source(env("REDIS_PORT"), "", "6379"), EnvValue: env("REDIS_PORT"), DBValue: "", DefaultValue: "6379", Editable: false, Conflict: false, Description: "Redis port; configure via .env"},
		{Category: "Redis", Key: "REDIS_PASSWORD", EffectiveValue: maskDatabaseURL(cfg.RedisPassword), EffectiveSource: source(env("REDIS_PASSWORD"), "", ""), EnvValue: maskEnvIfSet("REDIS_PASSWORD"), DBValue: "", DefaultValue: "(empty)", Editable: false, Conflict: false, Description: "Redis password; configure via .env only"},
		{Category: "Redis", Key: "REDIS_USER", EffectiveValue: orEmpty(cfg.RedisUser, " -"), EffectiveSource: source(env("REDIS_USER"), "", ""), EnvValue: env("REDIS_USER"), DBValue: "", DefaultValue: "(empty)", Editable: false, Conflict: false, Description: "Redis username (Redis 6+ ACL); configure via .env"},
		{Category: "Redis", Key: "REDIS_DB", EffectiveValue: strconv.Itoa(cfg.RedisDB), EffectiveSource: source(env("REDIS_DB"), "", "0"), EnvValue: env("REDIS_DB"), DBValue: "", DefaultValue: "0", Editable: false, Conflict: false, Description: "Redis database number; configure via .env"},
		{Category: "Redis", Key: "REDIS_TLS", EffectiveValue: boolStr(cfg.RedisTLS), EffectiveSource: source(env("REDIS_TLS"), "", "false"), EnvValue: env("REDIS_TLS"), DBValue: "", DefaultValue: "false", Editable: false, Conflict: false, Description: "Enable Redis TLS; configure via .env"},
		{Category: "Redis", Key: "REDIS_TLS_VERIFY", EffectiveValue: boolStr(os.Getenv("REDIS_TLS_VERIFY") != "false"), EffectiveSource: source(env("REDIS_TLS_VERIFY"), "", "true"), EnvValue: env("REDIS_TLS_VERIFY"), DBValue: "", DefaultValue: "true", Editable: false, Conflict: false, Description: "Verify Redis TLS cert; false to skip; configure via .env"},
		{Category: "Redis", Key: "REDIS_TLS_CA", EffectiveValue: orEmpty(cfg.RedisTLSCA, " -"), EffectiveSource: source(env("REDIS_TLS_CA"), "", ""), EnvValue: env("REDIS_TLS_CA"), DBValue: "", DefaultValue: "(empty)", Editable: false, Conflict: false, Description: "Redis TLS CA file path or PEM; configure via .env"},
		{Category: "Redis", Key: "REDIS_CONNECT_TIMEOUT_MS", EffectiveValue: strconv.Itoa(cfg.RedisConnectTimeout), EffectiveSource: source(env("REDIS_CONNECT_TIMEOUT_MS"), "", "60000"), EnvValue: env("REDIS_CONNECT_TIMEOUT_MS"), DBValue: "", DefaultValue: "60000", Editable: false, Conflict: false, Description: "Redis connect timeout (ms); configure via .env"},
		{Category: "Redis", Key: "REDIS_COMMAND_TIMEOUT_MS", EffectiveValue: strconv.Itoa(cfg.RedisCommandTimeout), EffectiveSource: source(env("REDIS_COMMAND_TIMEOUT_MS"), "", "60000"), EnvValue: env("REDIS_COMMAND_TIMEOUT_MS"), DBValue: "", DefaultValue: "60000", Editable: false, Conflict: false, Description: "Redis command timeout (ms); configure via .env"},
		{Category: "Encryption", Key: "AI_ENCRYPTION_KEY", EffectiveValue: maskDatabaseURL(os.Getenv("AI_ENCRYPTION_KEY")), EffectiveSource: source(maskEnvIfSet("AI_ENCRYPTION_KEY"), "", ""), EnvValue: maskEnvIfSet("AI_ENCRYPTION_KEY"), DBValue: "", DefaultValue: "(optional)", Editable: false, Conflict: false, Description: "Encryption key for AI keys, bootstrap tokens; configure via .env only"},
		{Category: "Encryption", Key: "SESSION_SECRET", EffectiveValue: maskDatabaseURL(os.Getenv("SESSION_SECRET")), EffectiveSource: source(maskEnvIfSet("SESSION_SECRET"), "", ""), EnvValue: maskEnvIfSet("SESSION_SECRET"), DBValue: "", DefaultValue: "(optional)", Editable: false, Conflict: false, Description: "Fallback encryption key; configure via .env only"},
		{Category: "Deployment", Key: "ENV_FILE", EffectiveValue: orEmpty(env("ENV_FILE"), ".env"), EffectiveSource: source(env("ENV_FILE"), "", ".env"), EnvValue: env("ENV_FILE"), DBValue: "", DefaultValue: ".env", Editable: false, Conflict: false, Description: "Path to .env file; configure via .env"},
		{Category: "Deployment", Key: "AGENTS_DIR", EffectiveValue: orEmpty(env("AGENTS_DIR"), " -"), EffectiveSource: source(env("AGENTS_DIR"), "", ""), EnvValue: env("AGENTS_DIR"), DBValue: "", DefaultValue: "(empty)", Editable: false, Conflict: false, Description: "Agent binaries directory for install script; configure via .env"},
		{Category: "Deployment", Key: "AGENT_BINARIES_DIR", EffectiveValue: orEmpty(env("AGENT_BINARIES_DIR"), " -"), EffectiveSource: source(env("AGENT_BINARIES_DIR"), "", ""), EnvValue: env("AGENT_BINARIES_DIR"), DBValue: "", DefaultValue: "(empty)", Editable: false, Conflict: false, Description: "Agent binaries path; overrides AGENTS_DIR; configure via .env"},
	}
	return out
}

func envTzOrTimezone() string {
	if v := os.Getenv("TZ"); v != "" {
		return strings.TrimSpace(v)
	}
	return os.Getenv("TIMEZONE")
}

// maskDatabaseURL returns "hidden" when the URL is set (sensitive); " -" when empty.
func maskDatabaseURL(s string) string {
	if s == "" {
		return " -"
	}
	return "hidden"
}

// maskEnvIfSet returns "Set" if the env var is non-empty (to avoid exposing secrets), else "".
func maskEnvIfSet(key string) string {
	if os.Getenv(key) != "" {
		return "Set"
	}
	return ""
}

func source(envVal, dbVal, defaultVal string) string {
	if envVal != "" {
		return "env"
	}
	if dbVal != "" {
		return "db"
	}
	return "default"
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func formatBytesEnv(n int64) string {
	if n >= 1024*1024*1024 {
		return strconv.FormatInt(n/(1024*1024*1024), 10) + "gb"
	}
	if n >= 1024*1024 {
		return strconv.FormatInt(n/(1024*1024), 10) + "mb"
	}
	if n >= 1024 {
		return strconv.FormatInt(n/1024, 10) + "kb"
	}
	return strconv.FormatInt(n, 10) + "b"
}

// UpdateEnvironmentConfig handles PATCH /settings/environment/:key.
func (h *SettingsHandler) UpdateEnvironmentConfig(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		Error(w, http.StatusBadRequest, "Key required")
		return
	}
	if h.cfg != nil && h.cfg.AdminMode && key != "TIMEZONE" {
		Error(w, http.StatusForbidden, "Setting not available")
		return
	}
	var req struct {
		Value interface{} `json:"value"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Value == nil {
		Error(w, http.StatusBadRequest, "Value is required")
		return
	}
	slog.Debug("env config update", "key", key, "value", req.Value)
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	if err := h.settings.UpdateConfigKey(r.Context(), s.ID, key, req.Value, s); err != nil {
		slog.Error("env config update failed", "key", key, "error", err)
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	slog.Debug("env config update saved", "key", key, "settings_id", s.ID)
	JSON(w, http.StatusOK, map[string]string{"message": "Saved. Restart the application for changes to take effect."})
}

func orEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// GetPublic handles GET /settings/public (read-only settings for all authenticated users).
func (h *SettingsHandler) GetPublic(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		// No settings row — resolve timezone from env/config defaults (no DB value).
		adminMode := h.cfg != nil && h.cfg.AdminMode
		JSON(w, http.StatusOK, map[string]interface{}{
			"auto_update":    false,
			"alerts_enabled": true,
			"timezone":       config.ResolveTimezone(nil, h.cfg),
			"admin_mode":     adminMode,
		})
		return
	}

	// Re-resolve timezone from this context's DB settings (not the startup-frozen h.resolved).
	// This ensures each context gets its own timezone in multi-context mode.
	timezone := config.ResolveTimezone(s.Timezone, h.cfg)

	adminMode := h.cfg != nil && h.cfg.AdminMode
	JSON(w, http.StatusOK, map[string]interface{}{
		"auto_update":    s.AutoUpdate,
		"alerts_enabled": s.AlertsEnabled,
		"logo_dark":      s.LogoDark,
		"logo_light":     s.LogoLight,
		"favicon":        s.Favicon,
		"updated_at":     s.UpdatedAt,
		"timezone":       timezone,
		"admin_mode":     adminMode,
	})
}

// Update handles PATCH or PUT /settings.
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	// In managed/multi-context mode, prevent self-registration from being enabled.
	if h.cfg != nil && h.cfg.AdminMode {
		delete(req, "signupEnabled")
		delete(req, "signup_enabled")
	}

	oldInterval := s.UpdateInterval
	oldComplianceScanInterval := s.ComplianceScanInterval
	oldPackageCacheRefreshMode := s.PackageCacheRefreshMode
	oldPackageCacheRefreshMaxAge := s.PackageCacheRefreshMaxAge
	applySettingsUpdate(s, req, h.enc)

	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update settings")
		return
	}

	intervalChanged := s.UpdateInterval != oldInterval && s.UpdateInterval > 0
	complianceIntervalChanged := s.ComplianceScanInterval != oldComplianceScanInterval && s.ComplianceScanInterval > 0
	cacheRefreshChanged := s.PackageCacheRefreshMode != oldPackageCacheRefreshMode || s.PackageCacheRefreshMaxAge != oldPackageCacheRefreshMaxAge
	if h.registry != nil && (intervalChanged || complianceIntervalChanged || cacheRefreshChanged) {
		msg := map[string]interface{}{
			"type":                          "settings_update",
			"update_interval":               s.UpdateInterval,
			"compliance_scan_interval":      s.ComplianceScanInterval,
			"package_cache_refresh_mode":    s.PackageCacheRefreshMode,
			"package_cache_refresh_max_age": s.PackageCacheRefreshMaxAge,
		}
		pushed := 0
		for _, apiID := range h.registry.GetConnectedApiIDs() {
			if err := h.registry.SendJSON(apiID, msg); err != nil {
				slog.Warn("failed to push settings_update to agent", "api_id", apiID, "error", err)
			} else {
				pushed++
			}
		}
		if pushed > 0 {
			slog.Info("pushed settings_update to connected agents", "interval", s.UpdateInterval, "compliance_scan_interval", s.ComplianceScanInterval, "count", pushed)
		}
	}

	JSON(w, http.StatusOK, settingsToResponse(s, h.enc))
}

func settingsToResponse(s *models.Settings, enc *util.Encryption) map[string]interface{} {
	discordSecretSet := false
	if s.DiscordClientSecret != nil && *s.DiscordClientSecret != "" && enc != nil {
		_, err := enc.Decrypt(*s.DiscordClientSecret)
		discordSecretSet = err == nil
	}
	res := map[string]interface{}{
		"id": s.ID, "server_url": s.ServerURL, "server_protocol": s.ServerProtocol,
		"server_host": s.ServerHost, "server_port": s.ServerPort,
		"created_at": s.CreatedAt, "updated_at": s.UpdatedAt,
		"update_interval": s.UpdateInterval, "auto_update": s.AutoUpdate,
		"default_compliance_mode": s.DefaultComplianceMode, "compliance_scan_interval": s.ComplianceScanInterval,
		"package_cache_refresh_mode": s.PackageCacheRefreshMode, "package_cache_refresh_max_age": s.PackageCacheRefreshMaxAge,
		"github_repo_url": s.GithubRepoURL,
		"ssh_key_path":    s.SSHKeyPath, "repository_type": s.RepositoryType,
		"last_update_check": s.LastUpdateCheck, "latest_version": s.LatestVersion,
		"update_available": s.UpdateAvailable,
		"signup_enabled":   s.SignupEnabled, "default_user_role": s.DefaultUserRole,
		"ignore_ssl_self_signed": s.IgnoreSSLSelfSigned,
		"logo_dark":              s.LogoDark, "logo_light": s.LogoLight, "favicon": s.Favicon,
		"metrics_enabled": s.MetricsEnabled, "metrics_anonymous_id": s.MetricsAnonymousID,
		"metrics_last_sent":            s.MetricsLastSent,
		"show_github_version_on_login": s.ShowGithubVersionOnLogin,
		"ai_enabled":                   s.AiEnabled, "ai_provider": s.AiProvider, "ai_model": s.AiModel,
		"alerts_enabled":        s.AlertsEnabled,
		"discord_oauth_enabled": s.DiscordOAuthEnabled, "discord_client_id": s.DiscordClientID,
		"discord_client_secret_set": discordSecretSet,
		"discord_redirect_uri":      s.DiscordRedirectURI, "discord_button_text": s.DiscordButtonText,
	}
	return res
}

// getReqString returns string from req for key or altKey (camelCase fallback).
func getReqString(req map[string]interface{}, key, altKey string) (string, bool) {
	if v, ok := req[key].(string); ok && v != "" {
		return v, true
	}
	if v, ok := req[altKey].(string); ok && v != "" {
		return v, true
	}
	return "", false
}

// getReqStringOrEmpty returns string from req for key or altKey; ok is true if key exists (even if empty).
func getReqStringOrEmpty(req map[string]interface{}, key, altKey string) (string, bool) {
	if v, ok := req[key].(string); ok {
		return v, true
	}
	if v, ok := req[altKey].(string); ok {
		return v, true
	}
	return "", false
}

// getReqBool returns bool from req for key or altKey.
func getReqBool(req map[string]interface{}, key, altKey string) (bool, bool) {
	if v, ok := req[key].(bool); ok {
		return v, true
	}
	if v, ok := req[altKey].(bool); ok {
		return v, true
	}
	return false, false
}

// getReqFloat64 returns float64 from req for key or altKey (for JSON numbers).
func getReqFloat64(req map[string]interface{}, key, altKey string) (float64, bool) {
	if v, ok := req[key].(float64); ok {
		return v, true
	}
	if v, ok := req[altKey].(float64); ok {
		return v, true
	}
	return 0, false
}

// constructServerURL builds a server URL from protocol, host, and port.
// Omits port for default ports (80 for http, 443 for https).
func constructServerURL(protocol, host string, port int) string {
	proto := strings.ToLower(protocol)
	if (proto == "https" && port == 443) || (proto == "http" && port == 80) {
		return proto + "://" + host
	}
	return proto + "://" + host + ":" + strconv.Itoa(port)
}

func applySettingsUpdate(s *models.Settings, req map[string]interface{}, enc *util.Encryption) {
	urlVal, hasExplicitURL := getReqString(req, "server_url", "serverUrl")
	if hasExplicitURL {
		s.ServerURL = urlVal
	}
	if v, ok := getReqString(req, "server_protocol", "serverProtocol"); ok {
		s.ServerProtocol = v
	}
	if v, ok := getReqString(req, "server_host", "serverHost"); ok {
		s.ServerHost = v
	}
	if v, ok := getReqFloat64(req, "server_port", "serverPort"); ok {
		s.ServerPort = int(v)
	}
	// Derive server_url from protocol/host/port when any of those were updated (matches Node backend behavior).
	// Only derive when server_url was not explicitly sent (explicit URL takes precedence).
	_, hasProtocol := getReqString(req, "server_protocol", "serverProtocol")
	_, hasHost := getReqString(req, "server_host", "serverHost")
	_, hasPort := getReqFloat64(req, "server_port", "serverPort")
	if !hasExplicitURL && (hasProtocol || hasHost || hasPort) {
		s.ServerURL = constructServerURL(s.ServerProtocol, s.ServerHost, s.ServerPort)
	}
	if v, ok := getReqFloat64(req, "update_interval", "updateInterval"); ok {
		s.UpdateInterval = int(v)
	}
	if v, ok := getReqBool(req, "auto_update", "autoUpdate"); ok {
		s.AutoUpdate = v
	}
	if v, ok := getReqString(req, "default_compliance_mode", "defaultComplianceMode"); ok {
		s.DefaultComplianceMode = v
	}
	if v, ok := getReqFloat64(req, "compliance_scan_interval", "complianceScanInterval"); ok {
		val := int(v)
		if val >= 60 && val <= 10080 {
			s.ComplianceScanInterval = val
		}
	}
	if v, ok := getReqString(req, "package_cache_refresh_mode", "packageCacheRefreshMode"); ok {
		if v == "always" || v == "if_stale" || v == "never" {
			s.PackageCacheRefreshMode = v
		}
	}
	if v, ok := getReqFloat64(req, "package_cache_refresh_max_age", "packageCacheRefreshMaxAge"); ok {
		val := int(v)
		if val >= 1 && val <= 1440 {
			s.PackageCacheRefreshMaxAge = val
		}
	}
	if v, ok := getReqString(req, "github_repo_url", "githubRepoUrl"); ok {
		s.GithubRepoURL = v
	}
	if v, ok := getReqString(req, "ssh_key_path", "sshKeyPath"); ok {
		s.SSHKeyPath = &v
	}
	if v, ok := getReqString(req, "repository_type", "repositoryType"); ok {
		s.RepositoryType = v
	}
	if v, ok := getReqBool(req, "signup_enabled", "signupEnabled"); ok {
		s.SignupEnabled = v
	}
	if v, ok := getReqString(req, "default_user_role", "defaultUserRole"); ok {
		s.DefaultUserRole = v
	}
	if v, ok := getReqBool(req, "ignore_ssl_self_signed", "ignoreSslSelfSigned"); ok {
		s.IgnoreSSLSelfSigned = v
	}
	if v, ok := getReqString(req, "logo_dark", "logoDark"); ok {
		s.LogoDark = &v
	}
	if v, ok := getReqString(req, "logo_light", "logoLight"); ok {
		s.LogoLight = &v
	}
	if v, ok := getReqString(req, "favicon", "favicon"); ok {
		s.Favicon = &v
	}
	if v, ok := getReqBool(req, "show_github_version_on_login", "showGithubVersionOnLogin"); ok {
		s.ShowGithubVersionOnLogin = v
	}
	if v, ok := getReqBool(req, "alerts_enabled", "alertsEnabled"); ok {
		s.AlertsEnabled = v
	}
	if v, ok := getReqBool(req, "discord_oauth_enabled", "discordOauthEnabled"); ok {
		s.DiscordOAuthEnabled = v
	}
	if v, ok := getReqString(req, "discord_client_id", "discordClientId"); ok {
		s.DiscordClientID = &v
	}
	if v, ok := getReqString(req, "discord_redirect_uri", "discordRedirectUri"); ok {
		s.DiscordRedirectURI = &v
	}
	if v, ok := getReqString(req, "discord_button_text", "discordButtonText"); ok {
		s.DiscordButtonText = &v
	}
	if v, ok := getReqStringOrEmpty(req, "discord_client_secret", "discordClientSecret"); ok {
		if v == "" {
			s.DiscordClientSecret = nil
		} else if util.IsEncrypted(v) {
			s.DiscordClientSecret = &v
		} else if enc != nil {
			encrypted, err := enc.Encrypt(v)
			if err == nil {
				s.DiscordClientSecret = &encrypted
			}
		} else {
			s.DiscordClientSecret = &v
		}
	}
	// AI settings
	if v, ok := getReqBool(req, "ai_enabled", "aiEnabled"); ok {
		s.AiEnabled = v
	}
	if v, ok := getReqString(req, "ai_provider", "aiProvider"); ok {
		if v == "openrouter" || v == "anthropic" || v == "openai" || v == "gemini" {
			s.AiProvider = v
		}
	}
	if v, ok := getReqString(req, "ai_model", "aiModel"); ok {
		s.AiModel = &v
		if v == "" {
			s.AiModel = nil
		}
	}
	if v, ok := getReqString(req, "ai_api_key", "aiApiKey"); ok && v != "" {
		if util.IsEncrypted(v) {
			s.AiAPIKey = &v
		} else if enc != nil {
			encrypted, err := enc.Encrypt(v)
			if err == nil {
				s.AiAPIKey = &encrypted
			}
		} else {
			s.AiAPIKey = &v
		}
	}
}

// logoUploadReq is the request body for POST /settings/logos/upload.
type logoUploadReq struct {
	LogoType    string `json:"logoType"`
	FileContent string `json:"fileContent"`
	FileName    string `json:"fileName"`
}

// logoResetReq is the request body for POST /settings/logos/reset.
type logoResetReq struct {
	LogoType string `json:"logoType"`
}

// UploadLogo handles POST /api/v1/settings/logos/upload.
// Accepts JSON with logoType (dark|light|favicon), fileContent (base64 or data URL), fileName.
// Stores logo in database; no filesystem writes.
func (h *SettingsHandler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	var req logoUploadReq
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.LogoType == "" || req.FileContent == "" {
		Error(w, http.StatusBadRequest, "Logo type and file content are required")
		return
	}
	if req.LogoType != "dark" && req.LogoType != "light" && req.LogoType != "favicon" {
		Error(w, http.StatusBadRequest, "Logo type must be 'dark', 'light', or 'favicon'")
		return
	}

	const maxFileSize = 5 * 1024 * 1024 // 5MB
	estimatedSize := (len(req.FileContent) * 3) / 4
	if estimatedSize > maxFileSize {
		Error(w, http.StatusBadRequest, "File size exceeds maximum allowed (5MB)")
		return
	}

	var rawBase64 string
	if strings.HasPrefix(req.FileContent, "data:") {
		idx := strings.Index(req.FileContent, ",")
		if idx < 0 {
			Error(w, http.StatusBadRequest, "Invalid data URL format")
			return
		}
		rawBase64 = req.FileContent[idx+1:]
	} else {
		rawBase64 = req.FileContent
	}
	fileBuf, err := base64.StdEncoding.DecodeString(rawBase64)
	if err != nil {
		Error(w, http.StatusBadRequest, "Invalid base64 file content")
		return
	}

	// Magic byte validation
	pngMagic := []byte{0x89, 0x50, 0x4e, 0x47}
	jpegMagic := []byte{0xff, 0xd8, 0xff}
	isPng := len(fileBuf) >= 4 && string(fileBuf[:4]) == string(pngMagic)
	isJpeg := len(fileBuf) >= 3 && string(fileBuf[:3]) == string(jpegMagic)
	isSvg := len(fileBuf) >= 100 && strings.Contains(string(fileBuf[:100]), "<svg")

	var contentType string
	if isPng {
		contentType = "image/png"
	} else if isJpeg {
		contentType = "image/jpeg"
	} else if isSvg {
		sanitizedSvg := sanitizeSVG(string(fileBuf))
		fileBuf = []byte(sanitizedSvg)
		contentType = "image/svg+xml"
	} else {
		Error(w, http.StatusBadRequest, "Invalid file type. Allowed: PNG, JPEG, SVG")
		return
	}

	logoPath := "/api/v1/settings/logos/" + req.LogoType
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	switch req.LogoType {
	case "dark":
		s.LogoDark = &logoPath
		s.LogoDarkData = fileBuf
		s.LogoDarkContentType = &contentType
	case "light":
		s.LogoLight = &logoPath
		s.LogoLightData = fileBuf
		s.LogoLightContentType = &contentType
	case "favicon":
		s.Favicon = &logoPath
		s.FaviconData = fileBuf
		s.FaviconContentType = &contentType
	}
	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update settings")
		return
	}

	size := int64(len(fileBuf))
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":       req.LogoType + " logo uploaded successfully",
		"fileName":      "logo_" + req.LogoType,
		"path":          logoPath,
		"size":          size,
		"sizeFormatted": strconv.FormatFloat(float64(size)/1024, 'f', 1, 64) + " KB",
	})
}

// ResetLogo handles POST /api/v1/settings/logos/reset.
func (h *SettingsHandler) ResetLogo(w http.ResponseWriter, r *http.Request) {
	var req logoResetReq
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.LogoType == "" {
		Error(w, http.StatusBadRequest, "Logo type is required")
		return
	}
	if req.LogoType != "dark" && req.LogoType != "light" && req.LogoType != "favicon" {
		Error(w, http.StatusBadRequest, "Logo type must be 'dark', 'light', or 'favicon'")
		return
	}

	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	switch req.LogoType {
	case "dark":
		s.LogoDark = nil
		s.LogoDarkData = nil
		s.LogoDarkContentType = nil
	case "light":
		s.LogoLight = nil
		s.LogoLightData = nil
		s.LogoLightContentType = nil
	case "favicon":
		s.Favicon = nil
		s.FaviconData = nil
		s.FaviconContentType = nil
	}
	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update settings")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":  req.LogoType + " logo reset to default successfully",
		"logoType": req.LogoType,
	})
}

// GetLogo handles GET /api/v1/settings/logos/{type} (public, no auth).
// Streams custom logo bytes from database. Returns 404 if no custom logo.
func (h *SettingsHandler) GetLogo(w http.ResponseWriter, r *http.Request) {
	logoType := chi.URLParam(r, "type")
	if logoType != "dark" && logoType != "light" && logoType != "favicon" {
		Error(w, http.StatusBadRequest, "Logo type must be 'dark', 'light', or 'favicon'")
		return
	}

	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}

	var data []byte
	var contentType string
	switch logoType {
	case "dark":
		data = s.LogoDarkData
		if s.LogoDarkContentType != nil {
			contentType = *s.LogoDarkContentType
		}
	case "light":
		data = s.LogoLightData
		if s.LogoLightContentType != nil {
			contentType = *s.LogoLightContentType
		}
	case "favicon":
		data = s.FaviconData
		if s.FaviconContentType != nil {
			contentType = *s.FaviconContentType
		}
	}

	if len(data) == 0 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(data)
}

// sanitizeSVG removes script tags, event handlers, and javascript: URLs from SVG to prevent XSS.
// Uses strings instead of Perl-style regex (lookahead) since Go's regexp is RE2-based.
func sanitizeSVG(s string) string {
	// Remove script tags and contents (loop: RE2 has no lookahead)
	s = removeTagAndContent(s, "script")
	// Remove self-closing script tags: <script ... />
	selfCloseScript := regexp.MustCompile(`(?i)<script[^>]*/\s*>`)
	s = selfCloseScript.ReplaceAllString(s, "")
	// Remove foreignObject (can embed arbitrary HTML)
	s = removeTagAndContent(s, "foreignObject")
	// Remove dangerous SVG animation/mutation elements
	for _, tag := range []string{"set", "animate", "animateTransform", "animateMotion"} {
		s = removeTagAndContent(s, tag)
		selfClose := regexp.MustCompile(`(?i)<` + tag + `[^>]*/\s*>`)
		s = selfClose.ReplaceAllString(s, "")
	}
	// Remove <use> elements with external references (can load external SVG)
	useExtRe := regexp.MustCompile(`(?i)<use[^>]+href\s*=\s*["']https?://[^"']*["'][^>]*/?\s*>`)
	s = useExtRe.ReplaceAllString(s, "")
	s = removeTagAndContent(s, "use")
	// Remove on* event handlers
	onRe := regexp.MustCompile(`(?i)\s+on\w+\s*=\s*["'][^"']*["']`)
	s = onRe.ReplaceAllString(s, "")
	onRe2 := regexp.MustCompile(`(?i)\s+on\w+\s*=\s*[^\s>]+`)
	s = onRe2.ReplaceAllString(s, "")
	// Remove javascript: URLs (href, xlink:href, and any src-like attributes)
	jsRe := regexp.MustCompile(`(?i)(href|src|xlink:href)\s*=\s*["']\s*javascript:[^"']*["']`)
	s = jsRe.ReplaceAllString(s, `$1=""`)
	// Remove data: URLs (RE2 has no lookahead, so match all data: and restore safe image types)
	dataRe := regexp.MustCompile(`(?i)(href|src|xlink:href)\s*=\s*["']\s*data:[^"']*["']`)
	s = dataRe.ReplaceAllStringFunc(s, func(m string) string {
		lower := strings.ToLower(m)
		for _, safe := range []string{"data:image/png", "data:image/jpeg", "data:image/jpg", "data:image/gif", "data:image/svg+xml", "data:image/webp"} {
			if strings.Contains(lower, safe) {
				return m
			}
		}
		eqIdx := strings.Index(m, "=")
		if eqIdx >= 0 {
			return m[:eqIdx] + `=""`
		}
		return m
	})
	// Remove CSS @import (can exfiltrate data or load external styles)
	importRe := regexp.MustCompile(`(?i)@import\s+[^;]+;?`)
	s = importRe.ReplaceAllString(s, "")
	return s
}

// removeTagAndContent removes <tag...>...</tag> and its contents. Case-insensitive.
func removeTagAndContent(s, tag string) string {
	openTag := "<" + tag
	closeTag := "</" + tag + ">"
	lower := strings.ToLower(s)
	for {
		i := strings.Index(lower, strings.ToLower(openTag))
		if i < 0 {
			break
		}
		j := strings.Index(lower[i:], strings.ToLower(closeTag))
		if j < 0 {
			break
		}
		end := i + j + len(closeTag)
		s = s[:i] + s[end:]
		lower = strings.ToLower(s)
	}
	return s
}
