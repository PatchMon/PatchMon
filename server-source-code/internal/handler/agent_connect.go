package handler

import (
	"context"
	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/alerts"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/hibiken/asynq"
)

// NewAgentConnectHandler returns an OnAgentConnect callback that resolves host_down
// alerts when an agent's WebSocket reconnects, and expedites any queued compliance
// scan for that host.
func NewAgentConnectHandler(db database.DBProvider, queueClient *asynq.Client, queueInspector *asynq.Inspector, emit *notifications.Emitter, log *slog.Logger) OnAgentConnect {
	return func(ctx context.Context, apiID string) {
		d := db.DB(ctx)
		// Expedite queued compliance scan when agent connects
		if queueClient != nil && queueInspector != nil {
			host, err := d.Queries.GetHostByApiID(ctx, apiID)
			if err == nil {
				taskID := "compliance-scan-" + host.ID
				info, err := queueInspector.GetTaskInfo(queue.QueueCompliance, taskID)
				if err == nil && (info.State == asynq.TaskStateScheduled || info.State == asynq.TaskStatePending) {
					_ = queueInspector.DeleteTask(queue.QueueCompliance, taskID)
					task, err := queue.NewRunScanTask(queue.RunScanPayload{
						HostID: host.ID, ApiID: apiID, ProfileType: "all",
					})
					if err == nil {
						if _, err := queueClient.Enqueue(task, asynq.ProcessIn(0)); err == nil {
							log.Info("agent connect: expedited queued compliance scan", "api_id", apiID, "host_id", host.ID)
						}
					}
				}
			}
		}

		alerts.OnConnect(ctx, d, apiID, hostctx.TenantHostKey(ctx), emit, log)
	}
}
