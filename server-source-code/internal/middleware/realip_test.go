package middleware

import (
	"net/http"
	"net/http/httptest"
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
			handler := RealIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
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

	handler := RealIP(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
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
