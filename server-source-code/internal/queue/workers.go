package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/alerts"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/hibiken/asynq"
)

const (
	TypeHostStatusMonitor        = "host-status-monitor"
	TypeAlertCleanup             = "alert-cleanup"
	hostStatusMonitorConcurrency = 20
)

// resolveDBFromPayload returns the DB to use: from poolCache when host is in payload, else defaultDB.
func resolveDBFromPayload(ctx context.Context, payload []byte, defaultDB *database.DB, poolCache *hostctx.PoolCache) *database.DB {
	db := defaultDB
	if len(payload) == 0 || poolCache == nil {
		return db
	}
	var p AutomationPayload
	if err := json.Unmarshal(payload, &p); err == nil && strings.TrimSpace(p.Host) != "" {
		if resolved, err := poolCache.GetOrCreate(ctx, p.Host); err == nil && resolved != nil {
			db = resolved
		}
	}
	return db
}

// workerTenantKey prefixes a Redis key with the context domain for multi-context isolation.
// Mirrors hostctx.TenantKey but works in worker context where only the host string
// (from payload) is available, not a full registry Entry in context.
func workerTenantKey(tenantHost, key string) string {
	if tenantHost != "" {
		return "t:" + tenantHost + ":" + key
	}
	return key
}

func tenantHostFromPayload(payload []byte) string {
	var p AutomationPayload
	if json.Unmarshal(payload, &p) == nil {
		return strings.TrimSpace(p.Host)
	}
	return ""
}

// forEachDB calls fn for the defaultDB and then for every context DB in the poolCache.
// When there is no poolCache (single-context), only defaultDB is processed.
// The tenantHost passed to fn is the domain (Entry.Host), matching TenantKey's prefix.
func forEachDB(ctx context.Context, defaultDB *database.DB, poolCache *hostctx.PoolCache, fn func(context.Context, *database.DB, string)) {
	fn(ctx, defaultDB, "")
	if poolCache == nil {
		return
	}
	for _, host := range poolCache.ListHosts() {
		d, err := poolCache.GetOrCreate(ctx, host)
		if err != nil || d == nil {
			continue
		}
		fn(ctx, d, host)
	}
}

// HostStatusMonitorHandler handles host-status-monitor jobs.
type HostStatusMonitorHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	emit      *notifications.Emitter
	log       *slog.Logger
}

// NewHostStatusMonitorHandler creates a host status monitor handler.
func NewHostStatusMonitorHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, emit *notifications.Emitter, log *slog.Logger) *HostStatusMonitorHandler {
	return &HostStatusMonitorHandler{defaultDB: defaultDB, poolCache: poolCache, emit: emit, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *HostStatusMonitorHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	payload := t.Payload()

	// Payload has host: single-host or manual trigger with host
	if len(payload) > 0 {
		db := resolveDBFromPayload(ctx, payload, h.defaultDB, h.poolCache)
		alertsCreated, err := alerts.ProcessHostStatusMonitor(ctx, db, tenantHostFromPayload(payload), h.emit, h.log)
		if err != nil {
			return err
		}
		h.log.Info("host status monitor completed", "alerts_created", alertsCreated)
		return nil
	}

	// Empty payload: single-host (PoolCache nil) or multi-host fan-out
	if h.poolCache == nil {
		alertsCreated, err := alerts.ProcessHostStatusMonitor(ctx, h.defaultDB, "", h.emit, h.log)
		if err != nil {
			return err
		}
		h.log.Info("host status monitor completed", "alerts_created", alertsCreated)
		return nil
	}

	// Multi-host: process defaultDB first, then all registry hosts in parallel
	var totalCreated atomic.Int64
	if n, err := alerts.ProcessHostStatusMonitor(ctx, h.defaultDB, "", h.emit, h.log); err == nil {
		totalCreated.Add(int64(n))
	}

	hosts := h.poolCache.ListHosts()
	if len(hosts) == 0 {
		h.log.Info("host status monitor completed", "alerts_created", totalCreated.Load(), "hosts_processed", 1)
		return nil
	}

	sem := make(chan struct{}, hostStatusMonitorConcurrency)
	var wg sync.WaitGroup
	for _, host := range hosts {
		host := host
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			db, err := h.poolCache.GetOrCreate(ctx, host)
			if err != nil || db == nil {
				return
			}
			if n, err := alerts.ProcessHostStatusMonitor(ctx, db, host, h.emit, h.log); err == nil {
				totalCreated.Add(int64(n))
			}
		}()
	}
	wg.Wait()

	h.log.Info("host status monitor completed", "alerts_created", totalCreated.Load(), "hosts_processed", len(hosts)+1)
	return nil
}

