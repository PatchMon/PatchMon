package network

import (
	"net"
	"reflect"
	"testing"
)

func ips(ss ...string) []net.IP {
	var out []net.IP
	for _, s := range ss {
		out = append(out, net.ParseIP(s))
	}
	return out
}

func sampleAdapters() []adapterInfo {
	return []adapterInfo{
		{Name: "Loopback Pseudo-Interface 1", Up: true, Loopback: true, Gateways: ips("127.0.0.1"), DNS: ips("127.0.0.1")},
		{Name: "Wi-Fi", Up: true, Metric4: 50, Metric6: 50, Gateways: ips("192.168.1.1", "fe80::1"), DNS: ips("192.168.1.1", "fec0:0:0:ffff::1", "fec0:0:0:ffff::2")},
		{Name: "Ethernet", Up: true, Metric4: 25, Metric6: 25, Gateways: ips("10.0.0.1"), DNS: ips("10.0.0.53", "192.168.1.1", "fec0:0:0:ffff::3")},
		{Name: "Disconnected", Up: false, Metric4: 1, Gateways: ips("172.16.0.1"), DNS: ips("172.16.0.53")},
		{Name: "vEthernet (Default Switch)", Up: true, Metric4: 5000, DNS: ips("0.0.0.0")},
	}
}

func TestSelectDefaultGateway(t *testing.T) {
	if got := selectDefaultGateway(sampleAdapters()); got != "10.0.0.1" {
		t.Errorf("gateway = %q, want the lowest-metric up adapter's", got)
	}
	tie := []adapterInfo{
		{Name: "a", Up: true, Metric4: 10, Gateways: ips("10.0.0.1")},
		{Name: "b", Up: true, Metric4: 10, Gateways: ips("10.0.0.2")},
	}
	if got := selectDefaultGateway(tie); got != "10.0.0.1" {
		t.Errorf("tie = %q, want the first adapter", got)
	}
	v6only := []adapterInfo{{Name: "a", Up: true, Gateways: ips("fe80::1", "0.0.0.0")}}
	if got := selectDefaultGateway(v6only); got != "fe80::1" {
		t.Errorf("IPv6 fallback = %q", got)
	}
	if got := selectDefaultGateway([]adapterInfo{{Name: "down", Gateways: ips("10.0.0.1")}}); got != "" {
		t.Errorf("down adapter used: %q", got)
	}
	if got := selectDefaultGateway(nil); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestSelectDNSServers(t *testing.T) {
	want := []string{"10.0.0.53", "192.168.1.1"}
	if got := selectDNSServers(sampleAdapters()); !reflect.DeepEqual(got, want) {
		t.Errorf("dns = %v, want %v (lowest metric first, deduplicated, no fec0 placeholders, loopback or down)", got, want)
	}
	if got := selectDNSServers(nil); got == nil || len(got) != 0 {
		t.Errorf("empty must be a non-nil empty list, got %#v", got)
	}
	keep := []adapterInfo{{Name: "a", Up: true, DNS: ips("fec0::1", "2001:4860:4860::8888")}}
	if got := selectDNSServers(keep); !reflect.DeepEqual(got, []string{"fec0::1", "2001:4860:4860::8888"}) {
		t.Errorf("only fec0:0:0:ffff::/64 placeholders are dropped, got %v", got)
	}
}

func TestAdapterGatewayTables(t *testing.T) {
	v4, v6 := adapterGatewayTables(sampleAdapters())
	if v4["Wi-Fi"] != "192.168.1.1" || v4["Ethernet"] != "10.0.0.1" || v6["Wi-Fi"] != "fe80::1" {
		t.Errorf("v4 = %v, v6 = %v", v4, v6)
	}
	if _, ok := v6["Ethernet"]; ok {
		t.Errorf("Ethernet has no IPv6 gateway: %v", v6)
	}
}
