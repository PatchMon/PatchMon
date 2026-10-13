package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

func TestRateLimitPerContext(t *testing.T) {
	client, _ := newRateLimitRedis(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	run := RateLimitPerContext(&hostctx.RedisResolver{Default: client}, "test-route", 2, time.Minute)(ok)
	other := RateLimitPerContext(&hostctx.RedisResolver{Default: client}, "other-route", 2, time.Minute)(ok)

	code := func(h http.Handler, ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := code(run, "203.0.113.1"); c != http.StatusOK {
		t.Fatalf("first request = %d", c)
	}
	if c := code(run, "203.0.113.2"); c != http.StatusOK {
		t.Fatalf("second request = %d", c)
	}
	if c := code(run, "203.0.113.3"); c != http.StatusTooManyRequests {
		t.Fatalf("third request from a different IP = %d, want 429", c)
	}
	if c := code(other, "203.0.113.1"); c != http.StatusOK {
		t.Fatalf("other route shared the bucket: %d", c)
	}

	unavailable := RateLimitPerContext(nil, "test-route", 1, time.Minute)(ok)
	if c := code(unavailable, "203.0.113.1"); c != http.StatusOK {
		t.Fatalf("no Redis = %d, want pass-through", c)
	}
}
