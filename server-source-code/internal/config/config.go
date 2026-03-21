// Package config provides application configuration from environment.
package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// DefaultVersion is the default server version. Bump this when releasing; config_test.go uses it.
const DefaultVersion = "1.4.93"

// Config holds application configuration loaded from environment.
// Uses same variable names as PatchMon/server for compatibility.
type Config struct {
	// Database
	DatabaseURL          string
	DBConnMaxAttempts    int
	DBConnWaitInterval   int
	DBConnectionLimit    int
	DBPoolTimeout        int
	DBConnectTimeout     int
	DBIdleTimeout        int
	DBMaxLifetime        int
	DBTransactionMaxWait int
	DBTransactionTimeout int

	// Server
	Port    int
	Env     string
	Version string

	// Auth
	JWTSecret    string
	JWTExpiresIn string
	// AuthBrowserSessionCookies: when true, token and refresh_token cookies omit Max-Age (session cookies)
	// so they are cleared when the browser session ends instead of persisting across restarts.
	AuthBrowserSessionCookies bool

	// CORS
	CORSOrigin string

	// Assets directory for custom branding (logos, favicons). Used for Docker volume mount.
	// AssetsDir is deprecated. Custom logos are now stored in the database and served via GET /api/v1/settings/logos/{type}.
	AssetsDir string

	// Logging
	EnableLogging bool
	LogLevel      string

	// Profiling (pprof, memstats)
	EnablePprof         bool
	MemstatsIntervalSec int

	// Redis (for bootstrap tokens, asynq job queues, TFA lockout, etc.)
	RedisHost           string
	RedisPort           int
	RedisPassword       string
	RedisUser           string
	RedisDB             int
	RedisTLS            bool
	RedisConnectTimeout int
	RedisCommandTimeout int

	// TFA (Two-Factor Authentication)
	MaxTfaAttempts         int
	TfaLockoutDurationMin  int
	TfaRememberMeExpiresIn string // e.g. "30d"

	// Auth/Lockout (env -> DB -> default)
	MaxLoginAttempts   int
	LockoutDurationMin int
	// Server
	EnableHSTS bool
	TrustProxy bool
	// Rate limits (env -> DB -> default)
	RateLimitWindowMs         int
	RateLimitMax              int
	AuthRateLimitWindowMs     int
	AuthRateLimitMax          int
	AgentRateLimitWindowMs    int
	AgentRateLimitMax         int
	PasswordRateLimitWindowMs int
	PasswordRateLimitMax      int
	// Password policy
	PasswordMinLength        int
	PasswordRequireUppercase bool
	PasswordRequireLowercase bool
	PasswordRequireNumber    bool
	PasswordRequireSpecial   bool
	// Body limits (bytes)
	JSONBodyLimitBytes        int64
	AgentUpdateBodyLimitBytes int64
	// Redis (env only)
	RedisTLSCA string
	// Timezone
	Timezone string
	// User
	DefaultUserRole string
	// Session
	SessionInactivityTimeoutMin int
	TfaMaxRememberSessions      int
	// DB
	DBTransactionLongTimeout int

	// Multi-host (registry + per-host pools)
	RegistryDatabaseURL  string
	RegistryReloadSecret string
	HostPoolMaxConns     int
	HostPoolMinConns     int
	HostCacheTTLMin      int

	// RDP (guacd for in-browser RDP)
	GuacdPath    string // Path to guacd binary, or empty for PATH
	GuacdAddress string // Listen address for guacd, e.g. 127.0.0.1:4822

	// OIDC (OpenID Connect / SSO)
	OidcEnabled          bool
	OidcIssuerURL        string
	OidcClientID         string
	OidcClientSecret     string
	OidcRedirectURI      string
	OidcScopes           string
	OidcAutoCreateUsers  bool
	OidcDefaultRole      string
	OidcDisableLocalAuth bool
	OidcButtonText       string
	OidcSessionTTL       int
	OidcPostLogoutURI    string
	OidcSyncRoles        bool
	// Group-to-role mapping
	OidcAdminGroup       string
	OidcSuperadminGroup  string
	OidcHostManagerGroup string
	OidcReadonlyGroup    string
	OidcUserGroup        string
	OidcEnforceHTTPS     bool

	// SSG (SCAP Security Guide) content directory for compliance scanning
	SSGContentDir string
}

