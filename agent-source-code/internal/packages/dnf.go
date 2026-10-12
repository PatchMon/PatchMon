package packages

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// Matches RHSA-2025:1234, FEDORA-2026-044e56d9ff, FEDORA-EPEL-2024-0d2c3ad7a1,
// ALAS2-2023-1957 and ALAS2KERNEL-5.10-2024-055.
var advisoryIDRe = regexp.MustCompile(`^[A-Z][A-Z0-9]+(?:-[A-Z0-9][A-Z0-9.]*)*-\d{4}[:-][0-9a-fA-F]+`)

var updateinfoPackageRe = regexp.MustCompile(`^\S+-(\d+:)?[^-\s]+-[^-\s]+\.[A-Za-z0-9_]+$`)

// DNFManager handles dnf/yum package information collection
type DNFManager struct {
	logger *logrus.Logger
}

// NewDNFManager creates a new DNF package manager
func NewDNFManager(logger *logrus.Logger) *DNFManager {
	return &DNFManager{
		logger: logger,
	}
}

// Only these are stripped. Splitting on a dot unconditionally would mangle
// real names that contain one, e.g. python3.11.
var rpmArchSuffixes = map[string]bool{
	"x86_64":      true,
	"x86_64_v2":   true,
	"i686":        true,
	"i586":        true,
	"i386":        true,
	"noarch":      true,
	"aarch64":     true,
	"arm64":       true,
	"armv7hl":     true,
	"ppc64le":     true,
	"ppc64":       true,
	"s390x":       true,
	"riscv64":     true,
	"loongarch64": true,
	"src":         true,
}

// An RPM EVR is "[epoch:]version-release": it always starts with a digit
// (epoch or version) and always carries a release separated by a hyphen.
// Both conditions together are enough to tell a version column apart from
// an English word.
func looksLikeRPMVersion(s string) bool {
	s = strings.TrimPrefix(s, "*") // yum marks obsoleting packages with a leading *
	return startsWithVersion(s) && strings.Contains(s, "-")
}

// Both parsers must agree: CombinePackageData keys its upgradable set on
// Package.Name.
func stripRPMArchSuffix(name string) string {
	idx := strings.LastIndex(name, ".")
	if idx <= 0 {
		return name
	}
	if rpmArchSuffixes[name[idx+1:]] {
		return name[:idx]
	}
	return name
}

// detectPackageManager detects whether to use dnf or yum
func (m *DNFManager) detectPackageManager() string {
	// Prefer dnf over yum for modern RHEL-based systems
	packageManager := "dnf"
	if _, err := exec.LookPath("dnf"); err != nil {
		// Fall back to yum if dnf is not available (legacy systems)
		packageManager = "yum"
	}
	return packageManager
}

// GetPackages gets package information for RHEL-based systems
func (m *DNFManager) GetPackages() ([]models.Package, error) {
	// Determine package manager
	packageManager := m.detectPackageManager()

	m.logger.WithField("manager", packageManager).Debug("Using package manager")

	// Note: We don't run 'makecache' because:
	// 1. It causes delays on systems without internet (tries to reach remote repos)
	// 2. It's not needed for listing installed packages
	// 3. The 'check-update' command already refreshes metadata when needed
	// 4. Fedora's cache issue (if any) is resolved by using proper update checks

	// Get installed packages
	// Note: yum (CentOS 7 / legacy) uses positional argument syntax: "yum list installed"
	// while dnf uses flag syntax: "dnf list --installed"
	m.logger.Debug("Getting installed packages...")
	var listCmd *exec.Cmd
	var cancelList context.CancelFunc
	if packageManager == "yum" {
		listCmd, cancelList = boundedCommand(collectorTimeout, packageManager, "list", "installed")
	} else {
		listCmd, cancelList = boundedCommand(collectorTimeout, packageManager, "list", "--installed")
	}
	defer cancelList()
	listCmd.Env = messagesCEnv(os.Environ())
	installedOutput, err := listCmd.Output()
	if err != nil {
		// An empty inventory reads as "fully patched" in the UI. Fail instead.
		return nil, commandError(packageManager+" list installed", err)
	}
	m.logger.WithField("outputSize", len(installedOutput)).Debug("Received output from list installed command")
	m.logger.Debug("Parsing installed packages...")
	installedPackages := m.parseInstalledPackages(string(installedOutput))
	m.logger.WithField("count", len(installedPackages)).Info("Found installed packages")

	if len(installedPackages) == 0 {
		m.logger.Warn("No installed packages found - this may indicate a parsing issue")
	}

	// Get security updates first to identify which packages are security updates
	m.logger.Debug("Getting security updates...")
	securityPackages := m.getSecurityPackages(packageManager)
	m.logger.WithField("count", len(securityPackages)).Debug("Found security packages")

	// Get upgradable packages
	m.logger.Debug("Getting upgradable packages...")
	checkCmd, cancelCheck := boundedCommand(networkCollectorTimeout, packageManager, "check-update")
	defer cancelCheck()
	checkCmd.Env = messagesCEnv(os.Environ())
	checkOutput, checkErr := checkCmd.Output()
	if checkErr != nil {
		// check-update exit codes: 0 none, 100 updates available, anything
		// else a real failure.
		if !isExitCode(checkErr, 100) {
			return nil, commandError(packageManager+" check-update", checkErr)
		}
	}

	var upgradablePackages []models.Package
	if len(checkOutput) > 0 {
		m.logger.Debug("Parsing DNF/yum check-update output...")
		upgradablePackages = m.parseUpgradablePackages(string(checkOutput), packageManager, installedPackages, securityPackages)
		m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")
	} else {
		m.logger.Debug("No updates available")
		upgradablePackages = []models.Package{}
	}

	// Merge and deduplicate packages (pass full installed packages to preserve descriptions)
	packages := CombinePackageData(installedPackages, upgradablePackages)

	// Enrich packages with repository attribution
	m.enrichWithRepoAttribution(packages)

	m.logger.WithFields(logrus.Fields{
		"total":             len(packages),
		"installed":         len(installedPackages),
		"upgradable":        len(upgradablePackages),
		"securityAvailable": len(securityPackages),
	}).Info("Package collection completed")

	if len(packages) == 0 {
		m.logger.Error("WARNING: Returning 0 packages - this will show as empty in PatchMon UI")
	}

	return packages, nil
}

