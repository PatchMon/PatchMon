package packages

import (
	"encoding/json"
	"strings"

	"patchmon-agent/internal/winexec"
	"patchmon-agent/pkg/models"
)

type pendingStoreUpdate struct {
	storeAppx
	ProductID string `json:"ProductId"`
	Version   string `json:"Version"`
	Installed bool   `json:"Installed"`
}

const storeQueueScript = `
$ErrorActionPreference = "Stop"
$pending = @{}
try {
  $null = [Windows.ApplicationModel.Store.Preview.InstallControl.AppInstallManager, Windows.ApplicationModel.Store.Preview, ContentType = WindowsRuntime]
  $mgr = New-Object Windows.ApplicationModel.Store.Preview.InstallControl.AppInstallManager
  $items = @($mgr.AppInstallItems)
} catch { "[]"; exit 0 }
foreach ($it in $items) {
  try {
    if ([string]$it.InstallType -ne "Update") { continue }
    if ([string]$it.GetCurrentStatus().InstallState -in "Completed", "Canceled") { continue }
    $pending[[string]$it.PackageFamilyName] = [string]$it.ProductId
  } catch { continue }
}
$ErrorActionPreference = "SilentlyContinue"
if ($pending.Count -eq 0) { "[]"; exit 0 }
@(Get-AppxPackage -AllUsers | Where-Object { $pending.ContainsKey([string]$_.PackageFamilyName) } | ForEach-Object {
  @{
    Name              = [string]$_.Name
    ProductId         = $pending[[string]$_.PackageFamilyName]
    PackageFamilyName = [string]$_.PackageFamilyName
    InstallLocation   = [string]$_.InstallLocation
    SignatureKind     = [string]$_.SignatureKind
    IsFramework       = [bool]$_.IsFramework
    IsResourcePackage = [bool]$_.IsResourcePackage
    Version           = [string]$_.Version
    Installed         = @($_.PackageUserInformation | Where-Object { [string]$_.InstallState -eq "Installed" }).Count -gt 0
  }
}) | ConvertTo-Json -Compress -Depth 2
`

func (m *WindowsManager) getPendingStoreUpdates() []pendingStoreUpdate {
	cmd, cancel := boundedCommand(collectorTimeout, winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(storeQueueScript))
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("Microsoft Store update queue query failed")
		return nil
	}
	return parsePendingStoreUpdates(out)
}

func parsePendingStoreUpdates(output []byte) []pendingStoreUpdate {
	s := strings.TrimSpace(string(winexec.TrimBOM(output)))
	if s == "" || s == "null" {
		return nil
	}
	if !strings.HasPrefix(s, "[") {
		s = "[" + s + "]"
	}
	var out []pendingStoreUpdate
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func pendingByFamily(pending []pendingStoreUpdate, windowsApps string) []pendingStoreUpdate {
	best := map[string]pendingStoreUpdate{}
	var order []string
	for _, p := range pending {
		if !p.Installed || !isStoreInstall(p.storeAppx, windowsApps) || !packageFamilyPattern.MatchString(p.PackageFamilyName) {
			continue
		}
		key := strings.ToLower(p.PackageFamilyName)
		cur, ok := best[key]
		if !ok {
			order = append(order, key)
		}
		if c, cmpOK := compareDottedVersions(p.Version, cur.Version); !ok || (cmpOK && c < 0) {
			best[key] = p
		}
	}
	out := make([]pendingStoreUpdate, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}

func addPendingStoreUpdates(pkgs []models.Package, pending []pendingStoreUpdate, windowsApps string) []models.Package {
	for _, u := range pendingByFamily(pending, windowsApps) {
		matched := false
		for i := range pkgs {
			target, ok := storeTarget(pkgs[i], []storeAppx{u.storeAppx}, windowsApps)
			if ok && target != "" && (strings.EqualFold(target, u.PackageFamilyName) || strings.EqualFold(target, u.ProductID)) {
				pkgs[i].NeedsUpdate = true
				pkgs[i].WingetStoreApp = true
				matched = true
			}
		}
		if matched {
			continue
		}
		pkgs = append(pkgs, models.Package{
			Name:             u.Name,
			CurrentVersion:   u.Version,
			NeedsUpdate:      true,
			SourceRepository: "msstore",
			WingetID:         u.PackageFamilyName,
			WingetStoreApp:   true,
		})
	}
	return pkgs
}
