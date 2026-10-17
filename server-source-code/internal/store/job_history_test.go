package store

import (
	"context"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/google/uuid"
)

type jobRow struct {
	status      string
	errMsg      *string
	completedAt *time.Time
}

func TestJobHistoryUpdatesOnlyLatestRun(t *testing.T) {
	d := pgtest.Open(t)
	ctx := context.Background()
	jobs := NewJobHistoryStore(d)
	jobID := "compliance-scan-" + uuid.New().String()

	insert := func(age string) string {
		t.Helper()
		id := uuid.New().String()
		if err := d.Queries.InsertJobHistory(ctx, db.InsertJobHistoryParams{
			ID: id, JobID: jobID, QueueName: "compliance", JobName: "run_scan", Status: "active", AttemptNumber: 1,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Exec(ctx, `UPDATE job_history SET created_at = NOW() - $2::interval WHERE id = $1`, id, age); err != nil {
			t.Fatal(err)
		}
		return id
	}
	get := func(id string) jobRow {
		t.Helper()
		var r jobRow
		if err := d.RawQueryRow(ctx, `SELECT status, error_message, completed_at FROM job_history WHERE id = $1`, id).
			Scan(&r.status, &r.errMsg, &r.completedAt); err != nil {
			t.Fatal(err)
		}
		return r
	}
	t.Cleanup(func() { _, _ = d.Exec(context.Background(), `DELETE FROM job_history WHERE job_id = $1`, jobID) })

	first := insert("2 hours")
	if err := jobs.RecordOutcome(ctx, jobID, ""); err != nil {
		t.Fatal(err)
	}
	before := get(first)
	if before.status != "completed" || before.completedAt == nil {
		t.Fatalf("first run = %+v, want completed", before)
	}

	second := insert("1 hour")
	if err := d.Queries.UpdateJobHistoryDelayed(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if err := jobs.RecordOutcome(ctx, jobID, "agent offline"); err != nil {
		t.Fatal(err)
	}

	if got := get(first); got.status != "completed" || got.errMsg != nil || !got.completedAt.Equal(*before.completedAt) {
		t.Errorf("earlier run rewritten: %+v", got)
	}
	if got := get(second); got.status != "failed" || got.errMsg == nil || *got.errMsg != "agent offline" {
		t.Errorf("latest run = %+v, want failed with message", got)
	}

	if err := jobs.RecordOutcome(ctx, jobID, ""); err != nil {
		t.Fatal(err)
	}
	if got := get(second); got.status != "completed" {
		t.Errorf("late outcome must still settle a failed latest run, got %q", got.status)
	}
	if got := get(first); !got.completedAt.Equal(*before.completedAt) {
		t.Errorf("earlier run touched by late outcome: %+v", got)
	}
}
