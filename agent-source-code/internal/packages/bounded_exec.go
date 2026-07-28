package packages

import (
	"context"
	"os/exec"
	"time"
)

// Timeouts for package-manager invocations made during inventory collection.
//
// None of these commands had any deadline. exec.Command with .Output() blocks
// for as long as the child runs, collectReportData joins its collector
// goroutines with an untimed wg.Wait(), and runCheckIn is called synchronously
// from the service loop's select. So a single blocked package manager took the
// whole agent down with it: no reporting, and no handling of report_now,
// run_patch, patch_run_stop or update_agent, with no self-recovery.
//
// The concrete trigger is routine. An operator running "dnf upgrade" by hand,
// or dnf-automatic firing, holds /var/lib/rpm/.rpm.lock; a second dnf prints
// "Waiting for process with pid NNNN to finish." and waits indefinitely rather
// than timing out. The same shape applies to apt's dpkg lock, "pkg audit -F"
// and "freebsd-update fetch" against a firewalled mirror.
const (
	// collectorTimeout bounds an ordinary local query (listing installed
	// packages, checking for updates against an existing cache).
	collectorTimeout = 2 * time.Minute

	// networkCollectorTimeout bounds a command that may legitimately reach the
	// network (cache refresh, vulnerability database fetch).
	networkCollectorTimeout = 5 * time.Minute

	// commandKillDelay is how long a process gets to exit after its context is
	// cancelled before it is killed outright. Without WaitDelay a child that
	// ignores the signal, or that leaves a grandchild holding the stdout pipe,
	// keeps .Output() blocked even though the context has expired.
	commandKillDelay = 10 * time.Second
)

// boundedCommand builds a package-manager command that cannot run forever.
//
// The returned cancel func MUST be deferred by the caller; it releases the
// context and, on the timeout path, is what stops the process.
func boundedCommand(timeout time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = commandKillDelay
	return cmd, cancel
}
