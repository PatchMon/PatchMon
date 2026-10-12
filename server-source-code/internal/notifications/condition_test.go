package notifications

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestClaimCheckDue_HonoursIntervalPerContextAndType(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	every := 90 * time.Minute

	if !e.ClaimCheckDue(ctx, "", "host_security_updates_exceeded", every, base) {
		t.Fatal("first run not due")
	}
	for _, at := range []time.Duration{time.Minute, 60 * time.Minute, 89 * time.Minute} {
		if e.ClaimCheckDue(ctx, "", "host_security_updates_exceeded", every, base.Add(at)) {
			t.Fatalf("due again %s after the last run with a 90 minute interval", at)
		}
	}
	if !e.ClaimCheckDue(ctx, "a.example.com", "host_security_updates_exceeded", every, base.Add(time.Minute)) {
		t.Fatal("another context shares the last-run time")
	}
	if !e.ClaimCheckDue(ctx, "", "host_pending_updates_exceeded", every, base.Add(time.Minute)) {
		t.Fatal("pending updates check shares the security check's last-run time")
	}
	if !e.ClaimCheckDue(ctx, "", "host_security_updates_exceeded", every, base.Add(every-10*time.Second)) {
		t.Fatal("tick landing a few seconds early skipped a whole interval")
	}
}

func TestClaimCheckDue_ShorterIntervalAppliesImmediately(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()

	e.ClaimCheckDue(ctx, "", "x", 24*time.Hour, base)
	if e.ClaimCheckDue(ctx, "", "x", 24*time.Hour, base.Add(10*time.Minute)) {
		t.Fatal("daily check ran after ten minutes")
	}
	if !e.ClaimCheckDue(ctx, "", "x", 5*time.Minute, base.Add(10*time.Minute)) {
		t.Fatal("lowering the interval did not take effect without a restart")
	}
}

func TestReleaseCheck_RunsAgainNextTick(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()

	e.ClaimCheckDue(ctx, "", "x", time.Hour, base)
	e.ReleaseCheck(ctx, "", "x")
	if !e.ClaimCheckDue(ctx, "", "x", time.Hour, base.Add(time.Minute)) {
		t.Fatal("released check not due")
	}
}

func TestSyncConditions_OncePerOccurrence(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	now := time.Now()
	typ := "host_security_updates_exceeded"

	sync := func(active ...string) ([]string, []string) {
		t.Helper()
		entered, left, err := e.SyncConditions(ctx, "", typ, active, now)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(entered)
		slices.Sort(left)
		return entered, left
	}

	if entered, _ := sync("h1", "h2"); !slices.Equal(entered, []string{"h1", "h2"}) {
		t.Fatalf("first occurrence entered = %v", entered)
	}
	for i := 0; i < 3; i++ {
		if entered, left := sync("h1", "h2"); len(entered) != 0 || len(left) != 0 {
			t.Fatalf("still exceeded re-entered %v / left %v", entered, left)
		}
	}
	if entered, left := sync("h2"); len(entered) != 0 || !slices.Equal(left, []string{"h1"}) {
		t.Fatalf("clearing h1: entered %v, left %v", entered, left)
	}
	if entered, _ := sync("h1", "h2"); !slices.Equal(entered, []string{"h1"}) {
		t.Fatalf("re-occurrence after clearing entered = %v", entered)
	}
	if _, left := sync(); !slices.Equal(left, []string{"h1", "h2"}) {
		t.Fatalf("empty sync left = %v", left)
	}
}

func TestSyncConditions_ScopedPerContextAndType(t *testing.T) {
	t.Parallel()
	e, rdb := newTestEmitter(t)
	ctx := context.Background()
	now := time.Now()

	if _, _, err := e.SyncConditions(ctx, "a.example.com", "host_security_updates_exceeded", []string{"h1"}, now); err != nil {
		t.Fatal(err)
	}
	entered, _, _ := e.SyncConditions(ctx, "b.example.com", "host_security_updates_exceeded", []string{"h1"}, now)
	if !slices.Equal(entered, []string{"h1"}) {
		t.Fatalf("context b suppressed by context a: %v", entered)
	}
	entered, _, _ = e.SyncConditions(ctx, "a.example.com", "host_pending_updates_exceeded", []string{"h1"}, now)
	if !slices.Equal(entered, []string{"h1"}) {
		t.Fatalf("pending updates suppressed by security updates: %v", entered)
	}
	key := tenantRedisKey("a.example.com", conditionKey("host_security_updates_exceeded"))
	if ttl := rdb.TTL(ctx, key).Val(); ttl <= 0 {
		t.Fatalf("condition key has no expiry: %v", ttl)
	}
}

func TestClearConditions_NextOccurrenceNotifies(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	now := time.Now()
	typ := "host_pending_updates_exceeded"

	_, _, _ = e.SyncConditions(ctx, "", typ, []string{"h1"}, now)
	e.ClearConditions(ctx, "", typ)
	entered, _, _ := e.SyncConditions(ctx, "", typ, []string{"h1"}, now)
	if !slices.Equal(entered, []string{"h1"}) {
		t.Fatalf("after clearing, entered = %v", entered)
	}
}

func TestForgetConditions_RefEntersAgain(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	now := time.Now()
	typ := "host_security_updates_exceeded"

	_, _, _ = e.SyncConditions(ctx, "", typ, []string{"h1", "h2"}, now)
	e.ForgetConditions(ctx, "", typ, []string{"h2"})
	entered, left, _ := e.SyncConditions(ctx, "", typ, []string{"h1", "h2"}, now)
	if !slices.Equal(entered, []string{"h2"}) || len(left) != 0 {
		t.Fatalf("after forgetting h2: entered %v, left %v", entered, left)
	}
}

func TestClaimCheckDue_EmitterWithoutRedisNeverRuns(t *testing.T) {
	t.Parallel()
	e := NewEmitter(nil, nil, nil)
	if e.ClaimCheckDue(context.Background(), "", "x", time.Hour, time.Now()) {
		t.Fatal("emitter without Redis would evaluate and notify on every tick")
	}
}

func TestConditionHelpers_NoRedis(t *testing.T) {
	t.Parallel()
	var e *Emitter
	ctx := context.Background()
	if !e.ClaimCheckDue(ctx, "", "x", time.Hour, time.Now()) {
		t.Fatal("nil emitter must not block the check")
	}
	entered, left, err := e.SyncConditions(ctx, "", "x", []string{"h1"}, time.Now())
	if err != nil || !slices.Equal(entered, []string{"h1"}) || left != nil {
		t.Fatalf("nil emitter sync = %v %v %v", entered, left, err)
	}
	e.ReleaseCheck(ctx, "", "x")
	e.ClearConditions(ctx, "", "x")
	e.ForgetConditions(ctx, "", "x", []string{"h1"})
}

func TestReleaseDedup_DroppedDeliveryCanBeRetried(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	if !e.tryDedup("a.example.com", "fp") {
		t.Fatal("first attempt deduplicated")
	}
	e.releaseDedup("a.example.com", "fp")
	if !e.tryDedup("a.example.com", "fp") {
		t.Fatal("retry after a dropped delivery swallowed by dedup")
	}
	if e.tryDedup("a.example.com", "fp") {
		t.Fatal("dedup no longer applies after a successful attempt")
	}
}
