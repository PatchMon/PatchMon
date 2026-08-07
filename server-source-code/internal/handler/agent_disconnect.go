package handler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
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
//
// Both side effects are gated on the agent still being gone at the moment they
// run. Agent reconnect backoff starts at ~1s and each of the DB calls below can
// take seconds, so a reconnect very often lands mid-callback: without the
// re-check we would raise a host_down alert for a host that is already back and
// kill a patch run the reconnected agent is still executing.
//
// registry may be nil, in which case the re-checks are skipped.
func NewAgentDisconnectHandler(
	db database.DBProvider,
	hostsStore *store.HostsStore,
	patchRuns *store.PatchRunsStore,
	registry *agentregistry.Registry,
	emit *notifications.Emitter,
	log *slog.Logger,
) OnAgentDisconnect {
	reconnected := func(apiID string) bool {
		return registry != nil && registry.IsConnected(apiID)
	}
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

		if reconnected(apiID) {
			log.Debug("agent disconnect: agent already reconnected, skipping host_down alert", "api_id", apiID)
		} else {
			alerts.OnDisconnect(dbCtx, resolvedDB, apiID, tenantHost, emit, log)
		}

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
		// Re-check immediately before the write: the lookup above is another
		// round-trip's worth of time for a reconnect to land, and marking a run
		// agent_disconnected while the agent is actively patching discards the
		// real outcome.
		if reconnected(apiID) {
			log.Debug("agent disconnect: agent already reconnected, leaving patch runs running", "api_id", apiID, "host_id", host.ID)
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
