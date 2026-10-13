//go:build windows

package packages

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"patchmon-agent/internal/winexec"

	"golang.org/x/sys/windows"
)

func powershellPath() (string, error) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		dir, err := windows.GetSystemWindowsDirectory()
		if err != nil {
			return "", fmt.Errorf("locate the Windows directory: %w", err)
		}
		root = dir
	}
	return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), nil
}

func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

const wingetTrustScript = `$p = [Console]::In.ReadToEnd().Trim()
$sig = $null
try { $sig = Get-AuthenticodeSignature -LiteralPath $p } catch {}
$pkgs = @(Get-AppxPackage -AllUsers -Name Microsoft.DesktopAppInstaller | ForEach-Object {
    @{
        publisher        = [string]$_.Publisher
        signature_kind   = [string]$_.SignatureKind
        family           = [string]$_.PackageFamilyName
        install_location = [string]$_.InstallLocation
    }
})
@{
    authenticode = if ($sig) { [string]$sig.Status } else { '' }
    signer       = if ($sig -and $sig.SignerCertificate) { [string]$sig.SignerCertificate.Subject } else { '' }
    packages     = $pkgs
} | ConvertTo-Json -Compress -Depth 4
`

func checkWingetTrust(exe string) error {
	ps, err := powershellPath()
	if err != nil {
		return fmt.Errorf("check winget package trust: %w", err)
	}
	cmd, cancel := boundedCommand(collectorTimeout, ps, "-NoProfile", "-NonInteractive", "-Command", winexec.Script(wingetTrustScript))
	defer cancel()
	cmd.Stdin = strings.NewReader(exe)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("check winget package trust: %w", err)
	}
	var report wingetTrustReport
	if err := json.Unmarshal(winexec.TrimBOM([]byte(strings.TrimSpace(string(out)))), &report); err != nil {
		return fmt.Errorf("check winget package trust: %w", err)
	}
	return evaluateWingetTrust(exe, report)
}
