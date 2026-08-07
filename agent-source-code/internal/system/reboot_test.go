package system

import "testing"

// Cases marked with an issue number reproduce a reported bug. Cases from the
// community pull requests #818 (egrueda) and #834 (jbcr) are carried over here.

func TestParseKernelVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    []string
	}{
		{"proxmox", "6.14.11-2-pve", []string{"6", "14", "11", "2", "pve"}},
		{"debian", "6.12.90+deb13-amd64", []string{"6", "12", "90", "deb13", "amd64"}},
		{"debian sub-revision", "6.12.90+deb13.1-amd64", []string{"6", "12", "90", "deb13", "1", "amd64"}},
		{"ubuntu", "5.15.0-100-generic", []string{"5", "15", "0", "100", "generic"}},
		{"raspberry pi", "6.12.47+rpt-rpi-2712", []string{"6", "12", "47", "rpt", "rpi", "2712"}},
		{"rhel", "5.14.0-427.13.1.el9_4.x86_64", []string{"5", "14", "0", "427", "13", "1", "el9_4", "x86_64"}},
		{"arch unversioned", "linux", []string{"linux"}},
		{"empty", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseKernelVersion(tt.version)
			if len(got) != len(tt.want) {
				t.Fatalf("parseKernelVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseKernelVersion(%q) = %q, want %q", tt.version, got, tt.want)
				}
			}
		})
	}
}

func TestCompareKernelVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		// Issue #946: the patch number must not fuse to the Debian suffix.
		// "101+deb13" compares as text and ranks below "96+deb13".
		{"946 higher patch wins", "6.12.101+deb13-amd64", "6.12.96+deb13-amd64", 1},
		{"946 lower patch loses", "6.12.96+deb13-amd64", "6.12.101+deb13-amd64", -1},
		{"946 latest beats both", "6.12.101+deb13-amd64", "6.12.90+deb13.1-amd64", 1},

		// Issue #814: a Debian sub-revision is newer than the base revision.
		{"814 deb13.1 newer than deb13", "6.12.90+deb13.1-amd64", "6.12.90+deb13-amd64", 1},
		{"814 deb13 older than deb13.1", "6.12.90+deb13-amd64", "6.12.90+deb13.1-amd64", -1},
		{"814 identical", "6.12.90+deb13.1-amd64", "6.12.90+deb13.1-amd64", 0},
		{"814 upstream beats sub-revision", "6.12.91+deb13-amd64", "6.12.90+deb13.1-amd64", 1},
		{"deb13.2 newer than deb13.1", "6.12.90+deb13.2-amd64", "6.12.90+deb13.1-amd64", 1},
		{"major wins over deb suffix", "7.0.10+deb13-amd64", "6.12.90+deb13.1-amd64", 1},
		{"minor compared numerically", "7.0.10+deb13-amd64", "7.0.7+deb13-amd64", 1},

		// Proxmox, the format the original implementation documented.
		{"pve higher minor", "6.14.11-2-pve", "6.8.12-9-pve", 1},
		{"pve higher build", "6.8.12-10-pve", "6.8.12-9-pve", 1},
		{"pve lower build", "6.8.12-2-pve", "6.8.12-9-pve", -1},
		{"pve higher patch", "6.8.15-1-pve", "6.8.12-9-pve", 1},
		{"pve identical", "6.14.11-2-pve", "6.14.11-2-pve", 0},
		{"pve beats ubuntu major", "6.14.11-2-pve", "5.19.0-1-generic", 1},

		// Ubuntu ABI numbers must compare numerically, not as text.
		{"ubuntu higher abi", "5.15.0-100-generic", "5.15.0-91-generic", 1},
		{"ubuntu identical", "5.15.0-91-generic", "5.15.0-91-generic", 0},

		// RHEL point releases.
		{"rhel newer point release", "5.14.0-503.11.1.el9_5.x86_64", "5.14.0-427.13.1.el9_4.x86_64", 1},
		{"rhel identical", "5.14.0-427.13.1.el9_4.x86_64", "5.14.0-427.13.1.el9_4.x86_64", 0},

		// Arch.
		{"arch identical", "7.0.9-hardened1-1-hardened", "7.0.9-hardened1-1-hardened", 0},
		{"arch newer patch", "6.13.1-arch1-1", "6.12.4-arch1-1", 1},

		// Raspberry Pi, same version across two SoC variants.
		{"rpi same version", "6.12.62+rpt-rpi-2712", "6.12.62+rpt-rpi-2712", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareKernelVersions(tt.v1, tt.v2); got != tt.want {
				t.Errorf("compareKernelVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestKernelFlavour(t *testing.T) {
	tests := []struct {
		release string
		want    string
	}{
		{"6.12.101+deb13-amd64", "amd64"},
		{"6.12.90+deb13.1-amd64", "amd64"},
		{"6.14.11-2-pve", "pve"},
		{"5.15.0-100-generic", "generic"},
		{"5.15.0-100-lowlatency", "lowlatency"},
		{"5.14.0-427.13.1.el9_4.x86_64", "x86_64"},
		// Issue #647: the Pi 5 and Pi 4 builds are separate kernel lines.
		{"6.12.47+rpt-rpi-2712", "2712"},
		{"6.12.47+rpt-rpi-v8", "v8"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.release, func(t *testing.T) {
			if got := kernelFlavour(tt.release); got != tt.want {
				t.Errorf("kernelFlavour(%q) = %q, want %q", tt.release, got, tt.want)
			}
		})
	}
}

func TestHasVersionCore(t *testing.T) {
	tests := []struct {
		release string
		want    bool
	}{
		{"6.12.101+deb13-amd64", true},
		{"6.14.11-2-pve", true},
		{"6.12.4-arch1-1", true},
		// Issue #553: Arch installs an unversioned image, which carries no
		// version information and must never be treated as the latest kernel.
		{"linux", false},
		{"linux-lts", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.release, func(t *testing.T) {
			if got := hasVersionCore(tt.release); got != tt.want {
				t.Errorf("hasVersionCore(%q) = %v, want %v", tt.release, got, tt.want)
			}
		})
	}
}

func TestSelectLatestKernel(t *testing.T) {
	tests := []struct {
		name       string
		candidates []string
		flavour    string
		want       string
	}{
		{
			// Issue #946: the true latest was never selected, and dropped further
			// behind once apt autoremove cleared the intermediate kernel.
			name:       "946 picks the highest patch",
			candidates: []string{"6.12.90+deb13.1-amd64", "6.12.96+deb13-amd64", "6.12.101+deb13-amd64"},
			flavour:    "amd64",
			want:       "6.12.101+deb13-amd64",
		},
		{
			name:       "946 still correct after autoremove",
			candidates: []string{"6.12.90+deb13.1-amd64", "6.12.101+deb13-amd64"},
			flavour:    "amd64",
			want:       "6.12.101+deb13-amd64",
		},
		{
			// Issue #647: a 2712 host must never be compared against the v8 build,
			// even when the v8 build carries a higher version.
			name:       "647 keeps to the running SoC variant",
			candidates: []string{"6.12.47+rpt-rpi-2712", "6.12.50+rpt-rpi-v8"},
			flavour:    "2712",
			want:       "6.12.47+rpt-rpi-2712",
		},
		{
			name:       "647 v8 host sees only v8",
			candidates: []string{"6.12.47+rpt-rpi-2712", "6.12.50+rpt-rpi-v8"},
			flavour:    "v8",
			want:       "6.12.50+rpt-rpi-v8",
		},
		{
			// Issue #553: an unversioned Arch image yields no candidate, so the
			// caller falls through to a source that reports real versions.
			name:       "553 discards unversioned entries",
			candidates: []string{"linux"},
			flavour:    "1",
			want:       "",
		},
		{
			name:       "553 modules directory resolves arch",
			candidates: []string{"6.12.4-arch1-1", "6.13.1-arch1-1"},
			flavour:    "1",
			want:       "6.13.1-arch1-1",
		},
		{
			name:       "falls back to all when flavour matches nothing",
			candidates: []string{"6.14.11-2-pve", "6.8.12-9-pve"},
			flavour:    "amd64",
			want:       "6.14.11-2-pve",
		},
		{
			name:       "unknown running flavour considers everything",
			candidates: []string{"5.15.0-91-generic", "5.15.0-100-generic"},
			flavour:    "",
			want:       "5.15.0-100-generic",
		},
		{
			name:       "ignores the other ubuntu line",
			candidates: []string{"5.15.0-100-lowlatency", "5.15.0-91-generic"},
			flavour:    "generic",
			want:       "5.15.0-91-generic",
		},
		{"no candidates", nil, "amd64", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectLatestKernel(tt.candidates, tt.flavour); got != tt.want {
				t.Errorf("selectLatestKernel(%q, %q) = %q, want %q", tt.candidates, tt.flavour, got, tt.want)
			}
		})
	}
}

func TestKernelIsOutdated(t *testing.T) {
	tests := []struct {
		name    string
		running string
		latest  string
		want    bool
	}{
		{"running is older", "6.12.96+deb13-amd64", "6.12.101+deb13-amd64", true},
		// Issue #831: the flag must clear once the host has actually rebooted.
		{"831 clears after reboot", "6.12.101+deb13-amd64", "6.12.101+deb13-amd64", false},
		// Issue #647: same version across variants is not a pending reboot.
		{"647 same version", "6.12.47+rpt-rpi-2712", "6.12.47+rpt-rpi-2712", false},
		{"running is newer after rollback", "6.12.101+deb13-amd64", "6.12.96+deb13-amd64", false},
		{"no latest known", "6.12.101+deb13-amd64", "", false},
		{"no running known", "", "6.12.101+deb13-amd64", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kernelIsOutdated(tt.running, tt.latest); got != tt.want {
				t.Errorf("kernelIsOutdated(%q, %q) = %v, want %v", tt.running, tt.latest, got, tt.want)
			}
		})
	}
}
