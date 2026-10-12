// Package system provides system-level operations including reboot functionality
package system

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"patchmon-agent/internal/logutil"
	"patchmon-agent/internal/winexec"
)

// CheckRebootRequired checks if the system requires a reboot
// Returns (needsReboot bool, reason string)
func (d *Detector) CheckRebootRequired() (bool, string) {
	// Windows: check registry keys and CBS reboot-pending (per UsoClient/WUA docs)
	if runtime.GOOS == "windows" {
		return d.checkWindowsRebootRequired()
	}

	runningKernel := d.getRunningKernel()
	latestKernel := d.latestInstalledKernel(runningKernel)
	kernelStale := kernelIsOutdated(runningKernel, latestKernel)

	// Check Debian/Ubuntu - reboot-required flag file
	if _, err := os.Stat("/var/run/reboot-required"); err == nil {
		d.logger.Debug("Reboot required: /var/run/reboot-required file exists")
		reason := "Reboot flag file exists (/var/run/reboot-required)"
		if kernelStale {
			reason += fmt.Sprintf(" | Running kernel: %s, Installed kernel: %s", runningKernel, latestKernel)
		}
		return true, reason
	}

	// Check RHEL/Fedora - needs-restarting utility
	if needsRestart, reason := d.checkNeedsRestarting(); needsRestart {
		d.logger.WithField("reason", reason).Debug("Reboot required: needs-restarting check")
		if kernelStale {
			reason += fmt.Sprintf(" | Running kernel: %s, Installed kernel: %s", runningKernel, latestKernel)
		}
		return true, reason
	}

	// Universal kernel check - a reboot is only needed when the running kernel is
	// genuinely older than the newest installed kernel of the same line. Comparing
	// the two as raw strings reports a reboot for any formatting difference, and
	// never clears once the host has actually rebooted.
	if kernelStale {
		d.logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
			"running": runningKernel,
			"latest":  latestKernel,
		})).Debug("Reboot required: kernel version mismatch")
		reason := fmt.Sprintf("Kernel version mismatch | Running kernel: %s, Installed kernel: %s", runningKernel, latestKernel)
		return true, reason
	}

	d.logger.Debug("No reboot required")
	return false, ""
}

// kernelIsOutdated reports whether the running kernel is older than the newest
// installed one. Equal or newer means no reboot is pending.
func kernelIsOutdated(running, latest string) bool {
	if running == "" || latest == "" {
		return false
	}
	return compareKernelVersions(running, latest) < 0
}

