//go:build windows

package packages

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"patchmon-agent/internal/winexec"
)

func powershellPath() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	}
	return "powershell.exe"
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

const authenticodeScript = `$p = [Console]::In.ReadToEnd().Trim()
$s = Get-AuthenticodeSignature -LiteralPath $p
"$($s.Status)|$($s.SignerCertificate.Subject)"
`

func checkAuthenticode(exe string) error {
	cmd, cancel := boundedCommand(collectorTimeout, powershellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(authenticodeScript))
	defer cancel()
	cmd.Stdin = strings.NewReader(exe)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("check winget signature: %w", err)
	}
	status, subject, _ := strings.Cut(strings.TrimSpace(string(winexec.TrimBOM(out))), "|")
	if status != "Valid" || !strings.Contains(subject, "Microsoft Corporation") {
		return fmt.Errorf("winget.exe signature is not a valid Microsoft signature (%s)", status)
	}
	return nil
}
