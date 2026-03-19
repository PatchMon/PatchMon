package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/logger"
	"github.com/PatchMon/PatchMon/server-source-code/internal/migrate"
	"github.com/PatchMon/PatchMon/server-source-code/internal/monitor"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/redis"
	"github.com/PatchMon/PatchMon/server-source-code/internal/server"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/hibiken/asynq"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	// Bootstrap logger (env-only) for migrations and pre-DB errors
	bootstrapLog := logger.New(logger.Config{
		Enabled:    cfg.EnableLogging,
		Level:      cfg.LogLevel,
		JSONFormat: cfg.Env == "production",
	})
	bootstrapSlog := bootstrapLog.With("version", cfg.Version, "port", cfg.Port)

	ctx := context.Background()

	if err := migrate.Run(cfg.DatabaseURL, bootstrapSlog); err != nil {
		bootstrapSlog.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	db, err := database.NewDB(ctx, cfg)
	if err != nil {
		bootstrapSlog.Error("database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Resolve config (env → DB → default) and create real logger
	settingsStore := store.NewSettingsStore(db)
	settings, _ := settingsStore.GetFirst(ctx)
	resolved := config.ResolveConfig(ctx, cfg, settings)

	log := logger.New(logger.Config{
		Enabled:    resolved.EnableLogging,
		Level:      resolved.LogLevel,
		JSONFormat: cfg.Env == "production",
	})
	slog := log.With("version", cfg.Version, "port", cfg.Port)

	slog.Info("database connected")

	rdb := redis.NewClient()
	if err := redis.Ping(ctx, rdb); err != nil {
		slog.Error("redis", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redis.Close(rdb) }()
	slog.Info("redis connected")

	var ctxRegistry *hostctx.Registry
	var poolCache *hostctx.PoolCache
	var redisCache *hostctx.RedisCache
	if cfg.RegistryDatabaseURL != "" {
		// Poll interval is a failsafe - the primary path for registry updates is the
		// immediate reload webhook (POST /api/v1/internal/reload-registry-map) triggered
		// by the provisioner after every tenant create/update/delete.
		ctxRegistry, err = hostctx.NewRegistry(ctx, cfg.RegistryDatabaseURL, 60*time.Second, slog)
		if err != nil {
			slog.Error("context registry", "error", err)
			os.Exit(1)
		}
		defer ctxRegistry.Close()
		poolCache = hostctx.NewPoolCache(ctxRegistry, cfg, cfg.HostCacheTTLMin, slog)
		redisCache = hostctx.NewRedisCache(ctxRegistry, rdb, cfg.HostCacheTTLMin, slog)
		slog.Info("multi-host mode enabled", "registry_poll_interval", "60s")
	}

	// Validate encryption (required for bootstrap/install flow and notification destination secrets)
	enc, encErr := util.NewEncryption()
	if encErr != nil {
		slog.Error("encryption init failed (bootstrap tokens will be unavailable)", "error", encErr)
		slog.Info("hint: set DATABASE_URL, SESSION_SECRET, or AI_ENCRYPTION_KEY in environment")
	}

	registry := agentregistry.New()
	queueOpts := queue.RedisOpts()
	queueClient := queue.NewClient(queueOpts)
	defer func() { _ = queueClient.Close() }()

	queueInspector := asynq.NewInspector(queueOpts)
	defer func() { _ = queueInspector.Close() }()

	queueSrv := queue.NewServer(queueOpts, registry, db, slog)
	notifyEmit := notifications.NewEmitter(queueClient, rdb, slog)
	queueMux := queue.Mux(queue.MuxOpts{
		Registry:      registry,
		DB:            db,
		RDB:           rdb,
		RedisCache:    redisCache,
		PoolCache:     poolCache,
		QueueClient:   queueClient,
		ServerVersion: cfg.Version,
		SSGContentDir: cfg.SSGContentDir,
		Log:           slog,
		Emit:          notifyEmit,
		Enc:           enc,
	})
	go func() {
		if err := queueSrv.Run(queueMux); err != nil {
			slog.Error("queue server", "error", err)
		}
	}()
	slog.Info("queue server started")

	scheduler, err := queue.NewScheduler(queueOpts, slog)
	if err != nil {
		slog.Error("scheduler init", "error", err)
	} else {
		go func() {
			if err := scheduler.Run(); err != nil {
				slog.Error("scheduler", "error", err)
			}
		}()
		slog.Info("scheduler started")
	}

	// Fire an SSG update check shortly after startup so agents are notified
	// of new SSG content immediately after a server deployment, rather than
	// waiting for the next daily 5 AM scheduled run.
	go func() {
		time.Sleep(30 * time.Second)
		ssgTask := asynq.NewTask(queue.TypeSSGUpdateCheck, []byte("{}"))
		if _, err := queueClient.Enqueue(ssgTask, asynq.Queue(queue.QueueSSGUpdateCheck)); err != nil {
			slog.Debug("startup ssg-update-check enqueue skipped", "error", err)
		} else {
			slog.Info("startup ssg-update-check enqueued")
		}
	}()

	httpHandler, guacdProc := server.NewRouter(ctx, cfg, db, rdb, registry, queueClient, queueInspector, ctxRegistry, poolCache, redisCache, notifyEmit, slog, frontendFS)

	var memstatsCancel context.CancelFunc
	if cfg.EnablePprof {
		memstatsCtx, cancel := context.WithCancel(context.Background())
		memstatsCancel = cancel
		go monitor.StartMemStats(memstatsCtx, slog, cfg.MemstatsIntervalSec)
		slog.Info("pprof enabled", "memstats_interval_sec", cfg.MemstatsIntervalSec)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           httpHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout is 0 (unlimited) intentionally: the chi Timeout middleware
		// (30s) handles request deadlines at the handler level. A non-zero
		// WriteTimeout would kill WebSocket, SSE, and long-poll connections before
		// the handler middleware can send a proper timeout response.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")

	// Stop accepting new jobs and wait for in-flight jobs to finish.
	// Shutdown before HTTP server so agents can still report results during drain.
	if scheduler != nil {
		scheduler.Shutdown()
		slog.Info("scheduler stopped")
	}
	queueSrv.Shutdown()
	slog.Info("queue server stopped")

	if memstatsCancel != nil {
		memstatsCancel()
	}
	if guacdProc != nil {
		guacdProc.Stop()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "error", err)
	}
	slog.Info("server stopped")
}
