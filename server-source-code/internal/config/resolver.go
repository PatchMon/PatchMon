// Package config provides env -> DB -> default resolution for application config.
package config

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

// ResolvedConfig holds the effective configuration after merging env, DB, and defaults.
// Env takes priority over DB, which takes priority over code defaults.
type ResolvedConfig struct {
	CORSOrigin                  string
	EnableLogging               bool
	LogLevel                    string
	MaxLoginAttempts            int
	LockoutDurationMin          int
	EnableHSTS                  bool
	TrustProxy                  bool
	RateLimitWindowMs           int
	RateLimitMax                int
	AuthRateLimitWindowMs       int
	AuthRateLimitMax            int
	AgentRateLimitWindowMs      int
	AgentRateLimitMax           int
	PasswordRateLimitWindowMs   int
	PasswordRateLimitMax        int
	PasswordMinLength           int
	PasswordRequireUppercase    bool
	PasswordRequireLowercase    bool
	PasswordRequireNumber       bool
	PasswordRequireSpecial      bool
	JSONBodyLimitBytes          int64
	AgentUpdateBodyLimitBytes   int64
	AgentPingBodyLimitBytes     int64
	Timezone                    string
	DefaultUserRole             string
	SessionInactivityTimeoutMin int
	PatchRunStallTimeoutMin     int
	TfaMaxRememberSessions      int
	DBTransactionLongTimeout    int
	JwtExpiresIn                string
	AuthBrowserSessionCookies   bool
	MaxTfaAttempts              int
	TfaLockoutDurationMin       int
	TfaRememberMeExpiresIn      string
}

