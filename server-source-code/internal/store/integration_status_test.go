package store

import (
	"context"
	"testing"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newIntegrationStatusStore(t *testing.T) (*IntegrationStatusStore, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewIntegrationStatusStore(&hostctx.RedisResolver{Default: client}), client
}

func installSteps(t *testing.T, status map[string]interface{}) []string {
	t.Helper()
	events, _ := status["install_events"].([]interface{})
	steps := make([]string, 0, len(events))
	for _, e := range events {
		m, _ := e.(map[string]interface{})
		step, _ := m["step"].(string)
		steps = append(steps, step)
	}
	return steps
}

func TestSetKeepingInstallEvents(t *testing.T) {
	s, _ := newIntegrationStatusStore(t)
	ctx := context.Background()

	install := map[string]interface{}{"status": "installing", "install_events": []interface{}{
		map[string]interface{}{"step": "detect_os", "status": "done"},
		map[string]interface{}{"step": "complete", "status": "done"},
	}}
	if _, err := s.SetKeepingInstallEvents(ctx, "api-1", "compliance", install); err != nil {
		t.Fatal(err)
	}

	stored, err := s.SetKeepingInstallEvents(ctx, "api-1", "compliance", map[string]interface{}{"status": "partial"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, "api-1", "compliance")
	if got["status"] != "partial" || len(installSteps(t, got)) != 2 || len(installSteps(t, stored)) != 2 {
		t.Fatalf("availability report should keep install events, got %v", got)
	}

	if err := s.ClearInstallEvents(ctx, "api-1", "compliance"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Get(ctx, "api-1", "compliance")
	if got["status"] != "partial" || len(installSteps(t, got)) != 0 {
		t.Fatalf("clear should empty only install events, got %v", got)
	}
}

func TestUpdateRetriesOnConcurrentWrite(t *testing.T) {
	s, client := newIntegrationStatusStore(t)
	ctx := context.Background()
	key := "integration_status:api-1:compliance"

	if err := s.Set(ctx, "api-1", "compliance", map[string]interface{}{"status": "partial"}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	err := s.update(ctx, "api-1", "compliance", func(current map[string]interface{}) (map[string]interface{}, bool) {
		calls++
		if calls == 1 {
			final := `{"status":"ready","install_events":[{"step":"complete","status":"done"}]}`
			if err := client.Set(ctx, key, final, 0).Err(); err != nil {
				t.Fatal(err)
			}
		}
		next := map[string]interface{}{"status": "partial", "install_events": current["install_events"]}
		return next, true
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected a retry after the concurrent write, got %d calls", calls)
	}
	got, _ := s.Get(ctx, "api-1", "compliance")
	if steps := installSteps(t, got); len(steps) != 1 || steps[0] != "complete" {
		t.Fatalf("concurrent complete event was lost: %v", got)
	}
}
