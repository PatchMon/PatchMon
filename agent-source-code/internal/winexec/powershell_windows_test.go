//go:build windows

package winexec

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"
)

// powershell_test.go asserts the preamble's text. This asserts its effect,
// which is the only thing that actually matters and the only thing that can
// only be checked on Windows: that a redirected stdout carrying non-ASCII
// survives the trip back into Go as UTF-8.
//
// Without the preamble, Windows PowerShell 5.1 encodes redirected stdout with
// the console OEM code page and these strings come back as mojibake.
func requireWindowsIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("PATCHMON_WINDOWS_INTEGRATION") != "1" {
		t.Skip("set PATCHMON_WINDOWS_INTEGRATION=1 to shell out to PowerShell")
	}
}

func runPS(t *testing.T, body string) []byte {
	t.Helper()
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", Script(body))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("powershell failed: %v\noutput: %s", err, out)
	}
	return TrimBOM(out)
}

// The sample strings are the real failure cases from the field: a French
// package name, Cyrillic, and a CJK title.
func TestPreambleRoundTripsNonASCII(t *testing.T) {
	requireWindowsIntegration(t)

	want := []string{
		"Mise à jour de sécurité",
		"Обновление безопасности",
		"セキュリティ更新プログラム",
		"Microsoft Visual C++ 2015-2022",
	}

	var sb strings.Builder
	for _, s := range want {
		sb.WriteString("Write-Output '" + s + "'\n")
	}

	got := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(runPS(t, sb.String()))), "\r\n", "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d round-tripped as %q, want %q; the console encoding is not UTF-8", i, got[i], want[i])
		}
	}
}

// The collectors parse ConvertTo-Json output, so a stray BOM is the difference
// between a working collector and a silent unmarshal failure.
func TestConvertToJSONOutputIsDecodable(t *testing.T) {
	requireWindowsIntegration(t)

	out := runPS(t, `@{ Name = 'Mise à jour'; Version = '1.0' } | ConvertTo-Json -Compress`)

	if !utf8.Valid(out) {
		t.Fatalf("PowerShell JSON output is not valid UTF-8: %q", out)
	}

	var got struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("json.Unmarshal failed on PowerShell output %q: %v", out, err)
	}
	if got.Name != "Mise à jour" {
		t.Errorf("Name decoded as %q, want %q", got.Name, "Mise à jour")
	}
}

// The preamble is wrapped in try/catch because the agent runs in Session 0
// with no console attached. If that assumption ever breaks, every collector
// dies rather than degrading to mojibake, so it is worth asserting.
func TestPreambleDoesNotThrowWithoutAConsole(t *testing.T) {
	requireWindowsIntegration(t)

	out := runPS(t, `Write-Output 'ok'`)
	if strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("preamble interfered with output: %q", out)
	}
}
