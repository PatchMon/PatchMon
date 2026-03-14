// Package config provides OIDC config resolution (env > DB > disabled).
package config

import (
	"context"
	"os"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

// ResolvedOidcConfig holds the effective OIDC configuration after merging env and DB.
// Env takes priority over DB for each field.
type ResolvedOidcConfig struct {
	Enabled          bool
	IssuerURL        string
	ClientID         string
	ClientSecret     string
	RedirectURI      string
	Scopes           string
	AutoCreateUsers  bool
	DefaultRole      string
	DisableLocalAuth bool
	ButtonText       string
	SyncRoles        bool
	AdminGroup       string
	SuperadminGroup  string
	HostManagerGroup string
	ReadonlyGroup    string
	UserGroup        string
	EnforceHTTPS     bool
	// ConfiguredViaEnv is true when any OIDC_* env var is set (for import banner).
	ConfiguredViaEnv bool
}

// Resolver resolves OIDC config from env and DB. Env takes priority.
type Resolver struct {
	cfg      *Config
	settings *models.Settings
}

// NewOidcResolver creates a resolver that uses env first, then the given settings.
func NewOidcResolver(cfg *Config, settings *models.Settings) *Resolver {
	return &Resolver{cfg: cfg, settings: settings}
}

// Resolve returns the effective OIDC config. Caller must pass non-nil settings (from DB).
// Env takes priority: when any OIDC_* env var is set, env values are used; otherwise DB.
func (r *Resolver) Resolve() ResolvedOidcConfig {
	out := ResolvedOidcConfig{
		ConfiguredViaEnv: r.configuredViaEnv(),
	}
	useEnv := out.ConfiguredViaEnv
	if useEnv {
		out.IssuerURL = strings.TrimSpace(r.cfg.OidcIssuerURL)
		out.ClientID = strings.TrimSpace(r.cfg.OidcClientID)
		out.ClientSecret = strings.TrimSpace(r.cfg.OidcClientSecret)
		redirectURI := strings.TrimSpace(r.cfg.OidcRedirectURI)
		if redirectURI == "" {
			base := strings.TrimSuffix(r.cfg.CORSOrigin, "/")
			if r.settings != nil && strings.TrimSpace(r.settings.ServerURL) != "" {
				base = strings.TrimSuffix(strings.TrimSpace(r.settings.ServerURL), "/")
			}
			redirectURI = base + "/api/v1/auth/oidc/callback"
		}
		out.RedirectURI = redirectURI
		out.Scopes = r.pickString(r.cfg.OidcScopes, "openid email profile groups")
		out.Enabled = r.cfg.OidcEnabled
		out.ButtonText = r.pickString(r.cfg.OidcButtonText, "Login with SSO")
		out.AutoCreateUsers = r.cfg.OidcAutoCreateUsers
		out.DefaultRole = r.pickString(r.cfg.OidcDefaultRole, "user")
		out.DisableLocalAuth = r.cfg.OidcDisableLocalAuth
		out.SyncRoles = r.cfg.OidcSyncRoles
		out.AdminGroup = strings.TrimSpace(r.cfg.OidcAdminGroup)
		out.SuperadminGroup = strings.TrimSpace(r.cfg.OidcSuperadminGroup)
		out.HostManagerGroup = strings.TrimSpace(r.cfg.OidcHostManagerGroup)
		out.ReadonlyGroup = strings.TrimSpace(r.cfg.OidcReadonlyGroup)
		out.UserGroup = strings.TrimSpace(r.cfg.OidcUserGroup)
		out.EnforceHTTPS = r.cfg.OidcEnforceHTTPS
	} else {
		out.IssuerURL = r.ptrStr(r.settings.OidcIssuerURL)
		out.ClientID = r.ptrStr(r.settings.OidcClientID)
		out.ClientSecret = r.ptrStr(r.settings.OidcClientSecret)
		redirectURI := r.ptrStr(r.settings.OidcRedirectURI)
		if redirectURI == "" {
			base := strings.TrimSuffix(r.cfg.CORSOrigin, "/")
			if strings.TrimSpace(r.settings.ServerURL) != "" {
				base = strings.TrimSuffix(strings.TrimSpace(r.settings.ServerURL), "/")
			}
			redirectURI = base + "/api/v1/auth/oidc/callback"
		}
		out.RedirectURI = redirectURI
		out.Scopes = r.pickString(r.ptrStr(r.settings.OidcScopes), "openid email profile groups")
		out.Enabled = r.settings.OidcEnabled
		out.ButtonText = r.pickString(r.ptrStr(r.settings.OidcButtonText), "Login with SSO")
		out.AutoCreateUsers = r.settings.OidcAutoCreateUsers
		out.DefaultRole = r.pickString(r.ptrStr(r.settings.OidcDefaultRole), "user")
		out.DisableLocalAuth = r.settings.OidcDisableLocalAuth
		out.SyncRoles = r.settings.OidcSyncRoles
		out.AdminGroup = r.ptrStr(r.settings.OidcAdminGroup)
		out.SuperadminGroup = r.ptrStr(r.settings.OidcSuperadminGroup)
		out.HostManagerGroup = r.ptrStr(r.settings.OidcHostManagerGroup)
		out.ReadonlyGroup = r.ptrStr(r.settings.OidcReadonlyGroup)
		out.UserGroup = r.ptrStr(r.settings.OidcUserGroup)
		out.EnforceHTTPS = r.settings.OidcEnforceHTTPS
	}
	return out
}

func (r *Resolver) configuredViaEnv() bool {
	return configuredViaEnvStrict()
}

func (r *Resolver) ptrStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func (r *Resolver) pickString(val, def string) string {
	if strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return def
}

// EnvPreview returns non-secret env values that are explicitly set. Never includes client_secret.
// Uses os.Getenv so only actual .env entries are shown, not config defaults.
func EnvPreview(cfg *Config) map[string]string {
	m := make(map[string]string)
	if v := strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL")); v != "" {
		m["oidc_issuer_url"] = v
	}
	if v := strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID")); v != "" {
		m["oidc_client_id"] = v
	}
	if v := strings.TrimSpace(os.Getenv("OIDC_REDIRECT_URI")); v != "" {
		m["oidc_redirect_uri"] = v
	}
	if v := strings.TrimSpace(os.Getenv("OIDC_SCOPES")); v != "" {
		m["oidc_scopes"] = v
	}
	return m
}

