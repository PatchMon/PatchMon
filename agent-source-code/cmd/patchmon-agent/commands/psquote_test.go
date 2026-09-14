package commands

import "testing"

// The update script interpolates paths into single-quoted PowerShell literals.
// An apostrophe in an install path is legal on Windows, so without doubling it
// the literal closes early and the remainder of the path is parsed as code.
func TestPsQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ordinary path", `C:\Program Files\PatchMon\agent.exe`, `C:\Program Files\PatchMon\agent.exe`},
		{"apostrophe in a folder name", `C:\Users\Toby's PC\agent.exe`, `C:\Users\Toby''s PC\agent.exe`},
		{"closing then reopening a literal", `C:\a'; Stop-Computer; '\b.exe`, `C:\a''; Stop-Computer; ''\b.exe`},
		{"repeated quotes", `a''b`, `a''''b`},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := psQuote(tt.in); got != tt.want {
				t.Errorf("psQuote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The service name is referenced from code that compiles on every platform, so
// a second literal in the PowerShell would drift silently: Stop-Service runs
// with -ErrorAction SilentlyContinue, so a mismatched name fails quietly and
// the copy then hits a locked image.
func TestServiceNameIsDeclaredOnce(t *testing.T) {
	if serviceName != "PatchMonAgent" {
		t.Errorf("serviceName = %q; the installer and the update script must agree", serviceName)
	}
}
