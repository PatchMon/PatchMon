package handler

import (
	"context"
	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/alerts"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
)

// NewAgentDisconnectHandler returns an OnAgentDisconnect callback that creates host_down
// alerts when an agent's WebSocket disconnects. This enables immediate alerting in the
// Reporting module when the real-time connection is lost (vs. waiting for the host
// status monitor to detect stale last_update).
func NewAgentDisconnectHandler(db database.DBProvider, log *slog.Logger) OnAgentDisconnect {
	return func(ctx context.Context, apiID string) {
		alerts.OnDisconnect(ctx, db.DB(ctx), apiID, log)
	}
}
