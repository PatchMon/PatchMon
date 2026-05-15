//go:build darwin

package commands

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// sysProcAttrForDetach returns SysProcAttr to detach a child process (new session).
// Darwin: Setsid creates a new session so the child survives parent exit.
func sysProcAttrForDetach() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// getConsoleUser returns the logged-in console user. Falls back to
// stat /dev/console so it works when running as a launchd service
// (where SUDO_USER is not set).
func getConsoleUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	out, err := exec.Command("stat", "-f", "%Su", "/dev/console").Output()
	if err == nil {
		if u := strings.TrimSpace(string(out)); u != "" && u != "root" {
			return u
		}
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "root"
}
