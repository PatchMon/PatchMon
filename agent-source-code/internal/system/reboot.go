package system

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// CheckRebootRequired checks if the system requires a reboot
// Returns (needsReboot bool, reason string)
func (d *Detector) CheckRebootRequired() (bool, string) {
	runningKernel := d.getRunningKernel()
	latestKernel := d.getLatestInstalledKernel()

	// Check Debian/Ubuntu - reboot-required flag file
	if _, err := os.Stat("/var/run/reboot-required"); err == nil {
		d.logger.Debug("Reboot required: /var/run/reboot-required file exists")
		reason := "Reboot flag file exists (/var/run/reboot-required)"
		if runningKernel != latestKernel && latestKernel != "" {
			reason += fmt.Sprintf(" | Running kernel: %s, Installed kernel: %s", runningKernel, latestKernel)
		}
		return true, reason
	}

	// Check RHEL/Fedora - needs-restarting utility
	if needsRestart, reason := d.checkNeedsRestarting(); needsRestart {
		d.logger.WithField("reason", reason).Debug("Reboot required: needs-restarting check")
		if runningKernel != latestKernel && latestKernel != "" {
			reason += fmt.Sprintf(" | Running kernel: %s, Installed kernel: %s", runningKernel, latestKernel)
		}
		return true, reason
	}

	// Universal kernel check - compare running vs latest installed
	if runningKernel != latestKernel && latestKernel != "" {
		d.logger.WithFields(map[string]interface{}{
			"running": runningKernel,
			"latest":  latestKernel,
		}).Debug("Reboot required: kernel version mismatch")
		reason := fmt.Sprintf("Kernel version mismatch | Running kernel: %s, Installed kernel: %s", runningKernel, latestKernel)
		return true, reason
	}

	d.logger.Debug("No reboot required")
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
	return d.getLatestInstalledKernel()
}

// getLatestInstalledKernel gets the latest installed kernel version
func (d *Detector) getLatestInstalledKernel() string {
	// Try different methods based on common distro patterns

	// Method 1: Debian/Ubuntu - check /boot for vmlinuz files
	if latest := d.getLatestKernelFromBoot(); latest != "" {
		return latest
	}

	// Method 2: RHEL/Fedora - use rpm to query installed kernels
	if latest := d.getLatestKernelFromRPM(); latest != "" {
		return latest
	}

	// Method 3: Try dpkg for Debian-based systems
	if latest := d.getLatestKernelFromDpkg(); latest != "" {
		return latest
	}

	d.logger.Debug("Could not determine latest installed kernel")
	return ""
}

// getLatestKernelFromBoot scans /boot for vmlinuz files
func (d *Detector) getLatestKernelFromBoot() string {
	entries, err := os.ReadDir("/boot")
	if err != nil {
		d.logger.WithError(err).Debug("Failed to read /boot directory")
		return ""
	}

	var kernels []string
	for _, entry := range entries {
		name := entry.Name()
		// Look for vmlinuz-* files
		if strings.HasPrefix(name, "vmlinuz-") {
			version := strings.TrimPrefix(name, "vmlinuz-")
			// Skip generic/recovery kernels
			if strings.Contains(version, "generic") || strings.Contains(version, "recovery") {
				continue
			}
			kernels = append(kernels, version)
		}
	}

	if len(kernels) == 0 {
		return ""
	}

	// Sort kernels by version and return the latest
	sort.Slice(kernels, func(i, j int) bool {
		return compareKernelVersions(kernels[i], kernels[j]) < 0
	})

	return kernels[len(kernels)-1]
}

// compareKernelVersions compares two kernel version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
// Handles formats like "6.14.11-2-pve" and "6.8.12-9-pve"
func compareKernelVersions(v1, v2 string) int {
	// Split version into parts: "6.14.11-2-pve" -> ["6", "14", "11", "2", "pve"]
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

		// Try to compare as numbers first
		n1, err1 := strconv.Atoi(p1)
		n2, err2 := strconv.Atoi(p2)

		if err1 == nil && err2 == nil {
			// Both are numbers
			if n1 < n2 {
				return -1
			}
			if n1 > n2 {
				return 1
			}
		} else {
			// At least one is not a number, compare as strings
			if p1 < p2 {
				return -1
			}
			if p1 > p2 {
				return 1
			}
		}
	}

	return 0
}

// parseKernelVersion parses a kernel version string into comparable parts
// "6.14.11-2-pve" -> ["6", "14", "11", "2", "pve"]
func parseKernelVersion(version string) []string {
	// Replace dots and dashes with spaces, then split
	version = strings.ReplaceAll(version, ".", " ")
	version = strings.ReplaceAll(version, "-", " ")
	parts := strings.Fields(version)
	return parts
}

// getLatestKernelFromRPM queries RPM for installed kernel packages
func (d *Detector) getLatestKernelFromRPM() string {
	// Check if rpm command exists
	if _, err := exec.LookPath("rpm"); err != nil {
		return ""
	}

	cmd := exec.Command("rpm", "-q", "kernel", "--last")
	output, err := cmd.Output()
	if err != nil {
		d.logger.WithError(err).Debug("Failed to query RPM for kernel packages")
		return ""
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 && lines[0] != "" {
		// Parse first line which should be the latest kernel
		// Format: kernel-VERSION DATE
		parts := strings.Fields(lines[0])
		if len(parts) > 0 {
			// Extract version from kernel-X.Y.Z
			kernelPkg := parts[0]
			version := strings.TrimPrefix(kernelPkg, "kernel-")
			return version
		}
	}

	return ""
}

// getLatestKernelFromDpkg queries dpkg for installed kernel packages
func (d *Detector) getLatestKernelFromDpkg() string {
	// Check if dpkg command exists
	if _, err := exec.LookPath("dpkg"); err != nil {
		return ""
	}

	cmd := exec.Command("dpkg", "-l")
	output, err := cmd.Output()
	if err != nil {
		d.logger.WithError(err).Debug("Failed to query dpkg for kernel packages")
		return ""
	}

	var latestVersion string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Look for installed kernel image packages
		if fields[0] == "ii" && strings.HasPrefix(fields[1], "linux-image-") {
			// Extract version from package name
			// Format: linux-image-VERSION or linux-image-X.Y.Z-N-generic
			pkgName := fields[1]
			version := strings.TrimPrefix(pkgName, "linux-image-")

			// Skip meta packages
			if version == "generic" || version == "lowlatency" {
				continue
			}

			latestVersion = version
		}
	}

	return latestVersion
}
