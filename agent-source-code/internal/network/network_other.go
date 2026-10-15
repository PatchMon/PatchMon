//go:build !windows

package network

import (
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func (m *Manager) gatewayIP() string { return m.getGatewayIP() }

func (m *Manager) dnsServers() []string { return m.getDNSServers() }

func (m *Manager) interfaceGatewayTables() (ipv4, ipv6 map[string]string) {
	return m.gatewayTablesByInterface()
}

// getGatewayIP tries to get the default gateway IP (IPv4 first, then IPv6)
func (m *Manager) getGatewayIP() string {
	// Try IPv4 first
	if gw := m.getIPv4GatewayIP(); gw != "" {
		return gw
	}

	// If no IPv4 gateway found, try IPv6
	if gw := m.getIPv6GatewayIP(); gw != "" {
		return gw
	}

	return ""
}

// getIPv4GatewayIP gets the default gateway IP from IPv4 routing table
func (m *Manager) getIPv4GatewayIP() string {
	// Try reading /proc/net/route first (Linux)
	data, err := os.ReadFile("/proc/net/route")
	if err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			fields := strings.Fields(line)
			// Field 1 is Destination, Field 2 is Gateway
			if len(fields) >= 3 && fields[1] == "00000000" { // Default route
				// Convert hex gateway to IP
				if gateway := m.hexToIPv4(fields[2]); gateway != "" {
					return gateway
				}
			}
		}
	} else {
		// Fallback to netstat for non-Linux systems (FreeBSD, macOS, etc.)
		m.logger.WithError(err).Debug("Failed to read /proc/net/route, trying netstat")
		return m.getGatewayViaNetstat(false)
	}

	return ""
}

// getIPv6GatewayIP gets the default gateway IP from IPv6 routing table
func (m *Manager) getIPv6GatewayIP() string {
	// Try reading /proc/net/ipv6_route first (Linux)
	data, err := os.ReadFile("/proc/net/ipv6_route")
	if err != nil {
		m.logger.WithError(err).Debug("Failed to read /proc/net/ipv6_route, trying netstat")
		return m.getGatewayViaNetstat(true)
	}

	// Format of /proc/net/ipv6_route:
	// 1. Dest network (32 hex chars)
	// 2. Prefix length (2 hex chars)
	// 3. Source network (32 hex chars)
	// 4. Source prefix length (2 hex chars)
	// 5. Next hop / Gateway (32 hex chars)
	// ... other flags ...

	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			dest := fields[0]
			prefixLen := fields[1]
			gatewayHex := fields[4]

			// Check for default route: Dest is all zeros and prefix length is 00
			if dest == "00000000000000000000000000000000" && prefixLen == "00" {
				// Ignore if gateway is also 0 (means on-link) unless that's what we want,
				// but usually we want the router address.
				if gatewayHex != "00000000000000000000000000000000" {
					return m.hexToIPv6(gatewayHex)
				}
			}
		}
	}

	return ""
}

// getGatewayViaNetstat gets the default gateway using netstat command
// Works on FreeBSD, macOS, and other BSD-like systems
// ipv6 indicates whether to get IPv6 gateway (true) or IPv4 gateway (false)
func (m *Manager) getGatewayViaNetstat(ipv6 bool) string {
	var cmd *exec.Cmd

	if ipv6 {
		// Get IPv6 default gateway
		cmd = exec.Command("netstat", "-rn", "-f", "inet6")
	} else {
		// Get IPv4 default gateway
		cmd = exec.Command("netstat", "-rn", "-f", "inet")
	}

	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("Failed to run netstat command")
		return ""
	}

	// Parse netstat output
	// Format: Destination  Gateway  Flags  Netif Expire
	// Look for default route
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Check for default route
		// IPv4: "default" or "0.0.0.0"
		// IPv6: "default" or "::/0"
		if fields[0] == "default" || fields[0] == "0.0.0.0" || fields[0] == "::/0" {
			gateway := fields[1]
			// Skip if gateway is link-local or link#
			if !strings.HasPrefix(gateway, "link#") && !strings.HasPrefix(gateway, "fe80:") {
				// Validate it's an IP address
				if net.ParseIP(gateway) != nil {
					return gateway
				}
			}
		}
	}

	return ""
}

// hexToIPv6 converts standard hex IPv6 string (32 chars) to IP string
func (m *Manager) hexToIPv6(hexIP string) string {
	if len(hexIP) != 32 {
		return ""
	}
	// IPv6 in /proc/net/ipv6_route is simply a 32-char hex string (Big Endian usually)
	bytes, err := hex.DecodeString(hexIP)
	if err != nil {
		return ""
	}
	return net.IP(bytes).String()
}

// getDNSServers gets the configured DNS servers from resolv.conf
func (m *Manager) getDNSServers() []string {
	// Initialize as empty slice (not nil) to ensure JSON marshals as [] instead of null
	servers := []string{}

	// Read /etc/resolv.conf
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		m.logger.WithError(err).Warn("Failed to read /etc/resolv.conf")
		return servers
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				servers = append(servers, fields[1])
			}
		}
	}

	return servers
}

// gatewayTablesByInterface shells out once for IPv4 and once for IPv6 (in
// parallel) and returns two maps: interface name -> default gateway address.
// This replaces the per-interface, per-family subprocess spawn that used to
// dominate network collection on hosts with many interfaces.
func (m *Manager) gatewayTablesByInterface() (ipv4, ipv6 map[string]string) {
	ipv4 = make(map[string]string)
	ipv6 = make(map[string]string)

	if _, err := exec.LookPath("ip"); err != nil {
		return ipv4, ipv6
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		out, err := exec.Command("ip", "route", "show").Output()
		if err != nil {
			return
		}
		parseIPRouteDefaults(string(out), ipv4)
	}()
	go func() {
		defer wg.Done()
		out, err := exec.Command("ip", "-6", "route", "show").Output()
		if err != nil {
			return
		}
		parseIPRouteDefaults(string(out), ipv6)
	}()
	wg.Wait()

	return ipv4, ipv6
}

// parseIPRouteDefaults scans `ip route show` / `ip -6 route show` output for
// default routes and records "iface -> gateway" in out. Lines look like:
//
//	default via 10.0.0.1 dev eth0 proto dhcp src 10.0.0.42 metric 100
//	0.0.0.0/0 via 10.0.0.1 dev eth0
//	::/0 via fe80::1 dev eth0
//
// We only care about default routes that have an explicit "via" and "dev".
func parseIPRouteDefaults(output string, out map[string]string) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if fields[0] != "default" && fields[0] != "0.0.0.0/0" && fields[0] != "::/0" {
			continue
		}
		if fields[1] != "via" {
			continue
		}
		gateway := fields[2]
		// Find "dev <iface>" anywhere after the gateway token
		for i := 3; i+1 < len(fields); i++ {
			if fields[i] == "dev" {
				iface := fields[i+1]
				if _, exists := out[iface]; !exists {
					out[iface] = gateway
				}
				break
			}
		}
	}
}
