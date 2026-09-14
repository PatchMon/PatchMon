package packages

import "testing"

// #867: the advisory filter was a fixed list of vendor prefixes, so Rocky's
// RLSA-* notices were discarded and a host with 188 security errata reported
// zero. Matching on shape instead means a new distribution does not silently
// under-report.
func TestAdvisoryIDRe(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"rocky", "RLSA-2025:5678", true},
		{"redhat", "RHSA-2025:1234", true},
		{"alma", "ALSA-2025:11140", true},
		{"centos", "CESA-2025:0001", true},
		{"oracle dash separator", "ELSA-2025-1234", true},
		{"fedora style", "FEDORA-2025:9999", true},

		{"summary header", "Updates", false},
		{"metadata header", "Last", false},
		{"expiration header", "expiration", false},
		{"lowercase prefix", "rhsa-2025:1234", false},
		{"package name", "glib2-2.68.4-16.el9_6.2.x86_64", false},
		{"no year", "RHSA-25:1234", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := advisoryIDRe.MatchString(tt.id); got != tt.want {
				t.Errorf("advisoryIDRe.MatchString(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
