//go:build windows

package commands

import "syscall"

// sysProcAttrForDetach returns nil on Windows (Setsid is Unix-only).
// Child process detachment is handled differently on Windows.
func sysProcAttrForDetach() *syscall.SysProcAttr {
	return nil
}
