package util

import (
	"strings"
	"testing"
)

func TestHasVersionPrefix(t *testing.T) {
	for v, want := range map[string]bool{
		"2.1.5":               true,
		"2.1.5-rc.1":          true,
		"2.1.3-main-93c589a4": true,
		"v2.1.5":              false,
		"":                    false,
		"2.1":                 false,
		"unknown":             false,
	} {
		if got := HasVersionPrefix(v); got != want {
			t.Errorf("HasVersionPrefix(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestValidConfigVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"2.1.5":                            true,
		"2.1.5-rc.1":                       true,
		"2.1.3-main-93c589a4":              true,
		"0.0.0":                            true,
		"2.0.3+build.7":                    true,
		"":                                 false,
		"unknown":                          false,
		"2.1.5 <script>":                   false,
		"2.1.5\n":                          false,
		"2.1.5-" + strings.Repeat("a", 60): false,
	} {
		if got := ValidConfigVersion(v); got != want {
			t.Errorf("ValidConfigVersion(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestValidHostID(t *testing.T) {
	for id, want := range map[string]bool{
		"3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41": true,
		"abc":                                  true,
		"":                                     false,
		"3b0c2f7e 6a1d":                        false,
		"../etc/passwd":                        false,
		"id'; DROP TABLE hosts;--":             false,
		strings.Repeat("a", 65):                false,
		strings.Repeat("a", 64):                true,
	} {
		if got := ValidHostID(id); got != want {
			t.Errorf("ValidHostID(%q) = %v, want %v", id, got, want)
		}
	}
}
