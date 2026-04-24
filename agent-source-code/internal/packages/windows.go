package packages

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"patchmon-agent/internal/logutil"
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
func (m *WindowsManager) GetPackages() []models.Package {
	if runtime.GOOS != "windows" {
		return nil
	}

	// 1. Registry is the primary source — works reliably as SYSTEM in Session 0
	//    (same approach as rmmagent's go-win64api which reads Uninstall registry keys)
	regPackages := m.getPackagesFromRegistry()
	m.logger.WithField("count", len(regPackages)).Info("Collected packages via registry")

	// 2. Try WinGet for update availability enrichment
	//    WinGet may not be available in Session 0 (per-user UWP app); that's OK
	wingetPackages := m.getPackagesFromWinget()
	if len(wingetPackages) > 0 {
		m.logger.WithField("count", len(wingetPackages)).Info("Collected packages via WinGet (supplementary)")
	}

	// 3. Merge: start with registry baseline, enrich with WinGet update info
	appPackages := m.mergeRegistryAndWinget(regPackages, wingetPackages)
	m.logger.WithField("count", len(appPackages)).Info("Application packages after merge")

	// 4. Collect Windows OS updates (installed KBs + pending updates)
	winUpdates := m.getWindowsUpdates()
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
		return []models.Package{}
	}
	return all
}

