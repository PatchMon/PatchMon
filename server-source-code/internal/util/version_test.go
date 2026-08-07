package util

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{"equal", "2.0.2", "2.0.2", 0},
		{"patch newer", "2.0.3", "2.0.2", 1},
		{"patch older", "2.0.2", "2.0.3", -1},
		{"minor beats patch", "2.1.0", "2.0.9", 1},
		{"major beats minor", "3.0.0", "2.9.9", 1},
		{"v prefix ignored", "v2.0.3", "2.0.3", 0},
		{"whitespace ignored", " 2.0.3 ", "2.0.3", 0},
		{"missing parts are zero", "2.0", "2.0.0", 0},
		{"extra part outranks", "2.0.3.1", "2.0.3", 1},

		// The edge-release relationship this whole scheme depends on.
		{"rc ranks below its release", "2.0.3-rc.4", "2.0.3", -1},
		{"release outranks its rc", "2.0.3", "2.0.3-rc.4", 1},
		{"rc still outranks previous release", "2.0.3-rc.1", "2.0.2", 1},
		{"previous release below rc", "2.0.2", "2.0.3-rc.1", -1},
		{"rc numbers compare numerically", "2.0.3-rc.9", "2.0.3-rc.10", -1},
		{"rc numbers are not lexical", "2.0.3-rc.10", "2.0.3-rc.9", 1},
		{"identical rc", "2.0.3-rc.4", "2.0.3-rc.4", 0},
		{"rc below next release", "2.0.3-rc.99", "2.0.4", -1},

		// Semver §11.4 identifier rules.
		{"numeric ranks below alphanumeric", "1.0.0-1", "1.0.0-alpha", -1},
		{"alpha before beta", "1.0.0-alpha", "1.0.0-beta", -1},
		{"longer identifier list wins", "1.0.0-alpha.1", "1.0.0-alpha", 1},
		{"shorter identifier list loses", "1.0.0-alpha", "1.0.0-alpha.1", -1},

		// Build metadata carries no ordering weight.
		{"build metadata ignored", "2.0.3+abc", "2.0.3", 0},
		{"build metadata on rc ignored", "2.0.3-rc.4+abc", "2.0.3-rc.4", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareVersions(tt.v1, tt.v2); got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

// A malformed version must not panic or silently outrank a real one. The old
// parser discarded Atoi errors, so "2.0.2-60-gABC" compared as 2.0.0.
func TestCompareVersions_MalformedInput(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{"git describe output is a pre-release of its tag", "2.0.2-60-gABC", "2.0.2", -1},
		{"empty is lowest", "", "2.0.2", -1},
		{"garbage parses as zero", "not-a-version", "0.0.0", -1},
		{"both empty", "", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareVersions(tt.v1, tt.v2); got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

// Ordering must be antisymmetric: if a > b then b < a. A one-sided bug here
// would make the update checker agree in one direction only.
func TestCompareVersions_IsAntisymmetric(t *testing.T) {
	versions := []string{
		"2.0.2", "2.0.3-rc.1", "2.0.3-rc.2", "2.0.3-rc.10", "2.0.3", "2.0.4-rc.1", "2.1.0", "3.0.0",
	}
	for i, a := range versions {
		for j, b := range versions {
			got := CompareVersions(a, b)
			rev := CompareVersions(b, a)
			if got != -rev {
				t.Errorf("CompareVersions(%q, %q) = %d but reverse = %d", a, b, got, rev)
			}
			// The slice is in ascending order, so index order must match.
			var want int
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			if got != want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d (ascending fixture)", a, b, got, want)
			}
		}
	}
}
