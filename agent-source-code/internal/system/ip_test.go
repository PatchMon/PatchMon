package system

import (
	"net"
	"testing"
)

// TestSelectPrimaryIPAddress verifies that IPv4 is preferred when available
// and that a global IPv6 address is used as a fallback on IPv6-only systems.
func TestSelectPrimaryIPAddress(t *testing.T) {
	tests := []struct {
		name string
		ips  []net.IP
		want string
	}{
		{
			name: "IPv4 only",
			ips: []net.IP{
				net.ParseIP("192.0.2.10"),
			},
			want: "192.0.2.10",
		},
		{
			name: "IPv6 only prefers global address over link-local",
			ips: []net.IP{
				net.ParseIP("fe80::10"),
				net.ParseIP("2001:db8::10"),
			},
			want: "2001:db8::10",
		},
		{
			name: "dual stack prefers IPv4",
			ips: []net.IP{
				net.ParseIP("2001:db8::10"),
				net.ParseIP("192.0.2.10"),
			},
			want: "192.0.2.10",
		},
		{
			name: "IPv4 link-local falls back to IPv6",
			ips: []net.IP{
				net.ParseIP("169.254.10.20"),
				net.ParseIP("2001:db8::10"),
			},
			want: "2001:db8::10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectPrimaryIPAddress(tt.ips)
			if got != tt.want {
				t.Fatalf("selectPrimaryIPAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}
