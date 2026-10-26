package compliance

import (
	"testing"

	"patchmon-agent/pkg/models"
)

func TestReportedProfileName(t *testing.T) {
	tests := []struct {
		name      string
		osName    string
		requested string
		want      string
	}{
		{"alias unchanged", "debian", "level1_server", "level1_server"},
		{"debian level 1 xccdf folds to alias", "debian", "xccdf_org.ssgproject.content_profile_cis_level1_server", "level1_server"},
		{"debian level 2 xccdf folds to alias", "debian", "xccdf_org.ssgproject.content_profile_cis_level2_server", "level2_server"},
		{"ubuntu level 1 xccdf folds to alias", "ubuntu", "xccdf_org.ssgproject.content_profile_cis_level1_server", "level1_server"},
		{"rocky cis folds to level 1 alias", "rocky", "xccdf_org.ssgproject.content_profile_cis", "level1_server"},
		{"profile with no alias keeps its id", "debian", "xccdf_org.ssgproject.content_profile_anssi_bp28_minimal", "xccdf_org.ssgproject.content_profile_anssi_bp28_minimal"},
		{"other os mapping is not borrowed", "debian", "xccdf_org.ssgproject.content_profile_cis", "xccdf_org.ssgproject.content_profile_cis"},
		{"os without mappings keeps its id", "arch", "xccdf_org.ssgproject.content_profile_cis_level1_server", "xccdf_org.ssgproject.content_profile_cis_level1_server"},
		{"docker bench untouched", "debian", "docker-bench", "docker-bench"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &OpenSCAPScanner{osInfo: models.ComplianceOSInfo{Name: tt.osName}}
			got := s.reportedProfileName(tt.requested)
			if got != tt.want {
				t.Fatalf("reportedProfileName(%q) on %s = %q, want %q", tt.requested, tt.osName, got, tt.want)
			}
			if got != tt.requested && s.getProfileID(got) != tt.requested {
				t.Fatalf("alias %q resolves to %q on %s, want %q", got, s.getProfileID(got), tt.osName, tt.requested)
			}
		})
	}
}
