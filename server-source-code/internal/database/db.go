package database

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/safeconv"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBProvider resolves the database for a request. Used by stores in multi-host mode.
type DBProvider interface {
	DB(ctx context.Context) *DB
}

// DB provides database access via pgxpool and sqlc-generated queries.
type DB struct {
	pool    *pgxpool.Pool
	Queries *db.Queries
	cfg     *config.Config
}

// NewDB creates a pgx pool and sqlc Queries with retry logic.
func NewDB(ctx context.Context, cfg *config.Config) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	poolCfg.MaxConns = safeconv.ClampToInt32(cfg.DBConnectionLimit)
	poolCfg.ConnConfig.ConnectTimeout = time.Duration(cfg.DBConnectTimeout) * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	d := &DB{
		pool:    pool,
		Queries: db.New(pool),
		cfg:     cfg,
	}
	if err := d.waitForDB(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return d, nil
}

// NewFromURL creates a pgx pool and sqlc Queries from a raw database URL.
// Used for per-host pools; no retry logic. Caller must ensure the database is reachable.
// maxConns and minConns override pool size (0 uses defaults: 5 max, 1 min).
func NewFromURL(ctx context.Context, databaseURL string, maxConns, minConns int, cfg *config.Config) (*DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	if maxConns <= 0 {
		maxConns = 5
	}
	if minConns < 0 {
		minConns = 1
	}
	poolCfg.MaxConns = safeconv.ClampToInt32(maxConns)
	poolCfg.MinConns = safeconv.ClampToInt32(minConns)
	poolCfg.ConnConfig.ConnectTimeout = time.Duration(cfg.DBConnectTimeout) * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping host database: %w", err)
	}
	return &DB{
		pool:    pool,
		Queries: db.New(pool),
		cfg:     cfg,
	}, nil
}

// waitForDB retries connection until DB is available.
func (d *DB) waitForDB(ctx context.Context) error {
	maxAttempts := d.cfg.DBConnMaxAttempts
	interval := time.Duration(d.cfg.DBConnWaitInterval) * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := d.pool.Ping(ctx); err == nil {
			return nil
		}

		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
	}

	return fmt.Errorf("database unavailable after %d attempts", maxAttempts)
}

// DB returns the receiver, satisfying DBProvider for single-host mode.
func (d *DB) DB(ctx context.Context) *DB {
	return d
}

// Health checks database and returns nil if healthy.
func (d *DB) Health(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Close closes the pool.
func (d *DB) Close() {
	d.pool.Close()
}

// Begin starts a pgx transaction for use with sqlc Queries.WithTx.
func (d *DB) Begin(ctx context.Context) (pgx.Tx, error) {
	return d.pool.Begin(ctx)
}

// BeginLong starts a transaction with extended timeout for long-running operations (e.g. compliance scans, bulk updates).
// Uses DBTransactionLongTimeout from config (default 60000ms).
// If SET LOCAL statement_timeout fails (e.g. with PgBouncer transaction pooling), the transaction is rolled back
// and we retry with a plain transaction so the operation can still succeed.
func (d *DB) BeginLong(ctx context.Context) (pgx.Tx, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	timeoutMs := d.cfg.DBTransactionLongTimeout
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}
	// SET LOCAL does not support parameter placeholders ($1); use string interpolation.
	// timeoutMs is from config, not user input, so this is safe.
	_, err = tx.Exec(ctx, "SET LOCAL statement_timeout = "+strconv.Itoa(timeoutMs))
	if err != nil {
		slog.Warn("SET LOCAL statement_timeout failed, retrying without timeout",
			"error", err, "timeout_ms", timeoutMs)
		_ = tx.Rollback(ctx)
		// Retry without statement_timeout - some connection poolers don't support SET LOCAL
		tx, err = d.pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
	}
	return tx, nil
}

// Raw executes a raw query using the pgx pool (for information_schema, etc).
func (d *DB) Raw(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return d.pool.Query(ctx, query, args...)
}

// RawQueryRow executes a raw query returning one row.
func (d *DB) RawQueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	return d.pool.QueryRow(ctx, query, args...)
}

// Exec executes a raw command (INSERT, UPDATE, DELETE) and returns rows affected.
func (d *DB) Exec(ctx context.Context, query string, args ...interface{}) (int64, error) {
	tag, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
