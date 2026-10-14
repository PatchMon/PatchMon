package handler

import (
	"encoding/json"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestProxyHostBoundToHost(t *testing.T) {
	t.Parallel()

	ip := "203.0.113.5"
	hostname := "web01.example.com"
	ifaces := []models.NetworkInterface{{
		Name:      "eth0",
		Addresses: []models.NetworkAddress{{Address: "10.1.2.3/24", Family: "inet"}},
	}}
	raw, err := json.Marshal(ifaces)
	if err != nil {
		t.Fatalf("marshal interfaces: %v", err)
	}
	host := &models.Host{IP: &ip, Hostname: &hostname, NetworkInterfaces: models.JSON(raw)}

	tests := []struct {
		name      string
		proxyHost string
		want      bool
	}{
		{"registered ip", "203.0.113.5", true},
		{"registered hostname", "web01.example.com", true},
		{"hostname case insensitive", "WEB01.EXAMPLE.COM", true},
		{"interface address with mask", "10.1.2.3", true},
		{"localhost name", "localhost", true},
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"unrelated internal host", "10.9.9.9", false},
		{"cloud metadata", "169.254.169.254", false},
		{"empty", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := proxyHostBoundToHost(host, tc.proxyHost); got != tc.want {
				t.Fatalf("proxyHostBoundToHost(%q) = %v, want %v", tc.proxyHost, got, tc.want)
			}
		})
	}
}