// enrichWithRepoAttribution populates SourceRepository for each package by running
// repoquery to get the from_repo field for installed packages.
func (m *DNFManager) enrichWithRepoAttribution(packages []models.Package) {
	if len(packages) == 0 {
		return
	}

	packageManager := m.detectPackageManager()

	var cmd *exec.Cmd
	var cancel context.CancelFunc
	if packageManager == "dnf" {
		cmd, cancel = boundedCommand(collectorTimeout, "dnf", "repoquery", "--installed", "--cacheonly", "--qf", "%{name}\t%{from_repo}")
	} else {
		// yum: try repoquery from yum-utils
		if _, err := exec.LookPath("repoquery"); err == nil {
			cmd, cancel = boundedCommand(collectorTimeout, "repoquery", "--installed", "--qf", "%{name}\t%{ui_from_repo}")
		} else {
			// Try yum repoquery (available on some systems)
			cmd, cancel = boundedCommand(collectorTimeout, "yum", "repoquery", "--installed", "--qf", "%{name}\t%{ui_from_repo}")
		}
	}
	defer cancel()
	cmd.Env = messagesCEnv(os.Environ())

	output, err := cmd.Output()
	if err != nil {
		m.logger.WithError(err).Warn("repoquery failed, skipping repo attribution")
		return
	}

	// Parse tab-separated output: name -> from_repo
	repoByName := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[0]
		repo := parts[1]

		// Normalise unknown values
		repo = strings.TrimPrefix(repo, "@") // yum sometimes prefixes with @
		if repo == "" || repo == "<unknown>" || repo == "@@commandline" || repo == "commandline" {
			repo = "unknown"
		}

		repoByName[name] = repo
	}

	// Apply to packages
	attributed := 0
	for i := range packages {
		// Try exact match, then try stripping arch suffix from package name
		name := packages[i].Name
		if repo, ok := repoByName[name]; ok {
			packages[i].SourceRepository = repo
			attributed++
			continue
		}
		// Strip arch suffix (e.g. "glibc.x86_64" -> "glibc")
		if baseName := stripRPMArchSuffix(name); baseName != name {
			if repo, ok := repoByName[baseName]; ok {
				packages[i].SourceRepository = repo
				attributed++
			}
		}
	}

	m.logger.WithField("attributed", attributed).Debug("Enriched packages with repository attribution")
}

