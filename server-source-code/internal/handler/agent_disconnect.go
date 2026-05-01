package handler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/alerts"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// agentDisconnectDBTimeout bounds the DB calls that happen after a WebSocket
// drop. The caller's ctx is the WS request context which is already cancelled
// by the time we run, so we detach to a fresh context for these short queries.
const agentDisconnectDBTimeout = 5 * time.Second

// NewAgentDisconnectHandler returns an OnAgentDisconnect callback that:
//   - creates host_down alerts via alerts.OnDisconnect (immediate alerting in
//     the Reporting module without waiting for the host status monitor),
//   - marks any in-flight patch_runs for the host as agent_disconnected so a
//     dropped agent can't leave runs stuck in "running" indefinitely.
func NewAgentDisconnectHandler(
	db database.DBProvider,
	hostsStore *store.HostsStore,
	patchRuns *store.PatchRunsStore,
	emit *notifications.Emitter,
	log *slog.Logger,
) OnAgentDisconnect {
	return func(ctx context.Context, apiID string) {
		// The WS-derived ctx is typically cancelled the moment we get here.
		// context.WithTimeout on a cancelled parent is still short-circuited,
		// so detach via context.Background and re-thread the tenant DB
		// explicitly through the resolver to preserve tenant routing.
		// Both alerts.OnDisconnect and the patch-run cleanup below depend on
		// this — the previous version passed the cancelled WS ctx to alerts,
		// silently failing every host_down alert query at boot.
		resolvedDB := db.DB(ctx)
		tenantHost := hostctx.TenantHostKey(ctx)
		dbCtx, cancel := context.WithTimeout(hostctx.WithDB(context.Background(), resolvedDB), agentDisconnectDBTimeout)
		defer cancel()

		alerts.OnDisconnect(dbCtx, resolvedDB, apiID, tenantHost, emit, log)

		if patchRuns == nil || hostsStore == nil {
			return
		}

		host, err := hostsStore.GetByApiID(dbCtx, apiID)
		if err != nil || host == nil {
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Debug("agent disconnect: host lookup failed", "api_id", apiID, "error", err)
			}
			return
		}
		count, err := patchRuns.MarkRunsAgentDisconnected(dbCtx, host.ID, "Agent disconnected during patch run")
		if err != nil {
			log.Warn("agent disconnect: mark runs agent_disconnected failed", "host_id", host.ID, "api_id", apiID, "error", err)
			return
		}
		if count > 0 {
			log.Info("patching: marked runs as agent_disconnected", "count", count, "host_id", host.ID, "api_id", apiID)
		}
	}
}
