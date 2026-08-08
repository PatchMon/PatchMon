// Package clientip resolves the client IP address of an HTTP request from
// forwarded headers, in a way that cannot be spoofed by the client.
//
// It exists because chi's middleware.RealIP was deprecated for taking the
// LEFTMOST X-Forwarded-For entry (GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp,
// GHSA-9g5q-2w5x-hmxf). That entry is whatever the client chose to send, so any
// caller could pick their own rate-limit bucket, evade login lockout, or poison
// audit log IPs simply by setting the header.
//
// Every consumer of the client IP (rate limiting, API auth, login lockout,
// auto-enrolment, device fingerprinting, audit logging) must resolve it through
// this package so they all agree on one trustworthy value.
package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxies converts CIDR strings (or bare IPs) into prefixes.
//
// Bare addresses are treated as single-host prefixes, so "10.0.0.5" and
// "10.0.0.5/32" mean the same thing. Empty entries are skipped. Invalid entries
// are returned in the second slice so the caller can log them rather than
// silently trusting a smaller set than the operator intended.
func ParseTrustedProxies(entries []string) ([]netip.Prefix, []string) {
	var prefixes []netip.Prefix
	var invalid []string

	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}

		if prefix, err := netip.ParsePrefix(entry); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}

		addr, err := netip.ParseAddr(entry)
		if err != nil {
			invalid = append(invalid, entry)
			continue
		}
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return prefixes, invalid
}

// Resolve returns the client IP for a request, or "" when nothing usable is
// found.
//
// X-Forwarded-For is walked from the RIGHT. A reverse proxy appends the address
// it actually saw (nginx's proxy_add_x_forwarded_for expands to
// "$http_x_forwarded_for, $remote_addr"), so the rightmost entry is the only one
// the infrastructure vouches for. Anything the client forged sits to its left
// and is ignored.
//
// Entries belonging to trustedProxies are skipped while walking, so a chain such
// as Cloudflare in front of Nginx Proxy Manager resolves to the original client
// rather than to Cloudflare's egress address. When every entry is trusted, the
// leftmost is returned: the chain is fully accounted for, so that entry is the
// real client.
//
// With trustedProxies empty the rightmost entry is used, which is correct for
// the single reverse proxy PatchMon's Docker deployment assumes.
func Resolve(r *http.Request, trustedProxies []netip.Prefix) string {
	peer := hostOnly(r.RemoteAddr)

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peer
	}

	entries := strings.Split(xff, ",")
	for i := len(entries) - 1; i >= 0; i-- {
		addr, ok := parseAddr(entries[i])
		if !ok {
			// A malformed entry means the chain can no longer be trusted past
			// this point. Stop rather than reaching further left into values
			// the proxy never validated.
			return peer
		}
		if !isTrusted(addr, trustedProxies) {
			return addr.String()
		}
	}

	// Every hop was trusted, so the leftmost entry is the originating client.
	if addr, ok := parseAddr(entries[0]); ok {
		return addr.String()
	}
	return peer
}

// FromRequest returns the client IP for a request that has already passed
// through the RealIP middleware, which rewrites RemoteAddr to the resolved
// address. Use this in handlers instead of parsing RemoteAddr by hand, so IPv6
// addresses are not mangled by naive colon splitting.
func FromRequest(r *http.Request) string {
	return hostOnly(r.RemoteAddr)
}

// hostOnly strips the port from a "host:port" pair, tolerating values that
// carry no port and IPv6 literals in either bracketed or bare form.
func hostOnly(remoteAddr string) string {
	addr := strings.TrimSpace(remoteAddr)
	if addr == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(addr); err == nil {
		return strings.Trim(host, "[]")
	}

	// No port, or an unbracketed IPv6 literal that SplitHostPort rejects.
	return strings.Trim(addr, "[]")
}

// parseAddr parses one X-Forwarded-For entry, tolerating surrounding spaces,
// bracketed IPv6 literals, and entries that carry a port.
func parseAddr(entry string) (netip.Addr, bool) {
	candidate := hostOnly(entry)
	if candidate == "" {
		return netip.Addr{}, false
	}

	addr, err := netip.ParseAddr(candidate)
	if err != nil {
		return netip.Addr{}, false
	}

	// Normalise IPv4-mapped IPv6 (::ffff:203.0.113.7) to plain IPv4 so trusted
	// prefix matching and downstream string comparisons behave predictably.
	return addr.Unmap(), true
}

func isTrusted(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
