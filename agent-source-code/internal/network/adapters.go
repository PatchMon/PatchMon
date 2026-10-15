package network

import (
	"net"
	"sort"
)

type adapterInfo struct {
	Name     string
	Up       bool
	Loopback bool
	Metric4  uint32
	Metric6  uint32
	Gateways []net.IP
	DNS      []net.IP
}

var siteLocalDNSPlaceholder = &net.IPNet{IP: net.ParseIP("fec0:0:0:ffff::"), Mask: net.CIDRMask(64, 128)}

func usableAdapters(adapters []adapterInfo) []adapterInfo {
	var out []adapterInfo
	for _, a := range adapters {
		if a.Up && !a.Loopback {
			out = append(out, a)
		}
	}
	return out
}

func gatewaysOf(a adapterInfo, v4 bool) []net.IP {
	var out []net.IP
	for _, ip := range a.Gateways {
		if ip == nil || ip.IsUnspecified() || (ip.To4() != nil) != v4 {
			continue
		}
		out = append(out, ip)
	}
	return out
}

func lowestMetricGateway(adapters []adapterInfo, v4 bool) string {
	best := ""
	var bestMetric uint32
	for _, a := range usableAdapters(adapters) {
		gws := gatewaysOf(a, v4)
		if len(gws) == 0 {
			continue
		}
		metric := a.Metric6
		if v4 {
			metric = a.Metric4
		}
		if best == "" || metric < bestMetric {
			best, bestMetric = gws[0].String(), metric
		}
	}
	return best
}

func selectDefaultGateway(adapters []adapterInfo) string {
	if gw := lowestMetricGateway(adapters, true); gw != "" {
		return gw
	}
	return lowestMetricGateway(adapters, false)
}

func selectDNSServers(adapters []adapterInfo) []string {
	usable := usableAdapters(adapters)
	sort.SliceStable(usable, func(i, j int) bool { return usable[i].Metric4 < usable[j].Metric4 })
	seen := map[string]bool{}
	out := []string{}
	for _, a := range usable {
		for _, ip := range a.DNS {
			if ip == nil || ip.IsUnspecified() || siteLocalDNSPlaceholder.Contains(ip) {
				continue
			}
			s := ip.String()
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

func adapterGatewayTables(adapters []adapterInfo) (ipv4, ipv6 map[string]string) {
	ipv4 = map[string]string{}
	ipv6 = map[string]string{}
	for _, a := range adapters {
		if gws := gatewaysOf(a, true); len(gws) > 0 {
			ipv4[a.Name] = gws[0].String()
		}
		if gws := gatewaysOf(a, false); len(gws) > 0 {
			ipv6[a.Name] = gws[0].String()
		}
	}
	return ipv4, ipv6
}
