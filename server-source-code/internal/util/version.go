package util

import (
	"net"
	"strconv"
	"strings"
)

// CompareVersions compares two semantic versions.
// Returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal.
func CompareVersions(v1, v2 string) int {
	p1 := parseVersionParts(v1)
	p2 := parseVersionParts(v2)
	maxLen := len(p1)
	if len(p2) > maxLen {
		maxLen = len(p2)
	}
	for i := 0; i < maxLen; i++ {
		var a, b int
		if i < len(p1) {
			a = p1[i]
		}
		if i < len(p2) {
			b = p2[i]
		}
		if a > b {
			return 1
		}
		if a < b {
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.TrimPrefix(p, "v"))
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}

// LookupVersionFromDNS performs a DNS TXT lookup and returns the first record as version string.
func LookupVersionFromDNS(domain string) (string, error) {
	records, err := net.LookupTXT(domain)
	if err != nil || len(records) == 0 {
		return "", err
	}
	v := strings.Trim(strings.Trim(records[0], "\"'"), " ")
	return v, nil
}
