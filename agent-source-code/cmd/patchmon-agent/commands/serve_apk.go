package commands

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"patchmon-agent/internal/logutil"
	"patchmon-agent/internal/packages"
)

// apkPatchIO is what an apk patch run needs from runPatch, kept narrow so tests can fake apk.
type apkPatchIO struct {
	run       func(args ...string) (string, error)
	note      func(msg string)
	inventory func() (packages.APKInventory, error)
}

// runPatchAPK holds the in-process apk slot for the whole run, so a report collected
// meanwhile does not start its own apk update against the database lock.
func runPatchAPK(ctx context.Context, sink *streamSink, env []string, patchType string, packageNames []string, dryRun bool) error {
	if patchType != "patch_all" && len(packageNames) == 0 {
		return errors.New("package_names required for patch_package")
	}
	release, err := packages.AcquireAPK(ctx, func() {
		sink.WriteString("[patchmon] Waiting for another apk operation by this agent to finish.\n")
		sink.Flush()
	})
	if err != nil {
		return err
	}
	defer release()

	err = runAPKPatch(apkPatchIO{
		run: func(args ...string) (string, error) {
			sink.WriteString(formatCmd("apk", args...))
			sink.Flush()
			return runStreamingPatchStep(ctx, sink, env, "apk", args...)
		},
		note: func(msg string) {
			sink.WriteString("[patchmon] " + msg + "\n")
		},
		inventory: func() (packages.APKInventory, error) {
			return packages.LoadAPKInventory(ctx)
		},
	}, patchType, packageNames, dryRun)
	if err != nil {
		logger.WithField("error", logutil.Sanitize(err.Error())).Warn("apk patch run failed")
		sink.WriteString(fmt.Sprintf("\n[apk error] %s\n", err.Error()))
		sink.Flush()
	}
	return err
}

var apkUnavailableRe = regexp.MustCompile(`(^|\D)[1-9]\d* unavailable,`)

func runAPKPatch(io apkPatchIO, patchType string, packageNames []string, dryRun bool) error {
	var targets []string
	if patchType != "patch_all" {
		targets = packageNames
	}

	if out, err := io.run(packages.APKUpdateArgs()...); err != nil {
		return apkStepError("apk update", out, err)
	}

	inv, err := io.inventory()
	if err != nil {
		return fmt.Errorf("reading the apk package database failed: %w", err)
	}
	holds, conflicts := inv.Holds()
	if len(conflicts) > 0 {
		return errors.New(packages.APKConflictMessage(conflicts[0]))
	}
	upgradeTargets := targets
	if targets != nil {
		upgradeTargets = slices.DeleteFunc(inv.ExpandTargets(targets, holds), func(n string) bool {
			return !validAptPackagePattern.MatchString(n)
		})
	}

	plan, err := apkSimulate(io, upgradeTargets, false)
	if err != nil {
		return err
	}
	if plan.Unreliable {
		preArgs := packages.APKPreupgradeArgs(inv.Major)
		switch {
		case dryRun:
			io.note("apk-tools has an update of its own. A real run installs it first and plans again, so the result can differ slightly from this plan.")
		case preArgs == nil:
			return errors.New("could not determine apk-tools version, and apk-tools has an update of its own, so the plan cannot be trusted. Nothing was changed. Check that apk --version works on the host, then retry")
		default:
			io.note("apk-tools has an update of its own. Installing it first, then planning again.")
			if out, err := io.run(preArgs...); err != nil {
				return apkStepError("apk-tools upgrade", out, err)
			}
			if inv, err = io.inventory(); err != nil {
				return fmt.Errorf("reading the apk package database failed: %w", err)
			}
			holds, conflicts = inv.Holds()
			if len(conflicts) > 0 {
				return errors.New(packages.APKConflictMessageAfterPreupgrade(conflicts[0]))
			}
			if plan, err = apkSimulate(io, upgradeTargets, true); err != nil {
				return err
			}
		}
	}

	if refused := plan.Refused(inv.WorldNames); len(refused) > 0 {
		return apkRefusalError(refused)
	}
	verb := "Removing"
	if dryRun {
		verb = "Would remove"
	}
	for _, s := range plan.Orphans(inv.WorldNames) {
		io.note(fmt.Sprintf("%s %s %s: no longer required by any package and not in /etc/apk/world.", verb, s.Name, strings.Trim(s.Detail, "()")))
	}

	for _, h := range holds {
		if targets == nil || slices.Contains(targets, h.Name) {
			io.note("Held by /etc/apk/world: " + h.String())
		}
	}
	for _, msg := range inv.NotUpgraded(plan, holds, targets) {
		io.note(msg)
	}

	if dryRun {
		return nil
	}
	if out, err := io.run(packages.APKUpgradeArgs(false, upgradeTargets)...); err != nil {
		return apkStepError("apk upgrade", out, err)
	}
	return nil
}

func apkRefusalError(refused []packages.APKPlanStep) error {
	steps := make([]string, len(refused))
	seen := map[string]bool{}
	var causes []string
	for i, s := range refused {
		steps[i] = s.String()
		cause := ""
		switch s.Action {
		case "Downgrading":
			cause = "a downgrade usually means a repository was removed or changed in /etc/apk/repositories, or a package came from a tagged repository that is no longer configured"
		case "Purging":
			cause = "apk would remove a package listed in /etc/apk/world, usually because of a conflict (!name) or a changed constraint there, or the package was renamed or replaced by another"
		case "Replacing":
			cause = "a same-version replace means a repository now serves a different build of a version that is already installed"
		default:
			cause = "apk printed a step this agent does not recognise"
		}
		if !seen[cause] {
			seen[cause] = true
			causes = append(causes, cause)
		}
	}
	return fmt.Errorf("apk plans changes PatchMon does not apply automatically (%s), so the plan was not applied. %s. Review the plan with apk upgrade --simulate on the host",
		strings.Join(steps, "; "), strings.Join(causes, "; "))
}

func apkSimulate(io apkPatchIO, targets []string, preupgraded bool) (packages.APKPlan, error) {
	out, err := io.run(packages.APKUpgradeArgs(true, targets)...)
	if err == nil && dryRunOutputIndicatesError(out) {
		err = errors.New("apk reported an error")
	}
	if err != nil {
		if c, ok := packages.APKConflictFromError(out); ok {
			if preupgraded {
				return packages.APKPlan{}, errors.New(packages.APKConflictMessageAfterPreupgrade(c))
			}
			return packages.APKPlan{}, errors.New(packages.APKConflictMessage(c))
		}
		return packages.APKPlan{}, apkStepError("apk upgrade --simulate", out, err)
	}
	plan := packages.ParseAPKPlan(out)
	if len(plan.Steps) == 0 && !plan.Complete {
		return plan, errors.New("apk upgrade --simulate printed no plan and no OK line, so the result cannot be trusted")
	}
	return plan, nil
}

func apkStepError(step, output string, err error) error {
	lower := strings.ToLower(output)
	var exitErr *exec.ExitError
	switch {
	case strings.Contains(lower, "unable to lock database"):
		return fmt.Errorf("%s failed: another apk process held the package database for more than %s seconds, retry when it has finished: %w", step, packages.APKLockWaitSeconds, err)
	case strings.Contains(lower, "stale/unavailable repositories"), apkUnavailableRe.MatchString(lower):
		return fmt.Errorf("%s failed: a repository in /etc/apk/repositories could not be reached: %w", step, err)
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 99:
		return fmt.Errorf("%s failed: apk could not open its database: %w", step, err)
	}
	return fmt.Errorf("%s failed: %w", step, err)
}
