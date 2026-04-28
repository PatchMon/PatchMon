package packages

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// OpenBSDManager handles OpenBSD package information collection
type OpenBSDManager struct {
	logger *logrus.Logger
}

// NewOpenBSDManager creates a new OpenBSD package manager
func NewOpenBSDManager(logger *logrus.Logger) *OpenBSDManager {
	return &OpenBSDManager{
		logger: logger,
	}
}

// GetPackages gets package information for OpenBSD systems.
// Collects from: pkg_info (binary packages) and syspatch (base system patches).
func (m *OpenBSDManager) GetPackages() ([]models.Package, error) {
	var allPackages []models.Package

	// 1. Get pkg binary packages
	pkgPackages, err := m.getPkgPackages()
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get OpenBSD pkg packages")
	} else {
		allPackages = append(allPackages, pkgPackages...)
	}

	// 2. Get syspatch base-system update info
	sysPackage := m.getSyspatchInfo()
	if sysPackage != nil {
		allPackages = append(allPackages, *sysPackage)
	}

	return allPackages, nil
}

// getPkgPackages collects installed packages via pkg_info and available upgrades
// via pkg_add -uqn (dry-run).
func (m *OpenBSDManager) getPkgPackages() ([]models.Package, error) {
	// --- installed packages ---
	m.logger.Debug("Getting installed packages with pkg_info...")
	installedOut, err := exec.Command("pkg_info").Output()
	installedPackages := make(map[string]models.Package)
	if err != nil {
		m.logger.WithError(err).Warn("Failed to get installed packages via pkg_info")
	} else {
		installedPackages = m.parseInstalledPackages(string(installedOut))
		m.logger.WithField("count", len(installedPackages)).Debug("Found installed packages")
	}

	// --- upgradable packages ---
	// pkg_add -u -v -n: dry-run upgrade with verbose output.
	// -v is required: without it pkg_add produces no "Update candidates:" lines.
	// CombinedOutput captures stderr too (pkg_add prints progress to stderr).
	m.logger.Debug("Checking for package upgrades via pkg_add -u -v -n...")
	upgradeOut, _ := exec.Command("pkg_add", "-u", "-v", "-n").CombinedOutput()
	upgradablePackages := m.parseUpgradeOutput(string(upgradeOut))
	m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")

	packages := CombinePackageData(installedPackages, upgradablePackages)
	return packages, nil
}

// parseInstalledPackages parses pkg_info output.
// Format: pkgname-version    Description
// Example: bash-5.2.15          GNU Project's Bourne Again SHell
func (m *OpenBSDManager) parseInstalledPackages(output string) map[string]models.Package {
	installedPackages := make(map[string]models.Package)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		// fields[0] is the full "pkgname-version" token as it appears in /var/db/pkg/
		fullToken := fields[0]
		pkgName, version := splitOpenBSDPackage(fullToken)
		if pkgName != "" && version != "" {
			description := ""
			if len(fields) > 1 {
				description = strings.Join(fields[1:], " ")
			}
			installedPackages[pkgName] = models.Package{
				Name:             pkgName,
				CurrentVersion:   version,
				Description:      description,
				SourceRepository: pkgSourceRepo(fullToken),
				NeedsUpdate:      false,
			}
		}
	}
	return installedPackages
}

// pkgSourceRepo reads the @url line from /var/db/pkg/<fullToken>/+CONTENTS and
// returns the mirror directory URL (everything up to and excluding the filename).
// Returns an empty string if the file cannot be read or the @url line is absent.
func pkgSourceRepo(fullToken string) string {
	contents, err := os.ReadFile("/var/db/pkg/" + fullToken + "/+CONTENTS")
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "@url ") {
			continue
		}
		url := strings.TrimPrefix(line, "@url ")
		// Strip the filename to return the mirror directory.
		if idx := strings.LastIndex(url, "/"); idx > 0 {
			return url[:idx]
		}
		return url
	}
	return ""
}

// parseUpgradeOutput parses pkg_add -uqn output.
// pkg_add prints candidate lines in the form:
//
//	Update candidates: curl-8.7.1 -> curl-8.10.0
func (m *OpenBSDManager) parseUpgradeOutput(output string) []models.Package {
	var packages []models.Package
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Update candidates:") {
			continue
		}
		// "Update candidates: pkgold-X.Y -> pkgnew-A.B"
		rest := strings.TrimSpace(strings.TrimPrefix(line, "Update candidates:"))
		parts := strings.SplitN(rest, "->", 2)
		if len(parts) != 2 {
			continue
		}
		oldPkg := strings.TrimSpace(parts[0])
		newPkg := strings.TrimSpace(parts[1])

		oldName, oldVersion := splitOpenBSDPackage(oldPkg)
		newName, newVersion := splitOpenBSDPackage(newPkg)

		if oldName == "" || oldVersion == "" || newVersion == "" {
			continue
		}
		// Skip entries where the version hasn't changed (already up-to-date).
		if oldVersion == newVersion {
			continue
		}
		name := oldName
		if newName != "" {
			name = newName
		}
		packages = append(packages, models.Package{
			Name:             name,
			CurrentVersion:   oldVersion,
			AvailableVersion: newVersion,
			NeedsUpdate:      true,
			IsSecurityUpdate: false,
		})
	}
	return packages
}

