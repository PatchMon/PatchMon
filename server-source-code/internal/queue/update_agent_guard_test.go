package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/jackc/pgx/v5"
)

type fakeRunChecker struct {
	hostErr error
	active  bool
}

func (f fakeRunChecker) GetHostByApiID(context.Context, string) (db.Host, error) {
	return db.Host{ID: "h"}, f.hostErr
}

func (f fakeRunChecker) HostHasActivePatchRun(context.Context, string) (bool, error) {
	return f.active, nil
}

func TestHostIsPatchingFailsClosed(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("connection reset")
	if _, err := hostIsPatching(ctx, fakeRunChecker{hostErr: boom}, "a"); !errors.Is(err, boom) {
		t.Fatalf("lookup error must be returned so the update retries, got %v", err)
	}
	if patching, err := hostIsPatching(ctx, fakeRunChecker{hostErr: pgx.ErrNoRows}, "a"); err != nil || patching {
		t.Fatalf("unknown host: patching=%v err=%v", patching, err)
	}
	if patching, err := hostIsPatching(ctx, fakeRunChecker{active: true}, "a"); err != nil || !patching {
		t.Fatalf("active run: patching=%v err=%v", patching, err)
	}
	if patching, err := hostIsPatching(ctx, fakeRunChecker{}, "a"); err != nil || patching {
		t.Fatalf("idle host: patching=%v err=%v", patching, err)
	}
}
