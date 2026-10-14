package packages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"patchmon-agent/internal/logutil"
	"patchmon-agent/internal/winexec"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// WindowsManager handles Windows package information collection via WinGet and Windows Update
type WindowsManager struct {
	logger *logrus.Logger
}

// NewWindowsManager creates a new Windows package manager
func NewWindowsManager(logger *logrus.Logger) *WindowsManager {
	return &WindowsManager{
		logger: logger,
	}
}

var wingetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+\-]{0,127}$`)

// wingetEntry holds parsed fields from winget list table output
type wingetEntry struct {
	Name      string
	ID        string
	Version   string
	Available string
	Source    string
}

// GetPackages returns installed applications merged with Windows OS updates.
//
// Collection strategy (inspired by rmmagent):
//  1. Always collect from Windows Registry (reliable in Session 0 / SYSTEM context)
//  2. Try WinGet as supplementary source for update availability enrichment
//  3. Merge: registry provides the baseline, WinGet adds NeedsUpdate/AvailableVersion
//  4. Collect Windows OS updates (installed KBs + pending via WUA COM API)
func (m *WindowsManager) GetPackages() (pkgs []models.Package, windowsUpdatesOmitted bool) {
	if runtime.GOOS != "windows" {
		return nil, false
	}

	appPackages := m.appPackages()

	// 4. Collect Windows OS updates (installed KBs + pending updates)
	winUpdates, omitted := m.windowsUpdatesGuarded()
	m.logger.WithField("count", len(winUpdates)).Info("Collected Windows OS updates")

	// 5. Set SourceRepository for Windows Update entries based on WSUS config
	wsusActive := m.isWSUSActive()
	wuRepo := "Microsoft Update"
	if wsusActive {
		wuRepo = "WSUS"
	}
	for i := range winUpdates {
		winUpdates[i].SourceRepository = wuRepo
	}

	all := make([]models.Package, 0, len(appPackages)+len(winUpdates))
	all = append(all, appPackages...)
	all = append(all, winUpdates...)

	if len(all) == 0 {
		return []models.Package{}, omitted
	}
	return all, omitted
}

// appPackages collects installed applications from the registry, enriched with WinGet.
func (m *WindowsManager) appPackages() []models.Package {
	// 1. Registry is the primary source — works reliably as SYSTEM in Session 0
	//    (same approach as rmmagent's go-win64api which reads Uninstall registry keys)
	regPackages := m.getPackagesFromRegistry()
	if hasUnversionedSQLServer(regPackages) {
		applySQLServerPatchLevels(regPackages, m.getSQLServerSetups())
	}
	m.logger.WithField("count", len(regPackages)).Info("Collected packages via registry")

	// 2. Try WinGet for update availability enrichment
	//    WinGet may not be available in Session 0 (per-user UWP app); that's OK
	wingetPackages := m.getPackagesFromWinget()
	if len(wingetPackages) > 0 {
		m.logger.WithField("count", len(wingetPackages)).Info("Collected packages via WinGet (supplementary)")
	}

	// 3. Merge: start with registry baseline, enrich with WinGet update info
	appPackages, origins := m.mergeRegistryAndWinget(regPackages, wingetPackages)
	if needsAppxState(appPackages, origins) {
		var suppressed, dropped []string
		appPackages, suppressed, dropped = applyAppxInstallState(appPackages, origins, m.getAppxPackages())
		if len(suppressed) > 0 {
			m.logger.WithFields(logutil.SanitizeMap(map[string]interface{}{"packages": suppressed})).Debug("Update flag cleared: a newer Appx copy is installed than winget reports")
		}
		if len(dropped) > 0 {
			m.logger.WithFields(logutil.SanitizeMap(map[string]interface{}{"packages": dropped})).Debug("Dropped Appx packages that are staged but not installed for any user")
		}
	}
	if hasWingetUpdate(appPackages) {
		markStoreApps(appPackages, m.getStoreAppx(), windowsAppsDir())
	}
	m.logger.WithField("count", len(appPackages)).Info("Application packages after merge")
	return appPackages
}

// mergeRegistryAndWinget merges registry-discovered packages with WinGet data.
// Registry provides the reliable baseline; WinGet enriches with update availability.
// Any WinGet-only entries (not in registry) are also included.
// SourceRepository is set to the WinGet source for matched packages, or "local" for registry-only.
func (m *WindowsManager) mergeRegistryAndWinget(regPkgs []models.Package, rows []wingetRow) ([]models.Package, []appOrigin) {
	origins := make([]appOrigin, len(regPkgs), len(regPkgs)+len(rows))
	for i := range origins {
		origins[i].registry = true
	}
	if len(rows) == 0 {
		for i := range regPkgs {
			regPkgs[i].SourceRepository = "local"
		}
		return regPkgs, origins
	}

	used := make([]bool, len(rows))
	matchOf := make([]int, len(regPkgs))
	for i := range matchOf {
		matchOf[i] = -1
	}
	for rank := 3; rank > 0; rank-- {
		for i := range regPkgs {
			if matchOf[i] >= 0 {
				continue
			}
			for j := range rows {
				if !used[j] && matchRank(regPkgs[i], rows[j]) == rank {
					matchOf[i], used[j] = j, true
					break
				}
			}
		}
	}

	matchedNames := make(map[string]bool)
	for i := range regPkgs {
		j := matchOf[i]
		if j < 0 {
			regPkgs[i].SourceRepository = "local"
			continue
		}
		row := rows[j].pkg
		origins[i].wingetID = rows[j].id
		matchedNames[normalizePackageName(row.Name)] = true
		regPkgs[i].SourceRepository = row.SourceRepository
		regPkgs[i].WingetID = row.WingetID
		regPkgs[i].WingetPinned = row.WingetPinned
		regPkgs[i].WingetStoreApp = row.WingetStoreApp
		if row.NeedsUpdate && installedIsOlder(regPkgs[i].CurrentVersion, row.AvailableVersion) {
			regPkgs[i].NeedsUpdate = true
			if regPkgs[i].AvailableVersion == "" {
				regPkgs[i].AvailableVersion = row.AvailableVersion
			}
		}
	}

	// winget truncates names to its column width, so unmatched rows can share a
	// name; the server needs one package per name.
	appended := make(map[string]int)
	for j := range rows {
		if used[j] {
			continue
		}
		key := normalizePackageName(rows[j].pkg.Name)
		if matchedNames[key] {
			continue
		}
		if k, seen := appended[key]; seen {
			if rows[j].pkg.NeedsUpdate && !regPkgs[k].NeedsUpdate {
				regPkgs[k] = rows[j].pkg
				origins[k].wingetID = rows[j].id
			}
			continue
		}
		regPkgs = append(regPkgs, rows[j].pkg)
		origins = append(origins, appOrigin{wingetID: rows[j].id})
		appended[key] = len(regPkgs) - 1
	}

	return regPkgs, origins
}

// A truncated-name prefix also requires an equal version; a prefix alone is too loose.
func matchRank(reg models.Package, row wingetRow) int {
	name := normalizePackageName(reg.Name)
	rowName := normalizePackageName(row.pkg.Name)
	sameVersion := versionsEqual(reg.CurrentVersion, row.pkg.CurrentVersion)
	switch {
	case rowName == name && sameVersion:
		return 3
	case rowName == name:
		return 2
	case row.truncated && rowName != "" && strings.HasPrefix(name, rowName) && sameVersion:
		return 1
	}
	return 0
}

func versionsEqual(a, b string) bool {
	if c, ok := compareDottedVersions(a, b); ok {
		return c == 0
	}
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// installedIsOlder is true unless both versions are numeric and the installed
// one is not below the available one. Non-numeric versions defer to winget.
func installedIsOlder(installed, available string) bool {
	if isUnknownVersion(installed) {
		return false
	}
	if c, ok := compareDottedVersions(installed, available); ok {
		return c < 0
	}
	return true
}

func isUnknownVersion(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || strings.EqualFold(v, "unknown")
}

// compareDottedVersions compares purely numeric dotted versions, treating
// missing components as zero. ok is false when either side is not numeric.
func compareDottedVersions(a, b string) (cmp int, ok bool) {
	pa, okA := parseDottedVersion(a)
	pb, okB := parseDottedVersion(b)
	if !okA || !okB {
		return 0, false
	}
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y uint64
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

func parseDottedVersion(v string) ([]uint64, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil, false
	}
	fields := strings.Split(v, ".")
	out := make([]uint64, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

type sqlServerSetup struct {
	Instance   string `json:"Instance"`
	Version    string `json:"Version"`
	PatchLevel string `json:"PatchLevel"`
}

var (
	sqlServerProductRe   = regexp.MustCompile(`^Microsoft SQL Server (\d{4}) \((?:64|32)-bit\)$`)
	sqlServerInstanceRe  = regexp.MustCompile(`^MSSQL(\d+)\.`)
	sqlServerMajorByYear = map[string]string{
		"2012": "11", "2014": "12", "2016": "13", "2017": "14", "2019": "15", "2022": "16", "2025": "17",
	}
)

func hasUnversionedSQLServer(pkgs []models.Package) bool {
	for _, p := range pkgs {
		if isUnknownVersion(p.CurrentVersion) && sqlServerProductRe.MatchString(p.Name) {
			return true
		}
	}
	return false
}

// With several instances of one major version the lowest PatchLevel wins.
func applySQLServerPatchLevels(pkgs []models.Package, setups []sqlServerSetup) {
	lowest := map[string]string{}
	for _, s := range setups {
		m := sqlServerInstanceRe.FindStringSubmatch(s.Instance)
		if m == nil {
			continue
		}
		level := strings.TrimSpace(s.PatchLevel)
		if _, ok := parseDottedVersion(level); !ok {
			level = strings.TrimSpace(s.Version)
		}
		if _, ok := parseDottedVersion(level); !ok {
			continue
		}
		if cur, seen := lowest[m[1]]; !seen {
			lowest[m[1]] = level
		} else if c, _ := compareDottedVersions(level, cur); c < 0 {
			lowest[m[1]] = level
		}
	}
	for i := range pkgs {
		if !isUnknownVersion(pkgs[i].CurrentVersion) {
			continue
		}
		m := sqlServerProductRe.FindStringSubmatch(pkgs[i].Name)
		if m == nil {
			continue
		}
		if level, ok := lowest[sqlServerMajorByYear[m[1]]]; ok {
			pkgs[i].CurrentVersion = level
		}
	}
}

func (m *WindowsManager) getSQLServerSetups() []sqlServerSetup {
	psScript := `
