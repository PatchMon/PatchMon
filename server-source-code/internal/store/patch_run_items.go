package store

import (
	"context"
	"encoding/json"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

// Patch run item kinds.
const (
	PatchItemKindWUA    = "wua"
	PatchItemKindWinget = "winget"
)

// NewPatchRunItem is a target written to patch_run_items when a run is created.
type NewPatchRunItem struct {
	Kind           string `json:"kind"`
	Identifier     string `json:"identifier"`
	RevisionNumber int32  `json:"revision_number"`
	Title          string `json:"title"`
	KB             string `json:"kb"`
}

func insertPatchRunItems(ctx context.Context, q *db.Queries, runID, hostID string, items []NewPatchRunItem) error {
	if len(items) == 0 {
		return nil
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return q.InsertPatchRunItems(ctx, db.InsertPatchRunItemsParams{PatchRunID: runID, HostID: hostID, Payload: payload})
}

// ResolveWindowsPatchTargets looks up the host's own host_packages rows for the given display names.
func (s *PatchRunsStore) ResolveWindowsPatchTargets(ctx context.Context, hostID string, names []string) ([]db.ResolveWindowsPatchTargetsRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ResolveWindowsPatchTargets(ctx, db.ResolveWindowsPatchTargetsParams{HostID: hostID, Names: names})
}

// ListItems returns every item recorded against a run.
func (s *PatchRunsStore) ListItems(ctx context.Context, runID string) ([]db.PatchRunItem, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListPatchRunItems(ctx, runID)
}

// ListPendingItems returns the run's items that the agent has not reported on yet.
func (s *PatchRunsStore) ListPendingItems(ctx context.Context, runID string) ([]db.PatchRunItem, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListPendingPatchRunItems(ctx, runID)
}

// ReplaceItems swaps a run's items for a freshly resolved set in one transaction.
func (s *PatchRunsStore) ReplaceItems(ctx context.Context, runID, hostID string, items []NewPatchRunItem) error {
	d := s.db.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()
	q := d.Queries.WithTx(tx)
	if err := q.DeletePatchRunItems(ctx, runID); err != nil {
		return err
	}
	if err := insertPatchRunItems(ctx, q, runID, hostID, items); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FailRun marks a run that never reached the agent as failed.
func (s *PatchRunsStore) FailRun(ctx context.Context, id, errorMessage string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdatePatchRunFailed(ctx, db.UpdatePatchRunFailedParams{ID: id, ShellOutput: "", ErrorMessage: &errorMessage})
}
