//go:build freebsd

package commands

import "syscall"

// sysProcAttrForDetach returns SysProcAttr to detach a child process (new session).
// FreeBSD: Setsid creates a new session so the child survives parent exit.
func sysProcAttrForDetach() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