$ErrorActionPreference = "SilentlyContinue"
$result = @()
Get-ChildItem -LiteralPath "HKLM:\SOFTWARE\Microsoft\Microsoft SQL Server" | Where-Object { $_.PSChildName -match '^MSSQL\d+\.' } | ForEach-Object {
  $setup = Get-ItemProperty -LiteralPath (Join-Path $_.PSPath "Setup")
  if ($setup) {
    $result += @{
      Instance   = [string]$_.PSChildName
      Version    = [string]$setup.Version
      PatchLevel = [string]$setup.PatchLevel
    }
  }
}
$result | ConvertTo-Json -Compress -Depth 2
`
	cmd, cancel := boundedCommand(networkCollectorTimeout, winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(psScript))
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("SQL Server setup registry query failed")
		return nil
	}
	return parseSQLServerSetups(output)
}

func parseSQLServerSetups(output []byte) []sqlServerSetup {
	s := strings.TrimSpace(string(winexec.TrimBOM(output)))
	if s == "" || s == "null" {
		return nil
	}
	if !strings.HasPrefix(s, "[") {
		s = "[" + s + "]"
	}
	var setups []sqlServerSetup
	if err := json.Unmarshal([]byte(s), &setups); err != nil {
		return nil
	}
	return setups
}

type appOrigin struct {
	registry bool
	wingetID string
}

func needsAppxState(pkgs []models.Package, origins []appOrigin) bool {
	if hasAppxStyleName(pkgs) {
		return true
	}
	for _, o := range origins {
		if _, ok := appxNameFromFamily(o.wingetID); ok && !o.registry {
			return true
		}
	}
	return false
}

func hasAppxStyleName(pkgs []models.Package) bool {
	for _, p := range pkgs {
		if looksLikeAppxName(p.Name) {
			return true
		}
	}
	return false
}

func looksLikeAppxName(name string) bool {
	return strings.Contains(name, ".") && !strings.ContainsAny(name, " \t")
}

var packageFamilyRe = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9.-]*)_[a-z0-9]{13}$`)

