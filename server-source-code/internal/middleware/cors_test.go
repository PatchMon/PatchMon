package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func serveCORS(t *testing.T, configured string, resolver OriginResolver, method, host string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(method, "http://"+host+"/api/v1/health", nil)
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	CORS(configured, resolver)(next).ServeHTTP(rec, req)
	return rec
}

func browser(extra map[string]string) map[string]string {
	h := map[string]string{"Sec-Fetch-Mode": "cors"}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func TestCORS_HostEnforcement(t *testing.T) {
	t.Parallel()

	contextResolver := func(r *http.Request) (string, bool) {
		if strings.EqualFold(r.Header.Get("X-Forwarded-Host"), "ctx-a.example") {
			return "https://CTX-A.Example", true
		}
		return "", false
	}

	cases := []struct {
		name       string
		configured string
		resolver   OriginResolver
		method     string
		host       string
		headers    map[string]string
		wantStatus int
		wantCode   string
		wantACAO   string
	}{
		{
			name:       "list allows the second origin via forwarded host",
			configured: "http://localhost:3110,http://localhost:3112",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "localhost:3112", "Origin": "http://localhost:3112"}),
			wantStatus: http.StatusOK,
			wantACAO:   "http://localhost:3112",
		},
		{
			name:       "single origin rejects another forwarded host and reflects it",
			configured: "http://localhost:3110",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "localhost:3112"}),
			wantStatus: http.StatusForbidden,
			wantCode:   "cors_mismatch",
			wantACAO:   "http://localhost:3112",
		},
		{
			name:       "whitespace and trailing slashes are tolerated",
			configured: " http://localhost:3110/ , http://localhost:3112/ ",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "localhost:3112", "Origin": "http://localhost:3112"}),
			wantStatus: http.StatusOK,
			wantACAO:   "http://localhost:3112",
		},
		{
			name:       "upper-case configured origin matches the browser origin",
			configured: "HTTP://LOCALHOST:3112",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "localhost:3112", "Origin": "http://localhost:3112"}),
			wantStatus: http.StatusOK,
			wantACAO:   "http://localhost:3112",
		},
		{
			name:       "explicit default port matches the browser origin without one",
			configured: "https://patchmon.example.com:443",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "patchmon.example.com", "Origin": "https://patchmon.example.com"}),
			wantStatus: http.StatusOK,
			wantACAO:   "https://patchmon.example.com",
		},
		{
			name:       "forwarded host carrying a default port matches a bare origin",
			configured: "https://patchmon.example.com",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "patchmon.example.com:443"}),
			wantStatus: http.StatusOK,
		},
		{
			name:       "forwarded host on a non-default port does not match a bare origin",
			configured: "https://patchmon.example.com",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "patchmon.example.com:8443"}),
			wantStatus: http.StatusForbidden,
			wantCode:   "cors_mismatch",
			wantACAO:   "http://patchmon.example.com:8443",
		},
		{
			name:       "chained proxies appending to the forwarded host still match",
			configured: "http://patchmon,http://patchmon.local",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "(null), patchmon"}),
			wantStatus: http.StatusOK,
		},
		{
			name:       "disallowed forwarded host is rejected and the request origin reflected",
			configured: "http://patchmon",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "evil.example", "Origin": "http://evil.example"}),
			wantStatus: http.StatusForbidden,
			wantCode:   "cors_mismatch",
			wantACAO:   "http://evil.example",
		},
		{
			name:       "wildcard allows any host without credentials",
			configured: "*",
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "evil.example", "Origin": "http://evil.example"}),
			wantStatus: http.StatusOK,
			wantACAO:   "*",
		},
		{
			name:       "wildcard inside a list behaves as a wildcard",
			configured: "http://patchmon,*",
			host:       "evil.example",
			headers:    browser(map[string]string{"Origin": "http://evil.example"}),
			wantStatus: http.StatusOK,
			wantACAO:   "*",
		},
		{
			name:       "direct access from a disallowed host is rejected",
			configured: "http://patchmon",
			host:       "evil.example",
			headers:    browser(nil),
			wantStatus: http.StatusForbidden,
			wantCode:   "host_mismatch",
		},
		{
			name:       "direct access from an allowed host",
			configured: "http://patchmon",
			host:       "patchmon",
			headers:    browser(nil),
			wantStatus: http.StatusOK,
		},
		{
			name:       "direct access with a port matches a default-port origin",
			configured: "http://patchmon",
			host:       "patchmon:3000",
			headers:    browser(nil),
			wantStatus: http.StatusOK,
		},
		{
			name:       "direct access with the wrong port on an explicit-port origin is rejected",
			configured: "http://patchmon:3000",
			host:       "patchmon:3001",
			headers:    browser(nil),
			wantStatus: http.StatusForbidden,
			wantCode:   "host_mismatch",
		},
		{
			name:       "ipv4 loopback is never rejected",
			configured: "http://patchmon",
			host:       "127.0.0.1:3000",
			headers:    browser(nil),
			wantStatus: http.StatusOK,
		},
		{
			name:       "ipv6 loopback is never rejected",
			configured: "http://patchmon",
			host:       "[::1]:3000",
			headers:    browser(nil),
			wantStatus: http.StatusOK,
		},
		{
			name:       "agent connecting directly by IP is not subject to the host check",
			configured: "https://patchmon.example.com",
			host:       "10.0.0.5:3000",
			wantStatus: http.StatusOK,
		},
		{
			name:       "agent through a proxy on another hostname is not subject to the host check",
			configured: "https://patchmon.example.com",
			host:       "127.0.0.1:3001",
			headers:    map[string]string{"X-Forwarded-Host": "patchmon.internal.lan"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "plain-http navigation is recognised by its accept header",
			configured: "http://patchmon",
			host:       "evil.example",
			headers:    map[string]string{"Accept": "text/html,application/xhtml+xml,*/*;q=0.8"},
			wantStatus: http.StatusForbidden,
			wantCode:   "host_mismatch",
		},
		{
			name:       "plain-http same-origin fetch is recognised by its referer",
			configured: "http://patchmon",
			host:       "127.0.0.1:3001",
			headers:    map[string]string{"X-Forwarded-Host": "evil.example", "Referer": "http://evil.example/login", "Accept": "application/json"},
			wantStatus: http.StatusForbidden,
			wantCode:   "cors_mismatch",
			wantACAO:   "http://evil.example",
		},
		{
			name:       "origin header alone marks a browser request",
			configured: "http://patchmon",
			host:       "127.0.0.1:3001",
			headers:    map[string]string{"X-Forwarded-Host": "evil.example", "Origin": "http://evil.example"},
			wantStatus: http.StatusForbidden,
			wantCode:   "cors_mismatch",
			wantACAO:   "http://evil.example",
		},
		{
			name:       "preflight is answered without reaching the handler",
			configured: "http://patchmon",
			method:     http.MethodOptions,
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "patchmon", "Origin": "http://patchmon"}),
			wantStatus: http.StatusNoContent,
			wantACAO:   "http://patchmon",
		},
		{
			name:       "resolver scopes the allowed host to the request's own context",
			configured: "http://patchmon",
			resolver:   contextResolver,
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "ctx-a.example", "Origin": "https://ctx-a.example"}),
			wantStatus: http.StatusOK,
			wantACAO:   "https://ctx-a.example",
		},
		{
			name:       "resolver miss falls back to the configured list",
			configured: "http://patchmon",
			resolver:   contextResolver,
			host:       "127.0.0.1:3001",
			headers:    browser(map[string]string{"X-Forwarded-Host": "ctx-b.example", "Origin": "https://ctx-b.example"}),
			wantStatus: http.StatusForbidden,
			wantCode:   "cors_mismatch",
			wantACAO:   "https://ctx-b.example",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			rec := serveCORS(t, tc.configured, tc.resolver, method, tc.host, tc.headers)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.wantACAO {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantACAO)
			}
			wantCreds := tc.wantACAO != "" && tc.wantACAO != "*"
			if got := rec.Header().Get("Access-Control-Allow-Credentials") == "true"; got != wantCreds {
				t.Errorf("Access-Control-Allow-Credentials present = %v, want %v", got, wantCreds)
			}
			if tc.wantCode != "" {
				var body struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode body %q: %v", rec.Body.String(), err)
				}
				if body.Code != tc.wantCode {
					t.Errorf("code = %q, want %q", body.Code, tc.wantCode)
				}
			}
			if rec.Code != http.StatusForbidden && rec.Header().Get("Vary") != "Origin" {
				t.Errorf("Vary = %q, want Origin", rec.Header().Get("Vary"))
			}
		})
	}
}

