package packages

import (
	"encoding/json"
	"regexp"
	"strings"

	"patchmon-agent/internal/winexec"
	"patchmon-agent/pkg/models"
)

// storeAppx is a Get-AppxPackage entry with the fields that tell a Store install from a
// framework, a resource pack or a sparse package that only adds shell integration.
type storeAppx struct {
	Name              string `json:"Name"`
	PackageFamilyName string `json:"PackageFamilyName"`
	InstallLocation   string `json:"InstallLocation"`
	SignatureKind     string `json:"SignatureKind"`
	IsFramework       bool   `json:"IsFramework"`
	IsResourcePackage bool   `json:"IsResourcePackage"`
}

var (
	storeProductIDPattern = regexp.MustCompile(`^(9|XP)[0-9A-Z]{10,13}$`)
	packageFamilyPattern  = regexp.MustCompile(`^[A-Za-z0-9.\-]+_[a-z0-9]{13}$`)
)

func hasWingetUpdate(pkgs []models.Package) bool {
	for _, p := range pkgs {
		if p.NeedsUpdate && p.WingetID != "" {
			return true
		}
	}
	return false
}

func isStoreInstall(a storeAppx, windowsApps string) bool {
	return !a.IsFramework && !a.IsResourcePackage &&
		(a.SignatureKind == "Store" || a.SignatureKind == "System") &&
		pathInside(a.InstallLocation, windowsApps)
}

// isStoreApp reports whether a winget row is an MSIX/Store install. winget run as SYSTEM
// upgrades only the machine's staged copy of those, never a user's.
func isStoreApp(p models.Package, appx []storeAppx, windowsApps string) bool {
	id := p.WingetID
	if id == "" {
		return false
	}
	if strings.EqualFold(p.SourceRepository, "msstore") || storeProductIDPattern.MatchString(id) || packageFamilyPattern.MatchString(id) {
		return true
	}
	for _, a := range appx {
		if !isStoreInstall(a, windowsApps) {
			continue
		}
		family, _, _ := strings.Cut(a.PackageFamilyName, "_")
		if strings.EqualFold(a.Name, id) || strings.EqualFold(family, id) {
			return true
		}
	}
	return false
}

func markStoreApps(pkgs []models.Package, appx []storeAppx, windowsApps string) {
	for i := range pkgs {
		if isStoreApp(pkgs[i], appx, windowsApps) {
			pkgs[i].WingetStoreApp = true
		}
	}
}

const storeAppxScript = `
$ErrorActionPreference = "SilentlyContinue"
@(Get-AppxPackage -AllUsers | ForEach-Object {
  @{
    Name              = [string]$_.Name
    PackageFamilyName = [string]$_.PackageFamilyName
    InstallLocation   = [string]$_.InstallLocation
    SignatureKind     = [string]$_.SignatureKind
    IsFramework       = [bool]$_.IsFramework
    IsResourcePackage = [bool]$_.IsResourcePackage
  }
}) | ConvertTo-Json -Compress -Depth 2
`

func (m *WindowsManager) getStoreAppx() []storeAppx {
	cmd, cancel := boundedCommand(collectorTimeout, winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(storeAppxScript))
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("Get-AppxPackage query for Store apps failed")
		return nil
	}
	return parseStoreAppx(out)
}

func parseStoreAppx(output []byte) []storeAppx {
	s := strings.TrimSpace(string(winexec.TrimBOM(output)))
	if s == "" || s == "null" {
		return nil
	}
	if !strings.HasPrefix(s, "[") {
		s = "[" + s + "]"
	}
	var out []storeAppx
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
