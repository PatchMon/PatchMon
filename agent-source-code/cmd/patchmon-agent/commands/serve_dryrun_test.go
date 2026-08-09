package commands

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDryRunOutputIndicatesError(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "yum declines and reports success",
			output: `Loaded plugins: extras_suggestions, langpacks, priorities, update-motd
239 packages excluded due to repository priority protections
Resolving Dependencies
--> Running transaction check
---> Package vim-common.x86_64 2:9.0.2153-1.amzn2.0.4 will be updated
---> Package vim-common.x86_64 2:9.0.2153-1.amzn2.0.6 will be an update
--> Finished Dependency Resolution

Dependencies Resolved

Transaction Summary
================================================================================
Upgrade  5 Packages

Total download size: 11 M
Exiting on user command
Your transaction was saved, rerun it with:
 yum load-transaction /tmp/yum_save_tx.yumtx`,
			want: false,
		},
		{
			name: "package named like an error is not an error",
			output: `Updating:
 libgpg-error       x86_64   1.42-1.el8      baseos      250 k
 perl-Error         noarch   1:0.17025-2     appstream    46 k
Operation aborted.`,
			want: false,
		},
		{
			name: "dnf declines and reports success",
			output: `Last metadata expiration check: 0:12:31 ago.
Dependencies resolved.
================================================================================
 Package        Arch     Version            Repository   Size
================================================================================
Upgrading:
 curl           x86_64   7.76.1-26.el9      baseos      301 k

Transaction Summary
Upgrade  1 Package

Operation aborted.`,
			want: false,
		},
		{
			name: "pacman print target list is not an error",
			output: `core/glibc 2.39-2
extra/vim 9.1.0-1`,
			want: false,
		},
		{
			name: "yum dependency failure",
			output: `--> Finished Dependency Resolution
Error: Package: vim-enhanced-2:9.0.2153-1.amzn2.0.6.x86_64 (amzn2-core)
           Requires: vim-common = 2:9.0.2153-1.amzn2.0.6
 You could try using --skip-broken to work around the problem`,
			want: true,
		},
		{
			name: "yum transaction check error",
			output: `Transaction Check Error:
  file /usr/bin/vim from install of vim conflicts with file from package vim-minimal`,
			want: true,
		},
		{
			name: "dnf problem block",
			output: `Error:
 Problem: package foo-1.0-1.el9.x86_64 requires bar >= 2.0, but none of the providers can be installed`,
			want: true,
		},
		{
			name:   "dnf no match for argument",
			output: "No match for argument: nosuchpackage\nError: Unable to find a match: nosuchpackage",
			want:   true,
		},
		{
			name:   "pacman target not found",
			output: "error: target not found: nosuchpackage",
			want:   true,
		},
		{
			name:   "pacman unresolvable conflicts",
			output: "error: unresolvable package conflicts detected\nerror: failed to prepare transaction (conflicting dependencies)",
			want:   true,
		},
		{
			name:   "pkg no packages available",
			output: "pkg: No packages available to install matching 'nosuchpackage' have been found in the repositories",
			want:   true,
		},
		{
			name: "pkg lists a package whose name ends in error",
			output: `Updating database digests format: 100%
The following 2 package(s) will be affected (of 0 checked):

Installed packages to be UPGRADED:
	libgpg-error: 1.51 -> 1.52
	curl: 8.11.0 -> 8.11.1

Number of packages to be upgraded: 2`,
			want: false,
		},
		{
			name: "dnf skips a broken package and resolves the rest",
			output: `Last metadata expiration check: 0:12:31 ago.
Problem: cannot install the best update candidate for package kernel-5.14.0-70.el9.x86_64
  - nothing provides kernel-core = 5.14.0-427 needed by kernel-5.14.0-427.el9.x86_64
Upgrading:
 curl        x86_64  7.76.1-26.el9   baseos  301 k
Skipping packages with broken dependencies:
 kernel      x86_64  5.14.0-427.el9  baseos  1.2 M

Transaction Summary
Upgrade  1 Package
Skip     1 Package

Operation aborted.`,
			want: false,
		},
		{
			name:   "dnf5 transaction failure",
			output: "Failed to resolve the transaction:\nProblem: package foo requires bar, but none of the providers can be installed",
			want:   true,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dryRunOutputIndicatesError(tt.output); got != tt.want {
				t.Fatalf("dryRunOutputIndicatesError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDryRunExit1Success(t *testing.T) {
	exit1 := func() error {
		err := exec.Command("sh", "-c", "exit 1").Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("expected an ExitError, got %v", err)
		}
		return err
	}()

	declined := "--> Finished Dependency Resolution\nExiting on user command"
	if !isDryRunExit1Success(exit1, declined) {
		t.Error("a declined yum dry run should count as success")
	}
	if isDryRunExit1Success(exit1, "Error: Package: foo requires bar") {
		t.Error("a genuine dependency failure should not count as success")
	}
	if isDryRunExit1Success(exit1, "") {
		t.Error("empty output should not count as success")
	}
	// runStreamingPatchStep joins its two captures with a newline, so a silent
	// command yields "\n" rather than "".
	if isDryRunExit1Success(exit1, "\n") {
		t.Error("a silent command should not count as success")
	}
	if isDryRunExit1Success(nil, declined) {
		t.Error("a nil error is not an exit-1 decline")
	}
}

// The sink interleaves stdout and stderr at byte boundaries, which splices a
// stderr diagnostic onto the tail of a partial stdout line. Classification must
// read the returned copy, where each stream is whole.
func TestRunStreamingPatchStepKeepsEachStreamWhole(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}

	var full strings.Builder
	sink := newStreamSink(nil, "test-run", &full)

	out, err := runStreamingPatchStep(context.Background(), sink, nil, "sh", "-c",
		`printf 'Total download size: 11 M'; printf 'Error: Package: foo requires bar\n' >&2; exit 1`)
	if err == nil {
		t.Fatal("expected the command to exit non-zero")
	}
	if !strings.Contains(out, "\nError: Package: foo requires bar") {
		t.Errorf("stderr diagnostic did not start a line in the returned output: %q", out)
	}
	if !dryRunOutputIndicatesError(out) {
		t.Errorf("genuine failure classified as success: %q", out)
	}
}