// getSecurityPackages gets the list of security packages from dnf/yum updateinfo
func (m *DNFManager) getSecurityPackages(packageManager string) map[string]bool {
	securityPackages := make(map[string]bool)

	// Scoped so each variant releases its own context: a function-level defer
	// would bind to the first cancel func only.
	runUpdateInfo := func(filter string) ([]byte, error) {
		cmd, cancel := boundedCommand(networkCollectorTimeout, packageManager, "updateinfo", "list", filter)
		defer cancel()
		cmd.Env = messagesCEnv(os.Environ())
		return cmd.Output()
	}

	// dnf5 reads a positional "security" as an advisory name and exits 0 with nothing.
	var updateInfoOutput []byte
	var err error
	for _, filter := range []string{"--security", "security", "sec"} {
		if updateInfoOutput, err = runUpdateInfo(filter); err == nil {
			break
		}
	}

	if err != nil {
		m.logger.WithError(err).Debug("Failed to get security updates, will not mark packages as security updates")
		return securityPackages
	}

	securityPackages = m.parseSecurityPackages(updateInfoOutput)
	m.logger.WithField("count", len(securityPackages)).Info("Security packages identified")
	return securityPackages
}

func (m *DNFManager) parseSecurityPackages(updateInfoOutput []byte) map[string]bool {
	securityPackages := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(updateInfoOutput)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip header lines and empty lines
		if line == "" || strings.Contains(line, "Last metadata") ||
			strings.Contains(line, "expiration") || strings.HasPrefix(line, "Loading") {
			continue
		}

		fields := slices.Collect(strings.FieldsSeq(line))
		if len(fields) < 3 {
			continue
		}

		// Match the advisory ID by shape rather than by a list of known
		// vendor prefixes, so a new distribution does not silently report
		// zero security updates. Filters out header lines like "expiration".
		advisoryID := fields[0]
		if !advisoryIDRe.MatchString(advisoryID) {
			continue
		}

		// dnf4/yum: ID Severity/Sec. NEVRA. dnf5: ID Type Severity NEVRA Issued.
		i := slices.IndexFunc(fields[1:], updateinfoPackageRe.MatchString)
		if i < 0 {
			continue
		}
		basePackageName := m.extractBasePackageName(fields[i+1])

		if basePackageName != "" {
			securityPackages[basePackageName] = true
			m.logger.WithFields(logrus.Fields{
				"advisory": advisoryID,
				"package":  basePackageName,
			}).Debug("Detected security package")
		}
	}

	return securityPackages
}

// yum 3 runs on Python 2 and crashes on non-ASCII metadata under a C LC_CTYPE, so only
// LC_MESSAGES is forced; LC_ALL would override it and is folded into LC_CTYPE instead.
func messagesCEnv(src []string) []string {
	env := make([]string, 0, len(src)+2)
	var lcAll string
	for _, kv := range src {
		if v, ok := strings.CutPrefix(kv, "LC_ALL="); ok {
			lcAll = v
		}
	}
	for _, kv := range src {
		switch {
		case strings.HasPrefix(kv, "LC_ALL="),
			strings.HasPrefix(kv, "LANGUAGE="),
			strings.HasPrefix(kv, "LC_MESSAGES="),
			lcAll != "" && strings.HasPrefix(kv, "LC_CTYPE="):
			continue
		}
		env = append(env, kv)
	}
	if lcAll != "" {
		env = append(env, "LC_CTYPE="+lcAll)
	}
	return append(env, "LC_MESSAGES=C")
}

// extractBasePackageName returns the name from name-[epoch:]version-release[.arch]. RPM forbids
// dashes in version and release, so split from the right; names may contain "-<digit>".
func (m *DNFManager) extractBasePackageName(packageString string) string {
	nvr := stripRPMArchSuffix(packageString)
	rel := strings.LastIndex(nvr, "-")
	if rel <= 0 {
		return nvr
	}
	ver := strings.LastIndex(nvr[:rel], "-")
	if ver <= 0 || !startsWithVersion(nvr[ver+1:rel]) {
		return nvr
	}
	return nvr[:ver]
}

