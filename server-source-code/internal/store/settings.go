package store

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

// SettingsStore provides settings access.
type SettingsStore struct {
	db database.DBProvider
}

// NewSettingsStore creates a new settings store.
func NewSettingsStore(db database.DBProvider) *SettingsStore {
	return &SettingsStore{db: db}
}

// GetFirst returns the first (and typically only) settings row.
func (s *SettingsStore) GetFirst(ctx context.Context) (*models.Settings, error) {
	d := s.db.DB(ctx)
	setting, err := d.Queries.GetFirstSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := dbSettingToModel(setting)
	return &out, nil
}

// Update updates settings by ID.
func (s *SettingsStore) Update(ctx context.Context, settings *models.Settings) error {
	d := s.db.DB(ctx)
	arg := settingsToUpdateParams(settings)
	return d.Queries.UpdateSettings(ctx, arg)
}

// UpdateConfigKey updates a single config key. Key must be one of the DB-backed config keys.
// current is used to preserve values for fields not being updated (e.g. default_user_role).
func (s *SettingsStore) UpdateConfigKey(ctx context.Context, settingsID, key string, value interface{}, current *models.Settings) error {
	d := s.db.DB(ctx)
	arg := db.UpdateSettingsConfigParams{ID: settingsID}
	// Preserve default_user_role when not updating it (COALESCE needs a value when param is non-nullable)
	if key != "DEFAULT_USER_ROLE" {
		if current != nil {
			arg.DefaultUserRole = current.DefaultUserRole
		} else {
			arg.DefaultUserRole = "user"
		}
	}
	switch key {
	case "MAX_LOGIN_ATTEMPTS":
		if v, ok := toInt32(value); ok {
			arg.MaxLoginAttempts = &v
		}
	case "LOCKOUT_DURATION_MINUTES":
		if v, ok := toInt32(value); ok {
			arg.LockoutDurationMinutes = &v
		}
	case "SESSION_INACTIVITY_TIMEOUT_MINUTES":
		if v, ok := toInt32(value); ok {
			arg.SessionInactivityTimeoutMinutes = &v
		}
	case "TFA_MAX_REMEMBER_SESSIONS":
		if v, ok := toInt32(value); ok {
			arg.TfaMaxRememberSessions = &v
		}
	case "PASSWORD_MIN_LENGTH":
		if v, ok := toInt32(value); ok {
			arg.PasswordMinLength = &v
		}
	case "PASSWORD_REQUIRE_UPPERCASE":
		if v, ok := toBool(value); ok {
			arg.PasswordRequireUppercase = &v
		}
	case "PASSWORD_REQUIRE_LOWERCASE":
		if v, ok := toBool(value); ok {
			arg.PasswordRequireLowercase = &v
		}
	case "PASSWORD_REQUIRE_NUMBER":
		if v, ok := toBool(value); ok {
			arg.PasswordRequireNumber = &v
		}
	case "PASSWORD_REQUIRE_SPECIAL":
		if v, ok := toBool(value); ok {
			arg.PasswordRequireSpecial = &v
		}
	case "ENABLE_HSTS":
		if v, ok := toBool(value); ok {
			arg.EnableHsts = &v
		}
	case "JSON_BODY_LIMIT":
		if v, ok := toStr(value); ok {
			arg.JsonBodyLimit = &v
		}
	case "AGENT_UPDATE_BODY_LIMIT":
		if v, ok := toStr(value); ok {
			arg.AgentUpdateBodyLimit = &v
		}
	case "DB_TRANSACTION_LONG_TIMEOUT":
		if v, ok := toInt32(value); ok {
			arg.DbTransactionLongTimeout = &v
		}
	case "CORS_ORIGIN":
		v, ok := toStr(value)
		if !ok {
			return errors.New("CORS_ORIGIN requires a string value")
		}
		arg.CorsOrigin = &v
	case "ENABLE_LOGGING":
		if v, ok := toBool(value); ok {
			arg.EnableLogging = &v
		}
	case "LOG_LEVEL":
		v, ok := toStr(value)
		if !ok {
			return errors.New("LOG_LEVEL requires a string value")
		}
		arg.LogLevel = &v
	case "TIMEZONE":
		v, ok := toStr(value)
		if !ok {
			return errors.New("TIMEZONE requires a string value")
		}
		arg.Timezone = &v
	case "JWT_EXPIRES_IN":
		v, ok := toStr(value)
		if !ok {
			return errors.New("JWT_EXPIRES_IN requires a string value (e.g. 1h, 30m)")
		}
		arg.JwtExpiresIn = &v
	case "AUTH_BROWSER_SESSION_COOKIES":
		if v, ok := toBool(value); ok {
			arg.AuthBrowserSessionCookies = &v
		}
	case "MAX_TFA_ATTEMPTS":
		if v, ok := toInt32(value); ok {
			arg.MaxTfaAttempts = &v
		}
	case "TFA_LOCKOUT_DURATION_MINUTES":
		if v, ok := toInt32(value); ok {
			arg.TfaLockoutDurationMinutes = &v
		}
	case "TFA_REMEMBER_ME_EXPIRES_IN":
		v, ok := toStr(value)
		if !ok {
			return errors.New("TFA_REMEMBER_ME_EXPIRES_IN requires a string value (e.g. 30d, 7d)")
		}
		arg.TfaRememberMeExpiresIn = &v
	case "DEFAULT_USER_ROLE":
		v, ok := toStr(value)
		if !ok {
			return errors.New("DEFAULT_USER_ROLE requires a string value")
		}
		arg.DefaultUserRole = v
	case "TRUST_PROXY":
		if v, ok := toBool(value); ok {
			arg.TrustProxy = &v
		}
	case "RATE_LIMIT_WINDOW_MS":
		if v, ok := toInt32(value); ok {
			arg.RateLimitWindowMs = &v
		}
	case "RATE_LIMIT_MAX":
		if v, ok := toInt32(value); ok {
			arg.RateLimitMax = &v
		}
	case "AUTH_RATE_LIMIT_WINDOW_MS":
		if v, ok := toInt32(value); ok {
			arg.AuthRateLimitWindowMs = &v
		}
	case "AUTH_RATE_LIMIT_MAX":
		if v, ok := toInt32(value); ok {
			arg.AuthRateLimitMax = &v
		}
	case "AGENT_RATE_LIMIT_WINDOW_MS":
		if v, ok := toInt32(value); ok {
			arg.AgentRateLimitWindowMs = &v
		}
	case "AGENT_RATE_LIMIT_MAX":
		if v, ok := toInt32(value); ok {
			arg.AgentRateLimitMax = &v
		}
	case "PASSWORD_RATE_LIMIT_WINDOW_MS":
		if v, ok := toInt32(value); ok {
			arg.PasswordRateLimitWindowMs = &v
		}
	case "PASSWORD_RATE_LIMIT_MAX":
		if v, ok := toInt32(value); ok {
			arg.PasswordRateLimitMax = &v
		}
	default:
		return errors.New("unknown config key")
	}
	return d.Queries.UpdateSettingsConfig(ctx, arg)
}

func toInt32(v interface{}) (int32, bool) {
	switch x := v.(type) {
	case int:
		return int32(x), true
	case int64:
		return int32(x), true
	case float64:
		return int32(x), true
	case string:
		n, err := strconv.ParseInt(x, 10, 32)
		if err != nil {
			return 0, false
		}
		return int32(n), true
	}
	return 0, false
}

func toBool(v interface{}) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		return strings.EqualFold(x, "true") || x == "1", true
	}
	return false, false
}

func toStr(v interface{}) (string, bool) {
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}
