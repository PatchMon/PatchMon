package compliance

import (
	"strings"
	"testing"

	"patchmon-agent/pkg/models"
)

func TestPickSSGFile(t *testing.T) {
	tests := []struct {
		name      string
		osName    string
		osVersion string
		idLike    string
		available []string
		want      string
	}{
		{
			name:      "rocky 9 rl datastream",
			osName:    "rocky",
			osVersion: "9.5",
			idLike:    "rhel centos fedora",
			available: []string{"ssg-rhel9-ds.xml", "ssg-rl9-ds.xml"},
			want:      "ssg-rl9-ds.xml",
		},
		{
			name:      "rocky 8 rl datastream",
			osName:    "rocky",
			osVersion: "8.10",
			idLike:    "rhel centos fedora",
			available: []string{"ssg-rl8-ds.xml", "ssg-rl9-ds.xml"},
			want:      "ssg-rl8-ds.xml",
		},
		{
			name:      "rocky prefers rocky-named datastream when present",
			osName:    "rocky",
			osVersion: "9.5",
			available: []string{"ssg-rocky9-ds.xml", "ssg-rl9-ds.xml"},
			want:      "ssg-rocky9-ds.xml",
		},
		{
			name:      "sles maps to sle product",
			osName:    "sles",
			osVersion: "15.6",
			available: []string{"ssg-sle15-ds.xml"},
			want:      "ssg-sle15-ds.xml",
		},
		{
			name:      "legacy alma maps to almalinux product",
			osName:    "alma",
			osVersion: "9.4",
			available: []string{"ssg-almalinux9-ds.xml"},
			want:      "ssg-almalinux9-ds.xml",
		},
		{
			name:      "opensuse leap falls back to opensuse product",
			osName:    "opensuse-leap",
			osVersion: "15.6",
			idLike:    "suse opensuse",
			available: []string{"ssg-opensuse-ds.xml"},
			want:      "ssg-opensuse-ds.xml",
		},
		{
			name:      "ubuntu keeps full version match",
			osName:    "ubuntu",
			osVersion: "24.04",
			available: []string{"ssg-ubuntu2204-ds.xml", "ssg-ubuntu2404-ds.xml"},
			want:      "ssg-ubuntu2404-ds.xml",
		},
		{
			name:      "no match",
			osName:    "rocky",
			osVersion: "9.5",
			available: []string{"ssg-debian12-ds.xml"},
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &OpenSCAPScanner{
				osInfo: models.ComplianceOSInfo{Name: tt.osName, Version: tt.osVersion},
				idLike: tt.idLike,
			}
			if got := s.pickSSGFile(tt.available); got != tt.want {
				t.Fatalf("pickSSGFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPolicyHasCandidate(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{
			name: "installable",
			out:  "openscap-scanner:\n  Installed: (none)\n  Candidate: 1.3.7+dfsg-1+deb12u1\n  Version table:\n",
			want: true,
		},
		{
			// A name apt knows but cannot install, which is how bookworm reports
			// libopenscap8.
			name: "known but no candidate",
			out:  "libopenscap8:\n  Installed: (none)\n  Candidate: (none)\n  Version table:\n",
			want: false,
		},
		{
			// A name apt does not know at all produces no output whatsoever, and
			// still exits zero.
			name: "unknown package",
			out:  "",
			want: false,
		},
		{
			name: "already installed",
			out:  "libopenscap8:\n  Installed: 1.2.17-0.1ubuntu7.22.04.3\n  Candidate: 1.2.17-0.1ubuntu7.22.04.3\n",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policyHasCandidate(tt.out); got != tt.want {
				t.Fatalf("policyHasCandidate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDebianScannerSet(t *testing.T) {
	tests := []struct {
		name      string
		available []string
		want      []string
	}{
		{
			name:      "bookworm and noble",
			available: []string{"openscap-scanner", "openscap-common", "ssg-base"},
			want:      []string{"openscap-scanner", "openscap-common"},
		},
		{
			name:      "jammy has only the library package",
			available: []string{"libopenscap8"},
			want:      []string{"libopenscap8"},
		},
		{
			name:      "buster has both, newest wins",
			available: []string{"libopenscap8", "openscap-scanner", "openscap-common"},
			want:      []string{"openscap-scanner", "openscap-common"},
		},
		{
			// A partial modern set must not be selected: installing
			// openscap-scanner without openscap-common fails the transaction.
			name:      "partial modern set falls through",
			available: []string{"openscap-scanner", "libopenscap8"},
			want:      []string{"libopenscap8"},
		},
		{
			name:      "bullseye packages no scanner at all",
			available: nil,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			avail := make(map[string]bool, len(tt.available))
			for _, p := range tt.available {
				avail[p] = true
			}
			got := debianScannerSet(func(p string) bool { return avail[p] })
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("debianScannerSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContentFileMatchesOSVersion(t *testing.T) {
	tests := []struct {
		name      string
		osName    string
		osVersion string
		baseName  string
		want      bool
	}{
		{"rocky rl datastream", "rocky", "9.5", "ssg-rl9-ds.xml", true},
		{"rocky wrong major", "rocky", "9.5", "ssg-rl8-ds.xml", false},
		{"sles sle datastream", "sles", "15.6", "ssg-sle15-ds.xml", true},
		{"rhel exact", "rhel", "9.4", "ssg-rhel9-ds.xml", true},
		{"ubuntu full version", "ubuntu", "24.04", "ssg-ubuntu2404-ds.xml", true},
		{"ubuntu mismatch", "ubuntu", "24.04", "ssg-ubuntu2204-ds.xml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &OpenSCAPScanner{
				osInfo: models.ComplianceOSInfo{Name: tt.osName, Version: tt.osVersion},
			}
			if got := s.contentFileMatchesOSVersion(tt.baseName); got != tt.want {
				t.Fatalf("contentFileMatchesOSVersion(%q) = %v, want %v", tt.baseName, got, tt.want)
			}
		})
	}
}
