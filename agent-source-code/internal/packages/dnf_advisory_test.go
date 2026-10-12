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
		{"fedora hex", "FEDORA-2026-044e56d9ff", true},
		{"fedora hex leading letter", "FEDORA-2026-abc1234567", true},
		{"amazon linux 2", "ALAS2-2023-1957", true},
		{"amazon linux 2023", "ALAS2023-2024-512", true},
		{"amazon linux 2 kernel extras", "ALAS2KERNEL-5.10-2024-055", true},
		{"amazon linux 2 livepatch", "ALAS2LIVEPATCH-2023-123", true},
		{"fedora epel", "FEDORA-EPEL-2024-0d2c3ad7a1", true},
		{"fedora epel next", "FEDORA-EPEL-NEXT-2024-0d2c3ad7a1", true},
		{"loaded plugins line", "Loaded", false},
		{"date field", "2026-09-09", false},
		{"mixed-case package", "NetworkManager-1:1.42.2-1.el9.x86_64", false},
		{"dnf5 header", "Name", false},

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