// getSyspatchInfo checks for available OpenBSD base-system patches via syspatch -c.
// Returns a single package entry summarising all pending errata, or nil when none are pending.
//
// syspatch only distributes security errata, so IsSecurityUpdate is always true.
// CurrentVersion is the OS release from uname -r; AvailableVersion is the patch count.
// Description is enriched with per-errata text fetched from openbsd.org/errataXX.html.
// SourceRepository is read from /etc/installurl (the configured mirror).
func (m *OpenBSDManager) getSyspatchInfo() *models.Package {
	// OS base version
	osVersion := "unknown"
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		osVersion = strings.TrimSpace(string(out))
	}

	// Configured mirror URL
	sourceRepo := ""
	if data, err := os.ReadFile("/etc/installurl"); err == nil {
		sourceRepo = strings.TrimSpace(string(data))
	}

	// Use CombinedOutput so we capture stdout even when syspatch -c exits
	// non-zero (e.g. exit 1 while a concurrent syspatch run is in progress,
	// or when the network is briefly unreachable). We treat the run as
	// "no patches available" only when the output is genuinely empty.
	out, _ := exec.Command("syspatch", "-c").CombinedOutput()

	var patchIDs []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id != "" {
			patchIDs = append(patchIDs, id)
		}
	}
	if len(patchIDs) == 0 {
		return nil
	}

	// Optionally enrich each patch ID with its errata description from openbsd.org.
	errataDescs := m.fetchErrataDescriptions(osVersion)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pending security errata (%d):\n", len(patchIDs)))
	for _, id := range patchIDs {
		// Patch IDs have the form "NNN_component"; the number is the errata key.
		num := ""
		if len(id) >= 3 {
			num = id[:3]
		}
		if desc, ok := errataDescs[num]; ok {
			sb.WriteString(fmt.Sprintf("%s: %s\n", id, desc))
		} else {
			sb.WriteString(id + "\n")
		}
	}
	description := strings.TrimRight(sb.String(), "\n")
	availableVersion := fmt.Sprintf("%d patch(es) available", len(patchIDs))

	return &models.Package{
		Name:             "syspatch",
		CurrentVersion:   osVersion,
		AvailableVersion: availableVersion,
		Description:      description,
		SourceRepository: sourceRepo,
		NeedsUpdate:      true,
		IsSecurityUpdate: true,
	}
}

// fetchErrataDescriptions fetches https://www.openbsd.org/errataXX.html (where XX
// is the OS version without the dot, e.g. "76" for 7.6) and returns a map from
// zero-padded errata number (e.g. "001") to a human-readable description line.
// Returns an empty map on any network or parse failure — callers treat it as
// optional enrichment and fall back to the bare patch ID.
func (m *OpenBSDManager) fetchErrataDescriptions(osVersion string) map[string]string {
	versionTag := strings.ReplaceAll(osVersion, ".", "")
	url := "https://www.openbsd.org/errata" + versionTag + ".html"

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		m.logger.WithError(err).Debug("Could not fetch OpenBSD errata page")
		return map[string]string{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		m.logger.WithField("status", resp.StatusCode).Debug("Unexpected status fetching OpenBSD errata page")
		return map[string]string{}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		m.logger.WithError(err).Debug("Failed to read OpenBSD errata page body")
		return map[string]string{}
	}

	// Each errata entry in the HTML has the form:
	//   <strong>NNN: TYPE FIX: date</strong>
	//   &nbsp; <i>architectures</i>
	//   <br>
	//   Description text CVE-XXXX
	//   <br>
	//   <a href="...NNN_component.patch.sig">...
	errataRe := regexp.MustCompile(
		`(?i)<strong>(\d{3}):\s+(SECURITY|RELIABILITY)\s+FIX:\s+([^<]+)</strong>` +
			`[\s\S]*?<br>\s*([\s\S]*?)\s*<br>\s*<a\s+href="[^"]*\.patch\.sig"`,
	)
	tagRe := regexp.MustCompile(`<[^>]+>`)
	spaceRe := regexp.MustCompile(`\s+`)

	result := make(map[string]string)
	for _, match := range errataRe.FindAllStringSubmatch(string(body), -1) {
		num := match[1]                     // "001"
		fixType := match[2]                 // "SECURITY" or "RELIABILITY"
		date := strings.TrimSpace(match[3]) // "October 14, 2024"
		rawDesc := match[4]                 // HTML fragment

		desc := tagRe.ReplaceAllString(rawDesc, " ")
		desc = strings.ReplaceAll(desc, "&nbsp;", " ")
		desc = strings.ReplaceAll(desc, "&amp;", "&")
		desc = strings.ReplaceAll(desc, "&lt;", "<")
		desc = strings.ReplaceAll(desc, "&gt;", ">")
		desc = strings.ReplaceAll(desc, "&#39;", "'")
		desc = strings.ReplaceAll(desc, "&quot;", `"`)
		desc = spaceRe.ReplaceAllString(desc, " ")
		desc = strings.TrimSpace(desc)

		result[num] = fmt.Sprintf("%s FIX (%s): %s", fixType, date, desc)
	}
	return result
}

// splitOpenBSDPackage splits an OpenBSD package token "pkgname-version" into its
// name and version components.  The version always starts with a digit.
//
// Examples:
//
//	"bash-5.2.15"           -> ("bash", "5.2.15")
//	"py3-setuptools-68.0.0" -> ("py3-setuptools", "68.0.0")
func splitOpenBSDPackage(s string) (name, version string) {
	for i := len(s) - 1; i >= 1; i-- {
		if s[i] == '-' && s[i+1] >= '0' && s[i+1] <= '9' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
