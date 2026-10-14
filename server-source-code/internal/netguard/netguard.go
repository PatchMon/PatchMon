// Package netguard validates and dials outbound HTTP destinations so that
// server-initiated requests (notification webhooks, ntfy) cannot be pointed at
// cloud-metadata or loopback addresses. RFC-1918 and ULA ranges stay reachable
// so self-hosters can target internal receivers.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a destination resolves to a denied address.
var ErrBlockedAddress = errors.New("destination address is not allowed")

// awsMetadataV6 is the AWS instance-metadata endpoint reachable over IPv6. It is a
// ULA address, which the policy otherwise allows, so it is denied by exact match.
var awsMetadataV6 = net.ParseIP("fd00:ec2::254")

// ValidateOutboundURL rejects URLs that are unsafe to fetch from the server:
// non-http(s) schemes, embedded userinfo, and hosts that resolve to loopback,
// link-local/cloud-metadata, or unspecified addresses. DNS resolution is
// best-effort; the guarded client re-checks the connect-time IP.
func ValidateOutboundURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	if u.User != nil {
		return errors.New("url must not contain userinfo")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("url host missing")
	}
	if isBlockedHostname(host) {
		return ErrBlockedAddress
	}
	if ip := net.ParseIP(host); ip != nil {
		if ipBlocked(ip) {
			return ErrBlockedAddress
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil
	}
	for _, a := range ips {
		if ipBlocked(a.IP) {
			return ErrBlockedAddress
		}
	}
	return nil
}

// GuardedClient returns an http.Client that re-checks every resolved connect-time
// IP against the deny rules (defeating DNS rebinding) and re-validates each
// redirect hop.
func GuardedClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlockedAddress
			}
			ip := net.ParseIP(host)
			if ip == nil || ipBlocked(ip) {
				return ErrBlockedAddress
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return ValidateOutboundURL(req.URL.String())
		},
	}
}

func isBlockedHostname(host string) bool {
	return strings.EqualFold(strings.TrimSuffix(host, "."), "metadata.google.internal")
}

func ipBlocked(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.Equal(awsMetadataV6) {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}
