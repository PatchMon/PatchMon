//go:build !windows

package packages

import (
	"errors"
	"os/exec"
)

var errWingetNotWindows = errors.New("winget is only available on Windows")

func hideWindow(*exec.Cmd) {}

func checkWingetTrust(string) error { return errWingetNotWindows }
