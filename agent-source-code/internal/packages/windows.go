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

// GetPackages returns installed applications (via WinGet or registry) merged with
// Windows OS updates (installed KB patches + any pending updates).
func (m *WindowsManager) GetPackages() []models.Package {
	if runtime.GOOS != "windows" {
		return nil
	}

	// Collect installed applications
	var appPackages []models.Package
	appPackages = m.getPackagesFromWinget()
	if len(appPackages) > 0 {
		m.logger.WithField("count", len(appPackages)).Info("Collected packages via WinGet")
	} else {
		appPackages = m.getPackagesFromRegistry()
		if len(appPackages) > 0 {
			m.logger.WithField("count", len(appPackages)).Info("Collected packages via registry (winget not available)")
		} else {
			m.logger.Warn("No application packages collected (winget and registry both returned empty)")
		}
	}

	// Collect Windows OS updates (installed KBs + pending updates)
	winUpdates := m.getWindowsUpdates()
	m.logger.WithField("count", len(winUpdates)).Info("Collected Windows OS updates")

	all := make([]models.Package, 0, len(appPackages)+len(winUpdates))
	all = append(all, appPackages...)
	all = append(all, winUpdates...)

	if len(all) == 0 {
		return []models.Package{}
	}
	return all
}

// getPackagesFromWinget runs winget list (text-table output) and parses it.
// Uses PowerShell wrapper for UTF-8 encoding to avoid U+FFFD mojibake when capturing output.
func (m *WindowsManager) getPackagesFromWinget() []models.Package {
	psScript := `
$ErrorActionPreference = "SilentlyContinue"
$env:TERM = 'dumb'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$out = & winget.exe list --accept-source-agreements --disable-interactivity 2>&1
if ($out) { $out | Out-String }
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("winget list failed (winget may not be installed)")
		return nil
	}

	entries := m.parseWingetTable(string(output))
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
		if up, ok := upgradeMap[e.ID]; ok {
			needsUpdate = true
			if avail == "" {
				avail = up
			}
		} else if avail != "" && avail != version {
			needsUpdate = true
		}
		packages = append(packages, models.Package{
			Name:             name,
			Category:         "Application",
			CurrentVersion:   version,
			AvailableVersion: avail,
			NeedsUpdate:      needsUpdate,
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
$out = & winget.exe list --upgrade-available --accept-source-agreements --disable-interactivity 2>&1
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
  $result += @{
    Name             = $id
    CurrentVersion   = $installedOn
    AvailableVersion = ""
    NeedsUpdate      = $false
    IsSecurityUpdate = ($hf.Description -match "Security")
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
    $kbs = @($u.KBArticleIDs | ForEach-Object { "KB$_" }) -join ", "
    $displayName = if ($kbs) { "$($u.Title) ($kbs)" } else { $u.Title }
    $result += @{
      Name             = $displayName
      CurrentVersion   = "pending"
      AvailableVersion = $u.Identity.UpdateID
      NeedsUpdate      = $true
      IsSecurityUpdate = $secFlag
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
          AvailableVersion = $u.UpdateID
          NeedsUpdate      = $true
          IsSecurityUpdate = ($u.MsrcSeverity -match "Critical|Important")
        }
      }
    }
  } catch {}
}

$result | ConvertTo-Json -Compress -Depth 3
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
		Name             string `json:"Name"`
		CurrentVersion   string `json:"CurrentVersion"`
		AvailableVersion string `json:"AvailableVersion"`
		NeedsUpdate      bool   `json:"NeedsUpdate"`
		IsSecurityUpdate bool   `json:"IsSecurityUpdate"`
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
			Name:             u.Name,
			Category:         "Windows Update",
			CurrentVersion:   cv,
			AvailableVersion: u.AvailableVersion,
			NeedsUpdate:      u.NeedsUpdate,
			IsSecurityUpdate: u.IsSecurityUpdate,
		})
	}
	return packages
}

// getPackagesFromRegistry uses registry Uninstall keys (HKLM + HKCU) to list installed programs
func (m *WindowsManager) getPackagesFromRegistry() []models.Package {
	psScript := `
$ErrorActionPreference = "Stop"
$seen = @{}
$result = @()
$paths = @(
  "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*",
  "HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*",
  "HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*"
)
foreach ($path in $paths) {
  Get-ItemProperty $path -ErrorAction SilentlyContinue | ForEach-Object {
    $name = $_.DisplayName
    $ver = $_.DisplayVersion
    if ($name -and $ver -and -not $seen[$name]) {
      $seen[$name] = $true
      $result += @{ Name = $name; Version = $ver }
    }
  }
}
if ($result.Count -gt 5000) { $result = $result[0..4999] }
$result | ConvertTo-Json -Compress
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Debug("Registry Uninstall query failed")
		return nil
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		return nil
	}

	var raw []struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
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
		packages = append(packages, models.Package{
			Name:           p.Name,
			Category:       "Application",
			CurrentVersion: version,
			NeedsUpdate:    false,
		})
	}
	return packages
}
