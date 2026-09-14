package packages

import (
	"context"
	"os/exec"
	"time"
)

// A blocked package manager (dnf waiting on the RPM lock, freebsd-update on a
// firewalled mirror) otherwise wedges the whole agent: collection runs on the
// service-loop goroutine and joins with an untimed wg.Wait().
const (
	collectorTimeout        = 2 * time.Minute
	networkCollectorTimeout = 5 * time.Minute

	// Without WaitDelay a child that ignores cancellation, or leaves a
	// grandchild holding the stdout pipe, keeps .Output() blocked.
	commandKillDelay = 10 * time.Second
)

// The returned cancel MUST be called: it is what stops the process on timeout.
func boundedCommand(timeout time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = commandKillDelay
	return cmd, cancel
}
