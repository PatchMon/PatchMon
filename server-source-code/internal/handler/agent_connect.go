package handler

import (
	"context"
	"errors"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/alerts"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// Spreads catch-up reports on a fleet-wide reconnect; report_now uploads a full report.
const catchUpJitterWindow = 60 * time.Second

// NewAgentConnectHandler returns an OnAgentConnect callback that resolves host_down
// alerts when an agent's WebSocket reconnects, expedites any queued compliance
// scan for that host, and pulls a report from a host that has missed its cadence.
func NewAgentConnectHandler(db database.DBProvider, queueClient *asynq.Client, queueInspector *asynq.Inspector, emit *notifications.Emitter, log *slog.Logger) OnAgentConnect {
	return func(ctx context.Context, apiID string) {
		d := db.DB(ctx)
		if queueClient != nil {
			host, err := d.Queries.GetHostByApiID(ctx, apiID)
			if err == nil {
				if queueInspector != nil {
					expediteComplianceScan(queueClient, queueInspector, apiID, host.ID, log)
				}
				requestCatchUpReport(ctx, db, queueClient, apiID, host.Status, host.LastUpdate, log)
			}
		}

		alerts.OnConnect(ctx, d, apiID, hostctx.TenantHostKey(ctx), emit, log)
	}
}

func expediteComplianceScan(queueClient *asynq.Client, queueInspector *asynq.Inspector, apiID, hostID string, log *slog.Logger) {
	taskID := "compliance-scan-" + hostID
	info, err := queueInspector.GetTaskInfo(queue.QueueCompliance, taskID)
	if err != nil || (info.State != asynq.TaskStateScheduled && info.State != asynq.TaskStatePending) {
		return
	}
	_ = queueInspector.DeleteTask(queue.QueueCompliance, taskID)
	task, err := queue.NewRunScanTask(queue.RunScanPayload{
		HostID: hostID, ApiID: apiID, ProfileType: "all",
	})
	if err != nil {
		return
	}
	if _, err := queueClient.Enqueue(task, asynq.ProcessIn(0)); err == nil {
		log.Info("agent connect: expedited queued compliance scan", "api_id", apiID, "host_id", hostID)
	}
}

// The agent's ticker is monotonic and does not advance across suspend, so a host
// that slept through its interval will not report until the remainder elapses awake.
func requestCatchUpReport(ctx context.Context, dbp database.DBProvider, queueClient *asynq.Client, apiID, status string, lastUpdate pgtype.Timestamp, log *slog.Logger) {
	cutoff := store.OverdueCutoff(time.Now(), store.UpdateIntervalMinutesFromDB(ctx, dbp))
	if !isOverdue(status, lastUpdate, cutoff) {
		return
	}

	task, err := queue.NewCatchUpReportTask(apiID, hostctx.TenantHostKey(ctx), catchUpReportDelay(apiID))
	if err != nil {
		log.Debug("agent connect: failed to build catch-up report task", "api_id", apiID, "error", err)
		return
	}
	if _, err := queueClient.Enqueue(task); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			log.Debug("agent connect: catch-up report already queued", "api_id", apiID)
			return
		}
		log.Debug("agent connect: failed to enqueue catch-up report", "api_id", apiID, "error", err)
		return
	}
	log.Info("agent connect: requested catch-up report", "api_id", apiID, "last_update", lastUpdate.Time)
}

// last_update defaults to the row's creation time, so status is what excludes a
// host that enrolled and never reported.
func isOverdue(status string, lastUpdate pgtype.Timestamp, cutoff time.Time) bool {
	return status == store.StatusActive && lastUpdate.Valid && lastUpdate.Time.Before(cutoff)
}

func catchUpReportDelay(apiID string) time.Duration {
	slots := uint32(catchUpJitterWindow / time.Second)
	if slots == 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(apiID))
	return time.Duration(h.Sum32()%slots) * time.Second
}
