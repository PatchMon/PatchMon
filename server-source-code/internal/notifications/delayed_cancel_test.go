package notifications

import (
	"context"
	"strconv"
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

func (e *Emitter) mustClaim(t *testing.T, tenantHost, refID string, emittedAt int64, ttl time.Duration) {
	t.Helper()
	if !e.claimPending(context.Background(), tenantHost, "host_down", refID, emittedAt, ttl) {
		t.Fatalf("claim for %s at %d refused", refID, emittedAt)
	}
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
	e.mustClaim(t, "", "h1", emitted, time.Minute)
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
	e.mustClaim(t, "", "h1", first, time.Minute)
	genBefore := e.recoveryGeneration(ctx, "", key)

	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(time.Second)})

	second := base.Add(2 * time.Second).UnixNano()
	genAfter := e.recoveryGeneration(ctx, "", key)
	e.mustClaim(t, "", "h1", second, time.Minute)

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
	e.mustClaim(t, "", "h1", first, time.Minute)
	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(time.Second)})
	if e.claimPending(ctx, "", "host_down", "h1", base.Add(500*time.Millisecond).UnixNano(), time.Minute) {
		t.Error("a re-emit for a down decision older than the recovery was enqueued")
	}

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

	e.mustClaim(t, "a.example.com", "h1", emitted, time.Minute)
	e.mustClaim(t, "b.example.com", "h1", emitted, time.Minute)
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

	e.mustClaim(t, "", "h1", emitted, time.Minute)
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

	e.mustClaim(t, "", "h1", emitted, time.Minute)
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

	e.mustClaim(t, "", "h1", emitted, delay+pendingGrace)
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

func TestRecordRecovery_ReconnectBeforePendingMarker(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	decidedDown := time.Now()
	connectedAt := decidedDown.Add(50 * time.Millisecond)

	e.RecordRecovery(ctx, "", "host_down", "h1", connectedAt)
	if e.claimPending(ctx, "", "host_down", "h1", decidedDown.UnixNano(), time.Minute) {
		t.Error("host reconnected before the pending marker was written, yet the notification was enqueued")
	}
	if n, _ := rdb.Exists(ctx, pendingKeyFor("host_down", "h1")).Result(); n != 0 {
		t.Error("refused claim still left a pending marker")
	}
}

func TestRecordRecovery_ReconnectAfterPendingMarker(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	decidedDown := time.Now()
	key := delayedCancelKey(hostDown("h1"))

	e.mustClaim(t, "", "h1", decidedDown.UnixNano(), time.Minute)
	e.RecordRecovery(ctx, "", "host_down", "h1", decidedDown.Add(time.Second))
	if !IsDelayedCancelled(rdb, "", key, decidedDown.UnixNano()) {
		t.Error("reconnect after the pending marker did not cancel the notification")
	}
	if n, _ := rdb.Exists(ctx, pendingKeyFor("host_down", "h1")).Result(); n != 0 {
		t.Error("recovery left the pending marker in place")
	}
}

func TestRecordRecovery_NothingPendingIsShortLived(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	e := NewEmitter(nil, rdb, nil)

	e.RecordRecovery(context.Background(), "a.example.com", "host_down", "h1", time.Now())
	key := tenantRedisKey("a.example.com", delayedCancelKey(hostDown("h1")))
	if ttl := mr.TTL(key); ttl <= 0 || ttl > recoveryRaceTTL {
		t.Fatalf("recovery with nothing pending has TTL %v, want (0, %v]", ttl, recoveryRaceTTL)
	}
	if mr.Exists(delayedCancelKey(hostDown("h1"))) {
		t.Fatal("recovery written outside the context prefix")
	}
}

func TestRecordRecovery_NeverMovesBackwards(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	key := delayedCancelKey(hostDown("h1"))

	e.RecordRecovery(ctx, "", "host_down", "h1", base)
	e.RecordRecovery(ctx, "", "host_down", "h1", base.Add(-time.Hour))
	if got := e.recoveryGeneration(ctx, "", key); got != strconv.FormatInt(base.UnixNano(), 10) {
		t.Fatalf("recovery moved backwards to %s", got)
	}
	if IsDelayedCancelled(rdb, "", key, base.Add(time.Second).UnixNano()) {
		t.Fatal("a recovery older than the outage cancelled it")
	}
}

func TestClaimPending_OneNotificationPerOutage(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	e := NewEmitter(nil, rdb, nil)
	ctx := context.Background()
	base := time.Now()
	ttl := time.Minute + pendingGrace

	e.mustClaim(t, "", "h1", base.UnixNano(), ttl)
	mr.FastForward(20 * time.Hour)
	for i := 1; i <= 3; i++ {
		if e.claimPending(ctx, "", "host_down", "h1", base.Add(time.Duration(i)*2*time.Minute).UnixNano(), ttl) {
			t.Fatalf("periodic re-emit %d during the same outage was enqueued again", i)
		}
	}
	if got := mr.TTL(pendingKeyFor("host_down", "h1")); got < ttl-time.Second {
		t.Errorf("re-emit during the outage did not extend the pending marker: TTL %v", got)
	}

	e.CancelPending(ctx, "", "host_down", map[string]time.Time{"h1": base.Add(10 * time.Minute)})
	if !e.claimPending(ctx, "", "host_down", "h1", base.Add(11*time.Minute).UnixNano(), ttl) {
		t.Fatal("a new outage after the recovery was not enqueued")
	}
}

func TestClaimPending_OtherTypesStillReemit(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	for i := 0; i < 2; i++ {
		if !e.claimPending(ctx, "", "host_security_updates_exceeded", "h1", base.Add(time.Duration(i)*time.Minute).UnixNano(), time.Hour) {
			t.Fatalf("claim %d for a type whose counterpart needs an alert row was refused", i)
		}
	}
}

func TestClaimPending_StaleMarkerAfterRecoveryDoesNotBlock(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	if err := rdb.Set(ctx, pendingKeyFor("host_down", "h1"), strconv.FormatInt(base.UnixNano(), 10), time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	if err := rdb.Set(ctx, cancelKeyFor("host_down", "h1"), strconv.FormatInt(base.Add(time.Second).UnixNano(), 10), time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	if !e.claimPending(ctx, "", "host_down", "h1", base.Add(2*time.Second).UnixNano(), time.Hour) {
		t.Fatal("a marker left behind by an already recovered outage blocked the next outage")
	}
}
