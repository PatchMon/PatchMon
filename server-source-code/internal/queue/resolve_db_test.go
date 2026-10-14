package queue

import (
	"context"
	"errors"
	"testing"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/hibiken/asynq"
)

func TestResolveDBForHostNeverFallsBack(t *testing.T) {
	def := &database.DB{}
	base := context.Background()

	ctx, d, err := resolveDBForHost(base, "a.example", def, nil)
	if err != nil || d != def || ctx != base {
		t.Fatal("nil poolCache must return defaultDB unchanged")
	}

	pc := hostctx.NewPoolCache(&hostctx.Registry{}, nil, 0, nil)
	for _, h := range []string{"", "  "} {
		ctx, d, err := resolveDBForHost(base, h, def, pc)
		if err != nil || d != def || hostctx.TenantHostKey(ctx) != "" {
			t.Fatalf("%q: empty host must use defaultDB untagged", h)
		}
	}

	ctx, d, err = resolveDBForHost(base, "unknown.example", def, pc)
	if d != nil || !errors.Is(err, asynq.SkipRetry) || hostctx.TenantHostKey(ctx) != "" {
		t.Fatalf("unknown host must return nil DB and SkipRetry, got db=%v err=%v", d, err)
	}

	_, d, err = resolveDBFromPayload(base, []byte(`{"host":"unknown.example"}`), def, pc)
	if d != nil || !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("payload with unknown host must not fall back, got db=%v err=%v", d, err)
	}

	_, d, err = resolveDBFromPayload(base, []byte(`{}`), def, pc)
	if err != nil || d != def {
		t.Fatal("payload without host must use defaultDB")
	}
}