// mergeRegistryAndWinget merges registry-discovered packages with WinGet data.
// Registry provides the reliable baseline; WinGet enriches with update availability.
// Any WinGet-only entries (not in registry) are also included.
// SourceRepository is set to the WinGet source for matched packages, or "local" for registry-only.
func (m *WindowsManager) mergeRegistryAndWinget(regPkgs, wingetPkgs []models.Package) []models.Package {
	if len(wingetPkgs) == 0 {
		// No WinGet data: all registry packages are local
		for i := range regPkgs {
			regPkgs[i].SourceRepository = "local"
		}
		return regPkgs
	}

	// Build lookup from WinGet by normalized name for fuzzy matching
	type wingetInfo struct {
		AvailableVersion string
		NeedsUpdate      bool
		SourceRepository string
		matched          bool
	}
	wingetByName := make(map[string]*wingetInfo, len(wingetPkgs))
	for i := range wingetPkgs {
		key := normalizePackageName(wingetPkgs[i].Name)
		wingetByName[key] = &wingetInfo{
			AvailableVersion: wingetPkgs[i].AvailableVersion,
			NeedsUpdate:      wingetPkgs[i].NeedsUpdate,
			SourceRepository: wingetPkgs[i].SourceRepository,
		}
	}

	// Enrich registry entries with WinGet update info and source repo
	for i := range regPkgs {
		key := normalizePackageName(regPkgs[i].Name)
		if winfo, ok := wingetByName[key]; ok {
			if winfo.NeedsUpdate {
				regPkgs[i].NeedsUpdate = true
				if regPkgs[i].AvailableVersion == "" {
					regPkgs[i].AvailableVersion = winfo.AvailableVersion
				}
			}
			regPkgs[i].SourceRepository = winfo.SourceRepository
			winfo.matched = true
		} else {
			// Not found in WinGet: local/registry-only install
			regPkgs[i].SourceRepository = "local"
		}
	}

	// Add WinGet-only entries that weren't in registry
	for i := range wingetPkgs {
		key := normalizePackageName(wingetPkgs[i].Name)
		if winfo, ok := wingetByName[key]; ok && !winfo.matched {
			regPkgs = append(regPkgs, wingetPkgs[i])
		}
	}

	return regPkgs
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
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Warn("Registry Uninstall query failed")
		return nil
	}

	outputStr := strings.TrimSpace(string(output))
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
// Uses PowerShell wrapper for UTF-8 encoding to avoid U+FFFD mojibake.
// Resolves the actual winget.exe path to work in SYSTEM/Session 0 context
// where winget may not be on PATH (it's a per-user UWP app).
func (m *WindowsManager) getPackagesFromWinget() []models.Package {
	// Resolve winget.exe — it's installed as a UWP app (App Installer) and not always on SYSTEM PATH.
	// Try PATH first, then known WindowsApps locations.
	psScript := `
$ErrorActionPreference = "SilentlyContinue"
$env:TERM = 'dumb'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# Resolve winget.exe path — handle SYSTEM/Session 0 where it's not on PATH
$wingetPath = $null
$candidate = Get-Command winget.exe -ErrorAction SilentlyContinue
if ($candidate) {
    $wingetPath = $candidate.Source
} else {
    # UWP package location (works for SYSTEM when App Installer is installed machine-wide)
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
    Write-Output "WINGET_NOT_FOUND"
    exit 0
}
$out = & $wingetPath list --accept-source-agreements --disable-interactivity 2>&1
if ($out) { $out | Out-String }
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("winget list failed")
		return nil
	}

	outputStr := string(output)
	if strings.Contains(outputStr, "WINGET_NOT_FOUND") {
		m.logger.Debug("WinGet not available (not found on PATH or in WindowsApps)")
		return nil
	}

	entries := m.parseWingetTable(outputStr)
	if len(entries) == 0 {
		return nil
	}

	// Second pass: winget list --upgrade-available for accurate NeedsUpdate/AvailableVersion
	upgradeMap := m.getWingetUpgradeAvailable()

	var packages []models.Package
	for _, e := range entries {
		name := strings.TrimSpace(stripEllipsis(e.Name))
		if name == "" {
			name = strings.TrimSpace(stripEllipsis(e.ID))
		}
		if name == "" {
			continue
		}
		version := strings.TrimSpace(stripEllipsis(e.Version))
		if version == "" {
			version = "unknown"
		}
		avail := strings.TrimSpace(stripEllipsis(e.Available))
		needsUpdate := false
		id := strings.TrimSpace(stripEllipsis(e.ID))
		if up, ok := upgradeMap[id]; ok {
			needsUpdate = true
			if avail == "" {
				avail = up
			}
		} else if avail != "" && avail != version {
			needsUpdate = true
		}
		// Forward WinGet source as SourceRepository
		source := strings.TrimSpace(e.Source)
		if source == "" {
			source = "winget" // Default for WinGet-discovered packages
		}

		packages = append(packages, models.Package{
			Name:             name,
			Category:         "Application",
			CurrentVersion:   version,
			AvailableVersion: avail,
			NeedsUpdate:      needsUpdate,
			SourceRepository: source,
		})
	}
	return packages
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

	// Derive column boundaries from header: find start of each word (column)
	wordRe := regexp.MustCompile(`\S+`)
	matches := wordRe.FindAllStringIndex(headerLine, -1)
	if len(matches) < 3 {
		m.logger.WithField("header", headerLine).Debug("parseWingetTable: header has fewer than 3 columns")
		return nil
	}

	// colStarts[i] = start of column i; colEnd for column i = colStarts[i+1] or len(line)
	colStarts := make([]int, len(matches))
	for i, m := range matches {
		colStarts[i] = m[0]
	}

	extractCol := func(line string, col int) string {
		if col < 0 || col >= len(colStarts) {
			return ""
		}
		start := colStarts[col]
		var end int
		if col+1 < len(colStarts) {
			end = colStarts[col+1]
		} else {
			end = len(line) + 1
		}
		if start >= len(line) {
			return ""
		}
		if end > len(line) {
			end = len(line)
		}
		return strings.TrimSpace(line[start:end])
	}

	// Map column index to field (handles "Name"/"SearchName", "Id"/"SearchId", etc.)
	nameCol, idCol, versionCol := 0, 1, 2
	availCol, sourceCol := -1, -1
	for i := 0; i < len(matches) && i < 5; i++ {
		word := strings.ToLower(strings.TrimSpace(headerLine[matches[i][0]:matches[i][1]]))
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

		name := extractCol(line, nameCol)
		id := extractCol(line, idCol)
		version := extractCol(line, versionCol)
		if name == "" && id == "" {
			continue
		}
		e := wingetEntry{Name: name, ID: id, Version: version}
		if availCol >= 0 {
			e.Available = extractCol(line, availCol)
		}
		if sourceCol >= 0 && sourceCol < len(colStarts) {
			e.Source = extractCol(line, sourceCol)
		}
		entries = append(entries, e)
	}
	return entries
}

// getWingetUpgradeAvailable runs winget list --upgrade-available and returns Id -> AvailableVersion map
func (m *WindowsManager) getWingetUpgradeAvailable() map[string]string {
	psScript := `
$ErrorActionPreference = "SilentlyContinue"
$env:TERM = 'dumb'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

# Resolve winget.exe path (same logic as main list)
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
if (-not $wingetPath) { exit 0 }
$out = & $wingetPath list --upgrade-available --accept-source-agreements --disable-interactivity 2>&1
if ($out) { $out | Out-String }
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("winget list --upgrade-available failed")
		return nil
	}
	entries := m.parseWingetTable(string(output))
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
	// Winget truncates with U+2026 HORIZONTAL ELLIPSIS; also handle mis-encoded ÔÇª
	const ellipsis = "\u2026"
	if strings.HasSuffix(s, ellipsis) {
		return strings.TrimSuffix(s, ellipsis)
	}
	if strings.HasSuffix(s, "\u00d4\u00c2\u00c9") { // ÔÇª in UTF-8
		return strings.TrimSuffix(s, "\u00d4\u00c2\u00c9")
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
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
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

// getWindowsUpdates queries installed KB patches and pending Windows updates.
// Uses Get-HotFix for installed; Microsoft.Update.Session for pending (may fail in Session 0).
func (m *WindowsManager) getWindowsUpdates() []models.Package {
	psScript := `
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
  foreach ($u in $results.Updates) {
    $secFlag = ($u.MsrcSeverity -eq "Critical" -or $u.MsrcSeverity -eq "Important")
    $kbs = @($u.KBArticleIDs | ForEach-Object { "KB$_" })
    $kbStr = ($kbs -join ", ")
    $displayName = if ($kbStr) { "$($u.Title) ($kbStr)" } else { $u.Title }
    $guid = ""
    try { $guid = $u.Identity.UpdateID } catch {}
    $cats = @($u.Categories | ForEach-Object { $_.Name })
    $supportUrl = ""
    try { $supportUrl = $u.SupportURL } catch {}
    $revNum = 0
    try { $revNum = [int]$u.Identity.RevisionNumber } catch {}
    $result += @{
      Name             = $displayName
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
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to query Windows updates")
		return nil
	}

	outputStr := strings.TrimSpace(string(output))
	// Check for COM error message (e.g. E_ACCESSDENIED in Session 0)
	if strings.Contains(outputStr, "WUA_COM_ERROR:") {
		idx := strings.Index(outputStr, "WUA_COM_ERROR:")
		end := idx + 120
		if end > len(outputStr) {
			end = len(outputStr)
		}
		msg := outputStr[idx:end]
		hint := wuaErrorHint(msg)
		m.logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
			"detail": msg,
			"hint":   hint,
		})).Warn("Windows Update COM API failed")
		// Extract JSON array (Get-HotFix results) - it starts with [
		if jsonStart := strings.Index(outputStr, "["); jsonStart >= 0 {
			outputStr = outputStr[jsonStart:]
		} else {
			outputStr = ""
		}
	}

	if outputStr == "" || outputStr == "null" || outputStr == "[]" {
		return nil
	}

	// PowerShell may output a single object (not array) when there is exactly one result
	if !strings.HasPrefix(outputStr, "[") {
		outputStr = "[" + outputStr + "]"
	}

	var raw []struct {
		Name              string   `json:"Name"`
		CurrentVersion    string   `json:"CurrentVersion"`
		AvailableVersion  string   `json:"AvailableVersion"`
		NeedsUpdate       bool     `json:"NeedsUpdate"`
		IsSecurityUpdate  bool     `json:"IsSecurityUpdate"`
		WUAGuid           string   `json:"WUAGuid"`
		WUAKb             string   `json:"WUAKb"`
		WUASeverity       string   `json:"WUASeverity"`
		WUACategories     []string `json:"WUACategories"`
		WUASupportURL     string   `json:"WUASupportURL"`
		WUARevisionNumber int32    `json:"WUARevisionNumber"`
	}
	if err := json.Unmarshal([]byte(outputStr), &raw); err != nil {
		m.logger.WithError(err).Warn("Failed to parse Windows updates JSON")
		return nil
	}

	var packages []models.Package
	for _, u := range raw {
		if u.Name == "" {
			continue
		}
		cv := u.CurrentVersion
		if cv == "" {
			cv = "pending"
		}
		packages = append(packages, models.Package{
			Name:              u.Name,
			Category:          "Windows Update",
			CurrentVersion:    cv,
			AvailableVersion:  u.AvailableVersion,
			NeedsUpdate:       u.NeedsUpdate,
			IsSecurityUpdate:  u.IsSecurityUpdate,
			WUAGuid:           u.WUAGuid,
			WUAKb:             u.WUAKb,
			WUASeverity:       u.WUASeverity,
			WUACategories:     u.WUACategories,
			WUASupportURL:     u.WUASupportURL,
			WUARevisionNumber: u.WUARevisionNumber,
		})
	}
	return packages
}
