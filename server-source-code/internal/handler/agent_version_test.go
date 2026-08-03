package handler

import (
	"regexp"
	"testing"
)

// The bundled-agent version is parsed out of the binary's own output. If the
// pre-release suffix is dropped here, the version the server advertises stops
// matching the version the agent reports, and the agent self-updates on every
// check forever.
func TestAgentVersionRe_CapturesPreRelease(t *testing.T) {
	re := regexp.MustCompile(agentVersionRe)

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"bare release", "2.0.2", "2.0.2"},
		{"prefixed release", "PatchMon Agent v2.0.2", "2.0.2"},
		{"lowercase binary name", "patchmon-agent v2.0.2", "2.0.2"},
		{"version word", "version 2.0.2", "2.0.2"},
		{"edge build", "PatchMon Agent v2.0.3-rc.61", "2.0.3-rc.61"},
		{"bare edge build", "2.0.3-rc.61", "2.0.3-rc.61"},
		{"edge build with trailing text", "PatchMon Agent v2.0.3-rc.61 (linux/amd64)", "2.0.3-rc.61"},
		{"edge build with newline", "PatchMon Agent v2.0.3-rc.61\n", "2.0.3-rc.61"},
		{"git describe style", "2.0.2-60-gABC123", "2.0.2-60-gABC123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := re.FindStringSubmatch(tt.output)
			if len(m) < 2 {
				t.Fatalf("no match for %q", tt.output)
			}
			if m[1] != tt.want {
				t.Errorf("parsed %q from %q, want %q", m[1], tt.output, tt.want)
			}
		})
	}
}
