// Package network provides network-related functionality
package network

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"
)

// Manager handles network information collection using standard library and file parsing
type Manager struct {
	logger *logrus.Logger
}

// New creates a new network manager
func New(logger *logrus.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// GetNetworkInfo collects network information.
//
// OPTIMIZATION: Default gateway, DNS servers, and interface enumeration are
// independent and IO-bound. Running them in parallel lets their /proc reads
// and any subprocess spawns overlap.
func (m *Manager) GetNetworkInfo() models.NetworkInfo {
	var info models.NetworkInfo

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		info.GatewayIP = m.getGatewayIP()
	}()
	go func() {
		defer wg.Done()
		info.DNSServers = m.getDNSServers()
	}()
	go func() {
		defer wg.Done()
		info.NetworkInterfaces = m.getNetworkInterfaces()
	}()

	wg.Wait()

	m.logger.WithFields(logrus.Fields{
		"gateway":     info.GatewayIP,
		"dns_servers": len(info.DNSServers),
		"interfaces":  len(info.NetworkInterfaces),
	}).Debug("Collected gateway, DNS, and interface information")

	return info
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

// hexToIPv4 converts hex IP address to dotted decimal notation
func (m *Manager) hexToIPv4(hexIP string) string {
	if len(hexIP) != 8 {
		return ""
	}

	// Convert little-endian hex to IP
	ip := make([]byte, 4)
	for i := 0; i < 4; i++ {
		if val, err := parseHexByte(hexIP[6-i*2 : 8-i*2]); err == nil {
			ip[i] = val
		} else {
			return ""
		}
	}

	return net.IP(ip).String()
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

// parseHexByte parses a 2-character hex string to byte
func parseHexByte(hex string) (byte, error) {
	var result byte
	for _, c := range hex {
		result <<= 4
		if c >= '0' && c <= '9' {
			result += byte(c - '0')
		} else if c >= 'A' && c <= 'F' {
			result += byte(c - 'A' + 10)
		} else if c >= 'a' && c <= 'f' {
			result += byte(c - 'a' + 10)
		} else {
			return 0, fmt.Errorf("invalid hex character: %c", c)
		}
	}
	return result, nil
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

// getNetworkInterfaces gets network interface information using standard library
//
// OPTIMIZATION: Previously this ran `ip route show dev <iface>` twice per
// interface (once for IPv4, once for IPv6), giving 2*N serial subprocesses.
// Now we shell out at most twice — a single `ip route` and `ip -6 route` —
// parse once, and look up results per interface in O(1).
func (m *Manager) getNetworkInterfaces() []models.NetworkInterface {
	interfaces, err := net.Interfaces()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get network interfaces")
		return []models.NetworkInterface{}
	}

	gw4, gw6 := m.gatewayTablesByInterface()

	var result []models.NetworkInterface

	for _, iface := range interfaces {
		// Skip loopback interface
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// Get IP addresses for this interface
		var addresses []models.NetworkAddress

		addrs, err := iface.Addrs()
		if err != nil {
			m.logger.WithError(err).WithField("interface", iface.Name).Warn("Failed to get addresses for interface")
			continue
		}

		ipv4Gateway := gw4[iface.Name]
		ipv6Gateway := gw6[iface.Name]
		if ipv4Gateway == "" {
			ipv4Gateway = m.getInterfaceGatewayFromProc(iface.Name)
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				var family string
				var gateway string

				if ipnet.IP.To4() != nil {
					family = constants.IPFamilyIPv4
					gateway = ipv4Gateway
				} else {
					family = constants.IPFamilyIPv6
					// Check if this is a link-local address (fe80::/64)
					// Link-local addresses don't have gateways
					if ipnet.IP.IsLinkLocalUnicast() {
						gateway = "" // No gateway for link-local addresses
					} else {
						gateway = ipv6Gateway
					}
				}

				// Calculate netmask in CIDR notation
				ones, _ := ipnet.Mask.Size()
				netmask := fmt.Sprintf("/%d", ones)

				addresses = append(addresses, models.NetworkAddress{
					Address: ipnet.IP.String(),
					Family:  family,
					Netmask: netmask,
					Gateway: gateway,
				})
			}
		}

		// Include interface even if it has no addresses (to show MAC, status, etc.)
		// But prefer interfaces with addresses
		if len(addresses) > 0 || iface.Flags&net.FlagUp != 0 {
			// Determine interface type
			interfaceType := constants.NetTypeEthernet
			if strings.HasPrefix(iface.Name, "wl") || strings.HasPrefix(iface.Name, "wifi") {
				interfaceType = constants.NetTypeWiFi
			} else if strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "br-") {
				interfaceType = constants.NetTypeBridge
			}

			// Get MAC address
			macAddress := ""
			if len(iface.HardwareAddr) > 0 {
				macAddress = iface.HardwareAddr.String()
			}

			// Get status
			status := "down"
			if iface.Flags&net.FlagUp != 0 {
				status = "up"
			}

			// Get link speed and duplex
			linkSpeed, duplex := m.getLinkSpeedAndDuplex(iface.Name)

			result = append(result, models.NetworkInterface{
				Name:       iface.Name,
				Type:       interfaceType,
				MACAddress: macAddress,
				MTU:        iface.MTU,
				Status:     status,
				LinkSpeed:  linkSpeed,
				Duplex:     duplex,
				Addresses:  addresses,
			})
		}
	}

	return result
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

// getInterfaceGatewayFromProc reads /proc/net/route for a per-interface IPv4
// default route. Used as a fallback when the batched `ip route` call didn't
// populate the interface (e.g. `ip` binary unavailable).
func (m *Manager) getInterfaceGatewayFromProc(interfaceName string) string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == interfaceName && fields[1] == "00000000" {
			if gateway := m.hexToIPv4(fields[2]); gateway != "" {
				return gateway
			}
		}
	}
	return ""
}

// getLinkSpeedAndDuplex gets the link speed (in Mbps) and duplex mode for an interface
func (m *Manager) getLinkSpeedAndDuplex(interfaceName string) (int, string) {
	// Read speed from /sys/class/net/<interface>/speed
	speedPath := fmt.Sprintf("/sys/class/net/%s/speed", interfaceName)
	speedData, err := os.ReadFile(speedPath)
	if err != nil {
		// Speed not available (common for virtual interfaces)
		return -1, ""
	}

	speedStr := strings.TrimSpace(string(speedData))
	speed, err := strconv.Atoi(speedStr)
	if err != nil {
		return -1, ""
	}

	// Read duplex from /sys/class/net/<interface>/duplex
	duplexPath := fmt.Sprintf("/sys/class/net/%s/duplex", interfaceName)
	duplexData, err := os.ReadFile(duplexPath)
	if err != nil {
		return speed, ""
	}

	duplex := strings.TrimSpace(string(duplexData))
	// Normalize duplex values
	if duplex == "full" || duplex == "half" {
		return speed, duplex
	}

	return speed, ""
}
