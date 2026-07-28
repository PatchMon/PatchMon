package packages

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"patchmon-agent/internal/logutil"
)

// maxStderrInError caps how much of a failed command's stderr is folded into
// the returned error. Package managers can emit very large diagnostics (full
// dependency resolution traces); the first part is what identifies the fault.
const maxStderrInError = 512

// commandError wraps a failed exec with the command's stderr.
//
// exec.Cmd.Output() captures stderr into ExitError.Stderr but the error string
// itself is only ever "exit status N". Without folding stderr in, an operator
// debugging a host that reports zero updates sees "exit status 100" and none of
// the actual diagnostic (held broken packages, missing repo metadata, an
// expired subscription entitlement).
//
// Stderr is host-controlled text that ends up in logs, so it is sanitised and
// truncated.
func commandError(name string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			if len(stderr) > maxStderrInError {
				stderr = stderr[:maxStderrInError] + "... (truncated)"
			}
			return fmt.Errorf("%s: %w: %s", name, err, logutil.Sanitize(stderr))
		}
	}
	return fmt.Errorf("%s: %w", name, err)
}

// isExitCode reports whether err is an ExitError carrying the given exit code.
// Used to distinguish a package manager's "success with information" exit codes
// from genuine failures.
func isExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == code
	}
	return false
}
