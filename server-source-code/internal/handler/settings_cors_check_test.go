package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
)

func TestCheckServerURLCORS(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		corsOrigin  string
		serverURL   string
		wantAllowed bool
		wantReason  string
	}{
		{"allowed host", "https://pm.example.com", "https://pm.example.com", true, ""},
		{"disallowed host", "https://pm.example.com", "https://other.example.com", false, "host_not_in_cors_origin"},
		{"default port equivalence", "https://pm.example.com", "https://pm.example.com:443/", true, ""},
		{"wildcard", "*", "http://10.0.0.5:3000", true, ""},
		{"list with spaces and slash", "https://a.example/ , https://pm.example.com/", "https://pm.example.com", true, ""},
		{"ip server url not listed", "https://pm.example.com", "http://192.168.1.10:3000", false, "host_not_in_cors_origin"},
		{"ip server url listed", "http://192.168.1.10:3000", "http://192.168.1.10:3000", true, ""},
		{"invalid url", "https://pm.example.com", "pm.example.com", false, "server_url_invalid"},
		{"invalid scheme", "https://pm.example.com", "ftp://pm.example.com", false, "server_url_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got serverURLCORSCheckResponse
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = checkServerURLCORS(r, tc.serverURL)
			})
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3001/api/v1/settings/server-url/cors-check", nil)
			req.Header.Set("X-Forwarded-Host", "admin.invalid")
			middleware.CORS(tc.corsOrigin+",https://admin.invalid", nil)(next).ServeHTTP(httptest.NewRecorder(), req)

			if got.ServerURL != tc.serverURL || got.Allowed != tc.wantAllowed || got.Reason != tc.wantReason {
				t.Fatalf("got %+v, want allowed=%v reason=%q", got, tc.wantAllowed, tc.wantReason)
			}
		})
	}
}
