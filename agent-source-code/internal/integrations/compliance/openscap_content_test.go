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

// Every profile ID DiscoverProfiles() hands to the server must be resolvable by
// RunScan for the same content. Ubuntu 24.04 reports
// xccdf_org.ssgproject.content_profile_cis_level1_server, which DiscoverProfiles
// shortens to "cis_level1_server"; that ID must round-trip back to the same
// XCCDF ID instead of being rejected as "not available".
func TestResolveDiscoveredProfileID(t *testing.T) {
	profiles := []models.ScanProfileInfo{
		{ID: "cis_level1_server", XCCDFId: "xccdf_org.ssgproject.content_profile_cis_level1_server"},
		{ID: "cis_level2_server", XCCDFId: "xccdf_org.ssgproject.content_profile_cis_level2_server"},
		{ID: "cis_level1_workstation", XCCDFId: "xccdf_org.ssgproject.content_profile_cis_level1_workstation"},
		{ID: "standard", XCCDFId: "xccdf_org.ssgproject.content_profile_standard"},
		{ID: "anssi_bp28_high", XCCDFId: "xccdf_org.ssgproject.content_profile_anssi_bp28_high"},
	}

	tests := []struct {
		name      string
		profileID string
		want      string
	}{
		{"ubuntu 24.04 server", "cis_level1_server", "xccdf_org.ssgproject.content_profile_cis_level1_server"},
		{"discovered full XCCDF ID", "xccdf_org.ssgproject.content_profile_cis_level1_workstation", "xccdf_org.ssgproject.content_profile_cis_level1_workstation"},
		{"unknown short ID", "not_a_profile", ""},
		{"unknown full XCCDF ID", "xccdf_org.ssgproject.content_profile_not_a_profile", ""},
		{"empty ID", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveDiscoveredProfileID(tt.profileID, profiles); got != tt.want {
				t.Fatalf("resolveDiscoveredProfileID(%q) = %q, want %q", tt.profileID, got, tt.want)
			}
		})
	}
}

func TestResolveDiscoveredProfileIDAmbiguity(t *testing.T) {
	tests := []struct {
		name     string
		profiles []models.ScanProfileInfo
		want     string
	}{
		{
			name: "conflicting duplicates fail closed",
			profiles: []models.ScanProfileInfo{
				{ID: "duplicate", XCCDFId: "xccdf_org.example_profile_one"},
				{ID: "duplicate", XCCDFId: "xccdf_org.example_profile_two"},
			},
			want: "",
		},
		{
			name: "identical duplicates resolve",
			profiles: []models.ScanProfileInfo{
				{ID: "duplicate", XCCDFId: "xccdf_org.example_profile_shared"},
				{ID: "duplicate", XCCDFId: "xccdf_org.example_profile_shared"},
			},
			want: "xccdf_org.example_profile_shared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveDiscoveredProfileID("duplicate", tt.profiles); got != tt.want {
				t.Fatalf("resolveDiscoveredProfileID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetProfileIDPreservesLegacyAliases(t *testing.T) {
	tests := []struct {
		name        string
		osName      string
		profileName string
		want        string
	}{
		{
			name:        "ubuntu legacy level1_server alias",
			osName:      "ubuntu",
			profileName: "level1_server",
			want:        "xccdf_org.ssgproject.content_profile_cis_level1_server",
		},
		{
			// The legacy aliases are not a plain prefix operation on RHEL, so the
			// static mapping must keep winning over ID reconstruction.
			name:        "rhel legacy level1_server alias maps to bare cis",
			osName:      "rhel",
			profileName: "level1_server",
			want:        "xccdf_org.ssgproject.content_profile_cis",
		},
		{
			name:        "rhel legacy level2_server alias maps to cis_server_l1",
			osName:      "rhel",
			profileName: "level2_server",
			want:        "xccdf_org.ssgproject.content_profile_cis_server_l1",
		},
		{
			name:        "empty profile stays unresolved",
			osName:      "ubuntu",
			profileName: "",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &OpenSCAPScanner{
				osInfo: models.ComplianceOSInfo{Name: tt.osName, Version: "24.04"},
			}
			if got := s.getProfileID(tt.profileName); got != tt.want {
				t.Fatalf("getProfileID(%q) = %q, want %q", tt.profileName, got, tt.want)
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
