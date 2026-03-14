package models

import "time"

// User matches users table.
type User struct {
	ID              string     `db:"id"`
	Username        string     `db:"username"`
	Email           string     `db:"email"`
	PasswordHash    *string    `db:"password_hash"`
	Role            string     `db:"role"`
	IsActive        bool       `db:"is_active"`
	LastLogin       *time.Time `db:"last_login"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
	TfaBackupCodes  *string    `db:"tfa_backup_codes"`
	TfaEnabled      bool       `db:"tfa_enabled"`
	TfaSecret       *string    `db:"tfa_secret"`
	FirstName       *string    `db:"first_name"`
	LastName        *string    `db:"last_name"`
	ThemePreference *string    `db:"theme_preference"`
	ColorTheme      *string    `db:"color_theme"`
	UIPreferences   JSON       `db:"ui_preferences"`
	OidcSub         *string    `db:"oidc_sub"`
	OidcProvider    *string    `db:"oidc_provider"`
	AvatarURL       *string    `db:"avatar_url"`
	DiscordID       *string    `db:"discord_id"`
	DiscordUsername *string    `db:"discord_username"`
	DiscordAvatar   *string    `db:"discord_avatar"`
	DiscordLinkedAt *time.Time `db:"discord_linked_at"`
}

// UserSession matches user_sessions table.
type UserSession struct {
	ID                string     `db:"id"`
	UserID            string     `db:"user_id"`
	RefreshToken      string     `db:"refresh_token"`
	AccessTokenHash   *string    `db:"access_token_hash"`
	IPAddress         *string    `db:"ip_address"`
	UserAgent         *string    `db:"user_agent"`
	DeviceFingerprint *string    `db:"device_fingerprint"`
	LastActivity      time.Time  `db:"last_activity"`
	ExpiresAt         time.Time  `db:"expires_at"`
	CreatedAt         time.Time  `db:"created_at"`
	IsRevoked         bool       `db:"is_revoked"`
	TfaRememberMe     bool       `db:"tfa_remember_me"`
	TfaBypassUntil    *time.Time `db:"tfa_bypass_until"`
	LoginCount        int        `db:"login_count"`
	LastLoginIP       *string    `db:"last_login_ip"`
}

// RolePermission matches role_permissions table.
type RolePermission struct {
	ID                  string    `db:"id"`
	Role                string    `db:"role"`
	CanViewDashboard    bool      `db:"can_view_dashboard"`
	CanViewHosts        bool      `db:"can_view_hosts"`
	CanManageHosts      bool      `db:"can_manage_hosts"`
	CanViewPackages     bool      `db:"can_view_packages"`
	CanManagePackages   bool      `db:"can_manage_packages"`
	CanViewUsers        bool      `db:"can_view_users"`
	CanManageUsers      bool      `db:"can_manage_users"`
	CanManageSuperusers bool      `db:"can_manage_superusers"`
	CanViewReports      bool      `db:"can_view_reports"`
	CanExportData       bool      `db:"can_export_data"`
	CanManageSettings   bool      `db:"can_manage_settings"`
	CreatedAt           time.Time `db:"created_at"`
	UpdatedAt           time.Time `db:"updated_at"`
}

// Settings matches settings table.
type Settings struct {
	ID                              string     `db:"id"`
	ServerURL                       string     `db:"server_url"`
	ServerProtocol                  string     `db:"server_protocol"`
	ServerHost                      string     `db:"server_host"`
	ServerPort                      int        `db:"server_port"`
	CreatedAt                       time.Time  `db:"created_at"`
	UpdatedAt                       time.Time  `db:"updated_at"`
	UpdateInterval                  int        `db:"update_interval"`
	AutoUpdate                      bool       `db:"auto_update"`
	DefaultComplianceMode           string     `db:"default_compliance_mode"`
	GithubRepoURL                   string     `db:"github_repo_url"`
	SSHKeyPath                      *string    `db:"ssh_key_path"`
	RepositoryType                  string     `db:"repository_type"`
	LastUpdateCheck                 *time.Time `db:"last_update_check"`
	LatestVersion                   *string    `db:"latest_version"`
	UpdateAvailable                 bool       `db:"update_available"`
	SignupEnabled                   bool       `db:"signup_enabled"`
	DefaultUserRole                 string     `db:"default_user_role"`
	IgnoreSSLSelfSigned             bool       `db:"ignore_ssl_self_signed"`
	LogoDark                        *string    `db:"logo_dark"`
	LogoLight                       *string    `db:"logo_light"`
	Favicon                         *string    `db:"favicon"`
	LogoDarkData                    []byte     `db:"logo_dark_data"`
	LogoLightData                   []byte     `db:"logo_light_data"`
	FaviconData                     []byte     `db:"favicon_data"`
	LogoDarkContentType             *string    `db:"logo_dark_content_type"`
	LogoLightContentType            *string    `db:"logo_light_content_type"`
	FaviconContentType              *string    `db:"favicon_content_type"`
	MetricsEnabled                  bool       `db:"metrics_enabled"`
	MetricsAnonymousID              *string    `db:"metrics_anonymous_id"`
	MetricsLastSent                 *time.Time `db:"metrics_last_sent"`
	ShowGithubVersionOnLogin        bool       `db:"show_github_version_on_login"`
	AiEnabled                       bool       `db:"ai_enabled"`
	AiProvider                      string     `db:"ai_provider"`
	AiModel                         *string    `db:"ai_model"`
	AiAPIKey                        *string    `db:"ai_api_key"`
	AlertsEnabled                   bool       `db:"alerts_enabled"`
	DiscordOAuthEnabled             bool       `db:"discord_oauth_enabled"`
	DiscordClientID                 *string    `db:"discord_client_id"`
	DiscordClientSecret             *string    `db:"discord_client_secret"`
	DiscordRedirectURI              *string    `db:"discord_redirect_uri"`
	DiscordButtonText               *string    `db:"discord_button_text"`
	OidcEnabled                     bool       `db:"oidc_enabled"`
	OidcIssuerURL                   *string    `db:"oidc_issuer_url"`
	OidcClientID                    *string    `db:"oidc_client_id"`
	OidcClientSecret                *string    `db:"oidc_client_secret"`
	OidcRedirectURI                 *string    `db:"oidc_redirect_uri"`
	OidcScopes                      *string    `db:"oidc_scopes"`
	OidcAutoCreateUsers             bool       `db:"oidc_auto_create_users"`
	OidcDefaultRole                 *string    `db:"oidc_default_role"`
	OidcDisableLocalAuth            bool       `db:"oidc_disable_local_auth"`
	OidcButtonText                  *string    `db:"oidc_button_text"`
	OidcSyncRoles                   bool       `db:"oidc_sync_roles"`
	OidcAdminGroup                  *string    `db:"oidc_admin_group"`
	OidcSuperadminGroup             *string    `db:"oidc_superadmin_group"`
	OidcHostManagerGroup            *string    `db:"oidc_host_manager_group"`
	OidcReadonlyGroup               *string    `db:"oidc_readonly_group"`
	OidcUserGroup                   *string    `db:"oidc_user_group"`
	OidcEnforceHTTPS                bool       `db:"oidc_enforce_https"`
	MaxLoginAttempts                *int       `db:"max_login_attempts"`
	LockoutDurationMinutes          *int       `db:"lockout_duration_minutes"`
	SessionInactivityTimeoutMinutes *int       `db:"session_inactivity_timeout_minutes"`
	TfaMaxRememberSessions          *int       `db:"tfa_max_remember_sessions"`
	PasswordMinLength               *int       `db:"password_min_length"`
	PasswordRequireUppercase        *bool      `db:"password_require_uppercase"`
	PasswordRequireLowercase        *bool      `db:"password_require_lowercase"`
	PasswordRequireNumber           *bool      `db:"password_require_number"`
	PasswordRequireSpecial          *bool      `db:"password_require_special"`
	EnableHSTS                      *bool      `db:"enable_hsts"`
	JSONBodyLimit                   *string    `db:"json_body_limit"`
	AgentUpdateBodyLimit            *string    `db:"agent_update_body_limit"`
	DBTransactionLongTimeout        *int       `db:"db_transaction_long_timeout"`
	CorsOrigin                      *string    `db:"cors_origin"`
	EnableLogging                   *bool      `db:"enable_logging"`
	LogLevel                        *string    `db:"log_level"`
	Timezone                        *string    `db:"timezone"`
	JwtExpiresIn                    *string    `db:"jwt_expires_in"`
	MaxTfaAttempts                  *int       `db:"max_tfa_attempts"`
	TfaLockoutDurationMinutes       *int       `db:"tfa_lockout_duration_minutes"`
	TfaRememberMeExpiresIn          *string    `db:"tfa_remember_me_expires_in"`
	TrustProxy                      *bool      `db:"trust_proxy"`
	RateLimitWindowMs               *int       `db:"rate_limit_window_ms"`
	RateLimitMax                    *int       `db:"rate_limit_max"`
	AuthRateLimitWindowMs           *int       `db:"auth_rate_limit_window_ms"`
	AuthRateLimitMax                *int       `db:"auth_rate_limit_max"`
	AgentRateLimitWindowMs          *int       `db:"agent_rate_limit_window_ms"`
	AgentRateLimitMax               *int       `db:"agent_rate_limit_max"`
	PasswordRateLimitWindowMs       *int       `db:"password_rate_limit_window_ms"`
	PasswordRateLimitMax            *int       `db:"password_rate_limit_max"`
}
