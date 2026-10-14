// Package network provides network-related functionality
package network

import (
	"fmt"
	"net"
	"os"
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
		info.GatewayIP = m.gatewayIP()
	}()
	go func() {
		defer wg.Done()
		info.DNSServers = m.dnsServers()
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

	gw4, gw6 := m.interfaceGatewayTables()

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
