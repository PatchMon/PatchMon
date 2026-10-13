//go:build !windows

package packages

import (
	"errors"
	"os/exec"
)

var (
	errWingetNotWindows     = errors.New("winget is only available on Windows")
	errPowerShellNotWindows = errors.New("PowerShell is only available on Windows")
)

func hideWindow(*exec.Cmd) {}

func checkWingetTrust(string) error { return errWingetNotWindows }

func powershellPath() (string, error) { return "", errPowerShellNotWindows }
