package distro

import "strings"

// This file implements the two package-version comparison algorithms used by
// the supported distros: dpkg (Debian/Ubuntu/Proxmox) and rpm (RHEL/Fedora/
// CentOS/Alma). They mirror `dpkg --compare-versions` and rpm's rpmvercmp so
// that "is the installed kernel >= the fixed version?" is decided exactly as
// the distro's own package manager would.

// CompareDpkg compares two Debian package versions ([epoch:]upstream[-revision]).
// Returns -1, 0 or 1.
func CompareDpkg(a, b string) int {
	ea, ua, ra := splitDpkg(a)
	eb, ub, rb := splitDpkg(b)
	if ea != eb {
		if ea < eb {
			return -1
		}
		return 1
	}
	if c := verrevcmp(ua, ub); c != 0 {
		return c
	}
	return verrevcmp(ra, rb)
}

// splitDpkg splits a Debian version into (epoch, upstream, revision).
func splitDpkg(v string) (epoch int, upstream, revision string) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		epoch = atoiSafe(v[:i])
		v = v[i+1:]
	}
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		revision = v[i+1:]
		upstream = v[:i]
	} else {
		upstream = v
	}
	return epoch, upstream, revision
}

// dpkgOrder assigns a sort weight to a character per dpkg's algorithm: '~'
// sorts before everything (including end of string), letters keep ASCII order,
// all other characters sort after letters.
func dpkgOrder(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return 0
	case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		return int(c)
	case c == '~':
		return -1
	default:
		return int(c) + 256
	}
}

// verrevcmp is dpkg's comparison for a single component (upstream or revision).
func verrevcmp(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		// Compare the leading non-digit run character by character.
		for (i < len(a) && !isDigit(a[i])) || (j < len(b) && !isDigit(b[j])) {
			var ca, cb byte
			if i < len(a) {
				ca = a[i]
			}
			if j < len(b) {
				cb = b[j]
			}
			if dpkgOrder(ca) != dpkgOrder(cb) {
				if dpkgOrder(ca) < dpkgOrder(cb) {
					return -1
				}
				return 1
			}
			i++
			j++
		}
		// Skip leading zeros in the digit run.
		for i < len(a) && a[i] == '0' {
			i++
		}
		for j < len(b) && b[j] == '0' {
			j++
		}
		// Compare digit runs: the longer run (more significant digits) wins;
		// equal length compares lexically.
		firstDiff := 0
		for i < len(a) && isDigit(a[i]) && j < len(b) && isDigit(b[j]) {
			if firstDiff == 0 {
				if a[i] < b[j] {
					firstDiff = -1
				} else if a[i] > b[j] {
					firstDiff = 1
				}
			}
			i++
			j++
		}
		if i < len(a) && isDigit(a[i]) {
			return 1
		}
		if j < len(b) && isDigit(b[j]) {
			return -1
		}
		if firstDiff != 0 {
			return firstDiff
		}
	}
	return 0
}

// CompareRPM compares two RPM versions ([epoch:]version-release). Returns
// -1, 0 or 1.
func CompareRPM(a, b string) int {
	ea, va, ra := splitRPM(a)
	eb, vb, rb := splitRPM(b)
	if ea != eb {
		if ea < eb {
			return -1
		}
		return 1
	}
	if c := rpmvercmp(va, vb); c != 0 {
		return c
	}
	return rpmvercmp(ra, rb)
}

func splitRPM(v string) (epoch int, version, release string) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		epoch = atoiSafe(v[:i])
		v = v[i+1:]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		release = v[i+1:]
		version = v[:i]
	} else {
		version = v
	}
	return epoch, version, release
}

// rpmvercmp mirrors rpm's rpmvercmp: segments of digits or letters are compared
// in order; a numeric segment outranks an alphabetic one; '~' sorts before
// anything (pre-release) and '^' after a version (post-release snapshot).
func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		// Tilde: lower than everything, including end of string.
		aT := i < len(a) && a[i] == '~'
		bT := j < len(b) && b[j] == '~'
		if aT || bT {
			if aT && !bT {
				return -1
			}
			if !aT && bT {
				return 1
			}
			i++
			j++
			continue
		}
		// Caret: higher than end of string, lower than any following segment.
		aC := i < len(a) && a[i] == '^'
		bC := j < len(b) && b[j] == '^'
		if aC || bC {
			if aC && i+1 == len(a) && !bC {
				return 1
			}
			if bC && j+1 == len(b) && !aC {
				return -1
			}
			if aC && !bC {
				if j >= len(b) {
					return 1
				}
				return -1
			}
			if bC && !aC {
				if i >= len(a) {
					return -1
				}
				return 1
			}
			i++
			j++
			continue
		}
		// Skip separators (anything not alphanumeric, '~' or '^').
		for i < len(a) && !isAlnum(a[i]) {
			i++
		}
		for j < len(b) && !isAlnum(b[j]) {
			j++
		}
		if i >= len(a) || j >= len(b) {
			break
		}
		// Grab the next segment: all-digit or all-alpha.
		startA, startB := i, j
		if isDigit(a[i]) {
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			// A numeric segment always outranks an alphabetic one.
			if !isDigit(b[j]) {
				return 1
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na := strings.TrimLeft(a[startA:i], "0")
			nb := strings.TrimLeft(b[startB:j], "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
		} else {
			for i < len(a) && isAlpha(a[i]) {
				i++
			}
			if isDigit(b[j]) {
				return -1
			}
			for j < len(b) && isAlpha(b[j]) {
				j++
			}
			if c := strings.Compare(a[startA:i], b[startB:j]); c != 0 {
				if c < 0 {
					return -1
				}
				return 1
			}
		}
	}
	// Whichever still has an alphanumeric segment left is greater.
	switch {
	case i < len(a) && j >= len(b):
		return 1
	case j < len(b) && i >= len(a):
		return -1
	default:
		return 0
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isAlnum(c byte) bool { return isDigit(c) || isAlpha(c) }

func atoiSafe(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return n
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}
