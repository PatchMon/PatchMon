package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/clientip"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// recordResourceAudit records identifiers only; request bodies may contain secrets.
func recordResourceAudit(r *http.Request, audit *store.AuditLogsStore, resource, id, action string) {
	if audit == nil {
		return
	}
	actorID, _ := r.Context().Value(middleware.UserIDKey).(string)
	details := map[string]any{"resource": resource, "resource_id": id}
	if token := middleware.GetApiToken(r.Context()); token != nil {
		details["api_token_id"] = token.ID
	}
	entry := newAuditEntry(r, resource+"_"+action, actorID, "", true, details)
	if err := audit.Record(r.Context(), entry); err != nil {
		slog.Error("Failed to record resource audit", "resource", resource, "resource_id", id, "action", action, "error", err)
	}
}

// newAuditEntry fills the request-derived fields of an audit record.
func newAuditEntry(r *http.Request, event, actorID, targetID string, success bool, metadata map[string]any) store.AuditEntry {
	return store.AuditEntry{
		Event:        event,
		ActorUserID:  actorID,
		TargetUserID: truncateForLog(targetID, 64),
		IPAddress:    requestClientIP(r),
		UserAgent:    truncateForLog(r.UserAgent(), 512),
		RequestID:    requestIDFrom(r.Context()),
		Success:      success,
		Metadata:     metadata,
	}
}

func requestIDFrom(ctx context.Context) string {
	reqID, _ := ctx.Value(middleware.RequestIDKey).(string)
	return truncateForLog(reqID, 64)
}

func requestClientIP(r *http.Request) string {
	if ip := clientip.FromRequest(r); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
