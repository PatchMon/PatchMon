package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/clientip"
)

// TestRealIP_RewritesRemoteAddr verifies the middleware puts the resolved
// address into RemoteAddr, which is the contract every downstream consumer
// (rate limiting, API auth, login lockout, audit logging) depends on.
func TestRealIP_RewritesRemoteAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		remoteAddr string
		xff        string
		trusted    []string
		want       string
	}{
		{
			name:       "spoofed leftmost entry is ignored",
			remoteAddr: "10.0.0.1:9000",
			xff:        "1.2.3.4, 203.0.113.7",
			want:       "203.0.113.7",
		},
		{
			name:       "no header leaves the peer address",
			remoteAddr: "203.0.113.7:44321",
			want:       "203.0.113.7",
		},
		{
			name:       "public peer without a proxy cannot spoof",
			remoteAddr: "198.51.100.210:5555",
			xff:        "203.0.113.9",
			want:       "198.51.100.210",
		},
		{
			name:       "trusted hop is skipped",
			remoteAddr: "10.0.0.1:9000",
			xff:        "203.0.113.7, 10.0.0.5",
			trusted:    []string{"10.0.0.0/8"},
			want:       "203.0.113.7",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			trusted, invalid := clientip.ParseTrustedProxies(tc.trusted)
			if len(invalid) > 0 {
				t.Fatalf("unexpected invalid trusted entries: %v", invalid)
			}

			var got string
			handler := RealIP(trusted, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}

			handler.ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.want {
				t.Fatalf("RemoteAddr: want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestRealIP_ConsumersAgree guards the invariant that the helpers used across
// the codebase all resolve to the same value once the middleware has run. If
// one of them starts reading X-Forwarded-For directly again, this fails.
func TestRealIP_ConsumersAgree(t *testing.T) {
	t.Parallel()

	handler := RealIP(nil, nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		rateLimitIP := rateLimitClientIP(r)
		apiAuthIP := clientIPFromRequest(r)

		if rateLimitIP != "203.0.113.7" {
			t.Errorf("rate limit keyed on %q, want 203.0.113.7 (spoofable if it reads XFF itself)", rateLimitIP)
		}
		if apiAuthIP != "203.0.113.7" {
			t.Errorf("api auth resolved %q, want 203.0.113.7", apiAuthIP)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9000"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.7")

	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestRealIP_WarnsOncePerUntrustedPeer(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	handler := RealIP(nil, slog.New(slog.NewTextHandler(&buf, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	send := func(peer string) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = peer + ":5555"
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	for range 3 {
		send("198.51.100.1")
		send("198.51.100.2")
	}
	for i := 10; i < 30; i++ {
		send("198.51.100." + strconv.Itoa(i))
	}
	send("10.0.0.1")
	if got := strings.Count(buf.String(), "ignoring X-Forwarded-For"); got != maxUntrustedPeerWarnings {
		t.Fatalf("want %d warnings (one per distinct peer, capped), got %d:\n%s", maxUntrustedPeerWarnings, got, buf.String())
	}
	if !strings.Contains(buf.String(), "peer=198.51.100.2") {
		t.Error("second distinct peer was not reported")
	}
}