// UpdateThresholdMonitorHandler handles update-threshold-monitor jobs.
type UpdateThresholdMonitorHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	emit      *notifications.Emitter
	log       *slog.Logger
}

// NewUpdateThresholdMonitorHandler creates an update threshold monitor handler.
func NewUpdateThresholdMonitorHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, emit *notifications.Emitter, log *slog.Logger) *UpdateThresholdMonitorHandler {
	return &UpdateThresholdMonitorHandler{defaultDB: defaultDB, poolCache: poolCache, emit: emit, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *UpdateThresholdMonitorHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	payload := t.Payload()

	// Payload has host: single-host or manual trigger with host
	if len(payload) > 0 {
		db := resolveDBFromPayload(ctx, payload, h.defaultDB, h.poolCache)
		alertsCreated, err := alerts.ProcessUpdateThresholdMonitor(ctx, db, tenantHostFromPayload(payload), h.emit, h.log)
		if err != nil {
			return err
		}
		h.log.Info("update threshold monitor completed", "alerts_created", alertsCreated)
		return nil
	}

	// Empty payload: single-host (PoolCache nil) or multi-host fan-out
	if h.poolCache == nil {
		alertsCreated, err := alerts.ProcessUpdateThresholdMonitor(ctx, h.defaultDB, "", h.emit, h.log)
		if err != nil {
			return err
		}
		h.log.Info("update threshold monitor completed", "alerts_created", alertsCreated)
		return nil
	}

	// Multi-host: process defaultDB first, then all registry hosts in parallel
	var totalCreated atomic.Int64
	if n, err := alerts.ProcessUpdateThresholdMonitor(ctx, h.defaultDB, "", h.emit, h.log); err == nil {
		totalCreated.Add(int64(n))
	}

	hosts := h.poolCache.ListHosts()
	if len(hosts) == 0 {
		h.log.Info("update threshold monitor completed", "alerts_created", totalCreated.Load(), "hosts_processed", 1)
		return nil
	}

	sem := make(chan struct{}, hostStatusMonitorConcurrency)
	var wg sync.WaitGroup
	for _, host := range hosts {
		host := host
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			db, err := h.poolCache.GetOrCreate(ctx, host)
			if err != nil || db == nil {
				return
			}
			if n, err := alerts.ProcessUpdateThresholdMonitor(ctx, db, host, h.emit, h.log); err == nil {
				totalCreated.Add(int64(n))
			}
		}()
	}
	wg.Wait()

	h.log.Info("update threshold monitor completed", "alerts_created", totalCreated.Load(), "hosts_processed", len(hosts)+1)
	return nil
}

// SessionCleanupHandler handles session-cleanup jobs.
type SessionCleanupHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewSessionCleanupHandler creates a session cleanup handler.
func NewSessionCleanupHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *SessionCleanupHandler {
	return &SessionCleanupHandler{defaultDB: defaultDB, poolCache: poolCache, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *SessionCleanupHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		return d.Queries.DeleteExpiredSessions(ctx)
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		if err := d.Queries.DeleteExpiredSessions(ctx); err != nil {
			h.log.Warn("session cleanup failed", "host", host, "error", err)
		}
	})
	h.log.Info("session cleanup completed")
	return nil
}

// OrphanedRepoCleanupHandler handles orphaned-repo-cleanup jobs.
type OrphanedRepoCleanupHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewOrphanedRepoCleanupHandler creates an orphaned repo cleanup handler.
func NewOrphanedRepoCleanupHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *OrphanedRepoCleanupHandler {
	return &OrphanedRepoCleanupHandler{defaultDB: defaultDB, poolCache: poolCache, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *OrphanedRepoCleanupHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		ctx = hostctx.WithDB(ctx, d)
		repos := store.NewRepositoriesStore(&hostctx.DBResolver{Default: d})
		_, count, err := repos.CleanupOrphaned(ctx)
		if err != nil {
			return err
		}
		h.log.Info("orphaned repo cleanup completed", "deleted", count)
		return nil
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		rCtx := hostctx.WithDB(ctx, d)
		repos := store.NewRepositoriesStore(&hostctx.DBResolver{Default: d})
		_, count, err := repos.CleanupOrphaned(rCtx)
		if err != nil {
			h.log.Warn("orphaned repo cleanup failed", "host", host, "error", err)
			return
		}
		if count > 0 {
			h.log.Info("orphaned repo cleanup", "host", host, "deleted", count)
		}
	})
	h.log.Info("orphaned repo cleanup completed")
	return nil
}

