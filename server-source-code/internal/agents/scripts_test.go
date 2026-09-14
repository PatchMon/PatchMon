package agents

import (
	"testing"
)

// TestWindowsScriptsAreASCII guards the fix for the removal script that would
// not parse under Windows PowerShell 5.1.
//
// When PS 5.1 decodes these scripts through the Windows ANSI code page (which
// Invoke-WebRequest does for a response whose Content-Type carries no charset),
// CP1252 maps 0x91-0x94 to the curly quotes ' ' " ", and the PowerShell
// tokenizer accepts those as string delimiters. The stop sign emoji is
// F0 9F 9B 91, so its last byte became a quote that opened a string mid-line
// and cascaded into "Unexpected token '}'" and "The string is missing the
// terminator" errors several lines later.
//
// Serving these as charset=utf-8 and downloading with -OutFile both avoid the
// bad decode, but keeping the scripts ASCII means an operator running a command
// they pasted a year ago is safe too. Do not add emoji back.
func TestWindowsScriptsAreASCII(t *testing.T) {
	scripts := map[string][]byte{
		"patchmon_install_windows.ps1": PatchmonWindowsInstallScript,
		"patchmon_remove_windows.ps1":  PatchmonWindowsRemoveScript,
	}
	for name, body := range scripts {
		t.Run(name, func(t *testing.T) {
			if len(body) == 0 {
				t.Fatal("script is empty, embed failed")
			}
			line := 1
			for i, b := range body {
				if b == '\n' {
					line++
				}
				if b > 127 {
					t.Fatalf("non-ASCII byte 0x%02X at offset %d (line %d); "+
						"PowerShell 5.1 mis-decodes these, see the doc comment", b, i, line)
				}
			}
		})
	}
}
