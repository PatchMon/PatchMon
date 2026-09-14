//go:build windows

package commands

import "syscall"

const createNewProcessGroup = 0x00000200 // CREATE_NEW_PROCESS_GROUP

// sysProcAttrForDetach returns SysProcAttr that creates a new process group so
// that the child PowerShell script is not killed when the parent agent exits.
func sysProcAttrForDetach() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
	}
}
