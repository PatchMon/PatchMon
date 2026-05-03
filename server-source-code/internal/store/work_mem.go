package store

import (
	"context"
	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

// withWorkMemTx opens a short-lived transaction, raises Postgres `work_mem`
// to 32 MB for the duration, and invokes fn with a sqlc Queries bound to
// that tx. The bump matters for queries that aggregate or sort large
// host_packages slices: at 1k+ host scale the default 4 MB work_mem
// forces an external on-disk sort (e.g. 95 MB temp / 33 batches), which
// dominates request latency. 32 MB keeps the working set in memory and
// drops the slow paths from many seconds to a few hundred ms.
//
// If `SET LOCAL` fails (some connection poolers / pgbouncer transaction
// modes refuse session-scope GUCs even inside an explicit tx), the
// failed tx is rolled back and we open a fresh tx without the bump —
// the query still runs, just slowly. This degrades cleanly rather than
// 500ing the request.
//
// Anything fn returns is propagated; success commits, error rolls back.
func withWorkMemTx(ctx context.Context, p database.DBProvider, fn func(q *db.Queries) error) error {
	d := p.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SET LOCAL work_mem = '32MB'"); err != nil {
		slog.Warn("store: SET LOCAL work_mem failed, retrying without bump", "error", err)
		_ = tx.Rollback(ctx)
		tx, err = d.Begin(ctx)
		if err != nil {
			return err
		}
	}

	if err := fn(d.Queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
