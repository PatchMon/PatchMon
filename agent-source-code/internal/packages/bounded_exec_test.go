package packages

import (
	"errors"
	"os/exec"
	"testing"
	"time"
)

// TestBoundedCommand_KillsAHangingProcess is the regression guard for the
// wedged-agent defect.
func TestBoundedCommand_KillsAHangingProcess(t *testing.T) {
	t.Parallel()

	start := time.Now()
	cmd, cancel := boundedCommand(300*time.Millisecond, "sh", "-c", "sleep 30")
	defer cancel()

	err := cmd.Run()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a command exceeding its deadline must not succeed")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("command was not killed promptly: took %s", elapsed)
	}
}

// TestBoundedCommand_AllowsNormalCompletion guards against over-correcting.
func TestBoundedCommand_AllowsNormalCompletion(t *testing.T) {
	t.Parallel()

	cmd, cancel := boundedCommand(30*time.Second, "sh", "-c", "echo hello")
	defer cancel()

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("a fast command must succeed, got %v", err)
	}
	if got := string(out); got != "hello\n" {
		t.Fatalf("unexpected output %q", got)
	}
}

// TestBoundedCommand_PreservesExitCodeAndStderr confirms the deadline wrapper
// does not break the exit-code and stderr handling the collectors rely on, in
// particular dnf check-update's exit 100.
func TestBoundedCommand_PreservesExitCodeAndStderr(t *testing.T) {
	t.Parallel()

	cmd, cancel := boundedCommand(30*time.Second, "sh", "-c", "echo diagnostic >&2; exit 100")
	defer cancel()

	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected a non-zero exit to surface as an error")
	}
	if !isExitCode(err, 100) {
		t.Fatalf("exit code must survive the wrapper, got %v", err)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || len(exitErr.Stderr) == 0 {
		t.Fatal("stderr must still be captured for commandError to fold in")
	}
}

// TestBoundedCommand_WaitDelaySet ensures a child that ignores cancellation,
// or leaves a grandchild holding the stdout pipe, is still killed.
func TestBoundedCommand_WaitDelaySet(t *testing.T) {
	t.Parallel()

	cmd, cancel := boundedCommand(time.Second, "sh", "-c", "true")
	defer cancel()

	if cmd.WaitDelay <= 0 {
		t.Fatal("WaitDelay must be set, otherwise .Output() can block past the deadline")
	}
}