// Load reads configuration from environment.
// Loads .env from current directory, or path from ENV_FILE if set.
func Load() (*Config, error) {
	envPath := os.Getenv("ENV_FILE")
	if envPath == "" {
		envPath = ".env"
	}
	_ = godotenv.Load(envPath)

	cfg := &Config{
		DatabaseURL:          getEnv("DATABASE_URL", ""),
		DBConnMaxAttempts:    getEnvInt("PM_DB_CONN_MAX_ATTEMPTS", 30),
		DBConnWaitInterval:   getEnvInt("PM_DB_CONN_WAIT_INTERVAL", 2),
		DBConnectionLimit:    getEnvInt("DB_CONNECTION_LIMIT", 30),
		DBPoolTimeout:        getEnvInt("DB_POOL_TIMEOUT", 20),
		DBConnectTimeout:     getEnvInt("DB_CONNECT_TIMEOUT", 10),
		DBIdleTimeout:        getEnvInt("DB_IDLE_TIMEOUT", 300),
		DBMaxLifetime:        getEnvInt("DB_MAX_LIFETIME", 1800),
		DBTransactionMaxWait: getEnvInt("DB_TRANSACTION_MAX_WAIT", 10000),
		DBTransactionTimeout: getEnvInt("DB_TRANSACTION_TIMEOUT", 30000),

		Port:    getEnvInt("PORT", 3000),
		Env:     getEnvEnv(),
		Version: DefaultVersion,

		JWTSecret:                 getEnv("JWT_SECRET", ""),
		JWTExpiresIn:              getEnv("JWT_EXPIRES_IN", "1h"),
		AuthBrowserSessionCookies: getEnv("AUTH_BROWSER_SESSION_COOKIES", "") == "true",

		CORSOrigin: getEnv("CORS_ORIGIN", "http://localhost:3000"),
		AssetsDir:  getEnv("ASSETS_DIR", ""),

		EnableLogging: getEnv("ENABLE_LOGGING", "") == "true",
		LogLevel:      getEnv("LOG_LEVEL", "info"),

		EnablePprof:         getEnv("ENABLE_PPROF", "") == "true" || getEnvEnv() != "production",
		MemstatsIntervalSec: getEnvInt("MEMSTATS_INTERVAL_SEC", 60),

		RedisHost:           getEnv("REDIS_HOST", "localhost"),
		RedisPort:           getEnvInt("REDIS_PORT", 6379),
		RedisPassword:       getEnv("REDIS_PASSWORD", ""),
		RedisUser:           getEnv("REDIS_USER", ""),
		RedisDB:             getEnvInt("REDIS_DB", 0),
		RedisTLS:            getEnv("REDIS_TLS", "") == "true",
		RedisConnectTimeout: getEnvInt("REDIS_CONNECT_TIMEOUT_MS", 60000),
		RedisCommandTimeout: getEnvInt("REDIS_COMMAND_TIMEOUT_MS", 60000),

		MaxTfaAttempts:         getEnvInt("MAX_TFA_ATTEMPTS", 5),
		TfaLockoutDurationMin:  getEnvInt("TFA_LOCKOUT_DURATION_MINUTES", 30),
		TfaRememberMeExpiresIn: getEnv("TFA_REMEMBER_ME_EXPIRES_IN", "30d"),

		OidcEnabled:          getEnv("OIDC_ENABLED", "") == "true",
		OidcIssuerURL:        getEnv("OIDC_ISSUER_URL", ""),
		OidcClientID:         getEnv("OIDC_CLIENT_ID", ""),
		OidcClientSecret:     getEnv("OIDC_CLIENT_SECRET", ""),
		OidcRedirectURI:      getEnv("OIDC_REDIRECT_URI", ""),
		OidcScopes:           getEnv("OIDC_SCOPES", "openid email profile groups"),
		OidcAutoCreateUsers:  getEnv("OIDC_AUTO_CREATE_USERS", "") == "true",
		OidcDefaultRole:      getEnv("OIDC_DEFAULT_ROLE", "user"),
		OidcDisableLocalAuth: getEnv("OIDC_DISABLE_LOCAL_AUTH", "") == "true",
		OidcButtonText:       getEnv("OIDC_BUTTON_TEXT", "Login with SSO"),
		OidcSessionTTL:       getEnvInt("OIDC_SESSION_TTL", 600),
		OidcPostLogoutURI:    getEnv("OIDC_POST_LOGOUT_URI", getEnv("FRONTEND_URL", getEnv("CORS_ORIGIN", "http://localhost:3000"))+"/login"),
		OidcSyncRoles:        getEnv("OIDC_SYNC_ROLES", "") == "true",
		OidcAdminGroup:       getEnv("OIDC_ADMIN_GROUP", ""),
		OidcSuperadminGroup:  getEnv("OIDC_SUPERADMIN_GROUP", ""),
		OidcHostManagerGroup: getEnv("OIDC_HOST_MANAGER_GROUP", ""),
		OidcReadonlyGroup:    getEnv("OIDC_READONLY_GROUP", ""),
		OidcUserGroup:        getEnv("OIDC_USER_GROUP", ""),
		OidcEnforceHTTPS:     getEnv("OIDC_ENFORCE_HTTPS", "true") != "false",

		SSGContentDir: getEnv("SSG_CONTENT_DIR", "./ssg-content"),

		MaxLoginAttempts:            getEnvInt("MAX_LOGIN_ATTEMPTS", 5),
		LockoutDurationMin:          getEnvInt("LOCKOUT_DURATION_MINUTES", 15),
		EnableHSTS:                  getEnv("ENABLE_HSTS", "") == "true",
		TrustProxy:                  getEnv("TRUST_PROXY", "") == "true",
		RateLimitWindowMs:           getEnvInt("RATE_LIMIT_WINDOW_MS", 900000),
		RateLimitMax:                getEnvInt("RATE_LIMIT_MAX", 5000),
		AuthRateLimitWindowMs:       getEnvInt("AUTH_RATE_LIMIT_WINDOW_MS", 600000),
		AuthRateLimitMax:            getEnvInt("AUTH_RATE_LIMIT_MAX", 500),
		AgentRateLimitWindowMs:      getEnvInt("AGENT_RATE_LIMIT_WINDOW_MS", 60000),
		AgentRateLimitMax:           getEnvInt("AGENT_RATE_LIMIT_MAX", 1000),
		PasswordRateLimitWindowMs:   getEnvInt("PASSWORD_RATE_LIMIT_WINDOW_MS", 900000),
		PasswordRateLimitMax:        getEnvInt("PASSWORD_RATE_LIMIT_MAX", 5),
		PasswordMinLength:           getEnvInt("PASSWORD_MIN_LENGTH", 8),
		PasswordRequireUppercase:    getEnv("PASSWORD_REQUIRE_UPPERCASE", "true") != "false",
		PasswordRequireLowercase:    getEnv("PASSWORD_REQUIRE_LOWERCASE", "true") != "false",
		PasswordRequireNumber:       getEnv("PASSWORD_REQUIRE_NUMBER", "true") != "false",
		PasswordRequireSpecial:      getEnv("PASSWORD_REQUIRE_SPECIAL", "true") != "false",
		JSONBodyLimitBytes:          getEnvBytes("JSON_BODY_LIMIT", 5),
		AgentUpdateBodyLimitBytes:   getEnvBytes("AGENT_UPDATE_BODY_LIMIT", 2),
		RedisTLSCA:                  getEnv("REDIS_TLS_CA", ""),
		Timezone:                    getEnv("TZ", getEnv("TIMEZONE", "UTC")),
		DefaultUserRole:             getEnv("DEFAULT_USER_ROLE", "user"),
		SessionInactivityTimeoutMin: getEnvInt("SESSION_INACTIVITY_TIMEOUT_MINUTES", 30),
		TfaMaxRememberSessions:      getEnvInt("TFA_MAX_REMEMBER_SESSIONS", 5),
		DBTransactionLongTimeout:    getEnvInt("DB_TRANSACTION_LONG_TIMEOUT", 60000),

		RegistryDatabaseURL:  getEnv("REGISTRY_DATABASE_URL", ""),
		RegistryReloadSecret: getEnv("REGISTRY_RELOAD_SECRET", ""),
		HostPoolMaxConns:     getEnvInt("HOST_POOL_MAX_CONNS", 5),
		HostPoolMinConns:     getEnvInt("HOST_POOL_MIN_CONNS", 1),
		HostCacheTTLMin:      getEnvInt("HOST_CACHE_TTL_MINUTES", 10),

		GuacdPath:    getEnv("GUACD_PATH", ""),
		GuacdAddress: getEnv("GUACD_ADDRESS", "127.0.0.1:4822"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks required configuration.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("PORT must be between 1 and 65535, got %d", c.Port)
	}
	if c.DBConnectionLimit < 0 || c.DBConnectionLimit > math.MaxInt32 {
		return fmt.Errorf("DB_CONNECTION_LIMIT must be 0..%d, got %d", math.MaxInt32, c.DBConnectionLimit)
	}
	level := strings.ToLower(c.LogLevel)
	switch level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error, got %q", c.LogLevel)
	}
	if c.OidcEnabled {
		if c.OidcIssuerURL == "" || c.OidcClientID == "" || c.OidcClientSecret == "" || c.OidcRedirectURI == "" {
			return fmt.Errorf("OIDC is enabled but missing required config: OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, OIDC_REDIRECT_URI")
		}
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// getEnvEnv returns APP_ENV if set, else NODE_ENV (for backward compatibility), else "production".
func getEnvEnv() string {
	return getEnv("APP_ENV", getEnv("NODE_ENV", "production"))
}

func getEnvInt(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

// getEnvBytes parses size strings like "5mb", "2mb", "1gb" into bytes.
// defaultMB is used when env is empty or parse fails.
func getEnvBytes(key string, defaultMB int) int64 {
	s := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if s == "" {
		return int64(defaultMB) * 1024 * 1024
	}
	var mult int64 = 1024 * 1024
	if strings.HasSuffix(s, "kb") {
		mult = 1024
		s = strings.TrimSuffix(s, "kb")
	} else if strings.HasSuffix(s, "mb") {
		s = strings.TrimSuffix(s, "mb")
	} else if strings.HasSuffix(s, "gb") {
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "gb")
	} else if strings.HasSuffix(s, "b") {
		mult = 1
		s = strings.TrimSuffix(s, "b")
	}
	s = strings.TrimSpace(s)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 1 {
		return int64(defaultMB) * 1024 * 1024
	}
	return v * mult
}
