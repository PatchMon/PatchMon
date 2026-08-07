package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// dbPoolWarnHostThreshold is the active-host count above which the startup
// health check evaluates whether DB_CONNECTION_LIMIT is large enough.
// Below the threshold the default 30-connection pool comfortably fits any
// realistic burst of concurrent agent reports.
const dbPoolWarnHostThreshold = 50

// recommendedDBPoolSize returns ceil(hosts * 1.5) rounded up to the nearest 10.
// Pure integer math: hosts*15+99 / 100 * 10. Examples: 51->80, 95->150,
// 100->150, 200->300.
func recommendedDBPoolSize(hosts int) int {
	return ((hosts*15 + 99) / 100) * 10
}

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

// warnDBPoolUndersized emits a WARN log at startup when the active host count
// exceeds the configured DB connection pool limit by a meaningful margin.
//
// The default DB_CONNECTION_LIMIT (30) is fine for small deployments. Beyond
// ~50 hosts, synchronised report bursts can exhaust the pool and cause request
// queuing, retry backoff, and ultimately HTTP timeouts on agent reports.
//
// Recommendation formula: ceil(host_count * 1.5) rounded up to the nearest 10.
// This gives roughly 1.5 connections per active host, which covers concurrent
// agent report bursts plus headroom for the API and queue workers. Examples:
//
//	 51 hosts ->  80
//	 95 hosts -> 150
//	100 hosts -> 150
//	200 hosts -> 300
//	355 hosts -> 540
//
// We only warn when the recommended value is strictly larger than the current
// limit, so operators who have already tuned the pool see no noise.
//
// Multi-host deployments skip this check because the primary database's host
// count is not representative of any tenant's host count, and the per-tenant
// pools are sized independently of cfg.DBConnectionLimit on the primary.
func warnDBPoolUndersized(
	ctx context.Context,
	cfg *config.Config,
	hostsStore *store.HostsStore,
	log *slog.Logger,
) {
	if cfg.RegistryDatabaseURL != "" {
		return // multi-host: per-tenant pool sizing is the wrong scope here
	}
	hosts, err := hostsStore.Count(ctx)
	if err != nil {
		return
	}
	if hosts <= dbPoolWarnHostThreshold {
		return
	}
	recommended := recommendedDBPoolSize(hosts)
	if cfg.DBConnectionLimit >= recommended {
		return
	}
	log.Warn(
		"database connection pool may be undersized for current host count",
		"active_hosts", hosts,
		"current_db_connection_limit", cfg.DBConnectionLimit,
		"recommended_db_connection_limit", recommended,
		"action", fmt.Sprintf(
			"CONSIDER INCREASING DB_CONNECTION_LIMIT environment variable to %d (current: %d, active hosts: %d). With many hosts reporting concurrently, an undersized pool causes request queuing, retry backoff, and ultimately HTTP timeouts on agent reports.",
			recommended, cfg.DBConnectionLimit, hosts,
		),
	)
}