func appxNameFromFamily(id string) (string, bool) {
	m := packageFamilyRe.FindStringSubmatch(id)
	if m == nil {
		return "", false
	}
	return m[1], true
}

type appxPackage struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
	States  string `json:"States"`
}

type appxVersions struct {
	installed string
	present   bool
	staged    []string
}

func (v *appxVersions) hasStaged(version string) bool {
	for _, s := range v.staged {
		if versionsEqual(s, version) {
			return true
		}
	}
	return false
}

func (v *appxVersions) newerThanAllStaged() bool {
	for _, s := range v.staged {
		if c, _ := compareDottedVersions(v.installed, s); c <= 0 {
			return false
		}
	}
	return true
}

func isDottedNumeric(v string) bool {
	_, ok := parseDottedVersion(v)
	return ok
}

func sameVersionScheme(a, b string) bool {
	pa, okA := parseDottedVersion(a)
	pb, okB := parseDottedVersion(b)
	return okA && okB && pa[0] == pb[0]
}

func indexAppxPackages(appx []appxPackage) map[string]*appxVersions {
	byName := map[string]*appxVersions{}
	for _, a := range appx {
		if _, ok := parseDottedVersion(a.Version); !ok {
			continue
		}
		key := normalizePackageName(a.Name)
		v := byName[key]
		if v == nil {
			v = &appxVersions{}
			byName[key] = v
		}
		var installed, present, staged bool
		for _, st := range strings.Split(strings.ToLower(a.States), ",") {
			switch st = strings.TrimSpace(st); st {
			case "", "notinstalled":
			case "staged":
				staged = true
			case "installed":
				installed, present = true, true
			default:
				present = true
			}
		}
		if present {
			v.present = true
		}
		switch {
		case installed:
			if c, _ := compareDottedVersions(a.Version, v.installed); v.installed == "" || c > 0 {
				v.installed = a.Version
			}
		case staged && !present:
			v.staged = append(v.staged, a.Version)
		}
	}
	return byName
}

func appxVersionsForRow(byName map[string]*appxVersions, p models.Package, o appOrigin) (*appxVersions, bool) {
	if !o.registry {
		if name, ok := appxNameFromFamily(o.wingetID); ok {
			if v, found := byName[normalizePackageName(name)]; found {
				return v, true
			}
		}
	}
	if looksLikeAppxName(p.Name) {
		return appxVersionsFor(byName, normalizePackageName(p.Name))
	}
	return nil, false
}

// winget as SYSTEM reports machine-staged MSIX copies, not the per-user installs.
func applyAppxInstallState(pkgs []models.Package, origins []appOrigin, appx []appxPackage) (kept []models.Package, suppressed, dropped []string) {
	byName := indexAppxPackages(appx)
	kept = pkgs[:0]
	for i := range pkgs {
		p := pkgs[i]
		var o appOrigin
		if i < len(origins) {
			o = origins[i]
		}
		numeric := isDottedNumeric(p.CurrentVersion)
		v, exact := appxVersionsForRow(byName, p, o)
		if v == nil || len(v.staged) == 0 {
			kept = append(kept, p)
			continue
		}
		if !v.present {
			if !o.registry && (exact || (numeric && v.hasStaged(p.CurrentVersion))) {
				dropped = append(dropped, p.Name)
				continue
			}
			kept = append(kept, p)
			continue
		}
		if !numeric {
			kept = append(kept, p)
			continue
		}
		switch {
		case v.installed == "":
		case v.hasStaged(p.CurrentVersion):
			p = applyInstalledOverStaged(p, v.installed, &suppressed)
		case p.NeedsUpdate && exact && !sameVersionScheme(p.CurrentVersion, v.installed) && v.newerThanAllStaged():
			p.CurrentVersion = v.installed
			p.NeedsUpdate = false
			p.AvailableVersion = ""
			suppressed = append(suppressed, p.Name)
		}
		kept = append(kept, p)
	}
	return kept, suppressed, dropped
}

