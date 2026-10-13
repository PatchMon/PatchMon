//go:build windows

package packages

import (
	"encoding/json"
	"fmt"
	"strings"

	"patchmon-agent/internal/winexec"
)

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
	cmd, cancel := boundedCommand(collectorTimeout, powershellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(wingetTrustScript))
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