// OrphanedPkgCleanupHandler handles orphaned-package-cleanup jobs.
type OrphanedPkgCleanupHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewOrphanedPkgCleanupHandler creates an orphaned package cleanup handler.
func NewOrphanedPkgCleanupHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *OrphanedPkgCleanupHandler {
	return &OrphanedPkgCleanupHandler{defaultDB: defaultDB, poolCache: poolCache, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *OrphanedPkgCleanupHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		return h.cleanupDB(ctx, d)
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		if err := h.cleanupDB(ctx, d); err != nil {
			h.log.Warn("orphaned package cleanup failed", "host", host, "error", err)
		}
	})
	h.log.Info("orphaned package cleanup completed")
	return nil
}

func (h *OrphanedPkgCleanupHandler) cleanupDB(ctx context.Context, d *database.DB) error {
	orphaned, err := d.Queries.ListOrphanedPackages(ctx)
	if err != nil {
		return err
	}
	if len(orphaned) == 0 {
		return nil
	}
	ids := make([]string, len(orphaned))
	for i, pkg := range orphaned {
		ids[i] = pkg.ID
	}
	return d.Queries.DeletePackagesByIDs(ctx, ids)
}

// DockerInvCleanupHandler handles docker-inventory-cleanup jobs.
type DockerInvCleanupHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewDockerInvCleanupHandler creates a docker inventory cleanup handler.
func NewDockerInvCleanupHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *DockerInvCleanupHandler {
	return &DockerInvCleanupHandler{defaultDB: defaultDB, poolCache: poolCache, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *DockerInvCleanupHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		return h.cleanupDB(ctx, d)
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		if err := h.cleanupDB(ctx, d); err != nil {
			h.log.Warn("docker inventory cleanup failed", "host", host, "error", err)
		}
	})
	h.log.Info("docker inventory cleanup completed")
	return nil
}

func (h *DockerInvCleanupHandler) cleanupDB(ctx context.Context, d *database.DB) error {
	containers, err := d.Queries.ListOrphanedContainers(ctx)
	if err != nil {
		return err
	}
	containerIDs := make([]string, len(containers))
	for i, c := range containers {
		containerIDs[i] = c.ID
	}
	if len(containerIDs) > 0 {
		if err := d.Queries.DeleteContainersByIDs(ctx, containerIDs); err != nil {
			return err
		}
		h.log.Info("deleted orphaned containers", "count", len(containerIDs))
	}

	images, err := d.Queries.ListOrphanedImages(ctx)
	if err != nil {
		return err
	}
	for _, img := range images {
		_ = d.Queries.DeleteImageUpdatesByImageID(ctx, img.ID)
		if err := d.Queries.DeleteImageByID(ctx, img.ID); err != nil {
			h.log.Warn("failed to delete orphaned image", "id", img.ID, "error", err)
			continue
		}
	}
	if len(images) > 0 {
		h.log.Info("deleted orphaned images", "count", len(images))
	}
	return nil
}

// SystemStatisticsHandler handles system-statistics jobs.
type SystemStatisticsHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewSystemStatisticsHandler creates a system statistics handler.
func NewSystemStatisticsHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *SystemStatisticsHandler {
	return &SystemStatisticsHandler{defaultDB: defaultDB, poolCache: poolCache, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *SystemStatisticsHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		return h.collectStats(ctx, d)
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		if err := h.collectStats(ctx, d); err != nil {
			h.log.Warn("system statistics failed", "host", host, "error", err)
		}
	})
	h.log.Info("system statistics completed")
	return nil
}

func (h *SystemStatisticsHandler) collectStats(ctx context.Context, d *database.DB) error {
	stats, err := d.Queries.GetSystemStatsForInsert(ctx)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("sys-%d", time.Now().UnixMilli())
	return d.Queries.InsertSystemStatistics(ctx, db.InsertSystemStatisticsParams{
		ID:                  id,
		UniquePackagesCount: stats.Column1,
		UniqueSecurityCount: stats.Column2,
		TotalPackages:       stats.Column3,
		TotalHosts:          stats.Column4,
		HostsNeedingUpdates: stats.Column5,
	})
}

