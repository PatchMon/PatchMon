package packages

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"patchmon-agent/internal/logutil"
)

const maxStderrInError = 512

var urlCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://)[^/\s@]+@`)

// Repo URLs in stderr routinely carry credentials in the baseurl.
// logutil.Sanitize escapes control characters; it does not redact.
func redactURLCredentials(s string) string {
	return urlCredentials.ReplaceAllString(s, "${1}[redacted]@")
}

// Output() captures stderr but the error string is only ever "exit status N",
// which is useless for diagnosing why a host reported zero updates.
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

// Distinguishes a package manager's "success with information" exit codes.
func isExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == code
	}
	return false
}
