package packages

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

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
	_, ok := storeTarget(p, appx, windowsApps)
	return ok
}

func storeTarget(p models.Package, appx []storeAppx, windowsApps string) (string, bool) {
	id := p.WingetID
	if id == "" {
		return "", false
	}
	if storeProductIDPattern.MatchString(id) || packageFamilyPattern.MatchString(id) {
		return id, true
	}
	for _, a := range appx {
		if !isStoreInstall(a, windowsApps) {
			continue
		}
		family, _, _ := strings.Cut(a.PackageFamilyName, "_")
		if strings.EqualFold(a.Name, id) || strings.EqualFold(family, id) {
			return a.PackageFamilyName, true
		}
	}
	if strings.EqualFold(p.SourceRepository, "msstore") {
		return "", true
	}
	return "", false
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

type storeUpdateRequest struct {
	Target  string `json:"target"`
	Seconds int    `json:"seconds"`
}

type storeUpdateResult struct {
	State            string `json:"state"`
	PackageFamily    string `json:"pfn"`
	ErrorCode        string `json:"errorCode"`
	Error            string `json:"error"`
	Before           string `json:"before"`
	After            string `json:"after"`
	InstalledForUser bool   `json:"installedForUser"`
}

const (
	msgStoreWouldUpdate = "Microsoft Store update would be requested"
	msgStoreUpdated     = "Updated through the Microsoft Store"
	msgStoreStaged      = "Updated through the Microsoft Store; each user gets the new version at their next sign-in"
	msgStoreNoUpdate    = "No Microsoft Store update available"
	msgStoreUnavailable = "Microsoft Store is not available on this host"
)

func parseStoreUpdateOutput(output []byte) (storeUpdateResult, error) {
	var last string
	for _, line := range strings.Split(string(winexec.TrimBOM(output)), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "{") {
			last = line
		}
	}
	var res storeUpdateResult
	if last == "" {
		return res, fmt.Errorf("no result from the Store update script")
	}
	if err := json.Unmarshal([]byte(last), &res); err != nil {
		return res, fmt.Errorf("unreadable Store update result: %w", err)
	}
	return res, nil
}

func classifyStoreUpdate(res storeUpdateResult, wait time.Duration) (status, message, hresult string) {
	switch res.State {
	case "Unavailable":
		return itemSkipped, msgStoreUnavailable, ""
	case "NoUpdate":
		return itemNotApplicable, msgStoreNoUpdate, ""
	case "Completed":
		switch {
		case res.After == "" || res.After == res.Before:
			return itemFailed, "Microsoft Store reported the update complete, but the installed version did not change", ""
		case !res.InstalledForUser:
			return itemInstalled, msgStoreStaged, ""
		}
		return itemInstalled, msgStoreUpdated, ""
	case "Error":
		msg := "Microsoft Store update failed"
		if res.Error != "" {
			msg += ": " + res.Error
		}
		return itemFailed, msg, normaliseHResult(res.ErrorCode)
	case "Canceled":
		return itemFailed, "Microsoft Store cancelled the update", normaliseHResult(res.ErrorCode)
	}
	if strings.HasPrefix(res.State, "Paused") {
		return itemFailed, "Microsoft Store paused the update (" + res.State + ")", normaliseHResult(res.ErrorCode)
	}
	state := res.State
	if state == "" {
		state = "unknown"
	}
	return itemFailed, fmt.Sprintf("Microsoft Store did not finish the update within %d minutes (last state: %s); it may still complete", max(1, minutes(wait)), state), normaliseHResult(res.ErrorCode)
}
