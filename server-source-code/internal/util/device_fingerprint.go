package util

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
)

// GenerateDeviceFingerprint produces a fingerprint from request data.
// Matches Node session_manager.js: deviceId + userAgent + acceptLanguage + ipSubnet.
// Returns empty string if X-Device-ID is missing.
func GenerateDeviceFingerprint(r *http.Request) string {
	deviceID := r.Header.Get("X-Device-ID")
	if deviceID == "" {
		return ""
	}
	userAgent := r.Header.Get("User-Agent")
	acceptLanguage := r.Header.Get("Accept-Language")
	ipSubnet := extractIPSubnet(r)
	fingerprintData := strings.Join([]string{deviceID, userAgent, acceptLanguage, ipSubnet}, "|")
	hash := sha256.Sum256([]byte(fingerprintData))
	return hex.EncodeToString(hash[:])[:32]
}

var ipv4SubnetRe = regexp.MustCompile(`(\d+\.\d+\.\d+)\.\d+`)

func extractIPSubnet(r *http.Request) string {
	ip := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First IP in X-Forwarded-For is the client
		if idx := strings.Index(xff, ","); idx != -1 {
			ip = strings.TrimSpace(xff[:idx])
		} else {
			ip = strings.TrimSpace(xff)
		}
	}
	// Strip port if present
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		possiblePort := ip[idx+1:]
		if len(possiblePort) <= 5 && strings.Trim(possiblePort, "0123456789") == "" {
			ip = ip[:idx]
		}
	}
	return getSubnet(ip)
}

func getSubnet(ip string) string {
	if ip == "" {
		return ""
	}
	// IPv4: first 3 octets
	if m := ipv4SubnetRe.FindStringSubmatch(ip); len(m) != 0 {
		return m[1]
	}
	// IPv6: first 4 segments
	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) >= 4 {
			return strings.Join(parts[:4], ":")
		}
	}
	return ip
}