// VersionUpdateCheckHandler handles version-update-check jobs.
type VersionUpdateCheckHandler struct {
	defaultDB     *database.DB
	poolCache     *hostctx.PoolCache
	serverVersion string
	emit          *notifications.Emitter
	log           *slog.Logger
}

// NewVersionUpdateCheckHandler creates a version update check handler.
func NewVersionUpdateCheckHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, serverVersion string, emit *notifications.Emitter, log *slog.Logger) *VersionUpdateCheckHandler {
	return &VersionUpdateCheckHandler{defaultDB: defaultDB, poolCache: poolCache, serverVersion: serverVersion, emit: emit, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *VersionUpdateCheckHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		th := tenantHostFromPayload(t.Payload())
		return h.checkVersions(ctx, d, th)
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		if err := h.checkVersions(ctx, d, host); err != nil {
			h.log.Warn("version update check failed", "host", host, "error", err)
		}
	})
	h.log.Info("version update check completed")
	return nil
}

func (h *VersionUpdateCheckHandler) checkVersions(ctx context.Context, d *database.DB, tenantHost string) error {
	if err := alerts.ProcessServerUpdate(ctx, d, h.serverVersion, tenantHost, h.emit, h.log); err != nil {
		return err
	}
	agentsDir := util.GetAgentsDir()
	return alerts.ProcessAgentUpdate(ctx, d, agentsDir, tenantHost, h.emit, h.log)
}

// ComplianceScanCleanupHandler handles compliance-scan-cleanup jobs.
type ComplianceScanCleanupHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewComplianceScanCleanupHandler creates a compliance scan cleanup handler.
func NewComplianceScanCleanupHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *ComplianceScanCleanupHandler {
	return &ComplianceScanCleanupHandler{defaultDB: defaultDB, poolCache: poolCache, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *ComplianceScanCleanupHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		return h.cleanupDB(ctx, d)
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		if err := h.cleanupDB(ctx, d); err != nil {
			h.log.Warn("compliance scan cleanup failed", "host", host, "error", err)
		}
	})
	h.log.Info("compliance scan cleanup completed")
	return nil
}

func (h *ComplianceScanCleanupHandler) cleanupDB(ctx context.Context, d *database.DB) error {
	pgThreshold := pgtime.From(time.Now().Add(-3 * time.Hour))
	msg := "Scan terminated automatically after running for more than 3 hours"
	return d.Queries.UpdateStalledComplianceScans(ctx, db.UpdateStalledComplianceScansParams{
		StartedAt:    pgThreshold,
		ErrorMessage: &msg,
	})
}

// PatchRunCleanupHandler handles patch-run-cleanup jobs.
type PatchRunCleanupHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
	// getStallTimeoutMin is invoked once per sweep so DB-edited values take
	// effect on the next cron tick without a restart.
	getStallTimeoutMin func() int
}

// NewPatchRunCleanupHandler creates a patch run cleanup handler.
// getStallTimeoutMin is called on each sweep to read the current effective
// timeout (env -> DB -> default). Passing a callback (rather than baking the
// value at construction) lets operators tweak the timeout via Settings →
// Environment without restarting the server.
func NewPatchRunCleanupHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger, getStallTimeoutMin func() int) *PatchRunCleanupHandler {
	return &PatchRunCleanupHandler{defaultDB: defaultDB, poolCache: poolCache, log: log, getStallTimeoutMin: getStallTimeoutMin}
}

// ProcessTask implements asynq.Handler.
func (h *PatchRunCleanupHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	// Resolve once per sweep: the timeout is a global tunable, so reading
	// fresh per-host would just add N redundant DB hits.
	stallMin := h.resolveStallTimeoutMin()
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		count, err := h.cleanupDB(ctx, d, stallMin)
		if err != nil {
			return err
		}
		if count > 0 {
			h.log.Info("patch run cleanup: marked timed_out", "count", count, "stall_min", stallMin)
		}
		return nil
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		count, err := h.cleanupDB(ctx, d, stallMin)
		if err != nil {
			h.log.Warn("patch run cleanup failed", "host", host, "error", err)
			return
		}
		if count > 0 {
			h.log.Info("patch run cleanup: marked timed_out", "host", host, "count", count, "stall_min", stallMin)
		}
	})
	h.log.Info("patch run cleanup completed", "stall_min", stallMin)
	return nil
}

