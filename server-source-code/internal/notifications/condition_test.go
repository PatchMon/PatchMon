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
		r, err := e.SyncConditions(ctx, "", typ, active, 0, nil, now)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(r.Entered)
		slices.Sort(r.Left)
		if len(r.Reminded) != 0 {
			t.Fatalf("reminders sent with reminders off: %v", r.Reminded)
		}
		return r.Entered, r.Left
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

	if _, err := e.SyncConditions(ctx, "a.example.com", "host_security_updates_exceeded", []string{"h1"}, 0, nil, now); err != nil {
		t.Fatal(err)
	}
	r, _ := e.SyncConditions(ctx, "b.example.com", "host_security_updates_exceeded", []string{"h1"}, 0, nil, now)
	if !slices.Equal(r.Entered, []string{"h1"}) {
		t.Fatalf("context b suppressed by context a: %v", r.Entered)
	}
	r, _ = e.SyncConditions(ctx, "a.example.com", "host_pending_updates_exceeded", []string{"h1"}, 0, nil, now)
	if !slices.Equal(r.Entered, []string{"h1"}) {
		t.Fatalf("pending updates suppressed by security updates: %v", r.Entered)
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

	_, _ = e.SyncConditions(ctx, "", typ, []string{"h1"}, 0, nil, now)
	e.ClearConditions(ctx, "", typ)
	r, _ := e.SyncConditions(ctx, "", typ, []string{"h1"}, 0, nil, now)
	if !slices.Equal(r.Entered, []string{"h1"}) {
		t.Fatalf("after clearing, entered = %v", r.Entered)
	}
}

func TestForgetConditions_RefEntersAgain(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	now := time.Now()
	typ := "host_security_updates_exceeded"

	_, _ = e.SyncConditions(ctx, "", typ, []string{"h1", "h2"}, 0, nil, now)
	e.ForgetConditions(ctx, "", typ, []string{"h2"})
	r, _ := e.SyncConditions(ctx, "", typ, []string{"h1", "h2"}, 0, nil, now)
	if !slices.Equal(r.Entered, []string{"h2"}) || len(r.Left) != 0 {
		t.Fatalf("after forgetting h2: entered %v, left %v", r.Entered, r.Left)
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
	r, err := e.SyncConditions(ctx, "", "x", []string{"h1"}, time.Hour, nil, time.Now())
	if err != nil || !slices.Equal(r.Entered, []string{"h1"}) || r.Left != nil || r.Reminded != nil {
		t.Fatalf("nil emitter sync = %+v %v", r, err)
	}
	e.ReleaseCheck(ctx, "", "x")
	e.ClearConditions(ctx, "", "x")
	e.ForgetConditions(ctx, "", "x", []string{"h1"})
	e.RewindConditions(ctx, "", "x", []string{"h1"})
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

func TestSyncConditions_Reminders(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	typ := "host_security_updates_exceeded"
	every := 4 * time.Hour

	sync := func(at time.Duration, active ...string) ConditionSync {
		t.Helper()
		r, err := e.SyncConditions(ctx, "", typ, active, every, nil, base.Add(at))
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(r.Entered)
		slices.Sort(r.Reminded)
		return r
	}

	if r := sync(0, "h1"); !slices.Equal(r.Entered, []string{"h1"}) || len(r.Reminded) != 0 {
		t.Fatalf("first occurrence: %+v", r)
	}
	if r := sync(time.Hour, "h1", "h2"); !slices.Equal(r.Entered, []string{"h2"}) || len(r.Reminded) != 0 {
		t.Fatalf("one hour in: %+v", r)
	}
	if r := sync(4*time.Hour-10*time.Second, "h1", "h2"); !slices.Equal(r.Reminded, []string{"h1"}) {
		t.Fatalf("h1 reminder not due at four hours (with tick jitter): %+v", r)
	}
	if r := sync(5*time.Hour, "h1", "h2"); !slices.Equal(r.Reminded, []string{"h2"}) {
		t.Fatalf("five hours in, expected only h2's reminder: %+v", r)
	}
	if r := sync(6*time.Hour, "h1", "h2"); len(r.Reminded) != 0 {
		t.Fatalf("reminder repeated before the interval: %+v", r)
	}
	if r := sync(8*time.Hour, "h2"); len(r.Reminded) != 0 || !slices.Equal(r.Left, []string{"h1"}) {
		t.Fatalf("cleared host still reminded: %+v", r)
	}
	if r := sync(8*time.Hour+time.Minute, "h1", "h2"); !slices.Equal(r.Entered, []string{"h1"}) || len(r.Reminded) != 0 {
		t.Fatalf("re-occurrence must notify immediately, not as a reminder: %+v", r)
	}
	if r, _ := e.SyncConditions(ctx, "", typ, []string{"h1", "h2"}, 0, nil, base.Add(48*time.Hour)); len(r.Reminded) != 0 {
		t.Fatalf("reminders sent after being switched off: %+v", r)
	}
}

func TestRewindConditions_UndeliveredReminderRetriedAsReminder(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	base := time.Now()
	typ := "host_security_updates_exceeded"

	_, _ = e.SyncConditions(ctx, "", typ, []string{"h1"}, time.Hour, nil, base)
	e.RewindConditions(ctx, "", typ, []string{"h1"})
	r, _ := e.SyncConditions(ctx, "", typ, []string{"h1"}, time.Hour, nil, base.Add(time.Minute))
	if len(r.Entered) != 0 || !slices.Equal(r.Reminded, []string{"h1"}) {
		t.Fatalf("rewound reminder: %+v", r)
	}
	if r, _ := e.SyncConditions(ctx, "", typ, []string{"h1"}, time.Hour, nil, base.Add(2*time.Minute)); len(r.Reminded) != 0 {
		t.Fatalf("delivered retry did not reset the reminder clock: %+v", r)
	}
}

func TestSyncConditions_HeldRefsKeepTheirClock(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	typ := "host_security_updates_exceeded"
	every := 4 * time.Hour
	at := func(d time.Duration, hold ...string) []string {
		t.Helper()
		r, err := e.SyncConditions(ctx, "", typ, []string{"h1"}, every, hold, base.Add(d))
		if err != nil {
			t.Fatal(err)
		}
		return r.Reminded
	}

	at(0)
	if got := at(5*time.Hour, "h1"); len(got) != 0 {
		t.Fatalf("held ref reminded: %v", got)
	}
	if got := at(6*time.Hour, "h1"); len(got) != 0 {
		t.Fatalf("held ref reminded: %v", got)
	}
	if got := at(6*time.Hour + 30*time.Minute); !slices.Equal(got, []string{"h1"}) {
		t.Fatalf("released ref overdue since 4h not reminded at the next check: %v", got)
	}
	if got := at(7 * time.Hour); len(got) != 0 {
		t.Fatalf("reminder repeated straight after release: %v", got)
	}
}

func TestSyncConditions_HoldNeverBlocksEntryOrExit(t *testing.T) {
	t.Parallel()
	e, _ := newTestEmitter(t)
	ctx := context.Background()
	now := time.Now()
	typ := "host_security_updates_exceeded"

	r, _ := e.SyncConditions(ctx, "", typ, []string{"h1"}, time.Hour, []string{"h1"}, now)
	if !slices.Equal(r.Entered, []string{"h1"}) {
		t.Fatalf("held new exceedance not entered: %+v", r)
	}
	r, _ = e.SyncConditions(ctx, "", typ, nil, time.Hour, []string{"h1"}, now.Add(time.Minute))
	if !slices.Equal(r.Left, []string{"h1"}) {
		t.Fatalf("held host that cleared did not leave: %+v", r)
	}
}
