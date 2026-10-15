package handler

import (
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/clientip"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// newAuditEntry fills the request-derived fields of an audit record.
func newAuditEntry(r *http.Request, event, actorID, targetID string, success bool, metadata map[string]any) store.AuditEntry {
	reqID, _ := r.Context().Value(middleware.RequestIDKey).(string)
	return store.AuditEntry{
		Event:        event,
		ActorUserID:  actorID,
		TargetUserID: truncateForLog(targetID, 64),
		IPAddress:    requestClientIP(r),
		UserAgent:    truncateForLog(r.UserAgent(), 512),
		RequestID:    truncateForLog(reqID, 64),
		Success:      success,
		Metadata:     metadata,
	}
}

func requestClientIP(r *http.Request) string {
	if ip := clientip.FromRequest(r); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