func (h *PatchRunCleanupHandler) resolveStallTimeoutMin() int {
	if h.getStallTimeoutMin == nil {
		// Defensive: keep cleanup operational even if the constructor
		// was passed a nil callback (e.g. test fixtures).
		return 30
	}
	v := h.getStallTimeoutMin()
	if v < 5 {
		return 5
	}
	return v
}

func (h *PatchRunCleanupHandler) cleanupDB(ctx context.Context, d *database.DB, stallMin int) (int64, error) {
	pgThreshold := pgtime.From(time.Now().Add(-time.Duration(stallMin) * time.Minute))
	msg := fmt.Sprintf("Marked as timed_out after running for more than %d minutes", stallMin)
	return d.Queries.MarkPatchRunsTimedOut(ctx, db.MarkPatchRunsTimedOutParams{
		StartedAt:    pgThreshold,
		ErrorMessage: &msg,
	})
}

// AgentReportsCleanupHandler sweeps update_history (the Agent Activity feed
// table) on a daily schedule, deleting rows older than the configured
// retention window. Mirrors PatchRunCleanupHandler exactly: a getRetentionDays
// closure is invoked once per sweep so DB-edited values take effect on the
// next cron tick without a server restart.
type AgentReportsCleanupHandler struct {
	defaultDB        *database.DB
	poolCache        *hostctx.PoolCache
	log              *slog.Logger
	getRetentionDays func() int
}

// NewAgentReportsCleanupHandler creates an agent-reports-cleanup handler.
// getRetentionDays is invoked once per sweep to read the current effective
// retention (env -> DB -> default). Pass nil only in test fixtures; the
// worker falls back to 30 days when the callback is missing.
func NewAgentReportsCleanupHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger, getRetentionDays func() int) *AgentReportsCleanupHandler {
	return &AgentReportsCleanupHandler{defaultDB: defaultDB, poolCache: poolCache, log: log, getRetentionDays: getRetentionDays}
}

// ProcessTask implements asynq.Handler.
func (h *AgentReportsCleanupHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	retentionDays := h.resolveRetentionDays()
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		count, err := h.cleanupDB(ctx, d, retentionDays)
		if err != nil {
			return err
		}
		if count > 0 {
			h.log.Info("agent reports cleanup: deleted", "count", count, "retention_days", retentionDays)
		}
		return nil
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		count, err := h.cleanupDB(ctx, d, retentionDays)
		if err != nil {
			h.log.Warn("agent reports cleanup failed", "host", host, "error", err)
			return
		}
		if count > 0 {
			h.log.Info("agent reports cleanup: deleted", "host", host, "count", count, "retention_days", retentionDays)
		}
	})
	h.log.Info("agent reports cleanup completed", "retention_days", retentionDays)
	return nil
}

func (h *AgentReportsCleanupHandler) resolveRetentionDays() int {
	if h.getRetentionDays == nil {
		return 30
	}
	v := h.getRetentionDays()
	// Mirror the env-loader bounds; defensive against a stale callback that
	// might temporarily return an out-of-range value during a config swap.
	if v < 7 {
		return 7
	}
	if v > 365 {
		return 365
	}
	return v
}

func (h *AgentReportsCleanupHandler) cleanupDB(ctx context.Context, d *database.DB, retentionDays int) (int64, error) {
	return d.Queries.DeleteOldUpdateHistory(ctx, int32(retentionDays))
}

// PackageStatsRefreshHandler refreshes mv_package_stats so the Packages
// list page can render its per-package counters via an indexed hash join
// against the matview instead of aggregating host_packages on every
// request. Runs CONCURRENTLY (briefly row-locks the matview rather than
// blocking readers), tolerates the rare lock-conflict case by surfacing
// the error to asynq's retry machinery rather than swallowing it.
type PackageStatsRefreshHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewPackageStatsRefreshHandler creates a package-stats-refresh handler.
func NewPackageStatsRefreshHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *PackageStatsRefreshHandler {
	return &PackageStatsRefreshHandler{defaultDB: defaultDB, poolCache: poolCache, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *PackageStatsRefreshHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		return h.refresh(ctx, d, "")
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		if err := h.refresh(ctx, d, host); err != nil {
			h.log.Warn("package stats refresh failed", "host", host, "error", err)
		}
	})
	h.log.Debug("package stats refresh completed")
	return nil
}