// checkWindowsRebootRequired checks if Windows requires a reboot (per UsoClient/WUA docs)
// Checks: Windows Update RebootRequired and Component Based Servicing reboot-pending.
//
// PendingFileRenameOperations is deliberately not consulted. Ordinary application
// updaters write to that key during routine self-updates, so treating it as a
// reboot signal marks healthy hosts as needing a reboot indefinitely.
func (d *Detector) checkWindowsRebootRequired() (bool, string) {
	psScript := `
$ErrorActionPreference = "SilentlyContinue"
$reasons = @()

# Windows Update RebootRequired
$wu = Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired" -ErrorAction SilentlyContinue
if ($wu) { $reasons += "Windows Update requires reboot" }

# Component Based Servicing (CBS) reboot pending
$cbs = Get-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending" -ErrorAction SilentlyContinue
if ($cbs) { $reasons += "Component Based Servicing reboot pending" }

if ($reasons.Count -gt 0) {
  Write-Output ("REBOOT_REQUIRED:" + ($reasons -join "; "))
} else {
  Write-Output "REBOOT_NOT_REQUIRED"
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", winexec.Script(psScript))
	output, err := cmd.Output()
	if err != nil {
		d.logger.WithError(err).Debug("Windows reboot check failed")
		return false, ""
	}
	out := strings.TrimSpace(string(output))
	if strings.HasPrefix(out, "REBOOT_REQUIRED:") {
		reason := strings.TrimPrefix(out, "REBOOT_REQUIRED:")
		d.logger.WithField("reason", reason).Debug("Windows reboot required")
		return true, reason
	}
	d.logger.Debug("Windows: no reboot required")
	return false, ""
}

// checkNeedsRestarting checks using needs-restarting command (RHEL/Fedora)
func (d *Detector) checkNeedsRestarting() (bool, string) {
	// Check if needs-restarting command exists
	if _, err := exec.LookPath("needs-restarting"); err != nil {
		d.logger.Debug("needs-restarting command not found, skipping check")
		return false, ""
	}

	cmd := exec.Command("needs-restarting", "-r")
	if err := cmd.Run(); err != nil {
		// Exit code != 0 means reboot is needed
		if _, ok := err.(*exec.ExitError); ok {
			return true, "needs-restarting indicates reboot needed"
		}
		d.logger.WithError(err).Debug("needs-restarting command failed")
	}

	return false, ""
}

// getRunningKernel gets the currently running kernel version
func (d *Detector) getRunningKernel() string {
	// Windows has no uname and no kernel-package model, so asking would fork a
	// process per report only to fail and warn.
	if runtime.GOOS == "windows" {
		return ""
	}

	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get running kernel version")
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetLatestInstalledKernel gets the latest installed kernel version (public method)
func (d *Detector) GetLatestInstalledKernel() string {
	return d.latestInstalledKernel(d.getRunningKernel())
}

// latestInstalledKernel returns the newest installed kernel that belongs to the
// same line as the running one. Sources are tried in order of authority; the
// first that yields a usable candidate wins.
func (d *Detector) latestInstalledKernel(runningKernel string) string {
	flavour := kernelFlavour(runningKernel)

	sources := []struct {
		name    string
		collect func() []string
	}{
		{"boot", d.collectKernelsFromBoot},
		{"rpm", d.collectKernelsFromRPM},
		{"dpkg", d.collectKernelsFromDpkg},
		{"modules", d.collectKernelsFromModules},
	}

	for _, source := range sources {
		if latest := d.selectLatestKernel(source.collect(), flavour); latest != "" {
			d.logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"source": source.name,
				"latest": latest,
			})).Debug("Resolved latest installed kernel")
			return latest
		}
	}

	d.logger.Debug("Could not determine latest installed kernel")
	return ""
}

// selectLatestKernel narrows candidates to the given flavour and returns the
// highest version among them.
//
// Filtering by flavour is what keeps hosts with several kernel lines installed
// side by side honest. A Raspberry Pi ships both the 2712 and v8 builds, and
// neither is an upgrade of the other, so a 2712 host must only ever be compared
// against 2712 kernels. If nothing matches the running flavour the whole set is
// considered, which keeps behaviour sane when the running kernel is unknown.
func (d *Detector) selectLatestKernel(candidates []string, flavour string) string {
	usable := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if hasVersionCore(candidate) {
			usable = append(usable, candidate)
		}
	}

	if len(usable) == 0 {
		return ""
	}

	matching := usable
	if flavour != "" {
		matching = make([]string, 0, len(usable))
		for _, candidate := range usable {
			if kernelFlavour(candidate) == flavour {
				matching = append(matching, candidate)
			}
		}
		if len(matching) == 0 {
			d.logger.WithFields(logutil.SanitizeMap(map[string]interface{}{
				"flavour":    flavour,
				"candidates": usable,
			})).Debug("No installed kernel matches the running line, comparing against all")
			matching = usable
		}
	}

	sort.Slice(matching, func(i, j int) bool {
		return compareKernelVersions(matching[i], matching[j]) < 0
	})

	return matching[len(matching)-1]
}

// collectKernelsFromBoot scans /boot for vmlinuz files
func (d *Detector) collectKernelsFromBoot() []string {
	entries, err := os.ReadDir("/boot")
	if err != nil {
		d.logger.WithError(err).Debug("Failed to read /boot directory")
		return nil
	}

	var kernels []string
	for _, entry := range entries {
		name := entry.Name()
		// Look for vmlinuz-* files
		if strings.HasPrefix(name, "vmlinuz-") {
			version := strings.TrimPrefix(name, "vmlinuz-")
			// Skip recovery kernels but keep generic kernels
			if strings.Contains(version, "recovery") {
				continue
			}
			kernels = append(kernels, version)
		}
	}

	return kernels
}

// collectKernelsFromModules lists /lib/modules, whose directory names are always
// full kernel release strings. This is the fallback for distributions that install
// an unversioned image such as Arch's /boot/vmlinuz-linux, where no version can be
// recovered from the boot directory at all.
func (d *Detector) collectKernelsFromModules() []string {
	entries, err := os.ReadDir("/lib/modules")
	if err != nil {
		d.logger.WithError(err).Debug("Failed to read /lib/modules directory")
		return nil
	}

	var kernels []string
	for _, entry := range entries {
		if entry.IsDir() {
			kernels = append(kernels, entry.Name())
		}
	}

	return kernels
}

// kernelCoreRe matches a leading numeric MAJOR.MINOR version.
var kernelCoreRe = regexp.MustCompile(`^\d+\.\d+`)

// hasVersionCore reports whether a release string carries a numeric version at
// all. Arch installs /boot/vmlinuz-linux, which yields the "version" linux and
// can never match a running release such as 6.12.4-arch1-1.
func hasVersionCore(release string) bool {
	return kernelCoreRe.MatchString(release)
}

// kernelFlavour returns a key identifying which kernel line a release belongs
// to: amd64, pve, generic, rpt-rpi for the Pi 5 build, arch or zen.
//
// It keeps the components that name the line and discards those that only carry
// a revision, so that releases from one line always agree. Purely numeric
// components are revisions (the 1 in deb13.1, the pkgrel in arch1-1), and a
// trailing number on a label marks a point release (el9_4 and el9_5 are both
// el), so both are dropped. A release with no labels at all yields an empty
// flavour, which means the running line is unknown and every candidate counts.
func kernelFlavour(release string) string {
	var labels []string
	for _, part := range parseKernelVersion(release) {
		if _, err := strconv.Atoi(part); err == nil {
			continue
		}
		labels = append(labels, strings.TrimRight(part, "0123456789_"))
	}
	return strings.Join(labels, "-")
}

// compareKernelVersions compares two kernel version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
// Handles formats like "6.14.11-2-pve" and "6.8.12-9-pve"
func compareKernelVersions(v1, v2 string) int {
	parts1 := parseKernelVersion(v1)
	parts2 := parseKernelVersion(v2)

	// Compare each part
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 string
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}

		if p1 == p2 {
			continue
		}

		// A version that has run out of components ranks below one that has not.
		if p1 == "" {
			return -1
		}
		if p2 == "" {
			return 1
		}

		n1, err1 := strconv.Atoi(p1)
		n2, err2 := strconv.Atoi(p2)

		switch {
		case err1 == nil && err2 == nil:
			// Both are numbers
			if n1 < n2 {
				return -1
			}
			if n1 > n2 {
				return 1
			}
		case err1 == nil:
			// A numeric sub-revision outranks a label at the same position, so
			// 6.12.90+deb13.1-amd64 is newer than 6.12.90+deb13-amd64.
			return 1
		case err2 == nil:
			return -1
		default:
			// Both are labels
			if p1 < p2 {
				return -1
			}
			return 1
		}
	}

	return 0
}

// parseKernelVersion parses a kernel version string into comparable parts.
// Splitting on "+" as well as "." and "-" keeps the patch number separate from
// any distribution suffix, so 6.12.101+deb13-amd64 does not tokenise to
// "101+deb13", which compares as text and ranks below "96+deb13".
//
//	"6.14.11-2-pve"          -> ["6" "14" "11" "2" "pve"]
//	"6.12.90+deb13.1-amd64"  -> ["6" "12" "90" "deb13" "1" "amd64"]
func parseKernelVersion(version string) []string {
	return strings.FieldsFunc(version, func(r rune) bool {
		return r == '.' || r == '-' || r == '+'
	})
}

// collectKernelsFromRPM queries RPM for installed kernel packages
func (d *Detector) collectKernelsFromRPM() []string {
	// Check if rpm command exists
	if _, err := exec.LookPath("rpm"); err != nil {
		return nil
	}

	cmd := exec.Command("rpm", "-q", "kernel", "--last")
	output, err := cmd.Output()
	if err != nil {
		d.logger.WithError(err).Debug("Failed to query RPM for kernel packages")
		return nil
	}

	var kernels []string
	for _, line := range strings.Split(string(output), "\n") {
		// Format: kernel-VERSION DATE
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if version := strings.TrimPrefix(fields[0], "kernel-"); version != fields[0] {
			kernels = append(kernels, version)
		}
	}

	return kernels
}

// collectKernelsFromDpkg queries dpkg for installed kernel packages
func (d *Detector) collectKernelsFromDpkg() []string {
	// Check if dpkg command exists
	if _, err := exec.LookPath("dpkg"); err != nil {
		return nil
	}

	cmd := exec.Command("dpkg", "-l")
	output, err := cmd.Output()
	if err != nil {
		d.logger.WithError(err).Debug("Failed to query dpkg for kernel packages")
		return nil
	}

	var kernels []string
	metaPackages := make(map[string]bool)

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Look for installed kernel image packages
		if fields[0] == "ii" && strings.HasPrefix(fields[1], "linux-image-") {
			pkgName := fields[1]
			version := strings.TrimPrefix(pkgName, "linux-image-")
			// Ubuntu ships linux-image-unsigned-<release>-<flavour> alongside
			// the signed build; both name the same kernel.
			version = strings.TrimPrefix(version, "unsigned-")

			if isKernelMetaPackage(version) {
				metaPackages[pkgName] = true
			} else {
				// This is an actual kernel package with version
				kernels = append(kernels, version)
			}
		}
	}

	if len(kernels) > 0 {
		return kernels
	}

	// If we only found meta-packages, resolve dependencies to find actual kernels
	for metaPkg := range metaPackages {
		if actualVersion := d.resolveMetaPackage(metaPkg); actualVersion != "" {
			kernels = append(kernels, actualVersion)
		}
	}

	return kernels
}

// isKernelMetaPackage identifies meta-packages (generic, virtual, lowlatency,
// etc.) including generic-hwe and other generic-* patterns.
func isKernelMetaPackage(version string) bool {
	return version == "generic" || version == "virtual" || version == "lowlatency" ||
		version == "server" || version == "cloud" || version == "kvm" ||
		version == "generic-hwe" || strings.HasPrefix(version, "generic-")
}

// resolveMetaPackage resolves a meta-package (like linux-image-virtual) to the actual kernel version
func (d *Detector) resolveMetaPackage(metaPkg string) string {
	// Use dpkg-query to get the dependencies
	cmd := exec.Command("dpkg-query", "-W", "-f=${Depends}", metaPkg)
	output, err := cmd.Output()
	if err != nil {
		d.logger.WithError(err).Debug("Failed to query package dependencies")
		return ""
	}

	depends := string(output)

	// Parse dependencies to find linux-image-X.Y.Z-N-generic
	// Dependencies format: "package1 (>= version), package2, ..."
	parts := strings.Split(depends, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Remove version constraints like (>= 6.8.0.31.31)
		if idx := strings.Index(part, " ("); idx != -1 {
			part = part[:idx]
		}

		// Check if this is a linux-image package with actual version
		if strings.HasPrefix(part, "linux-image-") {
			version := strings.TrimPrefix(part, "linux-image-")

			// Skip if this is another meta-package
			if isKernelMetaPackage(version) {
				continue
			}

			// This should be an actual kernel version
			return version
		}
	}

	return ""
}

// ExecuteReboot schedules a system reboot. delayMinutes is clamped to [0, 60]
// to bound the operator surprise window. delayMinutes=0 reboots immediately.
// The shutdown command is fire-and-forget — we do not wait for it to complete
// because the process gets killed by the reboot itself. The function returns
// once shutdown has been scheduled (or returns the error from spawning it).
//
// On Linux/BSD it shells out to `shutdown -r +N` so that any running tty
// users see the broadcast wall and the system passes through normal
// signal-everyone -> stop-services -> reboot ordering. On Windows it uses
// `shutdown /r /t <seconds>`.
//
// Reason is appended to the broadcast so logs/wall messages explain why the
// host is rebooting. Empty reason falls back to a generic string.
func (d *Detector) ExecuteReboot(delayMinutes int, reason string) error {
	if delayMinutes < 0 {
		delayMinutes = 0
	}
	if delayMinutes > 60 {
		delayMinutes = 60
	}
	if strings.TrimSpace(reason) == "" {
		reason = "Triggered by PatchMon"
	}

	if runtime.GOOS == "windows" {
		// /r reboot, /t seconds, /c comment (max 512 chars; truncate defensively)
		comment := reason
		if len(comment) > 500 {
			comment = comment[:500]
		}
		cmd := exec.Command("shutdown", "/r", "/t", strconv.Itoa(delayMinutes*60), "/c", comment)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("shutdown /r failed to start: %w", err)
		}
		d.logger.WithField("delay_minutes", delayMinutes).WithField("reason", logutil.Sanitize(reason)).Info("Reboot scheduled via shutdown /r")
		return nil
	}

	// Linux/BSD: shutdown -r accepts "+N" minutes or "now".
	timeArg := "+" + strconv.Itoa(delayMinutes)
	if delayMinutes == 0 {
		timeArg = "now"
	}
	cmd := exec.Command("shutdown", "-r", timeArg, reason)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("shutdown -r %s failed to start: %w", timeArg, err)
	}
	d.logger.WithField("delay_minutes", delayMinutes).WithField("reason", logutil.Sanitize(reason)).Info("Reboot scheduled via shutdown -r")
	return nil
}
