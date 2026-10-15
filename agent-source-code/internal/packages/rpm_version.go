package packages

import "strings"

// compareRPMEVR compares [epoch:]version-release strings the way rpm does.
func compareRPMEVR(a, b string) int {
	ea, va, ra := splitRPMEVR(a)
	eb, vb, rb := splitRPMEVR(b)
	if c := rpmvercmp(ea, eb); c != 0 {
		return c
	}
	if c := rpmvercmp(va, vb); c != 0 {
		return c
	}
	if ra == "" || rb == "" {
		return 0
	}
	return rpmvercmp(ra, rb)
}

func splitRPMEVR(evr string) (epoch, version, release string) {
	epoch = "0"
	if e, rest, ok := strings.Cut(evr, ":"); ok {
		if e != "" {
			epoch = e
		}
		evr = rest
	}
	if i := strings.LastIndex(evr, "-"); i >= 0 {
		return epoch, evr[:i], evr[i+1:]
	}
	return epoch, evr, ""
}

func isRPMDigit(c byte) bool { return c >= '0' && c <= '9' }
func isRPMAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

// rpmvercmp is a port of rpm's rpmvercmp, including ~ (sorts before anything) and ^.
func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for i < len(a) && !isRPMDigit(a[i]) && !isRPMAlpha(a[i]) && a[i] != '~' && a[i] != '^' {
			i++
		}
		for j < len(b) && !isRPMDigit(b[j]) && !isRPMAlpha(b[j]) && b[j] != '~' && b[j] != '^' {
			j++
		}
		if (i < len(a) && a[i] == '~') || (j < len(b) && b[j] == '~') {
			if i >= len(a) || a[i] != '~' {
				return 1
			}
			if j >= len(b) || b[j] != '~' {
				return -1
			}
			i++
			j++
			continue
		}
		if (i < len(a) && a[i] == '^') || (j < len(b) && b[j] == '^') {
			if i >= len(a) {
				return -1
			}
			if j >= len(b) {
				return 1
			}
			if a[i] != '^' {
				return 1
			}
			if b[j] != '^' {
				return -1
			}
			i++
			j++
			continue
		}
		if i >= len(a) || j >= len(b) {
			break
		}
		si, sj := i, j
		numeric := isRPMDigit(a[i])
		class := isRPMAlpha
		if numeric {
			class = isRPMDigit
		}
		for i < len(a) && class(a[i]) {
			i++
		}
		for j < len(b) && class(b[j]) {
			j++
		}
		sa, sb := a[si:i], b[sj:j]
		if sb == "" {
			if numeric {
				return 1
			}
			return -1
		}
		if numeric {
			sa = strings.TrimLeft(sa, "0")
			sb = strings.TrimLeft(sb, "0")
			if len(sa) != len(sb) {
				if len(sa) > len(sb) {
					return 1
				}
				return -1
			}
		}
		if c := strings.Compare(sa, sb); c != 0 {
			return c
		}
	}
	switch {
	case i >= len(a) && j >= len(b):
		return 0
	case i >= len(a):
		return -1
	default:
		return 1
	}
}
