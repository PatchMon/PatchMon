package store

import (
	"context"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

// JobHistoryStore updates job_history rows outside the queue workers.
type JobHistoryStore struct {
	db database.DBProvider
}

// NewJobHistoryStore creates a new job history store.
func NewJobHistoryStore(db database.DBProvider) *JobHistoryStore {
	return &JobHistoryStore{db: db}
}

// RecordOutcome marks a job completed, or failed with failure as the reason.
func (s *JobHistoryStore) RecordOutcome(ctx context.Context, jobID, failure string) error {
	d := s.db.DB(ctx)
	if failure == "" {
		return d.Queries.UpdateJobHistoryCompleted(ctx, jobID)
	}
	return d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: jobID, ErrorMessage: &failure})
}
