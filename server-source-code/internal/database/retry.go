package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL SQLSTATE codes that indicate a transient transaction failure
// where the *entire transaction* should be retried from scratch.
//
//   - 40P01 deadlock_detected: Postgres killed one transaction in a deadlock cycle.
//     Re-running the operation will usually succeed because the conflicting
//     transaction has now committed (or also aborted) and locks are free.
//   - 40001 serialization_failure: returned under SERIALIZABLE / REPEATABLE READ
//     isolation when the read/write set conflicts with a concurrent committed
//     transaction. Retry is the documented remedy.
//
// We deliberately do NOT retry on 23505 (unique_violation), 23503 (FK), or
// other application-logic errors: those indicate a real data problem that
// will reproduce on every attempt.
const (
	sqlStateDeadlockDetected     = "40P01"
	sqlStateSerializationFailure = "40001"
)

// RetryConfig controls retry behaviour for transient transaction errors.
//
// The zero value produces sane defaults so callers can pass RetryConfig{}.
// Defaults are tuned for short OLTP transactions: 4 attempts (1 + 3 retries),
// starting at 25ms and capped at 500ms with full jitter.
type RetryConfig struct {
	MaxAttempts int           // total attempts including the first; 0 → 4
	BaseDelay   time.Duration // initial backoff before the first retry; 0 → 25ms
	MaxDelay    time.Duration // upper cap on a single backoff sleep; 0 → 500ms
}

// withDefaults fills any zero fields in cfg with sensible values.
func (cfg RetryConfig) withDefaults() RetryConfig {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 4
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = 25 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 500 * time.Millisecond
	}
	return cfg
}

// IsRetryable reports whether err is a transient PostgreSQL transaction
// failure that should be retried by re-running the transaction.
//
// Returns true ONLY for SQLSTATE 40P01 (deadlock_detected) and 40001
// (serialization_failure). Every other class of error — including 23505
// unique violations, network errors, context cancellation — is treated as
// non-retryable so callers do not silently mask real bugs.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case sqlStateDeadlockDetected, sqlStateSerializationFailure:
		return true
	default:
		return false
	}
}

// sqlStateOf extracts the SQLSTATE from a wrapped pg error, or "" if none.
func sqlStateOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// WithRetry runs fn with exponential backoff + full jitter on transient
// transaction errors. The op string is used purely for log correlation.
//
// The closure is expected to be self-contained — i.e. it should open and
// commit its own transaction. On retry the previous attempt's writes are
// already rolled back by the database, so any IDs / timestamps allocated
// inside fn must be regenerated each call (callers handle this naturally
// by allocating UUIDs inside their store function).
//
// On retry exhaustion, returns the error from the FINAL attempt; each prior
// attempt's error is logged at WARN. The returned error is wrapped with
// retry context (op name and attempt count) so handler logs include
// retry-correlation data without callers needing to thread it through.
//
// Honors ctx.Done() during the backoff sleep and returns ctx.Err() if the
// caller cancels mid-retry.
func WithRetry(ctx context.Context, op string, cfg RetryConfig, fn func(ctx context.Context) error) error {
	cfg = cfg.withDefaults()

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		if !IsRetryable(err) {
			return err
		}

		// If we've exhausted attempts, stop without sleeping.
		if attempt >= cfg.MaxAttempts {
			break
		}

		// Full jitter: sleep ∈ [0, min(MaxDelay, BaseDelay * 2^(attempt-1))).
		// "Full jitter" is the variant from the AWS architecture blog and
		// avoids thundering-herd retries when many transactions deadlock at
		// the same instant.
		//
		// shift is capped at 30 to prevent int64 overflow when callers pass
		// arbitrarily-large MaxAttempts; the cap is well above any practical
		// retry budget (BaseDelay << 30 already exceeds 26 days for a 25ms
		// base) and the result is clamped to MaxDelay anyway.
		shift := min(attempt-1, 30)
		expBackoff := cfg.BaseDelay << shift // BaseDelay * 2^shift
		if expBackoff <= 0 || expBackoff > cfg.MaxDelay {
			expBackoff = cfg.MaxDelay
		}
		// Full jitter: half-open interval [0, expBackoff). rand.Int64N(n)
		// already returns [0, n), so do not add 1 — that would be the
		// "decorrelated jitter" variant and bias sleeps slightly upward.
		sleep := time.Duration(rand.Int64N(int64(expBackoff)))

		slog.Warn("retrying transaction after transient failure",
			"op", op,
			"attempt", attempt,
			"max", cfg.MaxAttempts,
			"sqlstate", sqlStateOf(err),
			"sleep_ms", sleep.Milliseconds(),
			"err", err,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
	}

	slog.Error("transaction retries exhausted",
		"op", op,
		"max", cfg.MaxAttempts,
		"sqlstate", sqlStateOf(lastErr),
		"err", lastErr,
	)
	return fmt.Errorf("WithRetry %s exhausted after %d attempts: %w", op, cfg.MaxAttempts, lastErr)
}
