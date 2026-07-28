package packages

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestIsExitCode guards the dnf/yum check-update contract. Exit 100 means
// "updates are available" and is a success; treating it as a failure would make
// every host with pending updates fail its report, and treating exit 1 as a
// success is the original bug (a host with no reachable repositories was
// reported as fully patched).
func TestIsExitCode(t *testing.T) {
	t.Parallel()

	updatesAvailable := exec.Command("sh", "-c", "exit 100").Run()
	if !isExitCode(updatesAvailable, 100) {
		t.Fatalf("expected exit 100 to be recognised, got %v", updatesAvailable)
	}
	if isExitCode(updatesAvailable, 1) {
		t.Fatalf("exit 100 must not be mistaken for exit 1")
	}

	realFailure := exec.Command("sh", "-c", "exit 1").Run()
	if isExitCode(realFailure, 100) {
		t.Fatalf("exit 1 must not be mistaken for the updates-available signal")
	}

	if isExitCode(nil, 100) {
		t.Fatalf("a nil error carries no exit code")
	}
	if isExitCode(errors.New("not an exec error"), 100) {
		t.Fatalf("a non-exec error carries no exit code")
	}
}

// TestCommandError_IncludesStderr covers the diagnostic that was previously
// lost. Output() captures stderr into ExitError.Stderr, but the bare error
// string is only ever "exit status N", so an operator saw nothing explaining
// why a host reported zero updates.
func TestCommandError_IncludesStderr(t *testing.T) {
	t.Parallel()

	const diagnostic = "E: Unable to correct problems, you have held broken packages"
	_, err := exec.Command("sh", "-c", "echo '"+diagnostic+"' >&2; exit 100").Output()
	if err == nil {
		t.Fatalf("expected the command to fail")
	}

	got := commandError("apt upgrade simulation", err).Error()
	if !strings.Contains(got, "apt upgrade simulation") {
		t.Errorf("expected the command name in %q", got)
	}
	if !strings.Contains(got, "exit status 100") {
		t.Errorf("expected the wrapped exec error in %q", got)
	}
	if !strings.Contains(got, "held broken packages") {
		t.Errorf("expected stderr to be folded in, got %q", got)
	}
}

// TestCommandError_TruncatesLargeStderr keeps a pathological dependency-
// resolution trace out of the logs while retaining the leading diagnostic.
func TestCommandError_TruncatesLargeStderr(t *testing.T) {
	t.Parallel()

	_, err := exec.Command("sh", "-c", "head -c 4000 /dev/zero | tr '\\0' 'x' >&2; exit 1").Output()
	if err == nil {
		t.Fatalf("expected the command to fail")
	}

	got := commandError("dnf check-update", err).Error()
	if !strings.Contains(got, "(truncated)") {
		t.Errorf("expected oversized stderr to be truncated, got %d chars", len(got))
	}
	if len(got) > maxStderrInError+256 {
		t.Errorf("truncated error is still too large: %d chars", len(got))
	}
}

// TestCommandError_NoStderr keeps the message clean when the command produced
// no diagnostic at all.
func TestCommandError_NoStderr(t *testing.T) {
	t.Parallel()

	_, err := exec.Command("sh", "-c", "exit 3").Output()
	if err == nil {
		t.Fatalf("expected the command to fail")
	}

	got := commandError("apk list --installed", err).Error()
	if strings.HasSuffix(got, ": ") || strings.Contains(got, ":  ") {
		t.Errorf("expected no dangling separator when stderr is empty, got %q", got)
	}
	if !strings.Contains(got, "exit status 3") {
		t.Errorf("expected the exec error in %q", got)
	}
}
