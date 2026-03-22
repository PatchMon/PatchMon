package store

import (
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/jackc/pgx/v5/pgtype"
)

func dbRolePermissionToModel(r db.RolePermission) models.RolePermission {
	return models.RolePermission{
		ID:                      r.ID,
		Role:                    r.Role,
		CanViewDashboard:        r.CanViewDashboard,
		CanViewHosts:            r.CanViewHosts,
		CanManageHosts:          r.CanManageHosts,
		CanViewPackages:         r.CanViewPackages,
		CanManagePackages:       r.CanManagePackages,
		CanViewUsers:            r.CanViewUsers,
		CanManageUsers:          r.CanManageUsers,
		CanManageSuperusers:     r.CanManageSuperusers,
		CanViewReports:          r.CanViewReports,
		CanExportData:           r.CanExportData,
		CanManageSettings:       r.CanManageSettings,
		CanManageNotifications:  r.CanManageNotifications,
		CanViewNotificationLogs: r.CanViewNotificationLogs,
		CreatedAt:               pgTime(r.CreatedAt),
		UpdatedAt:               pgTime(r.UpdatedAt),
	}
}

func dbSettingToModel(s db.Setting) models.Settings {
	return models.Settings{
		ID:                              s.ID,
		ServerURL:                       s.ServerUrl,
		ServerProtocol:                  s.ServerProtocol,
		ServerHost:                      s.ServerHost,
		ServerPort:                      int(s.ServerPort),
		CreatedAt:                       pgTime(s.CreatedAt),
		UpdatedAt:                       pgTime(s.UpdatedAt),
		UpdateInterval:                  int(s.UpdateInterval),
		AutoUpdate:                      s.AutoUpdate,
		DefaultComplianceMode:           s.DefaultComplianceMode,
		ComplianceScanInterval:          int(s.ComplianceScanInterval),
		GithubRepoURL:                   s.GithubRepoUrl,
		SSHKeyPath:                      s.SshKeyPath,
		RepositoryType:                  s.RepositoryType,
		LastUpdateCheck:                 pgTimePtr(s.LastUpdateCheck),
		LatestVersion:                   s.LatestVersion,
		UpdateAvailable:                 s.UpdateAvailable,
		SignupEnabled:                   s.SignupEnabled,
		DefaultUserRole:                 s.DefaultUserRole,
		IgnoreSSLSelfSigned:             s.IgnoreSslSelfSigned,
		LogoDark:                        s.LogoDark,
		LogoLight:                       s.LogoLight,
		Favicon:                         s.Favicon,
		LogoDarkData:                    s.LogoDarkData,
		LogoLightData:                   s.LogoLightData,
		FaviconData:                     s.FaviconData,
		LogoDarkContentType:             s.LogoDarkContentType,
		LogoLightContentType:            s.LogoLightContentType,
		FaviconContentType:              s.FaviconContentType,
		MetricsEnabled:                  s.MetricsEnabled,
		MetricsAnonymousID:              s.MetricsAnonymousID,
		MetricsLastSent:                 pgTimePtr(s.MetricsLastSent),
		ShowGithubVersionOnLogin:        s.ShowGithubVersionOnLogin,
		AiEnabled:                       s.AiEnabled,
		AiProvider:                      s.AiProvider,
		AiModel:                         s.AiModel,
		AiAPIKey:                        s.AiApiKey,
		AlertsEnabled:                   s.AlertsEnabled,
		DiscordOAuthEnabled:             s.DiscordOauthEnabled,
		DiscordClientID:                 s.DiscordClientID,
		DiscordClientSecret:             s.DiscordClientSecret,
		DiscordRedirectURI:              s.DiscordRedirectUri,
		DiscordButtonText:               s.DiscordButtonText,
		OidcEnabled:                     s.OidcEnabled,
		OidcIssuerURL:                   s.OidcIssuerUrl,
		OidcClientID:                    s.OidcClientID,
		OidcClientSecret:                s.OidcClientSecret,
		OidcRedirectURI:                 s.OidcRedirectUri,
		OidcScopes:                      s.OidcScopes,
		OidcAutoCreateUsers:             s.OidcAutoCreateUsers,
		OidcDefaultRole:                 s.OidcDefaultRole,
		OidcDisableLocalAuth:            s.OidcDisableLocalAuth,
		OidcButtonText:                  s.OidcButtonText,
		OidcSyncRoles:                   s.OidcSyncRoles,
		OidcAdminGroup:                  s.OidcAdminGroup,
		OidcSuperadminGroup:             s.OidcSuperadminGroup,
		OidcHostManagerGroup:            s.OidcHostManagerGroup,
		OidcReadonlyGroup:               s.OidcReadonlyGroup,
		OidcUserGroup:                   s.OidcUserGroup,
		OidcEnforceHTTPS:                s.OidcEnforceHttps,
		MaxLoginAttempts:                pgInt32ToIntPtr(s.MaxLoginAttempts),
		LockoutDurationMinutes:          pgInt32ToIntPtr(s.LockoutDurationMinutes),
		SessionInactivityTimeoutMinutes: pgInt32ToIntPtr(s.SessionInactivityTimeoutMinutes),
		TfaMaxRememberSessions:          pgInt32ToIntPtr(s.TfaMaxRememberSessions),
		PasswordMinLength:               pgInt32ToIntPtr(s.PasswordMinLength),
		PasswordRequireUppercase:        s.PasswordRequireUppercase,
		PasswordRequireLowercase:        s.PasswordRequireLowercase,
		PasswordRequireNumber:           s.PasswordRequireNumber,
		PasswordRequireSpecial:          s.PasswordRequireSpecial,
		EnableHSTS:                      s.EnableHsts,
		JSONBodyLimit:                   s.JsonBodyLimit,
		AgentUpdateBodyLimit:            s.AgentUpdateBodyLimit,
		DBTransactionLongTimeout:        pgInt32ToIntPtr(s.DbTransactionLongTimeout),
		CorsOrigin:                      s.CorsOrigin,
		EnableLogging:                   s.EnableLogging,
		LogLevel:                        s.LogLevel,
		Timezone:                        s.Timezone,
		JwtExpiresIn:                    s.JwtExpiresIn,
		MaxTfaAttempts:                  pgInt32ToIntPtr(s.MaxTfaAttempts),
		TfaLockoutDurationMinutes:       pgInt32ToIntPtr(s.TfaLockoutDurationMinutes),
		TfaRememberMeExpiresIn:          s.TfaRememberMeExpiresIn,
		TrustProxy:                      s.TrustProxy,
		RateLimitWindowMs:               pgInt32ToIntPtr(s.RateLimitWindowMs),
		RateLimitMax:                    pgInt32ToIntPtr(s.RateLimitMax),
		AuthRateLimitWindowMs:           pgInt32ToIntPtr(s.AuthRateLimitWindowMs),
		AuthRateLimitMax:                pgInt32ToIntPtr(s.AuthRateLimitMax),
		AgentRateLimitWindowMs:          pgInt32ToIntPtr(s.AgentRateLimitWindowMs),
		AgentRateLimitMax:               pgInt32ToIntPtr(s.AgentRateLimitMax),
		PasswordRateLimitWindowMs:       pgInt32ToIntPtr(s.PasswordRateLimitWindowMs),
		PasswordRateLimitMax:            pgInt32ToIntPtr(s.PasswordRateLimitMax),
		AuthBrowserSessionCookies:       s.AuthBrowserSessionCookies,
	}
}