func applyInstalledOverStaged(p models.Package, installed string, suppressed *[]string) models.Package {
	staged := p.CurrentVersion
	if versionsEqual(installed, staged) {
		return p
	}
	p.CurrentVersion = installed
	switch {
	case p.AvailableVersion == "":
		if c, _ := compareDottedVersions(installed, staged); c < 0 {
			p.NeedsUpdate = true
			p.AvailableVersion = staged
		}
	case p.NeedsUpdate && !installedIsOlder(installed, p.AvailableVersion):
		p.NeedsUpdate = false
		p.AvailableVersion = ""
		*suppressed = append(*suppressed, p.Name)
	}
	return p
}

func appxVersionsFor(byName map[string]*appxVersions, name string) (*appxVersions, bool) {
	if v, ok := byName[name]; ok {
		return v, true
	}
	var found *appxVersions
	for key, v := range byName {
		if name != "" && strings.HasPrefix(key, name) {
			if found != nil {
				return nil, false
			}
			found = v
		}
	}
	return found, false
}

func (m *WindowsManager) getAppxPackages() []appxPackage {
	psScript := `
$ErrorActionPreference = "SilentlyContinue"
@(Get-AppxPackage -AllUsers | ForEach-Object {
  @{
    Name    = [string]$_.Name
    Version = [string]$_.Version
    States  = (@($_.PackageUserInformation | ForEach-Object { [string]$_.InstallState }) -join ',')
  }
}) | ConvertTo-Json -Compress -Depth 2
`
	cmd, cancel := boundedCommand(collectorTimeout, winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(psScript))
	defer cancel()
	start := time.Now()
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("Get-AppxPackage query failed")
		return nil
	}
	pkgs := parseAppxPackages(output)
	m.logger.WithFields(logrus.Fields{"count": len(pkgs), "elapsed_ms": time.Since(start).Milliseconds()}).Debug("Collected Appx install state")
	return pkgs
}

func parseAppxPackages(output []byte) []appxPackage {
	s := strings.TrimSpace(string(winexec.TrimBOM(output)))
	if s == "" || s == "null" {
		return nil
	}
	if !strings.HasPrefix(s, "[") {
		s = "[" + s + "]"
	}
	var pkgs []appxPackage
	if err := json.Unmarshal([]byte(s), &pkgs); err != nil {
		return nil
	}
	return pkgs
}

// normalizePackageName lowercases and trims for fuzzy matching between registry and WinGet names
func normalizePackageName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// getPackagesFromRegistry uses registry Uninstall keys (HKLM + HKCU) to list installed programs.
// This is the primary collection method — works reliably as SYSTEM in Session 0.
// Inspired by rmmagent's use of go-win64api (which also reads these same registry keys).
// Collects: Name, Version, Publisher, InstallDate, EstimatedSize.
func (m *WindowsManager) getPackagesFromRegistry() []models.Package {
	// SilentlyContinue throughout — never let a single bad entry kill the whole collection.
	// rmmagent's go-win64api also silently skips entries with missing fields.
	psScript := `
$ErrorActionPreference = "SilentlyContinue"
$seen = @{}
$result = @()
$paths = @(
  "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*",
  "HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*",
  "HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*"
)
foreach ($path in $paths) {
  try {
    Get-ItemProperty $path -ErrorAction SilentlyContinue | ForEach-Object {
      $name = $_.DisplayName
      if (-not $name) { return }
      # SystemComponent=1 entries are hidden from Programs and Features (drivers, runtime deps)
      if ($_.SystemComponent -eq 1) { return }
      # Skip entries without a valid name or that are just GUIDs
      $trimmed = $name.Trim()
      if ($trimmed -eq "" -or $trimmed -match "^{[0-9a-fA-F-]+}$") { return }
      # Deduplicate by name (first seen wins — 64-bit path is first)
      if ($seen[$name]) { return }
      $seen[$name] = $true
      $ver = if ($_.DisplayVersion) { $_.DisplayVersion } else { "unknown" }
      $pub = if ($_.Publisher) { $_.Publisher } else { "" }
      $installDate = ""
      if ($_.InstallDate) {
        try {
          $d = $_.InstallDate
          if ($d -match "^\d{8}$") {
            $installDate = "$($d.Substring(0,4))-$($d.Substring(4,2))-$($d.Substring(6,2))"
          } else {
            $installDate = $d
          }
        } catch {}
      }
      $size = ""
      if ($_.EstimatedSize) {
        try {
          $kb = [int]$_.EstimatedSize
          if ($kb -gt 1048576) {
            $size = "{0:N1} GB" -f ($kb / 1048576)
          } elseif ($kb -gt 1024) {
            $size = "{0:N1} MB" -f ($kb / 1024)
          } else {
            $size = "$kb KB"
          }
        } catch {}
      }
      $result += @{
        Name        = $name
        Version     = $ver
        Publisher   = $pub
        InstallDate = $installDate
        Size        = $size
      }
    }
  } catch {}
}
if ($result.Count -gt 5000) { $result = $result[0..4999] }
$result | ConvertTo-Json -Compress -Depth 3
`
	cmd, cancel := boundedCommand(networkCollectorTimeout, winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(psScript))
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Warn("Registry Uninstall query failed")
		return nil
	}

	outputStr := strings.TrimSpace(string(winexec.TrimBOM(output)))
	if outputStr == "" || outputStr == "null" || outputStr == "[]" {
		return nil
	}

	// PowerShell outputs a single object (not array) when there is exactly one result
	if !strings.HasPrefix(outputStr, "[") {
		outputStr = "[" + outputStr + "]"
	}

	var raw []struct {
		Name        string `json:"Name"`
		Version     string `json:"Version"`
		Publisher   string `json:"Publisher"`
		InstallDate string `json:"InstallDate"`
		Size        string `json:"Size"`
	}
	if err := json.Unmarshal([]byte(outputStr), &raw); err != nil {
		m.logger.WithError(err).Warn("Failed to parse registry JSON")
		return nil
	}

	var packages []models.Package
	for _, p := range raw {
		if p.Name == "" {
			continue
		}
		version := p.Version
		if version == "" {
			version = "unknown"
		}
		// Build description from publisher + install date + size (like rmmagent's SoftwareList fields)
		desc := buildAppDescription(p.Publisher, p.InstallDate, p.Size)
		packages = append(packages, models.Package{
			Name:           p.Name,
			Description:    desc,
			Category:       "Application",
			CurrentVersion: version,
			NeedsUpdate:    false,
		})
	}
	return packages
}