// refresh is a thin wrapper so the per-host loop and the explicit-payload
// path share a single execution path. The CONCURRENTLY variant requires
// the unique index defined in migration 000047 (mv_package_stats_pkey).
func (h *PackageStatsRefreshHandler) refresh(ctx context.Context, d *database.DB, host string) error {
	start := time.Now()
	if _, err := d.Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY mv_package_stats"); err != nil {
		return err
	}
	h.log.Debug("package stats refreshed", "host", host, "duration_ms", time.Since(start).Milliseconds())
	return nil
}

// AlertCleanupHandler handles alert-cleanup jobs.
type AlertCleanupHandler struct {
	defaultDB   *database.DB
	poolCache   *hostctx.PoolCache
	alertConfig *store.AlertConfigStore
	log         *slog.Logger
}

// NewAlertCleanupHandler creates an alert cleanup handler.
func NewAlertCleanupHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, alertConfig *store.AlertConfigStore, log *slog.Logger) *AlertCleanupHandler {
	return &AlertCleanupHandler{defaultDB: defaultDB, poolCache: poolCache, alertConfig: alertConfig, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *AlertCleanupHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		ctx = hostctx.WithDB(ctx, d)
		return h.cleanupDB(ctx, d)
	}
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		dbCtx := hostctx.WithDB(ctx, d)
		if err := h.cleanupDB(dbCtx, d); err != nil {
			h.log.Warn("alert cleanup failed", "host", host, "error", err)
		}
	})
	h.log.Info("alert cleanup completed")
	return nil
}

func (h *AlertCleanupHandler) cleanupDB(ctx context.Context, d *database.DB) error {
	settings, err := d.Queries.GetFirstSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.AlertsEnabled {
		return nil
	}
	deleted, err := h.alertConfig.CleanupOldAlerts(ctx)
	if err != nil {
		return err
	}
	autoResolved, err := h.alertConfig.AutoResolveOldAlerts(ctx)
	if err != nil {
		h.log.Warn("alert cleanup: auto-resolve failed", "error", err)
	}
	if deleted > 0 || autoResolved > 0 {
		h.log.Info("alert cleanup", "deleted", deleted, "auto_resolved", autoResolved)
	}
	return nil
}

const defaultMetricsAPIURL = "https://metrics.patchmon.cloud"

// MetricsSendHandler handles metrics-send jobs (automatic anonymous telemetry).
type MetricsSendHandler struct {
	defaultDB     *database.DB
	poolCache     *hostctx.PoolCache
	serverVersion string
	log           *slog.Logger
}

// NewMetricsSendHandler creates a metrics send handler.
func NewMetricsSendHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, serverVersion string, log *slog.Logger) *MetricsSendHandler {
	return &MetricsSendHandler{defaultDB: defaultDB, poolCache: poolCache, serverVersion: serverVersion, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *MetricsSendHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		if err := h.sendMetrics(ctx, d, host); err != nil {
			h.log.Warn("metrics send failed", "host", host, "error", err)
		}
	})
	h.log.Info("metrics send completed")
	return nil
}

func (h *MetricsSendHandler) sendMetrics(ctx context.Context, d *database.DB, tenantHost string) error {
	dbResolver := &hostctx.DBResolver{Default: d}
	settingsStore := store.NewSettingsStore(dbResolver)
	hostsStore := store.NewHostsStore(dbResolver)

	// Use WithDB so the store resolves to the correct context DB.
	ctx = hostctx.WithDB(ctx, d)

	s, err := settingsStore.GetFirst(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if !s.MetricsEnabled {
		return nil
	}
	if s.MetricsAnonymousID == nil || *s.MetricsAnonymousID == "" {
		return nil
	}

	hostCount, err := hostsStore.Count(ctx)
	if err != nil {
		return fmt.Errorf("count hosts: %w", err)
	}

	version := h.serverVersion
	if version == "" {
		version = "unknown"
	}

	metricsData := map[string]any{
		"anonymous_id": *s.MetricsAnonymousID,
		"host_count":   hostCount,
		"version":      version,
	}
	body, _ := json.Marshal(metricsData)

	apiURL := os.Getenv("METRICS_API_URL")
	if apiURL == "" {
		apiURL = defaultMetricsAPIURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/metrics/submit", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("prepare request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("metrics API returned status %d", resp.StatusCode)
	}

	now := time.Now()
	s.MetricsLastSent = &now
	_ = settingsStore.Update(ctx, s)

	h.log.Info("metrics sent", "host", tenantHost, "host_count", hostCount)
	return nil
}
