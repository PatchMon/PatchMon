package network

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

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

// GetNetworkInfo collects network information
func (m *Manager) GetNetworkInfo() models.NetworkInfo {
	info := models.NetworkInfo{
		GatewayIP:         m.getGatewayIP(),
		DNSServers:        m.getDNSServers(),
		NetworkInterfaces: m.getNetworkInterfaces(),
	}

	m.logger.WithFields(logrus.Fields{
		"gateway":     info.GatewayIP,
		"dns_servers": len(info.DNSServers),
		"interfaces":  len(info.NetworkInterfaces),
	}).Debug("Collected gateway, DNS, and interface information")

	return info
}

// getGatewayIP gets the default gateway IP from routing table file
func (m *Manager) getGatewayIP() string {
	// Read /proc/net/route to find default gateway
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		m.logger.WithError(err).Warn("Failed to read /proc/net/route")
		return ""
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[1] == "00000000" { // Default route
			// Convert hex gateway to IP
			if gateway := m.hexToIP(fields[2]); gateway != "" {
				return gateway
			}
		}
	}

	return ""
}

// hexToIP converts hex IP address to dotted decimal notation
func (m *Manager) hexToIP(hexIP string) string {
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
func (m *Manager) getNetworkInterfaces() []models.NetworkInterface {
	interfaces, err := net.Interfaces()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get network interfaces")
		return []models.NetworkInterface{}
	}

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

		// Get gateways for this interface (separate for IPv4 and IPv6)
		ipv4Gateway := m.getInterfaceGateway(iface.Name, false)
		ipv6Gateway := m.getInterfaceGateway(iface.Name, true)

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

// getInterfaceGateway gets the gateway IP for a specific interface
// ipv6 specifies whether to get IPv6 gateway (true) or IPv4 gateway (false)
func (m *Manager) getInterfaceGateway(interfaceName string, ipv6 bool) string {
	// Try using 'ip route' command first (more reliable)
	if _, err := exec.LookPath("ip"); err == nil {
		var cmd *exec.Cmd
		if ipv6 {
			// Use ip -6 route for IPv6
			cmd = exec.Command("ip", "-6", "route", "show", "dev", interfaceName)
		} else {
			// Use ip route (defaults to IPv4)
			cmd = exec.Command("ip", "route", "show", "dev", interfaceName)
		}

		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				fields := strings.Fields(line)
				// Look for default route: "default via <gateway> dev <interface>"
				if len(fields) >= 3 && fields[0] == "default" && fields[1] == "via" {
					return fields[2]
				}
				// Look for route with gateway: "0.0.0.0/0 via <gateway>" (IPv4) or "::/0 via <gateway>" (IPv6)
				if len(fields) >= 4 {
					if !ipv6 && fields[0] == "0.0.0.0/0" && fields[1] == "via" {
						return fields[2]
					}
					if ipv6 && fields[0] == "::/0" && fields[1] == "via" {
						return fields[2]
					}
				}
			}
		}
	}

	// Fallback: parse /proc/net/route for IPv4 (IPv6 routing is more complex)
	if !ipv6 {
		data, err := os.ReadFile("/proc/net/route")
		if err != nil {
			return ""
		}

		for line := range strings.SplitSeq(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 3 && fields[0] == interfaceName && fields[1] == "00000000" {
				// Default route for this interface
				if gateway := m.hexToIP(fields[2]); gateway != "" {
					return gateway
				}
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