// buildAppDescription creates a human-readable description from registry metadata
func buildAppDescription(publisher, installDate, size string) string {
	var parts []string
	if publisher != "" {
		parts = append(parts, publisher)
	}
	if installDate != "" {
		parts = append(parts, "installed "+installDate)
	}
	if size != "" {
		parts = append(parts, size)
	}
	return strings.Join(parts, " | ")
}

// getPackagesFromWinget runs winget list and parses the text-table output.
func (m *WindowsManager) getPackagesFromWinget() []wingetRow {
	w, ok := m.resolveWingetCommand()
	if !ok {
		return nil
	}
	output, ok := m.runWingetList(w, "list")
	if !ok {
		return nil
	}
	entries := m.parseWingetTable(output)
	if len(entries) == 0 {
		return nil
	}
	return wingetEntriesToRows(entries, m.getWingetUpgradeAvailable(w))
}

type wingetCommand struct {
	exe string
	env []string
}

func (m *WindowsManager) resolveWingetCommand() (wingetCommand, bool) {
	exe, env, err := sharedWingetResolver.get()
	switch {
	case err == nil:
		return wingetCommand{exe: exe, env: env}, true
	case errors.Is(err, errWingetRecentlyFailed), errors.Is(err, errWingetNotFound):
		m.logger.WithError(err).Debug("WinGet not available")
	default:
		m.logger.WithError(err).Warn("WinGet not available")
	}
	return wingetCommand{}, false
}

func (m *WindowsManager) runWingetList(w wingetCommand, args ...string) (string, bool) {
	args = append(args, "--accept-source-agreements", "--disable-interactivity")
	ctx, cancel := context.WithTimeout(context.Background(), networkCollectorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.exe, args...)
	cmd.WaitDelay = commandKillDelay
	cmd.Env = w.env
	hideWindow(cmd)
	output, err := cmd.Output()
	fields := map[string]interface{}{"args": strings.Join(args, " ")}
	if ctx.Err() != nil {
		m.logger.WithError(ctx.Err()).WithFields(logutil.SanitizeMap(fields)).Warn("winget timed out")
		return "", false
	}
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			m.logger.WithError(err).WithFields(logutil.SanitizeMap(fields)).Warn("winget failed")
			return "", false
		}
		code = ee.ExitCode()
		fields["exit_code"] = fmt.Sprintf("0x%08X", uint32(code))
	}
	out, class := classifyWingetExit(code, string(output))
	switch class {
	case wingetExitPartial:
		m.logger.WithFields(logutil.SanitizeMap(fields)).Debug("winget exited non-zero but printed a table; using it")
	case wingetExitFailed:
		m.logger.WithError(err).WithFields(logutil.SanitizeMap(fields)).Warn("winget failed")
		return "", false
	}
	return out, true
}

type wingetRow struct {
	pkg       models.Package
	id        string
	truncated bool
}

