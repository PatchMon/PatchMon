package util

import (
	"fmt"
	"strconv"
	"strings"
)

// KernelRange represents a single version constraint. A kernel version matches
// the range when it falls within [Lo, Hi] (respecting inclusivity), or when it
// matches Exact as a numeric prefix (e.g. Exact "6.8" matches "6.8.0-51-generic").
//
// An empty Lo means unbounded below; an empty Hi means unbounded above.
type KernelRange struct {
	Lo     string
	LoIncl bool
	Hi     string
	HiIncl bool
	Exact  string
}

// KernelFilter matches a kernel version string against one or more ranges. A
// version matches the filter when it matches ANY of the ranges (logical OR).
// Label holds a human-readable description of what the filter represents (the
// original expression, or a CVE identifier).
type KernelFilter struct {
	Ranges []KernelRange
	Label  string
}

// kernelComponents extracts the ordered numeric components from a kernel
// version string. It handles the common shapes across distros, e.g.
//
//	"6.8.0-51-generic"          -> [6 8 0 51]
//	"5.15.0-89"                 -> [5 15 0 89]
//	"4.18.0-513.5.1.el8_9.x86_64" -> [4 18 0 513 5 1]
//	"6.1.2"                     -> [6 1 2]
//
// Once the version core (>=3 components) has been collected, an alphabetic
// segment (distro tag, "generic", arch suffix, ...) terminates parsing so that
// build/arch noise does not pollute comparisons.
func kernelComponents(v string) []int {
	v = strings.TrimSpace(v)
	var comps []int
	i, n := 0, len(v)
	for i < n {
		c := v[i]
		switch {
		case c >= '0' && c <= '9':
			j := i
			for j < n && v[j] >= '0' && v[j] <= '9' {
				j++
			}
			num, _ := strconv.Atoi(v[i:j])
			comps = append(comps, num)
			i = j
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			// Alphabetic segment: once we have the version core, stop. Distro
			// tags, "generic" and arch suffixes must not influence ordering.
			if len(comps) >= 3 {
				return comps
			}
			for i < n && !(v[i] >= '0' && v[i] <= '9') {
				i++
			}
		default:
			// Separator ('.', '-', '_', ...).
			i++
		}
	}
	return comps
}

// CompareKernelVersions compares two kernel version strings component-wise.
// Missing trailing components are treated as zero. It returns -1 if a < b, 0 if
// equal, and 1 if a > b.
func CompareKernelVersions(a, b string) int {
	ca, cb := kernelComponents(a), kernelComponents(b)
	n := len(ca)
	if len(cb) > n {
		n = len(cb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(ca) {
			x = ca[i]
		}
		if i < len(cb) {
			y = cb[i]
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

// kernelHasPrefix reports whether v shares the leading numeric components of
// prefix. "6.8" matches "6.8.0-51-generic"; "6.8.1" does not.
func kernelHasPrefix(v, prefix string) bool {
	pc := kernelComponents(prefix)
	if len(pc) == 0 {
		return false
	}
	vc := kernelComponents(v)
	for i, p := range pc {
		var got int
		if i < len(vc) {
			got = vc[i]
		}
		if got != p {
			return false
		}
	}
	return true
}

func (r KernelRange) matches(v string) bool {
	if r.Exact != "" {
		return kernelHasPrefix(v, r.Exact)
	}
	if r.Lo != "" {
		c := CompareKernelVersions(v, r.Lo)
		if r.LoIncl {
			if c < 0 {
				return false
			}
		} else if c <= 0 {
			return false
		}
	}
	if r.Hi != "" {
		c := CompareKernelVersions(v, r.Hi)
		if r.HiIncl {
			if c > 0 {
				return false
			}
		} else if c >= 0 {
			return false
		}
	}
	return true
}

// Matches reports whether the given kernel version satisfies the filter. An
// empty or unknown kernel version never matches a (positive) filter.
func (f *KernelFilter) Matches(kernel string) bool {
	if f == nil || len(f.Ranges) == 0 {
		return true
	}
	if strings.TrimSpace(kernel) == "" {
		return false
	}
	for _, r := range f.Ranges {
		if r.matches(kernel) {
			return true
		}
	}
	return false
}

// FromKernelRanges builds a filter from a set of ranges (e.g. resolved from a
// CVE advisory).
func FromKernelRanges(label string, ranges []KernelRange) *KernelFilter {
	return &KernelFilter{Ranges: ranges, Label: label}
}

// hasNumericComponent reports whether s contains at least one numeric version
// component, used to reject nonsense filter values early.
func hasNumericComponent(s string) bool {
	return len(kernelComponents(s)) > 0
}

// ParseKernelExpr parses a kernel-version filter expression into a KernelFilter.
//
// Supported forms:
//
//	"6.8"        exact numeric-prefix match (6.8.x)
//	"=6.8.0"     same as above
//	"<6.18.36"   strictly less than
//	"<=6.18.36"  less than or equal
//	">5.15"      strictly greater than
//	">=5.15"     greater than or equal
//	"5.15..6.1"  inclusive range [5.15, 6.1]
func ParseKernelExpr(expr string) (*KernelFilter, error) {
	raw := strings.TrimSpace(expr)
	if raw == "" {
		return nil, fmt.Errorf("empty kernel filter")
	}

	if strings.Contains(raw, "..") {
		parts := strings.SplitN(raw, "..", 2)
		lo := strings.TrimSpace(parts[0])
		hi := strings.TrimSpace(parts[1])
		if !hasNumericComponent(lo) || !hasNumericComponent(hi) {
			return nil, fmt.Errorf("invalid kernel range %q", raw)
		}
		return &KernelFilter{
			Label:  raw,
			Ranges: []KernelRange{{Lo: lo, LoIncl: true, Hi: hi, HiIncl: true}},
		}, nil
	}

	var r KernelRange
	var val string
	switch {
	case strings.HasPrefix(raw, "<="):
		val = strings.TrimSpace(raw[2:])
		r = KernelRange{Hi: val, HiIncl: true}
	case strings.HasPrefix(raw, ">="):
		val = strings.TrimSpace(raw[2:])
		r = KernelRange{Lo: val, LoIncl: true}
	case strings.HasPrefix(raw, "<"):
		val = strings.TrimSpace(raw[1:])
		r = KernelRange{Hi: val, HiIncl: false}
	case strings.HasPrefix(raw, ">"):
		val = strings.TrimSpace(raw[1:])
		r = KernelRange{Lo: val, LoIncl: false}
	case strings.HasPrefix(raw, "="):
		val = strings.TrimSpace(raw[1:])
		r = KernelRange{Exact: val}
	default:
		val = raw
		r = KernelRange{Exact: val}
	}

	if !hasNumericComponent(val) {
		return nil, fmt.Errorf("invalid kernel version %q", raw)
	}
	return &KernelFilter{Label: raw, Ranges: []KernelRange{r}}, nil
}
