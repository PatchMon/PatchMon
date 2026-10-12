package queue

import (
	"context"
	"testing"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
)

func TestResolveDBForHostTagsOnlyOnResolve(t *testing.T) {
	def := &database.DB{}
	base := context.Background()

	ctx, d := resolveDBForHost(base, "a.example", def, nil)
	if d != def || ctx != base {
		t.Fatal("nil poolCache must return inputs unchanged")
	}

	pc := hostctx.NewPoolCache(&hostctx.Registry{}, nil, 0, nil)
	for _, h := range []string{"", "  ", "unknown.example"} {
		ctx, d := resolveDBForHost(base, h, def, pc)
		if d != def || hostctx.TenantHostKey(ctx) != "" {
			t.Fatalf("%q: fallback must return defaultDB untagged", h)
		}
	}

	ctx, d = resolveDBFromPayload(base, []byte(`{"host":"unknown.example"}`), def, pc)
	if d != def || hostctx.TenantHostKey(ctx) != "" {
		t.Fatal("payload fallback must stay untagged")
	}
}