func pgInt32ToIntPtr(p *int32) *int {
	if p == nil {
		return nil
	}
	i := int(*p)
	return &i
}

func settingsToUpdateParams(s *models.Settings) db.UpdateSettingsParams {
	return db.UpdateSettingsParams{
		ServerUrl:                s.ServerURL,
		ServerProtocol:           s.ServerProtocol,
		ServerHost:               s.ServerHost,
		ServerPort:               int32(s.ServerPort),
		UpdateInterval:           int32(s.UpdateInterval),
		AutoUpdate:               s.AutoUpdate,
		DefaultComplianceMode:    s.DefaultComplianceMode,
		ComplianceScanInterval:   int32(s.ComplianceScanInterval),
		GithubRepoUrl:            s.GithubRepoURL,
		SshKeyPath:               s.SSHKeyPath,
		RepositoryType:           s.RepositoryType,
		LastUpdateCheck:          timeToPgTimestamp(s.LastUpdateCheck),
		LatestVersion:            s.LatestVersion,
		UpdateAvailable:          s.UpdateAvailable,
		SignupEnabled:            s.SignupEnabled,
		DefaultUserRole:          s.DefaultUserRole,
		IgnoreSslSelfSigned:      s.IgnoreSSLSelfSigned,
		LogoDark:                 s.LogoDark,
		LogoLight:                s.LogoLight,
		Favicon:                  s.Favicon,
		LogoDarkData:             s.LogoDarkData,
		LogoLightData:            s.LogoLightData,
		FaviconData:              s.FaviconData,
		LogoDarkContentType:      s.LogoDarkContentType,
		LogoLightContentType:     s.LogoLightContentType,
		FaviconContentType:       s.FaviconContentType,
		MetricsEnabled:           s.MetricsEnabled,
		MetricsAnonymousID:       s.MetricsAnonymousID,
		MetricsLastSent:          timeToPgTimestamp(s.MetricsLastSent),
		ShowGithubVersionOnLogin: s.ShowGithubVersionOnLogin,
		AiEnabled:                s.AiEnabled,
		AiProvider:               s.AiProvider,
		AiModel:                  s.AiModel,
		AiApiKey:                 s.AiAPIKey,
		AlertsEnabled:            s.AlertsEnabled,
		DiscordOauthEnabled:      s.DiscordOAuthEnabled,
		DiscordClientID:          s.DiscordClientID,
		DiscordClientSecret:      s.DiscordClientSecret,
		DiscordRedirectUri:       s.DiscordRedirectURI,
		DiscordButtonText:        s.DiscordButtonText,
		OidcEnabled:              s.OidcEnabled,
		OidcIssuerUrl:            s.OidcIssuerURL,
		OidcClientID:             s.OidcClientID,
		OidcClientSecret:         s.OidcClientSecret,
		OidcRedirectUri:          s.OidcRedirectURI,
		OidcScopes:               s.OidcScopes,
		OidcAutoCreateUsers:      s.OidcAutoCreateUsers,
		OidcDefaultRole:          s.OidcDefaultRole,
		OidcDisableLocalAuth:     s.OidcDisableLocalAuth,
		OidcButtonText:           s.OidcButtonText,
		OidcSyncRoles:            s.OidcSyncRoles,
		OidcAdminGroup:           s.OidcAdminGroup,
		OidcSuperadminGroup:      s.OidcSuperadminGroup,
		OidcHostManagerGroup:     s.OidcHostManagerGroup,
		OidcReadonlyGroup:        s.OidcReadonlyGroup,
		OidcUserGroup:            s.OidcUserGroup,
		OidcEnforceHttps:         s.OidcEnforceHTTPS,
		ID:                       s.ID,
	}
}