func startsWithVersion(s string) bool {
	if i := strings.Index(s, ":"); i > 0 && strings.Trim(s[:i], "0123456789") == "" {
		s = s[i+1:]
	}
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

// parseUpgradablePackages parses dnf/yum check-update output
func (m *DNFManager) parseUpgradablePackages(output string, packageManager string, installedPackages map[string]models.Package, securityPackages map[string]bool) []models.Package {
	var packages []models.Package

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip header lines and empty lines
		if line == "" || strings.Contains(line, "Loaded plugins") ||
			strings.Contains(line, "Last metadata") || strings.HasPrefix(line, "Loading") {
			continue
		}

		fields := slices.Collect(strings.FieldsSeq(line))
		if len(fields) < 3 {
			continue
		}

		// A real row is "name.arch  epoch:version-release  repo". Banner lines
		// such as "Updating Subscription Management repositories." also have
		// three or more fields, so the column has to be validated by shape or
		// they are ingested as packages.
		if !looksLikeRPMVersion(fields[1]) {
			continue
		}

		packageName := stripRPMArchSuffix(fields[0])
		availableVersion := fields[1]

		// Get current version from installed packages map (already collected).
		var currentVersion string
		if p, ok := installedPackages[packageName]; ok {
			currentVersion = p.CurrentVersion
		}

		// Tolerate an installed map that still holds arch-qualified keys.
		if currentVersion == "" {
			for installedName, p := range installedPackages {
				if stripRPMArchSuffix(installedName) == packageName {
					currentVersion = p.CurrentVersion
					break
				}
			}
		}

		// If still not found in installed packages, try to get it with a command as fallback
		if currentVersion == "" {
			// yum (CentOS 7 / legacy) requires positional argument; dnf accepts --installed flag
			var getCurrentCmd *exec.Cmd
			var cancelCurrent context.CancelFunc
			if packageManager == "yum" {
				getCurrentCmd, cancelCurrent = boundedCommand(collectorTimeout, packageManager, "list", "installed", packageName)
			} else {
				getCurrentCmd, cancelCurrent = boundedCommand(collectorTimeout, packageManager, "list", "--installed", packageName)
			}
			getCurrentCmd.Env = messagesCEnv(os.Environ())
			getCurrentOutput, err := getCurrentCmd.Output()
			// Released immediately rather than deferred: this runs once per
			// upgradable package, so a deferred cancel would accumulate.
			cancelCurrent()
			if err == nil {
				for _, currentLine := range strings.Split(string(getCurrentOutput), "\n") {
					if strings.Contains(currentLine, packageName) && !strings.Contains(currentLine, "Installed") && !strings.Contains(currentLine, "Available") {
						currentFields := slices.Collect(strings.FieldsSeq(currentLine))
						if len(currentFields) >= 2 {
							currentVersion = currentFields[1]
							break
						}
					}
				}
			}
		}

		// Only add package if we have both current and available versions
		// This prevents empty currentVersion errors on the server
		if packageName != "" && currentVersion != "" && availableVersion != "" {
			isSecurityUpdate := securityPackages[packageName]

			packages = append(packages, models.Package{
				Name:             packageName,
				CurrentVersion:   currentVersion,
				AvailableVersion: availableVersion,
				NeedsUpdate:      true,
				IsSecurityUpdate: isSecurityUpdate,
			})
		} else {
			m.logger.WithFields(logrus.Fields{
				"package":          packageName,
				"currentVersion":   currentVersion,
				"availableVersion": availableVersion,
			}).Debug("Skipping package due to missing version information")
		}
	}

	return packages
}

// parseInstalledPackages parses dnf/yum list installed output.
// On CentOS 7 / legacy yum, long package names are wrapped: the name appears alone
// on one line and the version + repo follow on the next indented line. We handle
// both the single-line and wrapped formats.
func (m *DNFManager) parseInstalledPackages(output string) map[string]models.Package {
	installedPackages := make(map[string]models.Package)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var pendingName string // holds a wrapped package name waiting for its version line
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "Installed Packages") ||
			strings.HasPrefix(trimmed, "Available Packages") ||
			strings.HasPrefix(trimmed, "Loaded plugins") {
			continue
		}

		parts := strings.Fields(trimmed)

		// Normal single-line format: "name.arch  version  repo"
		if len(parts) >= 3 && looksLikeRPMVersion(parts[1]) {
			packageName := stripRPMArchSuffix(parts[0])
			version := parts[1]
			installedPackages[packageName] = models.Package{
				Name:           packageName,
				CurrentVersion: version,
				NeedsUpdate:    false,
			}
			pendingName = ""
			continue
		}

		// Wrapped format, line 1: just the package name (no version/repo yet).
		// Detect by checking the original line starts without leading whitespace
		// and the trimmed text has no spaces (single token).
		if len(parts) == 1 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			// Looks like a bare package name line - remember it
			pendingName = stripRPMArchSuffix(parts[0])
			continue
		}

		// Wrapped format, line 2: "  version  repo" (starts with whitespace)
		if pendingName != "" && len(parts) >= 2 &&
			(strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			version := parts[0]
			installedPackages[pendingName] = models.Package{
				Name:           pendingName,
				CurrentVersion: version,
				NeedsUpdate:    false,
			}
			pendingName = ""
			continue
		}

		// Any other short line resets pending state
		pendingName = ""
	}

	return installedPackages
}
