//go:build linux

package commands

import "syscall"

// sysProcAttrForDetach returns SysProcAttr to detach a child process (new session).
// Linux-only: Setsid creates a new session so the child survives parent exit.
func sysProcAttrForDetach() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
