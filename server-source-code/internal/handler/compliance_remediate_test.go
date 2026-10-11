package handler

import (
	"strings"
	"testing"
)

func TestIsAgentProfileID(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"level1_server": true,
		"xccdf_org.ssgproject.content_profile_standard":          true,
		"xccdf_org.ssgproject.content_profile_cis_level2_server": true,
		"":                       false,
		"OpenSCAP Scan":          false,
		"Docker Bench Security":  false,
		"level1;rm -rf /":        false,
		strings.Repeat("a", 257): false,
	}
	for in, want := range cases {
		if got := isAgentProfileID(in); got != want {
			t.Errorf("isAgentProfileID(%q) = %v, want %v", in, got, want)
		}
	}
}
