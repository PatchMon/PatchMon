package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

func TestRateLimitPerUser(t *testing.T) {
	client, _ := newRateLimitRedis(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := RateLimitPerUser(&hostctx.RedisResolver{Default: client}, "reset-tfa", 2, time.Minute)(ok)

	code := func(h http.Handler, userID, ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = ip + ":1234"
		if userID != "" {
			req = req.WithContext(context.WithValue(req.Context(), UserIDKey, userID))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	got := []int{code(h, "alice", "203.0.113.1"), code(h, "alice", "203.0.113.2"), code(h, "alice", "203.0.113.3"), code(h, "bob", "203.0.113.1")}
	want := []int{200, 200, 429, 200}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("codes = %v, want %v", got, want)
		}
	}
	if c := code(h, "", "203.0.113.1"); c != http.StatusUnauthorized {
		t.Fatalf("no user = %d, want 401", c)
	}
	if c := code(RateLimitPerUser(&hostctx.RedisResolver{}, "reset-tfa", 2, time.Minute)(ok), "alice", "203.0.113.1"); c != http.StatusServiceUnavailable {
		t.Fatalf("no Redis = %d, want fail-closed 503", c)
	}
	if c := code(RateLimitPerUser(nil, "reset-tfa", 2, time.Minute)(ok), "alice", "203.0.113.1"); c != http.StatusServiceUnavailable {
		t.Fatalf("nil resolver = %d, want 503", c)
	}
}
