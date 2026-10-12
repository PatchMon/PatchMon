package context

import (
	stdctx "context"
	"errors"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
)

func TestWithConnectTimeout(t *testing.T) {
	cases := map[string]string{
		"postgresql://u:p@h:5432/d?sslmode=disable":   "postgresql://u:p@h:5432/d?connect_timeout=7&sslmode=disable",
		"postgresql://u:p@h:5432/d?connect_timeout=3": "postgresql://u:p@h:5432/d?connect_timeout=3",
		"host=h user=u dbname=d":                      "host=h user=u dbname=d",
	}
	for in, want := range cases {
		if got := withConnectTimeout(in, 7); got != want {
			t.Errorf("withConnectTimeout(%q) = %q, want %q", in, got, want)
		}
	}
}

func outageCache(host string) *PoolCache {
	reg := &Registry{byHost: map[string]*Entry{host: {Host: host, DatabaseURL: "postgresql://u:p@127.0.0.1:1/d"}}}
	return NewPoolCache(reg, &config.Config{DBConnectTimeout: 1}, 0, nil)
}

func TestGetOrCreateHonoursContextWhileLocked(t *testing.T) {
	pc := outageCache("a.example")
	lock := pc.hostLock("a.example")
	lock <- struct{}{}
	defer func() { <-lock }()

	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := pc.GetOrCreate(ctx, "a.example"); !errors.Is(err, stdctx.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("GetOrCreate ignored the context while another caller held the host lock")
	}
}

func TestGetOrCreateCachesFailure(t *testing.T) {
	pc := outageCache("a.example")
	_, first := pc.GetOrCreate(stdctx.Background(), "a.example")
	if first == nil {
		t.Fatal("expected the unreachable database to fail")
	}
	start := time.Now()
	if _, err := pc.GetOrCreate(stdctx.Background(), "a.example"); err != first {
		t.Fatalf("want the cached failure %v, got %v", first, err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("cached failure was not served immediately")
	}

	pc.Evict("a.example")
	if err := pc.recentFailure("a.example"); err != nil {
		t.Fatalf("Evict must clear the cached failure, got %v", err)
	}

	boom := errors.New("boom")

	pc.mu.Lock()
	pc.failures["a.example"] = failedEntry{err: boom, until: time.Now().Add(-time.Second)}
	pc.mu.Unlock()
	if err := pc.recentFailure("a.example"); err != nil {
		t.Fatalf("expired failure still returned: %v", err)
	}
}
