package handler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

func TestVerifyTfaCode(t *testing.T) {
	ctx := context.Background()
	d := pgtest.Open(t)
	x := newTfaResetHarness(t, d, d)

	t.Run("a TOTP code is accepted once", func(t *testing.T) {
		u := x.mkUser(ctx, "user", true)
		code := currentCode(t, u.secret)
		if ok, err := verifyTfaCode(ctx, x.users, x.replay, x.reload(ctx, u.ID), code); err != nil || !ok {
			t.Fatalf("first use: ok=%v err=%v", ok, err)
		}
		if ok, err := verifyTfaCode(ctx, x.users, x.replay, x.reload(ctx, u.ID), code); err != nil || ok {
			t.Fatalf("replay accepted: ok=%v err=%v", ok, err)
		}
		other := x.mkUser(ctx, "user", true)
		if ok, _ := verifyTfaCode(ctx, x.users, x.replay, x.reload(ctx, other.ID), currentCode(t, other.secret)); !ok {
			t.Fatal("replay key leaked across users")
		}
	})

	t.Run("parallel uses of one backup code succeed once", func(t *testing.T) {
		u := x.mkUser(ctx, "user", true)
		var wins atomic.Int32
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if ok, err := verifyTfaCode(ctx, x.users, x.replay, x.reload(ctx, u.ID), u.backupCodes[0]); err == nil && ok {
					wins.Add(1)
				}
			}()
		}
		wg.Wait()
		if n := wins.Load(); n != 1 {
			t.Fatalf("%d successes, want 1", n)
		}
		if left := len(util.ParseBackupCodesJSON(x.reload(ctx, u.ID).TfaBackupCodes)); left != len(u.backupCodes)-1 {
			t.Fatalf("%d codes left, want %d", left, len(u.backupCodes)-1)
		}
	})

	t.Run("parallel uses of two different backup codes both succeed", func(t *testing.T) {
		u := x.mkUser(ctx, "user", true)
		var wins atomic.Int32
		var wg sync.WaitGroup
		for _, c := range u.backupCodes[:2] {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if ok, err := verifyTfaCode(ctx, x.users, x.replay, x.reload(ctx, u.ID), c); err == nil && ok {
					wins.Add(1)
				}
			}()
		}
		wg.Wait()
		if n := wins.Load(); n != 2 {
			t.Fatalf("%d successes, want 2", n)
		}
	})

	t.Run("malformed tokens are rejected", func(t *testing.T) {
		u := x.mkUser(ctx, "user", true)
		for _, tok := range []string{"", "12345", "1234567", "ab-cd!"} {
			if ok, _ := verifyTfaCode(ctx, x.users, x.replay, x.reload(ctx, u.ID), tok); ok {
				t.Fatalf("%q accepted", tok)
			}
		}
	})
}
