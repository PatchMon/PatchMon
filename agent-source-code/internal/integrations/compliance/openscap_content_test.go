package compliance

import (
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
