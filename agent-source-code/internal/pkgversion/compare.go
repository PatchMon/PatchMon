package pkgversion

import (
	"strconv"
	"strings"
)

// Compare returns 1 if a > b, -1 if a < b, 0 if equal. A port of the server's
// util.CompareVersions: pre-releases rank below their release.
func Compare(a, b string) int {
	c1, pre1 := splitPreRelease(a)
	c2, pre2 := splitPreRelease(b)

	if c := compareNumericParts(parseVersionParts(c1), parseVersionParts(c2)); c != 0 {
		return c
	}

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

func comparePreRelease(pre1, pre2 string) int {
	a := strings.Split(pre1, ".")
	b := strings.Split(pre2, ".")
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
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
