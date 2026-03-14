package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/alerts"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
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

// HostStatusMonitorHandler handles host-status-monitor jobs.
type HostStatusMonitorHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewHostStatusMonitorHandler creates a host status monitor handler.
func NewHostStatusMonitorHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *HostStatusMonitorHandler {
	return &HostStatusMonitorHandler{defaultDB: defaultDB, poolCache: poolCache, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *HostStatusMonitorHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	payload := t.Payload()

	// Payload has host: single-host or manual trigger with host
	if len(payload) > 0 {
		db := resolveDBFromPayload(ctx, payload, h.defaultDB, h.poolCache)
		alertsCreated, err := alerts.ProcessHostStatusMonitor(ctx, db, h.log)
		if err != nil {
			return err
		}
		h.log.Info("host status monitor completed", "alerts_created", alertsCreated)
		return nil
	}

	// Empty payload: single-host (PoolCache nil) or multi-host fan-out
	if h.poolCache == nil {
		alertsCreated, err := alerts.ProcessHostStatusMonitor(ctx, h.defaultDB, h.log)
		if err != nil {
			return err
		}
		h.log.Info("host status monitor completed", "alerts_created", alertsCreated)
		return nil
	}

	// Multi-host: process defaultDB first, then all registry hosts in parallel
	var totalCreated atomic.Int64
	if n, err := alerts.ProcessHostStatusMonitor(ctx, h.defaultDB, h.log); err == nil {
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
			if n, err := alerts.ProcessHostStatusMonitor(ctx, db, h.log); err == nil {
				totalCreated.Add(int64(n))
			}
		}()
	}
	wg.Wait()

	h.log.Info("host status monitor completed", "alerts_created", totalCreated.Load(), "hosts_processed", len(hosts)+1)
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
	db := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	err := db.Queries.DeleteExpiredSessions(ctx)
	if err != nil {
		return err
	}
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
	db := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	ctx = hostctx.WithDB(ctx, db)
	repos := store.NewRepositoriesStore(&hostctx.DBResolver{Default: h.defaultDB})
	_, count, err := repos.CleanupOrphaned(ctx)
	if err != nil {
		return err
	}
	h.log.Info("orphaned repo cleanup completed", "deleted", count)
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
	db := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	orphaned, err := db.Queries.ListOrphanedPackages(ctx)
	if err != nil {
		return err
	}
	if len(orphaned) == 0 {
		h.log.Info("orphaned package cleanup completed", "deleted", 0)
		return nil
	}
	ids := make([]string, len(orphaned))
	for i, pkg := range orphaned {
		ids[i] = pkg.ID
	}
	if err := db.Queries.DeletePackagesByIDs(ctx, ids); err != nil {
		return err
	}
	h.log.Info("orphaned package cleanup completed", "deleted", len(ids))
	return nil
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
	db := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	containers, err := db.Queries.ListOrphanedContainers(ctx)
	if err != nil {
		return err
	}
	containerIDs := make([]string, len(containers))
	for i, c := range containers {
		containerIDs[i] = c.ID
	}
	if len(containerIDs) > 0 {
		if err := db.Queries.DeleteContainersByIDs(ctx, containerIDs); err != nil {
			return err
		}
		h.log.Info("deleted orphaned containers", "count", len(containerIDs))
	}

	images, err := db.Queries.ListOrphanedImages(ctx)
	if err != nil {
		return err
	}
	for _, img := range images {
		_ = db.Queries.DeleteImageUpdatesByImageID(ctx, img.ID)
		if err := db.Queries.DeleteImageByID(ctx, img.ID); err != nil {
			h.log.Warn("failed to delete orphaned image", "id", img.ID, "error", err)
			continue
		}
	}
	if len(images) > 0 {
		h.log.Info("deleted orphaned images", "count", len(images))
	}
	h.log.Info("docker inventory cleanup completed")
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
	d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	stats, err := d.Queries.GetSystemStatsForInsert(ctx)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("sys-%d", time.Now().UnixMilli())
	err = d.Queries.InsertSystemStatistics(ctx, db.InsertSystemStatisticsParams{
		ID:                  id,
		UniquePackagesCount: stats.Column1,
		UniqueSecurityCount: stats.Column2,
		TotalPackages:       stats.Column3,
		TotalHosts:          stats.Column4,
		HostsNeedingUpdates: stats.Column5,
	})
	if err != nil {
		return err
	}
	h.log.Info("system statistics completed")
	return nil
}

// VersionUpdateCheckHandler handles version-update-check jobs.
type VersionUpdateCheckHandler struct {
	defaultDB     *database.DB
	poolCache     *hostctx.PoolCache
	serverVersion string
	log           *slog.Logger
}

// NewVersionUpdateCheckHandler creates a version update check handler.
func NewVersionUpdateCheckHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, serverVersion string, log *slog.Logger) *VersionUpdateCheckHandler {
	return &VersionUpdateCheckHandler{defaultDB: defaultDB, poolCache: poolCache, serverVersion: serverVersion, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *VersionUpdateCheckHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	if err := alerts.ProcessServerUpdate(ctx, d, h.serverVersion, h.log); err != nil {
		return err
	}
	agentsDir := util.GetAgentsDir()
	if err := alerts.ProcessAgentUpdate(ctx, d, agentsDir, h.log); err != nil {
		return err
	}
	h.log.Info("version update check completed")
	return nil
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
	d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	threshold := time.Now().Add(-3 * time.Hour)
	pgThreshold := pgtype.Timestamp{Time: threshold, Valid: true}
	msg := "Scan terminated automatically after running for more than 3 hours"
	err := d.Queries.UpdateStalledComplianceScans(ctx, db.UpdateStalledComplianceScansParams{
		StartedAt:    pgThreshold,
		ErrorMessage: &msg,
	})
	if err != nil {
		return err
	}
	h.log.Info("compliance scan cleanup completed")
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
	d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	ctx = hostctx.WithDB(ctx, d)
	settings, err := d.Queries.GetFirstSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.AlertsEnabled {
		h.log.Debug("alert cleanup: alerts disabled, skipping")
		return nil
	}

	deleted, err := h.alertConfig.CleanupOldAlerts(ctx)
	if err != nil {
		return err
	}

	autoResolved, err := h.alertConfig.AutoResolveOldAlerts(ctx)
	if err != nil {
		h.log.Warn("alert cleanup: auto-resolve failed", "error", err)
		// Don't fail the whole task - deletion succeeded
	}

	h.log.Info("alert cleanup completed", "deleted", deleted, "auto_resolved", autoResolved)
	return nil
}
