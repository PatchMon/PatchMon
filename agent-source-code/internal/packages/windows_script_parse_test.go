//go:build windows

package packages

import (
	"os/exec"
	"strings"
	"testing"

	"patchmon-agent/internal/winexec"
)

const parseOnlyScript = `$s = [Console]::In.ReadToEnd()
$tokens = $null
$errs = $null
[void][System.Management.Automation.Language.Parser]::ParseInput($s, [ref]$tokens, [ref]$errs)
foreach ($e in $errs) { "line $($e.Extent.StartLineNumber): $($e.Message)" }
`

func powerShellParseErrors(t *testing.T, script string) []string {
	t.Helper()
	cmd := exec.Command(winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(parseOnlyScript))
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("PowerShell parser did not run: %v", err)
	}
	var errs []string
	for _, line := range strings.Split(strings.TrimSpace(string(winexec.TrimBOM(out))), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			errs = append(errs, line)
		}
	}
	return errs
}

func TestEmbeddedPowerShellScriptsParse(t *testing.T) {
	scripts := map[string]string{
		"Windows Update inventory": winexec.Script(wuaPendingScript + windowsUpdatesScript),
		"wua.ps1":                  winexec.Script(wuaPendingScript + wuaScript),
		"Store app query":          winexec.Script(storeAppxScript),
	}
	for name, script := range scripts {
		if errs := powerShellParseErrors(t, script); len(errs) > 0 {
			t.Errorf("%s does not parse:\n%s", name, strings.Join(errs, "\n"))
		}
	}
	if errs := powerShellParseErrors(t, "if ($true) {"); len(errs) == 0 {
		t.Fatal("the parse check reports nothing for a broken script, so it checks nothing")
	}
}
