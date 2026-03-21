package queue

import (
	"context"
	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// loggingHandler wraps an asynq.Handler with debug/error logging.
func loggingHandler(taskType string, h asynq.Handler, log *slog.Logger) asynq.Handler {
	if log == nil {
		return h
	}
	return &loggingHandlerImpl{taskType: taskType, inner: h, log: log}
}

type loggingHandlerImpl struct {
	taskType string
	inner    asynq.Handler
	log      *slog.Logger
}

func (h *loggingHandlerImpl) ProcessTask(ctx context.Context, t *asynq.Task) error {
	payload := string(t.Payload())
	if len(payload) > 200 {
		payload = payload[:200] + "..."
	}
	h.log.Debug("async task started", "type", h.taskType, "payload", payload)
	err := h.inner.ProcessTask(ctx, t)
	if err != nil {
		h.log.Error("async task failed", "type", h.taskType, "error", err, "payload", payload)
	} else {
		h.log.Debug("async task completed", "type", h.taskType)
	}
	return err
}

// NewServer creates an Asynq server with registered handlers.
func NewServer(opts asynq.RedisClientOpt, registry *agentregistry.Registry, db *database.DB, log *slog.Logger) *asynq.Server {
	srv := asynq.NewServer(opts, asynq.Config{
		Concurrency: 10,
		Queues: map[string]int{
			QueueAgentCommands:               3,
			QueueHostStatus:                  3,
			QueueAlertCleanup:                1,
			QueueSessionCleanup:              1,
			QueueOrphanedRepoCleanup:         1,
			QueueOrphanedPkgCleanup:          1,
			QueueDockerInvCleanup:            1,
			QueueSystemStatistics:            1,
			QueueVersionUpdateCheck:          1,
			QueueComplianceScanCleanup:       1,
			QueueSSGUpdateCheck:              1,
			QueueCompliance:                  2,
			QueuePatching:                    2,
			notifications.QueueNotifications: 2,
			QueueScheduledReports:            1,
		},
	})

	return srv
}

// MuxOpts configures the queue mux.
type MuxOpts struct {
	Registry      *agentregistry.Registry
	DB            *database.DB
	RDB           *redis.Client
	RedisCache    *hostctx.RedisCache // per-host Redis cache; nil in single-host mode
	PoolCache     *hostctx.PoolCache  // per-host DB pool; nil in single-host mode
	QueueClient   *asynq.Client
	ServerVersion string
	SSGContentDir string
	Log           *slog.Logger
	Emit          *notifications.Emitter
	Enc           *util.Encryption
	Timezone      string // IANA timezone from resolved config; used for scheduled report cron
}

// Mux returns a ServeMux with all handlers registered.
// When log is set and LOG_LEVEL=debug, task start/complete/error are logged.
func Mux(opts MuxOpts) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	registry, db, log := opts.Registry, opts.DB, opts.Log
	wrap := func(typ string, h asynq.Handler) asynq.Handler { return loggingHandler(typ, h, log) }
	mux.Handle(TypeReportNow, wrap(TypeReportNow, NewReportNowHandler(registry, db, log)))
	mux.Handle(TypeRefreshIntegrationStatus, wrap(TypeRefreshIntegrationStatus, NewRefreshIntegrationStatusHandler(registry, db, log)))
	mux.Handle(TypeDockerInventoryRefresh, wrap(TypeDockerInventoryRefresh, NewDockerInventoryRefreshHandler(registry, db, log)))
	mux.Handle(TypeUpdateAgent, wrap(TypeUpdateAgent, NewUpdateAgentHandler(registry, db, log)))
	dbResolver := &hostctx.DBResolver{Default: db}
	mux.Handle(TypeHostStatusMonitor, wrap(TypeHostStatusMonitor, NewHostStatusMonitorHandler(db, opts.PoolCache, opts.Emit, log)))
	mux.Handle(notifications.TypeNotificationDeliver, wrap(notifications.TypeNotificationDeliver, NewNotificationDeliverHandler(db, opts.PoolCache, opts.Enc, opts.RDB, log)))
	mux.Handle(TypeScheduledReportsDispatch, wrap(TypeScheduledReportsDispatch, NewScheduledReportsDispatchHandler(db, opts.PoolCache, opts.QueueClient, log)))
	mux.Handle(TypeScheduledReportRun, wrap(TypeScheduledReportRun, NewScheduledReportRunHandler(db, opts.PoolCache, opts.Enc, opts.Timezone, log)))
	mux.Handle(TypeAlertCleanup, wrap(TypeAlertCleanup, NewAlertCleanupHandler(db, opts.PoolCache, store.NewAlertConfigStore(dbResolver), log)))
	mux.Handle(TypeSessionCleanup, wrap(TypeSessionCleanup, NewSessionCleanupHandler(db, opts.PoolCache, log)))
	mux.Handle(TypeOrphanedRepoCleanup, wrap(TypeOrphanedRepoCleanup, NewOrphanedRepoCleanupHandler(db, opts.PoolCache, log)))
	mux.Handle(TypeOrphanedPkgCleanup, wrap(TypeOrphanedPkgCleanup, NewOrphanedPkgCleanupHandler(db, opts.PoolCache, log)))
	mux.Handle(TypeDockerInvCleanup, wrap(TypeDockerInvCleanup, NewDockerInvCleanupHandler(db, opts.PoolCache, log)))
	mux.Handle(TypeSystemStatistics, wrap(TypeSystemStatistics, NewSystemStatisticsHandler(db, opts.PoolCache, log)))
	mux.Handle(TypeVersionUpdateCheck, wrap(TypeVersionUpdateCheck, NewVersionUpdateCheckHandler(db, opts.PoolCache, opts.ServerVersion, opts.Emit, log)))
	mux.Handle(TypeComplianceScanCleanup, wrap(TypeComplianceScanCleanup, NewComplianceScanCleanupHandler(db, opts.PoolCache, log)))
	mux.Handle(TypeSSGUpdateCheck, wrap(TypeSSGUpdateCheck, NewSSGUpdateCheckHandler(registry, db, opts.PoolCache, opts.QueueClient, opts.SSGContentDir, log)))
	mux.Handle(TypeSSGUpgrade, wrap(TypeSSGUpgrade, NewSSGUpgradeHandler(registry, db, opts.PoolCache, log)))
	complianceStore := store.NewComplianceStore(db)
	var integrationStatusStore *store.IntegrationStatusStore
	if opts.RDB != nil {
		integrationStatusStore = store.NewIntegrationStatusStore(&hostctx.RedisResolver{Default: opts.RDB})
	}
	mux.Handle(TypeRunScan, wrap(TypeRunScan, NewRunScanHandler(registry, db, opts.PoolCache, complianceStore, opts.QueueClient, integrationStatusStore, log)))
	mux.Handle(TypeInstallComplianceTools, wrap(TypeInstallComplianceTools, NewInstallComplianceToolsHandler(registry, db, opts.RDB, opts.RedisCache, log)))
	patchRunsStore := store.NewPatchRunsStore(&hostctx.DBResolver{Default: db})
	mux.Handle(TypeRunPatch, wrap(TypeRunPatch, NewRunPatchHandler(registry, patchRunsStore, opts.PoolCache, opts.QueueClient, log)))
	return mux
}