func wingetEntriesToRows(entries []wingetEntry, upgradeMap map[string]string) []wingetRow {
	var rows []wingetRow
	for _, e := range entries {
		rawName := strings.TrimSpace(e.Name)
		name := strings.TrimSpace(stripEllipsis(rawName))
		truncated := name != rawName
		if name == "" {
			name = strings.TrimSpace(stripEllipsis(e.ID))
			truncated = false
		}
		if name == "" {
			continue
		}
		version := strings.TrimSpace(stripEllipsis(e.Version))
		if version == "" {
			version = "unknown"
		}
		avail := strings.TrimSpace(stripEllipsis(e.Available))
		rawID := strings.TrimSpace(e.ID)
		id := strings.TrimSpace(stripEllipsis(rawID))
		wingetID := ""
		if id == rawID && wingetIDPattern.MatchString(id) {
			wingetID = id
		}
		var needsUpdate, pinned bool
		if up, listed := upgradeMap[id]; listed {
			if avail == "" {
				avail = up
			}
			needsUpdate = installedIsOlder(version, avail)
		} else {
			needsUpdate = availableIsNewer(avail, version)
			pinned = needsUpdate && wingetID != "" && upgradeMap != nil && !listedUnderTruncatedID(upgradeMap, id)
		}
		if !needsUpdate {
			avail = ""
		}
		source := strings.TrimSpace(e.Source)
		if source == "" {
			source = "winget"
		}

		rows = append(rows, wingetRow{
			pkg: models.Package{
				Name:             name,
				Category:         "Application",
				CurrentVersion:   version,
				AvailableVersion: avail,
				NeedsUpdate:      needsUpdate,
				SourceRepository: source,
				WingetID:         wingetID,
				WingetPinned:     pinned,
			},
			id:        id,
			truncated: truncated,
		})
	}
	return rows
}

// Non-numeric versions keep the plain inequality so winget's own ordering is not second-guessed.
func availableIsNewer(available, installed string) bool {
	if available == "" || isUnknownVersion(installed) {
		return false
	}
	if c, ok := compareDottedVersions(available, installed); ok {
		return c > 0
	}
	return available != installed
}

// parseWingetTable parses winget list fixed-width text output.
// Uses header word positions to derive column boundaries; handles Name/Id/Version and optional Available/Source.
func (m *WindowsManager) parseWingetTable(output string) []wingetEntry {
	// Normalize line endings
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	lines := strings.Split(output, "\n")

	var headerLine string
	var headerIdx int
	for i, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if len(lower) < 10 {
			continue
		}
		// Header must contain name, id, version (handles "Name", "SearchName", etc.)
		if strings.Contains(lower, "name") && strings.Contains(lower, "id") && strings.Contains(lower, "version") {
			headerLine = line
			headerIdx = i
			break
		}
	}
	if headerLine == "" {
		m.logger.Debug("parseWingetTable: no header line found")
		return nil
	}

	// Column boundaries are measured in characters, not bytes. winget pads the
	// table to a fixed display width, and it truncates a long name with a
	// single ellipsis character that is three bytes of UTF-8. Slicing a data
	// line at a byte offset taken from the ASCII header therefore cuts that
	// character in half, leaving "\xe2\x80" on the end of the name and "\xa6"
	// on the front of the version. Both fields are then invalid UTF-8,
	// stripEllipsis never matches because the ellipsis is in fragments, and
	// the mangled names defeat deduplication in mergeRegistryAndWinget so the
	// same package is reported twice.
	headerRunes := []rune(headerLine)
	type colSpan struct{ start, end int }
	var spans []colSpan
	for i := 0; i < len(headerRunes); {
		if unicode.IsSpace(headerRunes[i]) {
			i++
			continue
		}
		start := i
		for i < len(headerRunes) && !unicode.IsSpace(headerRunes[i]) {
			i++
		}
		spans = append(spans, colSpan{start: start, end: i})
	}
	if len(spans) < 3 {
		m.logger.WithField("header", headerLine).Debug("parseWingetTable: header has fewer than 3 columns")
		return nil
	}

	// colStarts[i] = start of column i; colEnd for column i = colStarts[i+1] or end of line
	colStarts := make([]int, len(spans))
	for i, s := range spans {
		colStarts[i] = s.start
	}

	extractCol := func(lineRunes []rune, col int) string {
		if col < 0 || col >= len(colStarts) {
			return ""
		}
		start := colStarts[col]
		end := len(lineRunes)
		if col+1 < len(colStarts) {
			end = colStarts[col+1]
		}
		if start >= len(lineRunes) {
			return ""
		}
		if end > len(lineRunes) {
			end = len(lineRunes)
		}
		return strings.TrimSpace(string(lineRunes[start:end]))
	}

	// Map column index to field (handles "Name"/"SearchName", "Id"/"SearchId", etc.)
	nameCol, idCol, versionCol := 0, 1, 2
	availCol, sourceCol := -1, -1
	for i := 0; i < len(spans) && i < 5; i++ {
		word := strings.ToLower(strings.TrimSpace(string(headerRunes[spans[i].start:spans[i].end])))
		switch {
		case word == "name" || strings.HasSuffix(word, "name"):
			nameCol = i
		case word == "id" || strings.HasSuffix(word, "id"):
			idCol = i
		case word == "version" || strings.HasSuffix(word, "version"):
			versionCol = i
		case word == "available" || strings.HasSuffix(word, "available"):
			availCol = i
		case word == "source" || strings.HasSuffix(word, "source"):
			sourceCol = i
		}
	}

	separatorRe := regexp.MustCompile(`^[-_\s]+$`)
	progressRe := regexp.MustCompile(`[█▒░]`)

	var entries []wingetEntry
	for i := headerIdx + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if separatorRe.MatchString(trimmed) {
			continue
		}
		if progressRe.MatchString(line) {
			continue
		}
		if strings.Contains(strings.ToLower(line), "package(s)") {
			continue
		}

		lineRunes := []rune(line)
		name := extractCol(lineRunes, nameCol)
		id := extractCol(lineRunes, idCol)
		version := extractCol(lineRunes, versionCol)
		if name == "" && id == "" {
			continue
		}
		e := wingetEntry{Name: name, ID: id, Version: version}
		if availCol >= 0 {
			e.Available = extractCol(lineRunes, availCol)
		}
		if sourceCol >= 0 && sourceCol < len(colStarts) {
			e.Source = extractCol(lineRunes, sourceCol)
		}
		entries = append(entries, e)
	}
	return entries
}

