package agents

import (
	"os/exec"
	"strings"
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

func TestProxmoxDeriveMachineIDIsStable(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	script := string(ProxmoxAutoEnrollScript)
	start := strings.Index(script, "derive_machine_id() {")
	if start < 0 {
		t.Fatal("derive_machine_id not found in proxmox_auto_enroll.sh")
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatal("derive_machine_id has no closing brace")
	}
	fn := script[start : start+end+3]

	derive := func(raw, vmid, node string) string {
		t.Helper()
		out, err := exec.Command(bash, "-c", fn+`derive_machine_id "$1" "$2" "$3"`, "_", raw, vmid, node).Output()
		if err != nil {
			t.Fatalf("derive_machine_id(%q, %q, %q): %v", raw, vmid, node, err)
		}
		return strings.TrimSpace(string(out))
	}

	cases := []struct {
		name, raw, vmid, node, want string
	}{
		{"etc machine-id", "0123abcd\n\ndbus456\n", "101", "nodeA", "0123abcd"},
		{"empty etc machine-id falls to dbus", "\ndbus456\n", "101", "nodeA", "dbus456"},
		{"uninitialized is ignored", "uninitialized\n\n", "101", "nodeA", "proxmox-lxc-nodeA-101"},
		{"no ids", "", "101", "nodeA", "proxmox-lxc-nodeA-101"},
		{"whitespace only", " \n\t\n", "102", "nodeA", "proxmox-lxc-nodeA-102"},
	}
	for _, c := range cases {
		if got := derive(c.raw, c.vmid, c.node); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	if a, b := derive("", "101", "nodeA"), derive("", "101", "nodeA"); a != b {
		t.Errorf("fallback changed between runs: %q then %q", a, b)
	}
	if derive("", "101", "nodeA") == derive("", "101", "nodeB") {
		t.Error("same vmid on two nodes produced the same fallback")
	}
	if derive("", "101", "nodeA") == derive("", "1011", "nodeA") || derive("", "10", "nodeA-1") == derive("", "1", "nodeA-10") {
		t.Error("fallback is ambiguous between vmids")
	}
}
