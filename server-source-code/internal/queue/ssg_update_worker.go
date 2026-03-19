package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// SSGUpdateCheckHandler handles ssg-update-check jobs.
// It reads the server's embedded SSG version, queries for hosts with an older
// version, and enqueues per-host ssg_upgrade jobs so each gets job_history tracking.
type SSGUpdateCheckHandler struct {
	registry      *agentregistry.Registry
	defaultDB     *database.DB
	poolCache     *hostctx.PoolCache
	queueClient   *asynq.Client
	ssgContentDir string
	log           *slog.Logger
}

// NewSSGUpdateCheckHandler creates an SSG update check handler.
func NewSSGUpdateCheckHandler(registry *agentregistry.Registry, defaultDB *database.DB, poolCache *hostctx.PoolCache, queueClient *asynq.Client, ssgContentDir string, log *slog.Logger) *SSGUpdateCheckHandler {
	return &SSGUpdateCheckHandler{
		registry:      registry,
		defaultDB:     defaultDB,
		poolCache:     poolCache,
		queueClient:   queueClient,
		ssgContentDir: ssgContentDir,
		log:           log,
	}
}

// ProcessTask implements asynq.Handler.
func (h *SSGUpdateCheckHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	serverVersion := h.readSSGVersion()
	if serverVersion == "" {
		h.log.Warn("ssg-update-check: no .ssg-version file found, skipping")
		return nil
	}

	if len(t.Payload()) > 0 {
		d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
		th := tenantHostFromPayload(t.Payload())
		n := h.checkDB(ctx, d, th, serverVersion)
		h.log.Info("ssg-update-check completed", "server_version", serverVersion, "enqueued", n)
		return nil
	}

	totalEnqueued := 0
	forEachDB(ctx, h.defaultDB, h.poolCache, func(ctx context.Context, d *database.DB, host string) {
		totalEnqueued += h.checkDB(ctx, d, host, serverVersion)
	})
	h.log.Info("ssg-update-check completed", "server_version", serverVersion, "enqueued", totalEnqueued)
	return nil
}

func (h *SSGUpdateCheckHandler) checkDB(ctx context.Context, d *database.DB, tenantHost, serverVersion string) int {
	// Use array comparison for proper semantic version ordering.
	const query = `
		SELECT h.id, h.api_id
		FROM hosts h
		WHERE h.compliance_enabled = true
		  AND h.status = 'active'
		  AND (
		    h.compliance_scanner_status IS NULL
		    OR h.compliance_scanner_status->'scanner_info'->>'ssg_version' IS NULL
		    OR h.compliance_scanner_status->'scanner_info'->>'ssg_version' = ''
		    OR (SELECT array_agg(COALESCE(NULLIF(regexp_replace(elem, '[^0-9].*', ''), ''), '0')::int) FROM unnest(string_to_array(h.compliance_scanner_status->'scanner_info'->>'ssg_version', '.')) AS elem)
		       < (SELECT array_agg(COALESCE(NULLIF(regexp_replace(elem, '[^0-9].*', ''), ''), '0')::int) FROM unnest(string_to_array($1, '.')) AS elem)
		  )`

	rows, err := d.Raw(ctx, query, serverVersion)
	if err != nil {
		h.log.Warn("ssg-update-check: query failed", "host", tenantHost, "error", err)
		return 0
	}
	defer rows.Close()

	type outdatedHost struct {
		ID    string
		ApiID string
	}
	var hosts []outdatedHost
	for rows.Next() {
		var oh outdatedHost
		if err := rows.Scan(&oh.ID, &oh.ApiID); err != nil {
			h.log.Warn("ssg-update-check: scan failed", "error", err)
			return 0
		}
		hosts = append(hosts, oh)
	}
	if err := rows.Err(); err != nil {
		return 0
	}
	if len(hosts) == 0 {
		return 0
	}

	enqueued := 0
	for _, host := range hosts {
		task, err := NewSSGUpgradeTask(SSGUpgradePayload{
			HostID:     host.ID,
			ApiID:      host.ApiID,
			Host:       tenantHost,
			SSGVersion: serverVersion,
		})
		if err != nil {
			h.log.Warn("ssg-update-check: failed to create task", "host_id", host.ID, "error", err)
			continue
		}
		if _, err := h.queueClient.Enqueue(task); err != nil {
			if err != asynq.ErrDuplicateTask && err != asynq.ErrTaskIDConflict {
				h.log.Warn("ssg-update-check: enqueue failed", "host_id", host.ID, "error", err)
			}
			continue
		}
		enqueued++
	}
	return enqueued
}

// SSGUpgradeHandler handles per-host ssg_upgrade jobs.
// It sends the upgrade_ssg command to the agent and records job_history.
type SSGUpgradeHandler struct {
	registry  *agentregistry.Registry
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	log       *slog.Logger
}

// NewSSGUpgradeHandler creates an SSG upgrade handler.
func NewSSGUpgradeHandler(registry *agentregistry.Registry, defaultDB *database.DB, poolCache *hostctx.PoolCache, log *slog.Logger) *SSGUpgradeHandler {
	return &SSGUpgradeHandler{
		registry:  registry,
		defaultDB: defaultDB,
		poolCache: poolCache,
		log:       log,
	}
}

// ProcessTask implements asynq.Handler.
func (h *SSGUpgradeHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p SSGUpgradePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}

	d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)
	taskID, _ := asynq.GetTaskID(ctx)
	retryCount, _ := asynq.GetRetryCount(ctx)
	attempt := int32(retryCount + 1)

	// Record job_history on first attempt.
	if d != nil && taskID != "" && retryCount == 0 {
		apiIDPtr := &p.ApiID
		hostIDPtr := &p.HostID
		_ = d.Queries.InsertJobHistory(ctx, db.InsertJobHistoryParams{
			ID:            uuid.New().String(),
			JobID:         taskID,
			QueueName:     QueueCompliance,
			JobName:       TypeSSGUpgrade,
			HostID:        hostIDPtr,
			ApiID:         apiIDPtr,
			Status:        "active",
			AttemptNumber: attempt,
		})
	}

	conn := h.registry.GetConnection(p.ApiID)
	if conn == nil {
		h.log.Warn("ssg_upgrade: agent not connected", "api_id", p.ApiID, "host_id", p.HostID)
		if taskID != "" && d != nil {
			msg := "Agent not connected"
			_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &msg})
		}
		return nil
	}

	msg, _ := json.Marshal(map[string]string{
		"type":    "upgrade_ssg",
		"version": p.SSGVersion,
	})
	if err := conn.WriteMessage(1, msg); err != nil {
		h.log.Warn("ssg_upgrade: write failed", "api_id", p.ApiID, "error", err)
		if taskID != "" && d != nil {
			errMsg := "Failed to send upgrade command to agent"
			_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &errMsg})
		}
		return err
	}

	if taskID != "" && d != nil {
		_ = d.Queries.UpdateJobHistoryCompleted(ctx, taskID)
	}
	h.log.Info("ssg_upgrade sent", "api_id", p.ApiID, "host_id", p.HostID, "version", p.SSGVersion)
	return nil
}

func (h *SSGUpdateCheckHandler) readSSGVersion() string {
	if h.ssgContentDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(h.ssgContentDir, ".ssg-version"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
