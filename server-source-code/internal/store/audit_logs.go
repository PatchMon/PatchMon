package store

import (
	"context"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/google/uuid"
)

// AuditLogsStore writes security audit records.
type AuditLogsStore struct {
	db database.DBProvider
}

// NewAuditLogsStore creates a new audit logs store.
func NewAuditLogsStore(db database.DBProvider) *AuditLogsStore {
	return &AuditLogsStore{db: db}
}

// Insert writes one audit record. A blank ID is generated.
func (s *AuditLogsStore) Insert(ctx context.Context, e *models.AuditLog) error {
	return insertAuditLog(ctx, s.db.DB(ctx).Queries, e)
}

func insertAuditLog(ctx context.Context, q *db.Queries, e *models.AuditLog) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ID:           e.ID,
		Event:        e.Event,
		UserID:       e.UserID,
		TargetUserID: e.TargetUserID,
		IpAddress:    e.IPAddress,
		UserAgent:    e.UserAgent,
		RequestID:    e.RequestID,
		Details:      e.Details,
		Success:      e.Success,
	})
}