// configuredViaEnvStrict returns true when any OIDC_* env var is explicitly set.
// Uses os.Getenv directly to avoid false positives from config defaults
// (e.g. OIDC_SCOPES defaults to "openid email profile groups" when unset).
func configuredViaEnvStrict() bool {
	return os.Getenv("OIDC_ISSUER_URL") != "" ||
		os.Getenv("OIDC_CLIENT_ID") != "" ||
		os.Getenv("OIDC_CLIENT_SECRET") != "" ||
		os.Getenv("OIDC_REDIRECT_URI") != "" ||
		os.Getenv("OIDC_SCOPES") != "" ||
		os.Getenv("OIDC_ENABLED") == "true"
}

// ConfiguredViaEnv returns true when any OIDC_* env var is explicitly set in the environment.
func ConfiguredViaEnv(cfg *Config) bool {
	return configuredViaEnvStrict()
}

// ResolveOidcConfig resolves effective OIDC config from env and DB. Used at startup.
func ResolveOidcConfig(ctx context.Context, cfg *Config, getSettings func(context.Context) (*models.Settings, error)) (ResolvedOidcConfig, error) {
	s, err := getSettings(ctx)
	if err != nil || s == nil {
		// No DB settings; use env only
		res := NewOidcResolver(cfg, &models.Settings{})
		return res.Resolve(), nil
	}
	res := NewOidcResolver(cfg, s)
	return res.Resolve(), nil
}
