//go:build !windows

package packages

import (
	"errors"
	"os/exec"
)

var errNotWindows = errors.New("windows patching is not supported on this platform")

func hideWindow(*exec.Cmd) {}

func checkAuthenticode(string) error { return errNotWindows }
