package packages

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

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

// rpmArchSuffixes are the architecture tokens dnf/yum append to package names
// in "name.arch" form. Only these are stripped when normalising a name.
//
// Splitting on a dot unconditionally is wrong: plenty of real RPM names contain
// dots (python3.11, python3.12), and strings.Split(name, ".")[0] turns
// "python3.11.x86_64" into "python3", silently merging two distinct packages.
var rpmArchSuffixes = map[string]bool{
	"x86_64":  true,
	"i686":    true,
	"i586":    true,
	"i386":    true,
	"noarch":  true,
	"aarch64": true,
	"arm64":   true,
	"armv7hl": true,
	"ppc64le": true,
	"ppc64":   true,
	"s390x":   true,
	"riscv64": true,
	"src":     true,
}

// stripRPMArchSuffix removes a trailing ".<arch>" when <arch> is a recognised
// RPM architecture token, and leaves every other name untouched.
//
// Both the installed and the upgradable parser must agree on this, because
// CombinePackageData keys its upgradable set on Package.Name. When the two
// disagreed, every upgradable package was emitted twice: once arch-qualified
// and flagged NeedsUpdate, and once bare and flagged up to date.
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
	// OPTIMIZATION: Set minimal environment to reduce overhead
	listCmd.Env = append(os.Environ(), "LANG=C")
	installedOutput, err := listCmd.Output()
	if err != nil {
		// Do not fall back to an empty inventory: that renders the host as
		// having no packages and nothing pending, which reads as "fully
		// patched" on the dashboard. Fail the report instead.
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
	// check-update contacts the configured repositories, so it gets the longer
	// budget. This is the command that hangs indefinitely behind an operator's
	// own dnf holding the RPM lock.
	checkCmd, cancelCheck := boundedCommand(networkCollectorTimeout, packageManager, "check-update")
	defer cancelCheck()
	checkOutput, checkErr := checkCmd.Output()
	if checkErr != nil {
		// check-update signals its result through the exit code:
		//   0   - no updates available
		//   100 - updates are available (stdout carries the list)
		//   1   - a real failure (unreachable mirror, GPG error, expired
		//         subscription entitlement, no enabled repositories)
		//
		// Only 100 is a success for us. Previously the error was discarded
		// entirely, so an exit-1 failure produced empty stdout and fell through
		// to "No updates available" at Debug level, reporting a host with
		// pending security updates as fully patched.
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
	cmd.Env = append(os.Environ(), "LANG=C")

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

	// runUpdateInfo runs one updateinfo variant and releases its context before
	// returning. Deliberately not `defer cancel()` at function scope: the two
	// variants below would then share one deferred call bound to the FIRST
	// cancel func (defer captures the value at defer time), leaving the second
	// command's context to run to its own timeout.
	runUpdateInfo := func(subcommand string) ([]byte, error) {
		cmd, cancel := boundedCommand(networkCollectorTimeout, packageManager, "updateinfo", "list", subcommand)
		defer cancel()
		return cmd.Output()
	}

	// Try dnf updateinfo list security (works for dnf)
	updateInfoOutput, err := runUpdateInfo("security")
	if err != nil {
		// Fall back to "sec" if "security" doesn't work
		updateInfoOutput, err = runUpdateInfo("sec")
	}

	if err != nil {
		m.logger.WithError(err).Debug("Failed to get security updates, will not mark packages as security updates")
		return securityPackages
	}

	// Parse the output to extract package names
	scanner := bufio.NewScanner(strings.NewReader(string(updateInfoOutput)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip header lines and empty lines
		if line == "" || strings.Contains(line, "Last metadata") ||
			strings.Contains(line, "expiration") || strings.HasPrefix(line, "Loading") {
			continue
		}

		// Format: ALSA-2025:11140 Moderate/Sec.  glib2-2.68.4-16.el9_6.2.x86_64
		// We need to extract the package name (3rd field) and get the base name
		fields := slices.Collect(strings.FieldsSeq(line))
		if len(fields) < 3 {
			continue
		}

		// Skip lines that don't start with advisory IDs
		// Common advisory ID prefixes: RHSA (Red Hat), ALSA (AlmaLinux), ELSA (Oracle/Enterprise Linux), CESA (CentOS)
		// This filters out header lines like "expiration"
		advisoryID := fields[0]
		isAdvisory := strings.HasPrefix(advisoryID, "RHSA") ||
			strings.HasPrefix(advisoryID, "ALSA") ||
			strings.HasPrefix(advisoryID, "ELSA") ||
			strings.HasPrefix(advisoryID, "CESA")

		if !isAdvisory {
			continue
		}

		// The package name is in the format: package-name-version-release.arch
		// We need to extract just the base package name
		packageNameWithVersion := fields[2]
		basePackageName := m.extractBasePackageName(packageNameWithVersion)

		if basePackageName != "" {
			securityPackages[basePackageName] = true
			m.logger.WithFields(logrus.Fields{
				"advisory": advisoryID,
				"package":  basePackageName,
			}).Debug("Detected security package")
		}
	}

	m.logger.WithField("count", len(securityPackages)).Info("Security packages identified")
	return securityPackages
}

// extractBasePackageName extracts the base package name from a package string
// Handles formats like:
// - package-name-version-release.arch (from updateinfo)
// - package-name.arch (from check-update)
func (m *DNFManager) extractBasePackageName(packageString string) string {
	// Remove architecture suffix first (e.g., .x86_64, .noarch)
	baseName := stripRPMArchSuffix(packageString)

	// If the base name contains a version pattern (starts with a digit after a dash),
	// extract just the package name part
	// Format: package-name-version-release
	// We look for the FIRST dash that's followed by a digit (version starts)
	// This handles packages with dashes in their names like "glibc-common-2.34-168.el9_6.19"
	for i := 0; i < len(baseName); i++ {
		if baseName[i] == '-' && i+1 < len(baseName) {
			nextChar := baseName[i+1]
			// Check if the next character is a digit (version starts)
			if nextChar >= '0' && nextChar <= '9' {
				// This is the start of version, return everything before this dash
				return baseName[:i]
			}
		}
	}

	return baseName
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

		// Normalise to the same shape parseInstalledPackages emits, so the two
		// halves of CombinePackageData agree and an upgradable package is not
		// also emitted as a separate "up to date" row under its bare name.
		packageName := stripRPMArchSuffix(fields[0])
		availableVersion := fields[1]

		// Get current version from installed packages map (already collected).
		// Both sides are arch-stripped now, so this is the ordinary path.
		var currentVersion string
		if p, ok := installedPackages[packageName]; ok {
			currentVersion = p.CurrentVersion
		}

		// Defensive fallback: tolerate an installed map that still holds
		// arch-qualified keys (e.g. supplied by a caller or an older cache).
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
			// Extract base package name to check against security packages
			basePackageName := m.extractBasePackageName(packageName)
			isSecurityUpdate := securityPackages[basePackageName]

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
		if len(parts) >= 3 {
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