// A package manager can leave a grandchild holding the output pipes open long
// after it exits, for example an rpm scriptlet. The step must give up once
// WaitDelay has elapsed rather than waiting for the grandchild, otherwise Stop
// and the run timeout do nothing.
func TestRunStreamingPatchStepGivesUpOnAHeldPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}

	restore := patchStepWaitDelay
	patchStepWaitDelay = 500 * time.Millisecond
	t.Cleanup(func() { patchStepWaitDelay = restore })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var full strings.Builder
	sink := newStreamSink(nil, "test-run", &full)

	start := time.Now()
	out, err := runStreamingPatchStep(ctx, sink, nil, "sh", "-c",
		`sh -c 'sleep 5' & echo started; exit 0`)
	elapsed := time.Since(start)

	// Comfortably above the 500ms delay and below the grandchild's 5s, so this
	// fires if the step ever goes back to waiting for the pipe to close.
	if elapsed > 3*time.Second {
		t.Fatalf("step waited %v for a held pipe; the context and WaitDelay were ignored", elapsed)
	}
	// The command exited 0. Only its pipes outlived it, so the step succeeded.
	if err != nil {
		t.Errorf("a held pipe should not fail a command that exited cleanly: %v", err)
	}
	if !strings.Contains(out, "started") {
		t.Errorf("output written before the pipes were closed was lost: %q", out)
	}
	// The notice belongs in the terminal view, not in the copy callers parse.
	if !strings.Contains(full.String(), "still holding it open") {
		t.Errorf("truncation was not reported to the operator: %q", full.String())
	}
	if strings.Contains(out, "still holding it open") {
		t.Error("the notice leaked into the parsed output")
	}
}
