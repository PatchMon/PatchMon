//go:build darwin

package system

import (
	"os/exec"
	"strings"

	"patchmon-agent/internal/constants"
)

func (d *Detector) getDarwinOSInfo() (osType, osVersion string, err error) {
	osType = "macOS"

	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return osType, "Unknown", nil
	}

	osVersion = strings.TrimSpace(string(out))
	if osVersion == "" {
		osVersion = "Unknown"
	}

	return osType, osVersion, nil
}

func (d *Detector) checkDarwinRebootRequired() (bool, string) {
	if _, err := exec.LookPath("softwareupdate"); err != nil {
		return false, ""
	}

	out, err := exec.Command("softwareupdate", "--list", "--all").CombinedOutput()
	if err != nil && len(out) == 0 {
		return false, ""
	}

	text := strings.ToLower(string(out))
	if strings.Contains(text, "restart required") || strings.Contains(text, "restart is required") || strings.Contains(text, "restart now") {
		return true, "macOS software update requires restart"
	}
	if strings.Contains(text, "restart") && strings.Contains(text, "required") {
		return true, "macOS software update requires restart"
	}
	return false, ""
}

func (d *Detector) getDarwinKernelVersion() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		d.logger.WithError(err).Warn("Failed to get Darwin kernel version")
		return constants.ErrUnknownValue
	}
	return strings.TrimSpace(string(out))
}
