package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// warnOidcSuperadminLockoutRisk emits a WARN log at startup when the OIDC
// configuration cannot grant the superadmin role to any user.
//
// Trigger: oidc_enabled=true AND oidc_sync_roles=true AND oidc_superadmin_group
// is empty AND at least one superadmin exists in the primary database.
//
// In this state, the OIDC role-sync path maps groups to lesser roles and would
// silently demote existing superadmins on login. The handler-level guardrail
// in handler/oidc.go blocks that demotion at login time, but neither mechanism
// can grant superadmin via OIDC until the group is configured. This startup
// warning surfaces the misconfiguration early so operators can fix the IdP
// group mapping before users start signing in.
//
// Multi-host deployments skip this check because OIDC settings are per-tenant
// and the primary database's settings are not representative of tenant DBs.
func warnOidcSuperadminLockoutRisk(
	ctx context.Context,
	cfg *config.Config,
	settingsStore *store.SettingsStore,
	usersStore *store.UsersStore,
	log *slog.Logger,
) {
	if cfg.RegistryDatabaseURL != "" {
		return // multi-host: OIDC settings are per-tenant
	}
	oidc, err := config.ResolveOidcConfig(ctx, cfg, settingsStore.GetFirst)
	if err != nil {
		return
	}
	if !oidc.Enabled || !oidc.SyncRoles {
		return
	}
	if strings.TrimSpace(oidc.SuperadminGroup) != "" {
		return
	}
	count, err := usersStore.CountSuperadmins(ctx)
	if err != nil || count == 0 {
		return
	}
	log.Warn(
		"oidc role sync cannot grant superadmin; existing superadmins are protected from demotion but cannot be re-granted via OIDC",
		"reason", "oidc_sync_roles=true but oidc_superadmin_group is empty",
		"action", "set OIDC_SUPERADMIN_GROUP (or settings.oidc_superadmin_group) to the IdP group your superadmins belong to, or set oidc_sync_roles=false to keep DB roles authoritative",
		"affected_superadmin_count", count,
	)
}
