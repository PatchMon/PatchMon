//go:build windows

package network

import (
	"errors"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

func (m *Manager) adapters() []adapterInfo {
	list, err := readAdapters()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to read network adapters")
	}
	return list
}

func (m *Manager) gatewayIP() string { return selectDefaultGateway(m.adapters()) }

func (m *Manager) dnsServers() []string { return selectDNSServers(m.adapters()) }

func (m *Manager) interfaceGatewayTables() (ipv4, ipv6 map[string]string) {
	return adapterGatewayTables(m.adapters())
}

func readAdapters() ([]adapterInfo, error) {
	size := uint32(16 * 1024)
	flags := uint32(windows.GAA_FLAG_INCLUDE_GATEWAYS | windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST)
	for range 4 {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var out []adapterInfo
		for aa := first; aa != nil; aa = aa.Next {
			a := adapterInfo{
				Name:     windows.UTF16PtrToString(aa.FriendlyName),
				Up:       aa.OperStatus == windows.IfOperStatusUp,
				Loopback: aa.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK,
				Metric4:  aa.Ipv4Metric,
				Metric6:  aa.Ipv6Metric,
			}
			for gw := aa.FirstGatewayAddress; gw != nil; gw = gw.Next {
				a.Gateways = append(a.Gateways, cloneIP(gw.Address.IP()))
			}
			for dns := aa.FirstDnsServerAddress; dns != nil; dns = dns.Next {
				a.DNS = append(a.DNS, cloneIP(dns.Address.IP()))
			}
			out = append(out, a)
		}
		return out, nil
	}
	return nil, windows.ERROR_BUFFER_OVERFLOW
}

func cloneIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	return append(net.IP(nil), ip...)
}