func timeToPgTimestamp(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}

func dbUserSessionToModel(u db.UserSession) models.UserSession {
	return models.UserSession{
		ID:                u.ID,
		UserID:            u.UserID,
		RefreshToken:      u.RefreshToken,
		AccessTokenHash:   u.AccessTokenHash,
		IPAddress:         u.IpAddress,
		UserAgent:         u.UserAgent,
		DeviceFingerprint: u.DeviceFingerprint,
		LastActivity:      pgTime(u.LastActivity),
		ExpiresAt:         pgTime(u.ExpiresAt),
		CreatedAt:         pgTime(u.CreatedAt),
		IsRevoked:         u.IsRevoked,
		TfaRememberMe:     u.TfaRememberMe,
		TfaBypassUntil:    pgTimePtr(u.TfaBypassUntil),
		LoginCount:        int(u.LoginCount),
		LastLoginIP:       u.LastLoginIp,
	}
}

func dbFindSessionWithTfaBypassRowToModel(u db.FindSessionWithTfaBypassRow) models.UserSession {
	return models.UserSession{
		ID:                u.ID,
		UserID:            u.UserID,
		RefreshToken:      u.RefreshToken,
		AccessTokenHash:   u.AccessTokenHash,
		IPAddress:         u.IpAddress,
		UserAgent:         u.UserAgent,
		DeviceFingerprint: u.DeviceFingerprint,
		LastActivity:      pgTime(u.LastActivity),
		ExpiresAt:         pgTime(u.ExpiresAt),
		CreatedAt:         pgTime(u.CreatedAt),
		IsRevoked:         u.IsRevoked,
		TfaRememberMe:     u.TfaRememberMe,
		TfaBypassUntil:    pgTimePtr(u.TfaBypassUntil),
		LoginCount:        int(u.LoginCount),
		LastLoginIP:       u.LastLoginIp,
	}
}

