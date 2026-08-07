package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func pgErr(code string) error {
	return &pgconn.PgError{Code: code, Message: "synthetic " + code}
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"deadlock 40P01", pgErr("40P01"), true},
		{"serialization 40001", pgErr("40001"), true},
		{"unique violation 23505", pgErr("23505"), false},
		{"foreign key 23503", pgErr("23503"), false},
		{"check violation 23514", pgErr("23514"), false},
		{"wrapped deadlock", fmt.Errorf("ProcessReport: %w", pgErr("40P01")), true},
		{"context.Canceled", context.Canceled, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsRetryable(tc.err)
			if got != tc.want {
				t.Fatalf("IsRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestWithRetry_SucceedsFirstAttempt(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), "test_op", RetryConfig{}, func(_ context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestWithRetry_RetriesOnDeadlock(t *testing.T) {
	calls := 0
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}
	err := WithRetry(context.Background(), "test_deadlock", cfg, func(_ context.Context) error {
		calls++
		if calls == 1 {
			return pgErr("40P01")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after 1 retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestWithRetry_RetriesOnSerialization(t *testing.T) {
	calls := 0
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}
	err := WithRetry(context.Background(), "test_serial", cfg, func(_ context.Context) error {
		calls++
		if calls == 1 {
			return pgErr("40001")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after 1 retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestWithRetry_DoesNotRetryOnUniqueViolation(t *testing.T) {
	calls := 0
	cfg := RetryConfig{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}
	err := WithRetry(context.Background(), "test_unique", cfg, func(_ context.Context) error {
		calls++
		return pgErr("23505")
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	// Non-retryable errors are returned verbatim (NOT wrapped with retry
	// context, since no retries happened). errors.As must still find the
	// underlying *pgconn.PgError with code 23505.
	var pgE *pgconn.PgError
	if !errors.As(err, &pgE) || pgE.Code != "23505" {
		t.Fatalf("expected the original 23505 error to be returned, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call (no retry), got %d", calls)
	}
}

func TestWithRetry_ExhaustsAttempts(t *testing.T) {
	calls := 0
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}
	err := WithRetry(context.Background(), "test_exhaust", cfg, func(_ context.Context) error {
		calls++
		return pgErr("40P01")
	})
	if err == nil {
		t.Fatalf("expected error after exhaustion, got nil")
	}
	if !IsRetryable(err) {
		t.Fatalf("expected returned err to still be retryable (last attempt's err)")
	}
	if calls != cfg.MaxAttempts {
		t.Fatalf("expected %d calls, got %d", cfg.MaxAttempts, calls)
	}
}

func TestWithRetry_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	// BaseDelay == MaxDelay == 500ms ensures the *upper bound* of the first
	// jittered sleep is 500ms, comfortably outlasting the 20ms cancel.
	// (The full-jitter floor is 0ms, so in the unlucky case where
	// rand.Int64N picks a near-zero sleep the loop may slip in a second
	// attempt before the next 500ms-cap sleep is interrupted — hence
	// calls <= 2 rather than == 1.)
	cfg := RetryConfig{MaxAttempts: 5, BaseDelay: 500 * time.Millisecond, MaxDelay: 500 * time.Millisecond}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := WithRetry(ctx, "test_cancel", cfg, func(_ context.Context) error {
		calls++
		return pgErr("40P01")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// At least one attempt was made; the cancel must have interrupted backoff
	// well before exhausting the retry budget.
	if calls < 1 || calls > 2 {
		t.Fatalf("expected 1 or 2 calls before cancel (got %d)", calls)
	}
}

func TestWithRetry_ZeroConfigUsesDefaults(t *testing.T) {
	cfg := RetryConfig{}.withDefaults()
	if cfg.MaxAttempts != 4 {
		t.Fatalf("default MaxAttempts = %d, want 4", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != 25*time.Millisecond {
		t.Fatalf("default BaseDelay = %v, want 25ms", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 500*time.Millisecond {
		t.Fatalf("default MaxDelay = %v, want 500ms", cfg.MaxDelay)
	}
}