// getWingetUpgradeAvailable runs winget list --upgrade-available and returns Id -> AvailableVersion map
func (m *WindowsManager) getWingetUpgradeAvailable(w wingetCommand) map[string]string {
	output, ok := m.runWingetList(w, "list", "--upgrade-available")
	if !ok {
		return nil
	}
	entries := m.parseWingetTable(output)
	upgradeMap := make(map[string]string)
	for _, e := range entries {
		id := strings.TrimSpace(stripEllipsis(e.ID))
		if id == "" {
			continue
		}
		avail := strings.TrimSpace(stripEllipsis(e.Available))
		if avail == "" {
			avail = strings.TrimSpace(stripEllipsis(e.Version))
		}
		if avail != "" {
			upgradeMap[id] = avail
		}
	}
	return upgradeMap
}

func stripEllipsis(s string) string {
	s = strings.TrimSpace(s)
	// Winget truncates with U+2026 HORIZONTAL ELLIPSIS.
	const ellipsis = "\u2026"
	if strings.HasSuffix(s, ellipsis) {
		return strings.TrimSuffix(s, ellipsis)
	}
	// The same character written as UTF-8 (E2 80 A6) and read back through a
	// single-byte code page lands as ÔÇª under CP850 or â€¦ under CP1252.
	// The constant here used to spell ÔÂÉ, so it never matched anything.
	for _, mangled := range []string{"\u00d4\u00c7\u00aa", "\u00e2\u20ac\u00a6"} {
		if strings.HasSuffix(s, mangled) {
			return strings.TrimSuffix(s, mangled)
		}
	}
	return s
}

// isWSUSActive checks if WSUS is configured and active by reading the Windows registry.
func (m *WindowsManager) isWSUSActive() bool {
	if runtime.GOOS != "windows" {
		return false
	}

	psScript := `
$ErrorActionPreference = "SilentlyContinue"
$wuKey = "HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate"
$server = (Get-ItemProperty -Path $wuKey -Name WUServer -ErrorAction SilentlyContinue).WUServer
$useWU = (Get-ItemProperty -Path "$wuKey\AU" -Name UseWUServer -ErrorAction SilentlyContinue).UseWUServer
if ($server -and $useWU -eq 1) { "WSUS_ACTIVE" } else { "WSUS_INACTIVE" }
`
	cmd, cancel := boundedCommand(networkCollectorTimeout, winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(psScript))
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("Failed to check WSUS status")
		return false
	}

	return strings.Contains(strings.TrimSpace(string(output)), "WSUS_ACTIVE")
}

// wuaErrorHint returns a human-readable hint for common WUA HRESULTs (per UsoClient/WUA docs)
func wuaErrorHint(msg string) string {
	switch {
	case strings.Contains(msg, "0x80070005"):
		return "E_ACCESSDENIED: agent may be running as service in Session 0; COM requires interactive context"
	case strings.Contains(msg, "0x80240440"):
		return "WUA service unavailable; ensure wuauserv is running"
	case strings.Contains(msg, "0x8024402f"):
		return "Connection to update server failed; check network, proxy, firewall"
	case strings.Contains(msg, "0x80244007"):
		return "Update server not found; verify WSUS config or internet access"
	case strings.Contains(msg, "0x80070002"):
		return "File not found; try clearing SoftwareDistribution cache"
	default:
		return ""
	}
}

