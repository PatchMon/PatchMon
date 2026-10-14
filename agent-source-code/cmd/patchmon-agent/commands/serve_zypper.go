package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"patchmon-agent/internal/logutil"
	"patchmon-agent/internal/packages"
)

const zypperMaxPasses = 3

// zypperPatchIO is what a zypper patch run needs from runPatch, kept narrow so tests can fake zypper.
type zypperPatchIO struct {
	run           func(args ...string) (string, error)
	plan          func(args ...string) ([]byte, error)
	note          func(msg string)
	locks         func() ([]packages.ZypperLock, error)
	rolling       bool
	transactional bool
}

// runPatchZypper holds the in-process zypper slot for the whole run, so a report collected
// meanwhile waits instead of failing on the zypp lock.
func runPatchZypper(ctx context.Context, sink *streamSink, env []string, patchType string, packageNames []string, dryRun bool) error {
	release, err := packages.AcquireZypper(ctx, func() {
		sink.WriteString("[patchmon] Waiting for another zypper operation by this agent to finish.\n")
		sink.Flush()
	})
	if err != nil {
		return err
	}
	defer release()

	env = append(env, "ZYPP_LOCK_TIMEOUT="+packages.ZypperLockWaitSeconds)
	err = runZypperPatch(zypperPatchIO{
		run: func(args ...string) (string, error) {
			sink.WriteString(formatCmd("zypper", args...))
			sink.Flush()
			return runStreamingPatchStep(ctx, sink, env, "zypper", args...)
		},
		plan: func(args ...string) ([]byte, error) {
			sink.WriteString(formatCmd("zypper", args...))
			sink.Flush()
			cmd := exec.CommandContext(ctx, "zypper", args...)
			cmd.Env = env
			cmd.WaitDelay = patchStepWaitDelay
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = sink
			err := cmd.Run()
			return stdout.Bytes(), err
		},
		note: func(msg string) {
			sink.WriteString("[patchmon] " + msg + "\n")
		},
		locks:         packages.ZypperLocks,
		rolling:       packages.ZypperRolling(),
		transactional: packages.ZypperTransactional(),
	}, patchType, packageNames, dryRun)
	if err != nil {
		logger.WithField("error", logutil.Sanitize(err.Error())).Warn("zypper patch run failed")
		sink.WriteString(fmt.Sprintf("\n[zypper error] %s\n", err.Error()))
		sink.Flush()
	}
	return err
}

func runZypperPatch(io zypperPatchIO, patchType string, packageNames []string, dryRun bool) error {
	if io.transactional {
		return errors.New("this host has a read-only root filesystem and is updated with transactional-update, which PatchMon does not run yet. Nothing was changed. Update it with transactional-update on the host")
	}
	if patchType != "patch_all" && len(packageNames) == 0 {
		return errors.New("package_names required for patch_package")
	}

	if out, err := io.run(packages.ZypperRefreshArgs()...); err != nil {
		if packages.ZypperExitCode(err) == packages.ZypperExitLocked {
			return zypperStepError("zypper refresh", out, err)
		}
		io.note("zypper could not refresh every repository; continuing with cached metadata.")
	}

	locks, err := io.locks()
	if err != nil {
		io.note("Could not read zypper locks: " + err.Error())
	}
	var targets []string
	if patchType == "patch_all" {
		if len(locks) > 0 {
			names := make([]string, len(locks))
			for i, l := range locks {
				names[i] = l.Name
			}
			io.note("Locked with zypper addlock and left alone: " + strings.Join(names, ", "))
		}
	} else {
		var locked []string
		locked, targets = packages.ZypperLockedTargets(locks, packageNames)
		if len(locked) > 0 {
			io.note("Locked with zypper addlock and left alone: " + strings.Join(locked, ", "))
		}
		if len(targets) == 0 {
			return errors.New("every selected package is locked with zypper addlock, so there is nothing to update. Remove the lock with zypper removelock to patch it")
		}
		if io.rolling {
			io.note("Rolling releases are only supported when fully updated with zypper dist-upgrade. Updating selected packages leaves a partial update; use Patch all to bring the whole system up to date.")
		}
	}

	for pass := 1; pass <= zypperMaxPasses; pass++ {
		out, planErr := io.plan(packages.ZypperTransactionArgs(io.rolling, targets, true)...)
		plan, parseErr := packages.ParseZypperPlan(out)
		code := packages.ZypperExitCode(planErr)
		switch code {
		case 0, packages.ZypperExitRebootNeeded, packages.ZypperExitRestartNeeded, packages.ZypperExitReposSkipped:
		default:
			if pass > 1 {
				io.note("zypper's own update was installed. The remaining updates were not.")
			}
			return zypperPlanError(plan, planErr)
		}
		if parseErr != nil {
			return fmt.Errorf("zypper --dry-run printed no plan PatchMon could read, so nothing was changed: %w", parseErr)
		}
		for _, w := range plan.Warnings {
			io.note("zypper warning: " + w)
		}
		if refused := plan.Refused(); len(refused) > 0 {
			return fmt.Errorf("zypper plans changes PatchMon does not apply automatically (%s), so the plan was not applied. Review it with zypper on the host", strings.Join(refused, "; "))
		}
		zypperNotePlan(io, plan, dryRun)
		stackFirst := code == packages.ZypperExitRestartNeeded

		if dryRun {
			if stackFirst {
				io.note("zypper has to update itself first. A real run installs that update, then plans and applies the remaining updates, so it changes more than this plan shows.")
			}
			return nil
		}
		if plan.Nothing {
			return nil
		}

		out2, err := io.run(packages.ZypperTransactionArgs(io.rolling, targets, false)...)
		switch packages.ZypperExitCode(err) {
		case 0:
			return nil
		case packages.ZypperExitRebootNeeded:
			io.note("zypper reports that the host needs a reboot to finish this update.")
			return nil
		case packages.ZypperExitReposSkipped:
			io.note("zypper skipped a repository it could not refresh, so updates from it were not applied.")
			return nil
		case packages.ZypperExitRestartNeeded:
			if pass < zypperMaxPasses {
				io.note("zypper updated itself. Running it again for the remaining updates.")
			}
			continue
		default:
			return zypperStepError("zypper", out2, err)
		}
	}
	return fmt.Errorf("zypper still asked to be run again after %d passes. Run zypper patch on the host to see why", zypperMaxPasses)
}

