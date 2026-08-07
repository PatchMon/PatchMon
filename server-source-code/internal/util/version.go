package util

import (
	"net"
	"strconv"
	"strings"
)

// CompareVersions compares two semantic versions.
// Returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal.
//
// Pre-release versions are supported and rank BELOW their release, per semver
// §11: 2.0.3-rc.4 < 2.0.3. Edge images built from main carry a pre-release
// version (see .github/actions/release-context), so this ordering is what lets
// an edge instance recognise the eventual release as an upgrade. Getting it
// backwards would strand edge users on a mid-cycle build forever.
func CompareVersions(v1, v2 string) int {
	c1, pre1 := splitPreRelease(v1)
	c2, pre2 := splitPreRelease(v2)

	if c := compareNumericParts(parseVersionParts(c1), parseVersionParts(c2)); c != 0 {
		return c
	}

	// Cores are equal, so the pre-release decides. Absent pre-release wins:
	// 2.0.3 outranks 2.0.3-rc.4.
	switch {
	case pre1 == "" && pre2 == "":
		return 0
	case pre1 == "":
		return 1
	case pre2 == "":
		return -1
	}
	return comparePreRelease(pre1, pre2)
}

// splitPreRelease divides "2.0.3-rc.4" into "2.0.3" and "rc.4". Build metadata
// ("+abc") is discarded: semver says it carries no ordering weight.
func splitPreRelease(v string) (core, pre string) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func compareNumericParts(p1, p2 []int) int {
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

// comparePreRelease orders dot-separated pre-release identifiers per semver
// §11.4: numeric identifiers compare numerically and rank below alphanumeric
// ones, and a longer identifier list wins when all shared fields are equal.
func comparePreRelease(pre1, pre2 string) int {
	a := strings.Split(pre1, ".")
	b := strings.Split(pre2, ".")
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
		// Running out of identifiers means the shorter list ranks lower.
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		na, errA := strconv.Atoi(a[i])
		nb, errB := strconv.Atoi(b[i])
		switch {
		case errA == nil && errB == nil:
			if na != nb {
				if na > nb {
					return 1
				}
				return -1
			}
		case errA == nil:
			return -1
		case errB == nil:
			return 1
		default:
			if a[i] != b[i] {
				if a[i] > b[i] {
					return 1
				}
				return -1
			}
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