// ResolveConfig produces ResolvedConfig from env, DB settings, and Config defaults.
// Call after DB connect; pass settings from SettingsStore.GetFirst.
func ResolveConfig(ctx context.Context, cfg *Config, settings *models.Settings) *ResolvedConfig {
	if settings == nil {
		return resolveFromEnvAndDefaults(cfg)
	}
	out := &ResolvedConfig{}
	out.CORSOrigin = resolveString("CORS_ORIGIN", settings.CorsOrigin, cfg.CORSOrigin)
	out.EnableLogging = resolveBool("ENABLE_LOGGING", settings.EnableLogging, cfg.EnableLogging)
	out.LogLevel = validateLogLevel(resolveString("LOG_LEVEL", settings.LogLevel, cfg.LogLevel))
	out.MaxLoginAttempts = resolveInt("MAX_LOGIN_ATTEMPTS", settings.MaxLoginAttempts, cfg.MaxLoginAttempts)
	out.LockoutDurationMin = resolveInt("LOCKOUT_DURATION_MINUTES", settings.LockoutDurationMinutes, cfg.LockoutDurationMin)
	out.EnableHSTS = resolveBool("ENABLE_HSTS", settings.EnableHSTS, cfg.EnableHSTS)
	out.TrustProxy = resolveBool("TRUST_PROXY", settings.TrustProxy, cfg.TrustProxy)
	out.RateLimitWindowMs = resolveInt("RATE_LIMIT_WINDOW_MS", settings.RateLimitWindowMs, cfg.RateLimitWindowMs)
	out.RateLimitMax = resolveInt("RATE_LIMIT_MAX", settings.RateLimitMax, cfg.RateLimitMax)
	out.AuthRateLimitWindowMs = resolveInt("AUTH_RATE_LIMIT_WINDOW_MS", settings.AuthRateLimitWindowMs, cfg.AuthRateLimitWindowMs)
	out.AuthRateLimitMax = resolveInt("AUTH_RATE_LIMIT_MAX", settings.AuthRateLimitMax, cfg.AuthRateLimitMax)
	out.AgentRateLimitWindowMs = resolveInt("AGENT_RATE_LIMIT_WINDOW_MS", settings.AgentRateLimitWindowMs, cfg.AgentRateLimitWindowMs)
	out.AgentRateLimitMax = resolveInt("AGENT_RATE_LIMIT_MAX", settings.AgentRateLimitMax, cfg.AgentRateLimitMax)
	out.PasswordRateLimitWindowMs = resolveInt("PASSWORD_RATE_LIMIT_WINDOW_MS", settings.PasswordRateLimitWindowMs, cfg.PasswordRateLimitWindowMs)
	out.PasswordRateLimitMax = resolveInt("PASSWORD_RATE_LIMIT_MAX", settings.PasswordRateLimitMax, cfg.PasswordRateLimitMax)
	out.PasswordMinLength = resolveInt("PASSWORD_MIN_LENGTH", settings.PasswordMinLength, cfg.PasswordMinLength)
	out.PasswordRequireUppercase = resolveBool("PASSWORD_REQUIRE_UPPERCASE", settings.PasswordRequireUppercase, cfg.PasswordRequireUppercase)
	out.PasswordRequireLowercase = resolveBool("PASSWORD_REQUIRE_LOWERCASE", settings.PasswordRequireLowercase, cfg.PasswordRequireLowercase)
	out.PasswordRequireNumber = resolveBool("PASSWORD_REQUIRE_NUMBER", settings.PasswordRequireNumber, cfg.PasswordRequireNumber)
	out.PasswordRequireSpecial = resolveBool("PASSWORD_REQUIRE_SPECIAL", settings.PasswordRequireSpecial, cfg.PasswordRequireSpecial)
	out.JSONBodyLimitBytes = resolveBodyLimit("JSON_BODY_LIMIT", settings.JSONBodyLimit, cfg.JSONBodyLimitBytes)
	out.AgentUpdateBodyLimitBytes = resolveBodyLimit("AGENT_UPDATE_BODY_LIMIT", settings.AgentUpdateBodyLimit, cfg.AgentUpdateBodyLimitBytes)
	// Agent ping body limit is env / default only — there is no DB-settings
	// row for it yet. Default 8 KiB. Operators with extremely chatty pings
	// (custom integrations) can raise via env.
	out.AgentPingBodyLimitBytes = cfg.AgentPingBodyLimitBytes
	out.Timezone = resolveTimezone(settings.Timezone, cfg.Timezone)
	out.DefaultUserRole = resolveString("DEFAULT_USER_ROLE", strPtr(settings.DefaultUserRole), cfg.DefaultUserRole)
	out.SessionInactivityTimeoutMin = resolveInt("SESSION_INACTIVITY_TIMEOUT_MINUTES", settings.SessionInactivityTimeoutMinutes, cfg.SessionInactivityTimeoutMin)
	// Clamp to the same minimum the env-loader enforces (see config.Load).
	// A sub-5-minute stall window kills patch runs that are still
	// legitimately starting on slow hosts.
	out.PatchRunStallTimeoutMin = clampPatchRunStall(resolveInt("PATCH_RUN_STALL_TIMEOUT_MIN", settings.PatchRunStallTimeoutMinutes, cfg.PatchRunStallTimeoutMin))
	out.TfaMaxRememberSessions = resolveInt("TFA_MAX_REMEMBER_SESSIONS", settings.TfaMaxRememberSessions, cfg.TfaMaxRememberSessions)
	out.DBTransactionLongTimeout = resolveInt("DB_TRANSACTION_LONG_TIMEOUT", settings.DBTransactionLongTimeout, cfg.DBTransactionLongTimeout)
	out.JwtExpiresIn = resolveString("JWT_EXPIRES_IN", settings.JwtExpiresIn, cfg.JWTExpiresIn)
	out.AuthBrowserSessionCookies = resolveBool("AUTH_BROWSER_SESSION_COOKIES", settings.AuthBrowserSessionCookies, cfg.AuthBrowserSessionCookies)
	out.MaxTfaAttempts = resolveInt("MAX_TFA_ATTEMPTS", settings.MaxTfaAttempts, cfg.MaxTfaAttempts)
	out.TfaLockoutDurationMin = resolveInt("TFA_LOCKOUT_DURATION_MINUTES", settings.TfaLockoutDurationMinutes, cfg.TfaLockoutDurationMin)
	out.TfaRememberMeExpiresIn = resolveString("TFA_REMEMBER_ME_EXPIRES_IN", settings.TfaRememberMeExpiresIn, cfg.TfaRememberMeExpiresIn)
	return out
}