func dbHostGroupToModelFull(h db.HostGroup) models.HostGroup {
	return models.HostGroup{
		ID:          h.ID,
		Name:        h.Name,
		Description: h.Description,
		Color:       h.Color,
		CreatedAt:   pgTime(h.CreatedAt),
		UpdatedAt:   pgTime(h.UpdatedAt),
	}
}

func dbDashboardPreferenceToModel(d db.DashboardPreference) models.DashboardPreference {
	return models.DashboardPreference{
		ID:        d.ID,
		UserID:    d.UserID,
		CardID:    d.CardID,
		Enabled:   d.Enabled,
		Order:     int(d.Order),
		ColSpan:   int(d.ColSpan),
		CreatedAt: pgTime(d.CreatedAt),
		UpdatedAt: pgTime(d.UpdatedAt),
	}
}

func dbDashboardLayoutToModel(d db.DashboardLayout) models.DashboardLayout {
	return models.DashboardLayout{
		UserID:        d.UserID,
		StatsColumns:  int(d.StatsColumns),
		ChartsColumns: int(d.ChartsColumns),
		UpdatedAt:     pgTime(d.UpdatedAt),
	}
}

func dbDockerContainerToModel(d db.DockerContainer) models.DockerContainer {
	return models.DockerContainer{
		ID:          d.ID,
		HostID:      d.HostID,
		ContainerID: d.ContainerID,
		Name:        d.Name,
		ImageID:     d.ImageID,
		ImageName:   d.ImageName,
		ImageTag:    d.ImageTag,
		Status:      d.Status,
		State:       d.State,
		Ports:       models.JSON(d.Ports),
		Labels:      models.JSON(d.Labels),
		CreatedAt:   pgTime(d.CreatedAt),
		StartedAt:   pgTimePtr(d.StartedAt),
		UpdatedAt:   pgTime(d.UpdatedAt),
		LastChecked: pgTime(d.LastChecked),
	}
}

func dbDockerImageToModel(d db.DockerImage) models.DockerImage {
	return models.DockerImage{
		ID:          d.ID,
		Repository:  d.Repository,
		Tag:         d.Tag,
		ImageID:     d.ImageID,
		Digest:      d.Digest,
		SizeBytes:   d.SizeBytes,
		Source:      d.Source,
		CreatedAt:   pgTime(d.CreatedAt),
		LastPulled:  pgTimePtr(d.LastPulled),
		LastChecked: pgTime(d.LastChecked),
		UpdatedAt:   pgTime(d.UpdatedAt),
	}
}

