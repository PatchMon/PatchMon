package packages

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"patchmon-agent/internal/logutil"
)

// maxStderrInError caps how much of a failed command's stderr is folded into
// the returned error. Package managers can emit very large diagnostics (full
// dependency resolution traces); the first part is what identifies the fault.
const maxStderrInError = 512

// urlCredentials matches the userinfo component of a URL, i.e. the
// "user:password@" between the scheme and the host.
var urlCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)[^/\s@]+@`)

// redactURLCredentials removes embedded credentials from URL-shaped substrings.
//
// Package managers print failing repository URLs to stderr, and private
// repositories routinely carry credentials in the baseurl:
// https://user:token@repo.example.com/... is the normal shape for RHEL
// Satellite and for most vendor-hosted repos. Folding stderr into the returned
// error puts those in the agent log at error level.
//
// logutil.Sanitize does not help here: it escapes control characters to prevent
// log injection, it does not redact secrets.
func redactURLCredentials(s string) string {
	return urlCredentials.ReplaceAllString(s, "${1}[redacted]@")
}

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
			return fmt.Errorf("%s: %w: %s", name, err, logutil.Sanitize(redactURLCredentials(stderr)))
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