func resolveFromEnvAndDefaults(cfg *Config) *ResolvedConfig {
	return &ResolvedConfig{
		CORSOrigin:                  cfg.CORSOrigin,
		EnableLogging:               cfg.EnableLogging,
		LogLevel:                    validateLogLevel(cfg.LogLevel),
		MaxLoginAttempts:            cfg.MaxLoginAttempts,
		LockoutDurationMin:          cfg.LockoutDurationMin,
		EnableHSTS:                  cfg.EnableHSTS,
		TrustProxy:                  cfg.TrustProxy,
		RateLimitWindowMs:           cfg.RateLimitWindowMs,
		RateLimitMax:                cfg.RateLimitMax,
		AuthRateLimitWindowMs:       cfg.AuthRateLimitWindowMs,
		AuthRateLimitMax:            cfg.AuthRateLimitMax,
		AgentRateLimitWindowMs:      cfg.AgentRateLimitWindowMs,
		AgentRateLimitMax:           cfg.AgentRateLimitMax,
		PasswordRateLimitWindowMs:   cfg.PasswordRateLimitWindowMs,
		PasswordRateLimitMax:        cfg.PasswordRateLimitMax,
		PasswordMinLength:           cfg.PasswordMinLength,
		PasswordRequireUppercase:    cfg.PasswordRequireUppercase,
		PasswordRequireLowercase:    cfg.PasswordRequireLowercase,
		PasswordRequireNumber:       cfg.PasswordRequireNumber,
		PasswordRequireSpecial:      cfg.PasswordRequireSpecial,
		JSONBodyLimitBytes:          cfg.JSONBodyLimitBytes,
		AgentUpdateBodyLimitBytes:   cfg.AgentUpdateBodyLimitBytes,
		AgentPingBodyLimitBytes:     cfg.AgentPingBodyLimitBytes,
		Timezone:                    validateTimezone(cfg.Timezone),
		DefaultUserRole:             cfg.DefaultUserRole,
		SessionInactivityTimeoutMin: cfg.SessionInactivityTimeoutMin,
		PatchRunStallTimeoutMin:     clampPatchRunStall(cfg.PatchRunStallTimeoutMin),
		TfaMaxRememberSessions:      cfg.TfaMaxRememberSessions,
		DBTransactionLongTimeout:    cfg.DBTransactionLongTimeout,
		JwtExpiresIn:                cfg.JWTExpiresIn,
		AuthBrowserSessionCookies:   cfg.AuthBrowserSessionCookies,
		MaxTfaAttempts:              cfg.MaxTfaAttempts,
		TfaLockoutDurationMin:       cfg.TfaLockoutDurationMin,
		TfaRememberMeExpiresIn:      cfg.TfaRememberMeExpiresIn,
	}
}

// clampPatchRunStall enforces the same 5-minute floor as the env loader.
// Used at every resolve call so DB-edited values can't shrink below the
// safe minimum even after restart.
func clampPatchRunStall(v int) int {
	if v < 5 {
		return 5
	}
	return v
}

func resolveInt(envKey string, dbVal *int, defaultVal int) int {
	if v := os.Getenv(envKey); v != "" {
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i
		}
	}
	if dbVal != nil {
		return *dbVal
	}
	return defaultVal
}

func resolveBool(envKey string, dbVal *bool, defaultVal bool) bool {
	if v := os.Getenv(envKey); v != "" {
		return strings.ToLower(strings.TrimSpace(v)) == "true" || v == "1"
	}
	if dbVal != nil {
		return *dbVal
	}
	return defaultVal
}

func resolveString(envKey string, dbVal *string, defaultVal string) string {
	if v := os.Getenv(envKey); v != "" {
		return strings.TrimSpace(v)
	}
	if dbVal != nil && strings.TrimSpace(*dbVal) != "" {
		return strings.TrimSpace(*dbVal)
	}
	return defaultVal
}

func resolveBodyLimit(envKey string, dbVal *string, defaultBytes int64) int64 {
	if v := os.Getenv(envKey); v != "" {
		return parseBodyLimit(strings.TrimSpace(v), defaultBytes)
	}
	if dbVal != nil && *dbVal != "" {
		return parseBodyLimit(*dbVal, defaultBytes)
	}
	return defaultBytes
}

func parseBodyLimit(s string, fallback int64) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return fallback
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
		return fallback
	}
	return v * mult
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// validateLogLevel ensures the level is one of debug, info, warn, error. Falls back to "info" if invalid.
func validateLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "info", "warn", "error":
		return strings.ToLower(strings.TrimSpace(level))
	default:
		return "info"
	}
}

// ResolveTimezone resolves timezone from TZ env, TIMEZONE env, DB setting, or config default.
// Exported so handlers can re-resolve per-request for multi-context isolation.
func ResolveTimezone(dbVal *string, cfg *Config) string {
	defaultVal := "UTC"
	if cfg != nil {
		defaultVal = cfg.Timezone
	}
	return resolveTimezone(dbVal, defaultVal)
}

// resolveTimezone resolves from TZ env, TIMEZONE env, DB, or default. Validates via time.LoadLocation.
func resolveTimezone(dbVal *string, defaultVal string) string {
	if v := os.Getenv("TZ"); v != "" {
		return validateTimezone(strings.TrimSpace(v))
	}
	if v := os.Getenv("TIMEZONE"); v != "" {
		return validateTimezone(strings.TrimSpace(v))
	}
	if dbVal != nil && *dbVal != "" {
		return validateTimezone(*dbVal)
	}
	return validateTimezone(defaultVal)
}

// validateTimezone ensures the timezone is valid via time.LoadLocation. Falls back to "UTC" if invalid.
func validateTimezone(tz string) string {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		slog.Warn("validateTimezone: invalid timezone, falling back to UTC", "tz", tz, "error", err)
		return "UTC"
	}
	return tz
}
