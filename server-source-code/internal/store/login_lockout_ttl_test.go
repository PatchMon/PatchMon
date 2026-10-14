package store

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

func newMiniredisLockout(t *testing.T, maxAttempts, lockoutMin int) (*LoginLockoutStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewLoginLockoutStore(&hostctx.RedisResolver{Default: rdb}, nil, maxAttempts, lockoutMin), mr
}

func TestRecordFailedAttemptSetsCounterTTL(t *testing.T) {
	t.Parallel()
	s, mr := newMiniredisLockout(t, 5, 15)
	ctx := context.Background()
	id := s.Identifier("203.0.113.9", "alice")

	if n, locked := s.RecordFailedAttempt(ctx, id); n != 1 || locked {
		t.Fatalf("first attempt = (%d, %v), want (1, false)", n, locked)
	}
	if ttl := mr.TTL(LoginFailedPrefix + id); ttl <= 0 || ttl > 15*time.Minute {
		t.Fatalf("counter TTL = %v, want within the 15 minute lockout window", ttl)
	}
}

// A counter left without a TTL (an EXPIRE lost after a successful INCR) must
// regain one on the next attempt instead of persisting forever.
func TestRecordFailedAttemptHealsCounterWithoutTTL(t *testing.T) {
	t.Parallel()
	s, mr := newMiniredisLockout(t, 5, 15)
	ctx := context.Background()
	id := s.Identifier("203.0.113.9", "alice")
	key := LoginFailedPrefix + id

	if err := mr.Set(key, "2"); err != nil {
		t.Fatal(err)
	}
	if mr.TTL(key) != 0 {
		t.Fatal("precondition: stranded counter should have no TTL")
	}

	if n, locked := s.RecordFailedAttempt(ctx, id); n != 3 || locked {
		t.Fatalf("attempt = (%d, %v), want (3, false)", n, locked)
	}
	if ttl := mr.TTL(key); ttl <= 0 {
		t.Fatalf("stranded counter still has no TTL after another attempt (ttl=%v)", ttl)
	}
}

func TestRecordFailedAttemptKeepsWindowFixed(t *testing.T) {
	t.Parallel()
	s, mr := newMiniredisLockout(t, 5, 15)
	ctx := context.Background()
	id := s.Identifier("203.0.113.9", "alice")
	key := LoginFailedPrefix + id

	s.RecordFailedAttempt(ctx, id)
	mr.FastForward(10 * time.Minute)
	before := mr.TTL(key)
	s.RecordFailedAttempt(ctx, id)
	if after := mr.TTL(key); after != before {
		t.Fatalf("a later attempt re-armed the window: TTL %v -> %v", before, after)
	}
}

func TestRecordFailedAttemptLocksAtMax(t *testing.T) {
	t.Parallel()
	s, mr := newMiniredisLockout(t, 3, 15)
	ctx := context.Background()
	id := s.Identifier("203.0.113.9", "alice")

	for i := 1; i <= 2; i++ {
		if _, locked := s.RecordFailedAttempt(ctx, id); locked {
			t.Fatalf("locked early on attempt %d", i)
		}
	}
	if _, locked := s.RecordFailedAttempt(ctx, id); !locked {
		t.Fatal("not locked on the third attempt")
	}
	if locked, remaining := s.IsLocked(ctx, id); !locked || remaining <= 0 {
		t.Fatalf("IsLocked = (%v, %d), want locked with time remaining", locked, remaining)
	}
	if mr.Exists(LoginFailedPrefix + id) {
		t.Error("failed-attempt counter should be removed once the lock is set")
	}
}