func zypperNotePlan(io zypperPatchIO, plan packages.ZypperPlan, dryRun bool) {
	if plan.Nothing {
		io.note("zypper has nothing to do.")
		return
	}
	lines := plan.Lines()
	io.note(fmt.Sprintf("zypper plans %d package changes:\n%s", len(lines), strings.Join(lines, "\n")))
	if len(plan.Patches) > 0 {
		io.note("Patches: " + strings.Join(plan.Patches, " "))
	}
	verb := "Removing"
	if dryRun {
		verb = "Would remove"
	}
	for _, name := range plan.Removals() {
		io.note(fmt.Sprintf("%s %s: zypper's plan removes it, usually because an update obsoletes or replaces it.", verb, name))
	}
}

func zypperPlanError(plan packages.ZypperPlan, err error) error {
	if len(plan.Problems) > 0 {
		problem := strings.Join(plan.Problems, "\n")
		if len(problem) > 2000 {
			problem = problem[:2000] + "..."
		}
		return fmt.Errorf("zypper cannot resolve the dependencies of this update without a choice PatchMon does not make for you, so it was not applied. Resolve it on the host with zypper, then retry. zypper says:\n%s", problem)
	}
	if len(plan.Errors) > 0 {
		return zypperStepError("zypper --dry-run", strings.Join(plan.Errors, "; "), err)
	}
	return zypperStepError("zypper --dry-run", "", err)
}

func zypperStepError(step, output string, err error) error {
	detail := ""
	if o := strings.TrimSpace(output); o != "" && len(o) < 500 {
		detail = ": " + o
	}
	switch packages.ZypperExitCode(err) {
	case packages.ZypperExitLocked:
		return fmt.Errorf("%s failed: another process, often PackageKit, held the zypp lock for more than %s seconds. Retry when it has finished: %w", step, packages.ZypperLockWaitSeconds, err)
	case packages.ZypperExitCapNotFound:
		return fmt.Errorf("%s failed: a selected package is not available from any enabled repository: %w%s", step, err, detail)
	case packages.ZypperExitRPMScript:
		return fmt.Errorf("%s: the packages were installed but an RPM script failed. Check the output above and run zypper verify on the host: %w", step, err)
	case 6:
		return fmt.Errorf("%s failed: no repositories are defined. On SLES, register the system with SUSEConnect: %w", step, err)
	case 8:
		return fmt.Errorf("%s failed: zypper could not commit the transaction. Check the output above: %w", step, err)
	}
	return fmt.Errorf("%s failed: %w%s", step, err, detail)
}
