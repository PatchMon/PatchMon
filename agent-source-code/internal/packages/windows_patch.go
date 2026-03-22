//go:build windows

package packages

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// WindowsPatcher executes Windows patching operations via PowerShell.
// WUA (Windows Update Agent) is used for OS/KB updates; WinGet for application packages.
type WindowsPatcher struct{}

// NewWindowsPatcher creates a new Windows patcher.
func NewWindowsPatcher() *WindowsPatcher {
	return &WindowsPatcher{}
}

// InstallWindowsUpdate installs a single pending Windows Update by GUID using the WUA COM API.
// Returns a human-readable output string and any error.
// Note: WUA COM is not re-entrant; callers should not invoke this concurrently.
func (p *WindowsPatcher) InstallWindowsUpdate(ctx context.Context, guid string) (string, error) {
	if guid == "" {
		return "", fmt.Errorf("update GUID is required")
	}
	psScript := fmt.Sprintf(`
$ErrorActionPreference = "Stop"
$guid = '%s'
try {
    $session   = New-Object -ComObject Microsoft.Update.Session
    $searcher  = $session.CreateUpdateSearcher()
    $results   = $searcher.Search("UpdateID='$guid'")
    if ($results.Updates.Count -eq 0) {
        Write-Output "SUPERSEDED:$guid"
        exit 0
    }
    $u = $results.Updates.Item(0)
    $title = $u.Title
    Write-Output "Installing: $title"

    if (-not $u.EulaAccepted) { $u.AcceptEula() }

    $coll = New-Object -ComObject Microsoft.Update.UpdateColl
    $coll.Add($u) | Out-Null

    if (-not $u.IsDownloaded) {
        Write-Output "Downloading..."
        $dl = $session.CreateUpdateDownloader()
        $dl.Updates = $coll
        $dlResult = $dl.Download()
        Write-Output "Download result: $($dlResult.ResultCode)"
    }

    Write-Output "Installing..."
    $inst = $session.CreateUpdateInstaller()
    $inst.Updates = $coll
    $instResult = $inst.Install()
    Write-Output "Install result: $($instResult.ResultCode)"

    if ($instResult.ResultCode -eq 2) {
        Write-Output "SUCCESS"
    } else {
        Write-Output "FAILED:ResultCode=$($instResult.ResultCode)"
        exit 1
    }
} catch {
    $hresult = if ($_.Exception.HResult) { [Convert]::ToString([uint32]$_.Exception.HResult, 16) } else { "" }
    Write-Output "ERROR:$($_.Exception.Message) (0x$hresult)"
    exit 1
}
`, guid)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, fmt.Errorf("WUA install failed for %s: %w", guid, err)
	}
	if strings.Contains(output, "FAILED:") || strings.Contains(output, "ERROR:") {
		return output, fmt.Errorf("WUA install failed for %s: %s", guid, output)
	}
	return output, nil
}

// IsSuperseded returns true if the output from InstallWindowsUpdate indicates the update no longer exists.
func IsSuperseded(output string) bool {
	return strings.HasPrefix(output, "SUPERSEDED:")
}

// wingetResolveBlock is a PowerShell snippet that resolves the winget.exe path
// in SYSTEM/Session 0 context where it may not be on PATH.
const wingetResolveBlock = `
# Resolve winget.exe path — handle SYSTEM/Session 0 where it's not on PATH
$wingetPath = $null
$candidate = Get-Command winget.exe -ErrorAction SilentlyContinue
if ($candidate) {
    $wingetPath = $candidate.Source
} else {
    $candidates = @(
        "$env:LOCALAPPDATA\Microsoft\WindowsApps\winget.exe",
        "$env:ProgramFiles\WindowsApps\Microsoft.DesktopAppInstaller_*\winget.exe",
        "C:\Program Files\WindowsApps\Microsoft.DesktopAppInstaller_*\winget.exe"
    )
    foreach ($pattern in $candidates) {
        $found = Get-Item $pattern -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found) {
            $wingetPath = $found.FullName
            break
        }
    }
}
if (-not $wingetPath) {
    Write-Output "ERROR:winget.exe not found"
    exit 1
}
`

// WinGetUpgradeAll upgrades all installed applications via WinGet.
// dryRun=true lists what would be upgraded without actually installing.
func (p *WindowsPatcher) WinGetUpgradeAll(ctx context.Context, dryRun bool) (string, error) {
	var action string
	if dryRun {
		action = `& $wingetPath upgrade --accept-source-agreements --disable-interactivity 2>&1 | Out-String`
	} else {
		action = `& $wingetPath upgrade --all --silent --accept-source-agreements --accept-package-agreements --disable-interactivity 2>&1 | Out-String`
	}
	psScript := fmt.Sprintf(`
$ErrorActionPreference = "SilentlyContinue"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$env:TERM = 'dumb'
%s
%s
`, wingetResolveBlock, action)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, fmt.Errorf("winget upgrade --all failed: %w", err)
	}
	return output, nil
}

// WinGetUpgradePackage upgrades a specific application by WinGet package ID.
func (p *WindowsPatcher) WinGetUpgradePackage(ctx context.Context, packageID string, dryRun bool) (string, error) {
	if packageID == "" {
		return "", fmt.Errorf("package ID is required")
	}
	var action string
	if dryRun {
		action = fmt.Sprintf(`
$out = & $wingetPath upgrade --accept-source-agreements --disable-interactivity 2>&1 | Out-String
Write-Output "[dry-run] Would upgrade: %s"
Write-Output $out
`, packageID)
	} else {
		action = fmt.Sprintf(`& $wingetPath upgrade --id '%s' --silent --accept-source-agreements --accept-package-agreements --disable-interactivity 2>&1 | Out-String`, packageID)
	}
	psScript := fmt.Sprintf(`
$ErrorActionPreference = "SilentlyContinue"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$env:TERM = 'dumb'
%s
%s
`, wingetResolveBlock, action)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, fmt.Errorf("winget upgrade --id %s failed: %w", packageID, err)
	}
	return output, nil
}

// RebootRequired checks whether a Windows reboot is pending after update installation.
// It inspects the standard registry key that Windows sets when a reboot is needed.
func RebootRequired() bool {
	psScript := `
$key = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired'
if (Test-Path $key) { Write-Output 'true' } else { Write-Output 'false' }
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}
