package database

import (
	"context"
	"fmt"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps pgxpool.Pool with connection retry and health check.
type Pool struct {
	*pgxpool.Pool
	cfg *config.Config
}

// NewPool creates a connection pool with retry logic.
func NewPool(ctx context.Context, cfg *config.Config) (*Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.DBConnectionLimit)
	poolCfg.ConnConfig.ConnectTimeout = time.Duration(cfg.DBConnectTimeout) * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	p := &Pool{Pool: pool, cfg: cfg}
	if err := p.waitForDB(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return p, nil
}

// waitForDB retries connection until DB is available.
func (p *Pool) waitForDB(ctx context.Context) error {
	maxAttempts := p.cfg.DBConnMaxAttempts
	interval := time.Duration(p.cfg.DBConnWaitInterval) * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := p.Ping(ctx); err == nil {
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

// Ping checks database connectivity.
func (p *Pool) Ping(ctx context.Context) error {
	return p.Pool.Ping(ctx)
}

// Close closes the pool.
func (p *Pool) Close() {
	p.Pool.Close()
}

// Health checks database and returns nil if healthy.
func (p *Pool) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return p.Ping(ctx)
}