func TestParseOrigins(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: []string{}},
		{name: "single", in: "http://localhost:3000", want: []string{"http://localhost:3000"}},
		{name: "list with whitespace", in: " http://a:1 , http://b:2 ", want: []string{"http://a:1", "http://b:2"}},
		{name: "trailing slash and path are dropped", in: "https://a.example/,https://b.example/login", want: []string{"https://a.example", "https://b.example"}},
		{name: "scheme and host are lower-cased", in: "HTTPS://Patchmon.Example.COM", want: []string{"https://patchmon.example.com"}},
		{name: "default ports are dropped", in: "http://a.example:80,https://b.example:443,http://c.example:8080", want: []string{"http://a.example", "https://b.example", "http://c.example:8080"}},
		{name: "ipv6 default port keeps brackets", in: "https://[::1]:443", want: []string{"https://[::1]"}},
		{name: "wildcard and empty entries", in: "*,,http://a.example,", want: []string{"*", "http://a.example"}},
		{name: "wildcard subdomain used by the cloud env is kept", in: "https://*.example.com,https://example.com", want: []string{"https://*.example.com", "https://example.com"}},
		{name: "non-url entries are kept untouched", in: "patchmon.example.com,localhost:3000", want: []string{"patchmon.example.com", "localhost:3000"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseOrigins(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("ParseOrigins(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateOrigins(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "empty", in: ""},
		{name: "wildcard", in: "*"},
		{name: "single origin", in: "https://patchmon.example.com"},
		{name: "list with ports and slashes", in: "http://a.example:3000/, https://b.example"},
		{name: "upper-case scheme", in: "HTTPS://a.example"},
		{name: "wildcard subdomain used by the cloud env", in: "https://*.example.com,https://example.com"},
		{name: "missing scheme", in: "patchmon.example.com", wantErr: true},
		{name: "host and port only", in: "localhost:3000", wantErr: true},
		{name: "path", in: "https://a.example/patchmon", wantErr: true},
		{name: "query", in: "https://a.example?x=1", wantErr: true},
		{name: "credentials", in: "https://user:pw@a.example", wantErr: true},
		{name: "unsupported scheme", in: "ftp://a.example", wantErr: true},
		{name: "bad entry in a list", in: "https://a.example,b.example", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateOrigins(tc.in)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateOrigins(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			}
		})
	}
}