func dbDockerImageUpdateToModel(d db.DockerImageUpdate) models.DockerImageUpdate {
	return models.DockerImageUpdate{
		ID:               d.ID,
		ImageID:          d.ImageID,
		CurrentTag:       d.CurrentTag,
		AvailableTag:     d.AvailableTag,
		IsSecurityUpdate: d.IsSecurityUpdate,
		Severity:         d.Severity,
		ChangelogURL:     d.ChangelogUrl,
		CreatedAt:        pgTime(d.CreatedAt),
		UpdatedAt:        pgTime(d.UpdatedAt),
	}
}

func dbDockerVolumeToModel(d db.DockerVolume) models.DockerVolume {
	return models.DockerVolume{
		ID:          d.ID,
		HostID:      d.HostID,
		VolumeID:    d.VolumeID,
		Name:        d.Name,
		Driver:      d.Driver,
		Mountpoint:  d.Mountpoint,
		Renderer:    d.Renderer,
		Scope:       d.Scope,
		Labels:      models.JSON(d.Labels),
		Options:     models.JSON(d.Options),
		SizeBytes:   d.SizeBytes,
		RefCount:    int(d.RefCount),
		CreatedAt:   pgTime(d.CreatedAt),
		UpdatedAt:   pgTime(d.UpdatedAt),
		LastChecked: pgTime(d.LastChecked),
	}
}

func dbDockerNetworkToModel(d db.DockerNetwork) models.DockerNetwork {
	return models.DockerNetwork{
		ID:             d.ID,
		HostID:         d.HostID,
		NetworkID:      d.NetworkID,
		Name:           d.Name,
		Driver:         d.Driver,
		Scope:          d.Scope,
		IPv6Enabled:    d.Ipv6Enabled,
		Internal:       d.Internal,
		Attachable:     d.Attachable,
		Ingress:        d.Ingress,
		ConfigOnly:     d.ConfigOnly,
		Labels:         models.JSON(d.Labels),
		IPAM:           models.JSON(d.Ipam),
		ContainerCount: int(d.ContainerCount),
		CreatedAt:      pgTimePtr(d.CreatedAt),
		UpdatedAt:      pgTime(d.UpdatedAt),
		LastChecked:    pgTime(d.LastChecked),
	}
}

func dbRepositoryToModel(r db.Repository) models.Repository {
	prio := (*int)(nil)
	if r.Priority != nil {
		p := int(*r.Priority)
		prio = &p
	}
	return models.Repository{
		ID:           r.ID,
		Name:         r.Name,
		URL:          r.Url,
		Distribution: r.Distribution,
		Components:   r.Components,
		RepoType:     r.RepoType,
		IsActive:     r.IsActive,
		IsSecure:     r.IsSecure,
		Priority:     prio,
		Description:  r.Description,
		CreatedAt:    pgTime(r.CreatedAt),
		UpdatedAt:    pgTime(r.UpdatedAt),
	}
}

func dbPackageToModel(p db.Package) models.Package {
	return models.Package{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		Category:      p.Category,
		LatestVersion: p.LatestVersion,
		CreatedAt:     pgTime(p.CreatedAt),
		UpdatedAt:     pgTime(p.UpdatedAt),
	}
}

