package queue

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/ssgcontent"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
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
		// Absent content and unnameable content both stop the sweep, but they
		// need different fixes, and an operator looking at a directory full of
		// datastreams is not helped by being told there are none.
		if len(ssgcontent.Files(h.ssgContentDir)) == 0 {
			h.log.Warn("ssg-update-check: no SSG content on server, skipping", "dir", h.ssgContentDir)
		} else {
			h.log.Warn("ssg-update-check: SSG content present but its release could not be determined, skipping", "dir", h.ssgContentDir)
		}
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
	// Gate on the scanner binary, not on scanner availability. A host reports
	// openscap_available = false when the binary is fine but no datastream is on
	// disk, which is exactly the host that needs a content push; gating on it
	// would permanently strand any host whose first sync failed, because the
	// agent only ever acts on a server-pushed command and never retries by
	// itself. openscap_version is set from `oscap --version` before the content
	// check, so it means "scanner installed" regardless of content.
	//
	// A host with no scanner at all is excluded: it needs install_scanner, and
	// under the old NULL/empty arms it re-qualified on every sweep forever.
	//
	// on-demand hosts are deliberately still included. That flag governs scan
	// scheduling, not content distribution: an on-demand host still scans when
	// asked and needs current content to do it. A push is now one datastream
	// from this server, version-checked and skipped when already current, so
	// there is nothing to opt out of. #841 is the fix for on-demand hosts doing
	// unrequested work.
	//
	// Version ordering uses array comparison so 0.1.9 sorts below 0.1.10.
	//
	// Components cast to numeric rather than int deliberately. int4 stops at ten
	// digits and errors rather than wrapping, and the left-hand side is the
	// host's self-reported ssg_version, stored verbatim from the agent, so an
	// eleven-digit component from a single host would error this query and stop
	// SSG update checks for its whole context behind one log line. numeric cannot
	// overflow and compares element-wise identically.
	const query = `
		SELECT h.id, h.api_id
		FROM hosts h
		WHERE h.compliance_enabled = true
		  AND h.status = 'active'
		  AND COALESCE(h.compliance_scanner_status->'scanner_info'->>'openscap_version', '') <> ''
		  AND (
		    COALESCE(h.compliance_scanner_status->'scanner_info'->>'ssg_version', '') = ''
		    OR (SELECT array_agg(COALESCE(NULLIF(regexp_replace(elem, '[^0-9].*', ''), ''), '0')::numeric) FROM unnest(string_to_array(h.compliance_scanner_status->'scanner_info'->>'ssg_version', '.')) AS elem)
		       < (SELECT array_agg(COALESCE(NULLIF(regexp_replace(elem, '[^0-9].*', ''), ''), '0')::numeric) FROM unnest(string_to_array($1, '.')) AS elem)
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

	if !h.registry.IsConnected(p.ApiID) {
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
	if err := h.registry.SendMessage(p.ApiID, websocket.TextMessage, msg); err != nil {
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
	return ssgcontent.Version(h.ssgContentDir)
}
