package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestEmitter(t *testing.T) (*Emitter, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewEmitter(nil, rdb, nil), rdb
}

func hostDown(hostID string) Event {
	return Event{Type: "host_down", ReferenceType: "host", ReferenceID: hostID}
}

func TestCancelPending_RecoveryWithinWindowCancels(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()

	emitted := base.UnixNano()
	e.markPending(ctx, "", "host_down", "h1", emitted, time.Minute)
	key := delayedCancelKey(hostDown("h1"))
	if IsDelayedCancelled(rdb, "", key, emitted) {
		t.Fatal("cancelled before any recovery")
	}

	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(time.Second)})
	if !IsDelayedCancelled(rdb, "", key, emitted) {
		t.Fatal("pending host_down still delivered after recovery within the window")
	}
}

func TestCancelPending_DownUpDownStillAlerts(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	key := delayedCancelKey(hostDown("h1"))

	first := base.UnixNano()
	e.markPending(ctx, "", "host_down", "h1", first, time.Minute)
	genBefore := e.recoveryGeneration(ctx, "", key)

	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(time.Second)})

	second := base.Add(2 * time.Second).UnixNano()
	genAfter := e.recoveryGeneration(ctx, "", key)
	e.markPending(ctx, "", "host_down", "h1", second, time.Minute)

	if !IsDelayedCancelled(rdb, "", key, first) {
		t.Error("first outage should be cancelled by the recovery")
	}
	if IsDelayedCancelled(rdb, "", key, second) {
		t.Error("second outage after the recovery must still alert")
	}
	if fingerprint(hostDown("h1"), "d1", genBefore) == fingerprint(hostDown("h1"), "d1", genAfter) {
		t.Error("second outage shares a dedup fingerprint with the first and would be dropped")
	}
}

func TestCancelPending_ReemitDoesNotClearRecovery(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	key := delayedCancelKey(hostDown("h1"))

	first := base.UnixNano()
	e.markPending(ctx, "", "host_down", "h1", first, time.Minute)
	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(time.Second)})
	e.markPending(ctx, "", "host_down", "h1", base.Add(500*time.Millisecond).UnixNano(), time.Minute)

	if !IsDelayedCancelled(rdb, "", key, first) {
		t.Fatal("a later re-emit wiped the recovery recorded for the first outage")
	}
}

func TestCancelPending_NoPendingWritesNothing(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()

	now := time.Now()
	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": now, "h2": now})

	n, err := rdb.DBSize(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected no keys for hosts with nothing pending, got %d", n)
	}
}

func TestCancelPending_ScopedPerContext(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	key := delayedCancelKey(hostDown("h1"))
	emitted := base.UnixNano()

	e.markPending(ctx, "a.example.com", "host_down", "h1", emitted, time.Minute)
	e.markPending(ctx, "b.example.com", "host_down", "h1", emitted, time.Minute)
	e.CancelPending(ctx, "a.example.com", "host_down", map[string]time.Time{"h1": base.Add(time.Second)})

	if !IsDelayedCancelled(rdb, "a.example.com", key, emitted) {
		t.Error("recovery in context a did not cancel its own pending notification")
	}
	if IsDelayedCancelled(rdb, "b.example.com", key, emitted) {
		t.Error("recovery in context a cancelled a notification in context b")
	}
}

func TestIsDelayedCancelled_LegacyPayload(t *testing.T) {
	t.Parallel()
	_, rdb := newTestEmitter(t)
	key := delayedCancelKey(hostDown("h1"))
	if err := rdb.Set(context.Background(), key, "1", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if !IsDelayedCancelled(rdb, "", key, 0) {
		t.Error("payload queued by an older release should honour an existing cancel key")
	}
}

func TestFingerprint_IncludesRecoveryGeneration(t *testing.T) {
	t.Parallel()
	ev := hostDown("h1")
	if fingerprint(ev, "d1", "") == fingerprint(ev, "d1", "123") {
		t.Fatal("recovery generation not part of the fingerprint")
	}
}

func TestCancelPending_RecoveryOlderThanEmitDoesNotCancel(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	key := delayedCancelKey(hostDown("h1"))
	emitted := base.UnixNano()

	e.markPending(ctx, "", "host_down", "h1", emitted, time.Minute)
	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(-20 * time.Minute)})

	if IsDelayedCancelled(rdb, "", key, emitted) {
		t.Fatal("a last report from before the outage cancelled the pending notification")
	}
}

func TestCancelPending_NeverMovesRecoveryBackwards(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	key := delayedCancelKey(hostDown("h1"))
	emitted := base.UnixNano()

	e.markPending(ctx, "", "host_down", "h1", emitted, time.Minute)
	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(time.Second)})
	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(-time.Hour)})

	if !IsDelayedCancelled(rdb, "", key, emitted) {
		t.Fatal("an older recovery time overwrote a newer one and revived a cancelled notification")
	}
}

func TestCancelPending_SurvivesLateWorker(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	e := NewEmitter(nil, rdb, nil)
	ctx := context.Background()
	key := delayedCancelKey(hostDown("h1"))
	delay := 5 * time.Minute
	emitted := time.Now().UnixNano()

	e.markPending(ctx, "", "host_down", "h1", emitted, delay+pendingGrace)
	mr.FastForward(time.Minute)
	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": time.Now()})
	mr.FastForward(delay - time.Minute + 6*time.Hour)

	if !IsDelayedCancelled(rdb, "", key, emitted) {
		t.Fatal("worker ran six hours late and the recovery had already expired")
	}

	ttl := mr.TTL(tenantRedisKey("", key))
	if ttl <= 0 || ttl > delay+2*pendingGrace {
		t.Fatalf("cancel key TTL %v is not bounded by the delay plus twice the grace", ttl)
	}
}