func dbUserToModel(u db.User) models.User {
	return models.User{
		ID:              u.ID,
		Username:        u.Username,
		Email:           u.Email,
		PasswordHash:    u.PasswordHash,
		Role:            u.Role,
		IsActive:        u.IsActive,
		LastLogin:       pgTimePtr(u.LastLogin),
		CreatedAt:       pgTime(u.CreatedAt),
		UpdatedAt:       pgTime(u.UpdatedAt),
		TfaBackupCodes:  u.TfaBackupCodes,
		TfaEnabled:      u.TfaEnabled,
		TfaSecret:       u.TfaSecret,
		FirstName:       u.FirstName,
		LastName:        u.LastName,
		ThemePreference: u.ThemePreference,
		ColorTheme:      u.ColorTheme,
		UIPreferences:   models.JSON(u.UiPreferences),
		OidcSub:         u.OidcSub,
		OidcProvider:    u.OidcProvider,
		AvatarURL:       u.AvatarUrl,
		DiscordID:       u.DiscordID,
		DiscordUsername: u.DiscordUsername,
		DiscordAvatar:   u.DiscordAvatar,
		DiscordLinkedAt: pgTimePtr(u.DiscordLinkedAt),
	}
}

func dbHostToModel(h db.Host) *models.Host {
	return &models.Host{
		ID:                           h.ID,
		MachineID:                    h.MachineID,
		FriendlyName:                 h.FriendlyName,
		IP:                           h.Ip,
		OSType:                       h.OsType,
		OSVersion:                    h.OsVersion,
		Architecture:                 h.Architecture,
		LastUpdate:                   pgTime(h.LastUpdate),
		Status:                       h.Status,
		CreatedAt:                    pgTime(h.CreatedAt),
		UpdatedAt:                    pgTime(h.UpdatedAt),
		ApiID:                        h.ApiID,
		ApiKey:                       h.ApiKey,
		AgentVersion:                 h.AgentVersion,
		AutoUpdate:                   h.AutoUpdate,
		CPUCores:                     pgInt32ToInt(h.CpuCores),
		CPUModel:                     h.CpuModel,
		DiskDetails:                  models.JSON(h.DiskDetails),
		DNSServers:                   models.JSON(h.DnsServers),
		GatewayIP:                    h.GatewayIp,
		Hostname:                     h.Hostname,
		KernelVersion:                h.KernelVersion,
		InstalledKernelVersion:       h.InstalledKernelVersion,
		LoadAverage:                  models.JSON(h.LoadAverage),
		NetworkInterfaces:            models.JSON(h.NetworkInterfaces),
		RamInstalled:                 h.RamInstalled,
		SelinuxStatus:                h.SelinuxStatus,
		SwapSize:                     h.SwapSize,
		SystemUptime:                 h.SystemUptime,
		Notes:                        h.Notes,
		NeedsReboot:                  h.NeedsReboot,
		RebootReason:                 h.RebootReason,
		DockerEnabled:                h.DockerEnabled,
		ComplianceEnabled:            h.ComplianceEnabled,
		ComplianceOnDemandOnly:       h.ComplianceOnDemandOnly,
		ComplianceOpenscapEnabled:    h.ComplianceOpenscapEnabled,
		ComplianceDockerBenchEnabled: h.ComplianceDockerBenchEnabled,
		ComplianceScannerStatus:      models.JSON(h.ComplianceScannerStatus),
		ComplianceScannerUpdatedAt:   pgTimePtr(h.ComplianceScannerUpdatedAt),
		ComplianceDefaultProfileID:   h.ComplianceDefaultProfileID,
		HostDownAlertsEnabled:        h.HostDownAlertsEnabled,
		ExpectedPlatform:             h.ExpectedPlatform,
		PackageManager:               h.PackageManager,
		PrimaryInterface:             h.PrimaryInterface,
	}
}

func pgTime(t pgtype.Timestamp) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

func pgTimePtr(t pgtype.Timestamp) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}

func pgInt32ToInt(p *int32) *int {
	if p == nil {
		return nil
	}
	i := int(*p)
	return &i
}

func dbHostGroupToModel(r db.GetHostGroupsForHostsRow) models.HostGroup {
	return models.HostGroup{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Color:       r.Color,
		CreatedAt:   pgTime(r.CreatedAt),
		UpdatedAt:   pgTime(r.UpdatedAt),
	}
}
