package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newRateLimitRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

// TestRateLimitHit_ArmsExpiryOnFirstHit is the baseline the old code already
// satisfied: a fresh bucket must come back with a TTL, otherwise it counts for
// ever.
func TestRateLimitHit_ArmsExpiryOnFirstHit(t *testing.T) {
	t.Parallel()
	client, mr := newRateLimitRedis(t)
	ctx := context.Background()

	count, ttl, err := rateLimitHit(ctx, client, "ratelimit:general:203.0.113.7", 60000)
	if err != nil {
		t.Fatalf("rateLimitHit: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1 on first hit, got %d", count)
	}
	if ttl <= 0 {
		t.Fatalf("expected a positive TTL, got %s", ttl)
	}
	if got := mr.TTL("ratelimit:general:203.0.113.7"); got <= 0 {
		t.Fatalf("expected the key to carry an expiry, got %s", got)
	}
}

// TestRateLimitHit_ReArmsKeyThatLostItsExpiry is the regression this change
// exists for (issue #868).
//
// The old code issued EXPIRE only when INCR returned 1, and discarded its
// error. A single lost EXPIRE — a Redis blip, a failover mid-command — left the
// counter with no TTL, and because count == 1 never came round again the key
// climbed past the limit and stayed there. Every later request from that IP was
// answered 429 until an operator deleted the key by hand, and it survived a
// restart through the RDB snapshot.
//
// Stripping the TTL below reproduces that state directly, without needing to
// make Redis fail.
func TestRateLimitHit_ReArmsKeyThatLostItsExpiry(t *testing.T) {
	t.Parallel()
	client, mr := newRateLimitRedis(t)
	ctx := context.Background()
	const key = "ratelimit:general:203.0.113.9"

	if _, _, err := rateLimitHit(ctx, client, key, 60000); err != nil {
		t.Fatalf("first hit: %v", err)
	}

	// The stranded state: counter present, expiry gone. PERSIST is used rather
	// than miniredis' SetTTL(key, 0) because only PERSIST reproduces what Redis
	// actually reports for a key that never got an expiry — PTTL of -1, which is
	// what the script branches on. SetTTL(key, 0) leaves PTTL at 0 and the
	// re-arm never fires, so the test would pass against a broken fix.
	if err := client.Persist(ctx, key).Err(); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if got := client.PTTL(ctx, key).Val(); got >= 0 {
		t.Fatalf("precondition: expected the key to have lost its TTL, got %s", got)
	}

	count, ttl, err := rateLimitHit(ctx, client, key, 60000)
	if err != nil {
		t.Fatalf("second hit: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
	if ttl <= 0 {
		t.Fatalf("expected the reported TTL to be re-armed, got %s", ttl)
	}
	if got := mr.TTL(key); got <= 0 {
		t.Fatalf("expected the key's expiry to be re-armed, got %s", got)
	}
}

// TestRateLimitHit_KeepsCountingWithinTheWindow guards against the opposite
// mistake: re-arming must not reset the counter, or the limit never trips.
func TestRateLimitHit_KeepsCountingWithinTheWindow(t *testing.T) {
	t.Parallel()
	client, _ := newRateLimitRedis(t)
	ctx := context.Background()
	const key = "ratelimit:auth:203.0.113.11"

	for want := int64(1); want <= 5; want++ {
		count, _, err := rateLimitHit(ctx, client, key, 60000)
		if err != nil {
			t.Fatalf("hit %d: %v", want, err)
		}
		if count != want {
			t.Fatalf("expected count %d, got %d", want, count)
		}
	}
}

// TestRateLimitHit_DoesNotExtendTheWindowOnEveryHit is why the script tests the
// TTL rather than issuing PEXPIRE unconditionally: an unconditional expiry
// turns a fixed window into a sliding one, so a caller hitting the endpoint
// steadily would never see the window close.
func TestRateLimitHit_DoesNotExtendTheWindowOnEveryHit(t *testing.T) {
	t.Parallel()
	client, mr := newRateLimitRedis(t)
	ctx := context.Background()
	const key = "ratelimit:general:203.0.113.13"

	if _, _, err := rateLimitHit(ctx, client, key, 60000); err != nil {
		t.Fatalf("first hit: %v", err)
	}
	mr.FastForward(30 * time.Second)

	_, ttl, err := rateLimitHit(ctx, client, key, 60000)
	if err != nil {
		t.Fatalf("second hit: %v", err)
	}
	if ttl > 31*time.Second {
		t.Fatalf("window was extended: expected the remaining TTL to stay near 30s, got %s", ttl)
	}
}