// NewScheduler creates a scheduler with periodic tasks registered.
func NewScheduler(opts asynq.RedisClientOpt, log *slog.Logger) (*asynq.Scheduler, error) {
	scheduler := asynq.NewScheduler(opts, nil)

	hostStatusTask := asynq.NewTask(TypeHostStatusMonitor, nil)
	if _, err := scheduler.Register("*/5 * * * *", hostStatusTask, asynq.Queue(QueueHostStatus), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	alertCleanupTask := asynq.NewTask(TypeAlertCleanup, nil)
	if _, err := scheduler.Register("0 3 * * *", alertCleanupTask, asynq.Queue(QueueAlertCleanup), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	sessionCleanupTask := asynq.NewTask(TypeSessionCleanup, nil)
	if _, err := scheduler.Register("0 * * * *", sessionCleanupTask, asynq.Queue(QueueSessionCleanup), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	orphanedRepoTask := asynq.NewTask(TypeOrphanedRepoCleanup, nil)
	if _, err := scheduler.Register("0 2 * * *", orphanedRepoTask, asynq.Queue(QueueOrphanedRepoCleanup), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	orphanedPkgTask := asynq.NewTask(TypeOrphanedPkgCleanup, nil)
	if _, err := scheduler.Register("0 3 * * *", orphanedPkgTask, asynq.Queue(QueueOrphanedPkgCleanup), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	dockerInvTask := asynq.NewTask(TypeDockerInvCleanup, nil)
	if _, err := scheduler.Register("0 4 * * *", dockerInvTask, asynq.Queue(QueueDockerInvCleanup), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	systemStatsTask := asynq.NewTask(TypeSystemStatistics, nil)
	if _, err := scheduler.Register("*/30 * * * *", systemStatsTask, asynq.Queue(QueueSystemStatistics), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	versionUpdateTask := asynq.NewTask(TypeVersionUpdateCheck, nil)
	if _, err := scheduler.Register("0 0 * * *", versionUpdateTask, asynq.Queue(QueueVersionUpdateCheck), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	complianceScanTask := asynq.NewTask(TypeComplianceScanCleanup, nil)
	if _, err := scheduler.Register("0 1 * * *", complianceScanTask, asynq.Queue(QueueComplianceScanCleanup), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	ssgUpdateTask := asynq.NewTask(TypeSSGUpdateCheck, nil)
	if _, err := scheduler.Register("0 5 * * *", ssgUpdateTask, asynq.Queue(QueueSSGUpdateCheck), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	dispatchReports := asynq.NewTask(TypeScheduledReportsDispatch, nil)
	if _, err := scheduler.Register("* * * * *", dispatchReports, asynq.Queue(QueueScheduledReports), asynq.Retention(AutomationRetention)); err != nil {
		return nil, err
	}

	log.Info("scheduler: registered all automation tasks")
	return scheduler, nil
}
