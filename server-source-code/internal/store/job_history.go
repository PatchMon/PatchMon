package store

import (
	"context"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/google/uuid"
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

func (s *JobHistoryStore) HostHasJob(ctx context.Context, hostID, jobName string) (bool, error) {
	d := s.db.DB(ctx)
	return d.Queries.HostHasJobHistory(ctx, db.HostHasJobHistoryParams{HostID: &hostID, JobName: jobName})
}

func (s *JobHistoryStore) RecordCancelledBeforeStart(ctx context.Context, jobID, queueName, jobName, hostID, apiID string) error {
	d := s.db.DB(ctx)
	if err := d.Queries.InsertJobHistory(ctx, db.InsertJobHistoryParams{
		ID: uuid.New().String(), JobID: jobID, QueueName: queueName, JobName: jobName,
		HostID: &hostID, ApiID: &apiID, Status: "failed", AttemptNumber: 0,
	}); err != nil {
		return err
	}
	msg := "Cancelled before it started"
	return d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: jobID, ErrorMessage: &msg})
}