const windowsUpdatesScript = `
$ErrorActionPreference = "SilentlyContinue"
$result = @()

# --- Installed KBs from WMI (fast, reliable) ---
$hotfixes = Get-HotFix -ErrorAction SilentlyContinue
foreach ($hf in $hotfixes) {
  $id = $hf.HotFixID
  if (-not $id) { continue }
  $installedOn = ""
  try {
    if ($hf.InstalledOn -ne $null) {
      $installedOn = $hf.InstalledOn.ToString("yyyy-MM-dd")
    }
  } catch {}
  if (-not $installedOn) { $installedOn = "installed" }
  $desc = if ($installedOn -and $installedOn -ne "installed") { "Installed $installedOn" } else { "Installed" }
  $result += @{
    Name             = $id
    Description      = $desc
    CurrentVersion   = "installed"
    AvailableVersion = ""
    NeedsUpdate      = $false
    IsSecurityUpdate = ($hf.Description -match "Security")
    WUAGuid          = ""
    WUAKb            = $id
    WUASeverity      = ""
    WUACategories    = @()
    WUASupportURL    = ""
    WUARevisionNumber = 0
    WUADateInstalled = $installedOn
  }
}

# --- Pending updates via Windows Update COM API (fails in Session 0 / service context) ---
$comFailed = $false
try {
  $session   = New-Object -ComObject Microsoft.Update.Session
  $searcher  = $session.CreateUpdateSearcher()
  $results   = $searcher.Search("IsInstalled=0 AND IsHidden=0")
  $pendingRebootIds = @{}
  if (Get-SystemRebootRequired) { $pendingRebootIds = Get-InProgressIds $session }
  foreach ($u in $results.Updates) {
    $secFlag = ($u.MsrcSeverity -eq "Critical" -or $u.MsrcSeverity -eq "Important")
    $kbs = @($u.KBArticleIDs | ForEach-Object { "KB$_" })
    $guid = ""
    try { $guid = $u.Identity.UpdateID } catch {}
    $cats = @($u.Categories | ForEach-Object { $_.Name })
    $catIds = @($u.Categories | ForEach-Object { [string]$_.CategoryID })
    $supportUrl = ""
    try { $supportUrl = $u.SupportURL } catch {}
    $revNum = 0
    try { $revNum = [int]$u.Identity.RevisionNumber } catch {}
    $result += @{
      Name             = $u.Title
      Title            = $u.Title
      KBIDs            = @($u.KBArticleIDs | ForEach-Object { [string]$_ })
      CategoryIDs      = $catIds
      BrowseOnly       = [bool]$u.BrowseOnly
      TypeCode         = [int]$u.Type
      PendingReboot    = [bool]$pendingRebootIds[([string]$guid).ToLower()]
      CurrentVersion   = "pending"
      AvailableVersion = ""
      NeedsUpdate      = $true
      IsSecurityUpdate = $secFlag
      WUAGuid          = $guid
      WUAKb            = ($kbs -join ", ")
      WUASeverity      = if ($u.MsrcSeverity) { $u.MsrcSeverity } else { "" }
      WUACategories    = $cats
      WUASupportURL    = $supportUrl
      WUARevisionNumber = $revNum
    }
  }
} catch {
  $comFailed = $true
  $hresult = if ($_.Exception.HResult) { [Convert]::ToString([uint32]$_.Exception.HResult, 16) } else { "" }
  Write-Host "WUA_COM_ERROR:$($_.Exception.Message) (HRESULT: 0x$hresult)"
}

# --- Fallback: PSWindowsUpdate module when COM fails (Session 0, 0x80070005, etc.) ---
if ($comFailed) {
  try {
    if (Get-Module -ListAvailable -Name PSWindowsUpdate) {
      Import-Module PSWindowsUpdate -ErrorAction Stop
      $wuList = Get-WUList -MicrosoftUpdate -ErrorAction Stop
      foreach ($u in $wuList) {
        $kb = if ($u.KB) { " ($($u.KB))" } else { "" }
        $result += @{
          Name             = "$($u.Title)$kb"
          CurrentVersion   = "pending"
          AvailableVersion = ""
          NeedsUpdate      = $true
          IsSecurityUpdate = ($u.MsrcSeverity -match "Critical|Important")
          WUAGuid          = $u.UpdateID
          WUAKb            = $u.KB
          WUASeverity      = if ($u.MsrcSeverity) { $u.MsrcSeverity } else { "" }
          WUACategories    = @()
          WUASupportURL    = ""
          WUARevisionNumber = 0
        }
      }
    }
  } catch {}
}

$result | ConvertTo-Json -Compress -Depth 4
`

// getWindowsUpdates queries installed KB patches and pending Windows updates.
// Uses Get-HotFix for installed; Microsoft.Update.Session for pending (may fail in Session 0).
func (m *WindowsManager) getWindowsUpdates() ([]models.Package, wuSearch) {
	cmd, cancel := boundedCommand(networkCollectorTimeout, winexec.PowerShellPath(), "-NoProfile", "-NonInteractive", "-Command", winexec.Script(wuaPendingScript+windowsUpdatesScript))
	defer cancel()
	output, err := cmd.Output()
	if err != nil {
		return nil, wuSearch{status: wuSearchFailed, detail: err.Error()}
	}

	outputStr := strings.TrimSpace(string(winexec.TrimBOM(output)))
	// Check for COM error message (e.g. E_ACCESSDENIED in Session 0)
	result := wuSearch{status: wuSearchOK}
	if strings.Contains(outputStr, "WUA_COM_ERROR:") {
		idx := strings.Index(outputStr, "WUA_COM_ERROR:")
		end := idx + 120
		if end > len(outputStr) {
			end = len(outputStr)
		}
		msg := outputStr[idx:end]
		if hint := wuaErrorHint(msg); hint != "" {
			msg += " (" + hint + ")"
		}
		result = wuSearch{status: wuSearchCOMFailed, detail: msg}
		if jsonStart := strings.Index(outputStr, "["); jsonStart >= 0 {
			outputStr = outputStr[jsonStart:]
		} else {
			outputStr = ""
		}
	}

	if outputStr == "" || outputStr == "null" || outputStr == "[]" {
		return []models.Package{}, result
	}

	// PowerShell may output a single object (not array) when there is exactly one result
	if !strings.HasPrefix(outputStr, "[") {
		outputStr = "[" + outputStr + "]"
	}

	var raw []wuaRawUpdate
	if err := json.Unmarshal([]byte(outputStr), &raw); err != nil {
		return nil, wuSearch{status: wuSearchFailed, detail: "unreadable output: " + err.Error()}
	}

	var packages []models.Package
	for _, u := range raw {
		if pkg, ok := wuaRawToPackage(u); ok {
			packages = append(packages, pkg)
		}
	}
	if packages == nil {
		packages = []models.Package{}
	}
	return packages, result
}
