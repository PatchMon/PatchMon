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

	d := resolveDBFromPayload(ctx, t.Payload(), h.defaultDB, h.poolCache)

	// Use array comparison for proper semantic version ordering.
	// regexp_replace strips non-numeric suffixes (e.g. "79-1" → "79") before casting to int[].
	// COALESCE(NULLIF(...), '0') handles empty segments (e.g. "0.1." or malformed data) to avoid "invalid input syntax for type integer" errors.
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
		return err
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
			return err
		}
		hosts = append(hosts, oh)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(hosts) == 0 {
		h.log.Info("ssg-update-check: all hosts up to date", "server_version", serverVersion)
		return nil
	}

	// Resolve host for multi-tenant task routing.
	var hostField string
	var payload struct {
		Host string `json:"host"`
	}
	if err := json.Unmarshal(t.Payload(), &payload); err == nil {
		hostField = payload.Host
	}

	enqueued := 0
	for _, host := range hosts {
		task, err := NewSSGUpgradeTask(SSGUpgradePayload{
			HostID:     host.ID,
			ApiID:      host.ApiID,
			Host:       hostField,
			SSGVersion: serverVersion,
		})
		if err != nil {
			h.log.Warn("ssg-update-check: failed to create task", "host_id", host.ID, "error", err)
			continue
		}
		if _, err := h.queueClient.Enqueue(task); err != nil {
			// TaskID dedup may cause AlreadyExists — that's fine, an upgrade is already queued.
			if err != asynq.ErrDuplicateTask && err != asynq.ErrTaskIDConflict {
				h.log.Warn("ssg-update-check: enqueue failed", "host_id", host.ID, "error", err)
			}
			continue
		}
		enqueued++
	}

	h.log.Info("ssg-update-check completed",
		"server_version", serverVersion,
		"outdated_hosts", len(hosts),
		"enqueued", enqueued,
	)
	return nil
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
