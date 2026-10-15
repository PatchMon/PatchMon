package store

import (
	"context"
	"encoding/json"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/google/uuid"
)

// AuditEntry is one security audit record. Empty strings are stored as NULL.
type AuditEntry struct {
	Event        string
	ActorUserID  string
	TargetUserID string
	IPAddress    string
	UserAgent    string
	RequestID    string
	Success      bool
	Metadata     map[string]any
}

// AuditLogsStore writes security audit records.
type AuditLogsStore struct {
	db database.DBProvider
}

// NewAuditLogsStore creates a new audit logs store.
func NewAuditLogsStore(db database.DBProvider) *AuditLogsStore {
	return &AuditLogsStore{db: db}
}

// Record writes one audit record.
func (s *AuditLogsStore) Record(ctx context.Context, e AuditEntry) error {
	return insertAuditLog(ctx, s.db.DB(ctx).Queries, e)
}

func insertAuditLog(ctx context.Context, q *db.Queries, e AuditEntry) error {
	var details *string
	if len(e.Metadata) > 0 {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return err
		}
		s := string(b)
		details = &s
	}
	return q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ID:           uuid.New().String(),
		Event:        e.Event,
		UserID:       nilIfEmpty(e.ActorUserID),
		TargetUserID: nilIfEmpty(e.TargetUserID),
		IpAddress:    nilIfEmpty(e.IPAddress),
		UserAgent:    nilIfEmpty(e.UserAgent),
		RequestID:    nilIfEmpty(e.RequestID),
		Details:      details,
		Success:      e.Success,
	})
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
