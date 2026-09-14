package clientip

import (
	"net/http"
	"net/netip"
	"testing"
)

func request(remoteAddr, xff string) *http.Request {
	r := &http.Request{
		RemoteAddr: remoteAddr,
		Header:     http.Header{},
	}
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func mustTrusted(t *testing.T, entries ...string) []netip.Prefix {
	t.Helper()
	prefixes, invalid := ParseTrustedProxies(entries)
	if len(invalid) > 0 {
		t.Fatalf("unexpected invalid trusted proxy entries: %v", invalid)
	}
	return prefixes
}

// TestResolve_SpoofedLeftmostIgnored is the regression guard for the bug this
// package exists to fix. A client that sends its own X-Forwarded-For must not be
// able to choose the address PatchMon keys rate limiting and login lockout on.
// chi's RealIP returned the leftmost entry, which is exactly the forged value.
func TestResolve_SpoofedLeftmostIgnored(t *testing.T) {
	t.Parallel()

	// The client sent "X-Forwarded-For: 1.2.3.4"; the proxy appended the address
	// it actually saw, so the real client is the rightmost entry.
	r := request("10.0.0.1:9000", "1.2.3.4, 203.0.113.7")

	got := Resolve(r, nil)
	if got == "1.2.3.4" {
		t.Fatalf("regression: resolved to the client-supplied X-Forwarded-For entry %q; rate limits and lockout are spoofable", got)
	}
	if got != "203.0.113.7" {
		t.Fatalf("want 203.0.113.7 (the address the proxy appended), got %q", got)
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		remoteAddr string
		xff        string
		trusted    []string
		want       string
	}{
		{
			name:       "no forwarded header falls back to the peer",
			remoteAddr: "203.0.113.7:44321",
			want:       "203.0.113.7",
		},
		{
			name:       "single entry is the client",
			remoteAddr: "10.0.0.1:9000",
			xff:        "203.0.113.7",
			want:       "203.0.113.7",
		},
		{
			name:       "rightmost wins when no proxies are trusted",
			remoteAddr: "10.0.0.1:9000",
			xff:        "1.2.3.4, 198.51.100.9, 203.0.113.7",
			want:       "203.0.113.7",
		},
		{
			name:       "trusted hop is skipped to reach the real client",
			remoteAddr: "10.0.0.1:9000",
			xff:        "203.0.113.7, 10.0.0.5",
			trusted:    []string{"10.0.0.0/8"},
			want:       "203.0.113.7",
		},
		{
			name:       "chained trusted hops are skipped",
			remoteAddr: "10.0.0.1:9000",
			xff:        "203.0.113.7, 10.0.0.5, 10.0.0.6",
			trusted:    []string{"10.0.0.0/8"},
			want:       "203.0.113.7",
		},
		{
			name:       "spoofed entry behind a trusted hop is still ignored",
			remoteAddr: "10.0.0.1:9000",
			xff:        "1.2.3.4, 203.0.113.7, 10.0.0.5",
			trusted:    []string{"10.0.0.0/8"},
			want:       "203.0.113.7",
		},
		{
			name:       "all hops trusted returns the originating client",
			remoteAddr: "10.0.0.1:9000",
			xff:        "203.0.113.7, 10.0.0.5",
			trusted:    []string{"10.0.0.0/8", "203.0.113.0/24"},
			want:       "203.0.113.7",
		},
		{
			name:       "bare trusted IP is treated as a single host",
			remoteAddr: "10.0.0.1:9000",
			xff:        "203.0.113.7, 10.0.0.5",
			trusted:    []string{"10.0.0.5"},
			want:       "203.0.113.7",
		},
		{
			name:       "untrusted proxy address is returned rather than reaching further left",
			remoteAddr: "10.0.0.1:9000",
			xff:        "203.0.113.7, 192.0.2.50",
			trusted:    []string{"10.0.0.0/8"},
			want:       "192.0.2.50",
		},
		{
			name:       "ipv6 client",
			remoteAddr: "[fd00::1]:9000",
			xff:        "2001:db8::42",
			want:       "2001:db8::42",
		},
		{
			name:       "ipv6 entries with ports and brackets",
			remoteAddr: "[fd00::1]:9000",
			xff:        "[2001:db8::42]:1234",
			want:       "2001:db8::42",
		},
		{
			name:       "ipv4-mapped ipv6 is normalised",
			remoteAddr: "10.0.0.1:9000",
			xff:        "::ffff:203.0.113.7",
			want:       "203.0.113.7",
		},
		{
			name:       "malformed entry stops the walk and falls back to the peer",
			remoteAddr: "10.0.0.1:9000",
			xff:        "203.0.113.7, not-an-ip",
			want:       "10.0.0.1",
		},
		{
			name:       "empty forwarded header falls back to the peer",
			remoteAddr: "10.0.0.1:9000",
			xff:        "",
			want:       "10.0.0.1",
		},
		{
			name:       "peer without a port is still usable",
			remoteAddr: "203.0.113.7",
			want:       "203.0.113.7",
		},
		{
			name:       "whitespace around entries is tolerated",
			remoteAddr: "10.0.0.1:9000",
			xff:        "  1.2.3.4 ,   203.0.113.7   ",
			want:       "203.0.113.7",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Resolve(request(tc.remoteAddr, tc.xff), mustTrusted(t, tc.trusted...))
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	t.Parallel()

	prefixes, invalid := ParseTrustedProxies([]string{
		"10.0.0.0/8",
		" 192.168.1.1 ",
		"",
		"fd00::/8",
		"nonsense",
		"999.1.1.1",
	})

	if len(prefixes) != 3 {
		t.Fatalf("want 3 valid prefixes, got %d (%v)", len(prefixes), prefixes)
	}
	if len(invalid) != 2 {
		t.Fatalf("want 2 invalid entries reported, got %d (%v)", len(invalid), invalid)
	}

	// A bare address must behave as a single-host prefix.
	if !isTrusted(netip.MustParseAddr("192.168.1.1"), prefixes) {
		t.Fatalf("bare IP 192.168.1.1 should be trusted")
	}
	if isTrusted(netip.MustParseAddr("192.168.1.2"), prefixes) {
		t.Fatalf("192.168.1.2 must not be trusted by a bare 192.168.1.1 entry")
	}
}

func TestFromRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"ipv4 with port", "203.0.113.7:44321", "203.0.113.7"},
		{"ipv4 without port", "203.0.113.7", "203.0.113.7"},
		{"ipv6 bracketed with port", "[2001:db8::42]:44321", "2001:db8::42"},
		{"ipv6 bare", "2001:db8::42", "2001:db8::42"},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := FromRequest(request(tc.remoteAddr, "")); got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
