package system

import (
	"sort"
	"testing"
)

func TestCompareKernelVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int // -1, 0, or 1
	}{
		// Debian-style with + separator — the bug case
		{
			name: "deb13.1 is newer than deb13",
			v1:   "6.12.90+deb13.1-amd64",
			v2:   "6.12.90+deb13-amd64",
			want: 1,
		},
		{
			name: "deb13 is older than deb13.1",
			v1:   "6.12.90+deb13-amd64",
			v2:   "6.12.90+deb13.1-amd64",
			want: -1,
		},
		{
			name: "identical deb versions",
			v1:   "6.12.90+deb13.1-amd64",
			v2:   "6.12.90+deb13.1-amd64",
			want: 0,
		},
		{
			name: "higher upstream version wins over deb sub-revision",
			v1:   "6.12.91+deb13-amd64",
			v2:   "6.12.90+deb13.1-amd64",
			want: 1,
		},
		// PVE-style kernels
		{
			name: "higher minor PVE kernel",
			v1:   "6.14.11-2-pve",
			v2:   "6.8.12-9-pve",
			want: 1,
		},
		{
			name: "same minor, higher patch PVE kernel",
			v1:   "6.8.12-10-pve",
			v2:   "6.8.12-9-pve",
			want: 1,
		},
		{
			name: "identical PVE versions",
			v1:   "6.14.11-2-pve",
			v2:   "6.14.11-2-pve",
			want: 0,
		},
		// Ubuntu-style kernels
		{
			name: "higher ABI Ubuntu kernel",
			v1:   "5.15.0-100-generic",
			v2:   "5.15.0-91-generic",
			want: 1,
		},
		{
			name: "identical Ubuntu versions",
			v1:   "5.15.0-91-generic",
			v2:   "5.15.0-91-generic",
			want: 0,
		},
		// Generic version ordering
		{
			name: "higher patch version",
			v1:   "6.1.2",
			v2:   "6.1.1",
			want: 1,
		},
		{
			name: "lower minor version",
			v1:   "6.1.0",
			v2:   "6.2.0",
			want: -1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := compareKernelVersions(tc.v1, tc.v2)
			if got != tc.want {
				t.Errorf("compareKernelVersions(%q, %q) = %d, want %d", tc.v1, tc.v2, got, tc.want)
			}
		})
	}
}

// TestGetLatestKernelFromBootSort verifies that getLatestKernelFromBoot
// correctly picks the newest kernel when multiple versions coexist — the
// scenario that caused the false "reboot required" on Debian 13.
func TestKernelSortPicksLatest(t *testing.T) {
	kernels := []string{
		"6.12.90+deb13-amd64",
		"6.12.90+deb13.1-amd64",
	}

	sort.Slice(kernels, func(i, j int) bool {
		return compareKernelVersions(kernels[i], kernels[j]) < 0
	})
	latest := kernels[len(kernels)-1]

	want := "6.12.90+deb13.1-amd64"
	if latest != want {
		t.Errorf("sort picked %q as latest, want %q", latest, want)
	}
}
