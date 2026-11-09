package packages

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"patchmon-agent/pkg/models"
)

func validChocolateyTarget(id string) bool {
	return chocolateyIDPattern.MatchString(id) && !strings.EqualFold(id, "all")
}

type chocolateyBackend interface {
	Scan(context.Context) ([]models.Package, error)
	Upgrade(context.Context, string, string, func(string)) (uint32, error)
}

type chocolateyExecBackend struct{}

func (*chocolateyExecBackend) Scan(ctx context.Context) ([]models.Package, error) {
	exe, err := findChocolatey()
	if err != nil || exe == "" {
		return nil, err
	}
	return chocolateyPackages(chocolateyCommandRunner(ctx, exe))
}

func chocolateyUpgradeArgs(id, version string) []string {
	return []string{"upgrade", id, "--version=" + version, "--yes", "--no-progress", "--no-color", "--fail-on-not-installed", "--use-package-exit-codes", "--exit-when-reboot-detected", "--stop-on-first-failure"}
}

func (*chocolateyExecBackend) Upgrade(ctx context.Context, id, version string, log func(string)) (uint32, error) {
	if !validChocolateyTarget(id) || version == "" {
		return 0, errors.New("invalid Chocolatey upgrade target")
	}
	exe, err := findChocolatey()
	if err != nil {
		return 0, err
	}
	if exe == "" {
		return 0, errors.New("chocolatey is not installed")
	}
	cmd := exec.CommandContext(ctx, exe, chocolateyUpgradeArgs(id, version)...)
	hideWindow(cmd)
	cmd.WaitDelay = childWaitDelay
	lines := newLineLogger(log, 1<<20)
	w := &lockedWriter{w: lines}
	cmd.Stdout = w
	cmd.Stderr = w
	err = cmd.Run()
	lines.Flush()
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return uint32(exitErr.ExitCode()), nil
	}
	return 0, err
}

func chocolateyOutcome(code uint32) (status, message string, reboot bool) {
	switch code {
	case 0:
		return itemInstalled, "Upgraded", false
	case 2:
		return itemNotApplicable, "No upgrade performed", false
	case 1641, 3010:
		return itemPendingReboot, "Upgrade succeeded; restart required", true
	case 350:
		return itemSkipped, "Pending restart detected; no upgrade performed", true
	case 1604:
		return itemFailed, "Installation suspended; restart required", true
	default:
		return itemFailed, fmt.Sprintf("Chocolatey exited with code %d", int32(code)), false
	}
}

func findChocolateyPackage(pkgs []models.Package, id string) (models.Package, bool) {
	for _, p := range pkgs {
		if strings.EqualFold(p.ChocolateyID, id) {
			return p, true
		}
	}
	return models.Package{}, false
}

func (r *windowsPatchRun) runChocolateyPhase(runCtx, stopCtx context.Context, ids []string) {
	if r.chocolatey == nil {
		r.runErrors = append(r.runErrors, "Chocolatey backend is unavailable")
		return
	}
	scan := func() ([]models.Package, error) {
		ctx, cancel := context.WithTimeout(stopCtx, r.timeouts.scan)
		defer cancel()
		return r.chocolatey.Scan(ctx)
	}
	if r.req.PatchAll {
		pkgs, err := scan()
		if err != nil {
			r.runErrors = append(r.runErrors, "Chocolatey scan failed: "+err.Error())
			return
		}
		seen := map[string]bool{}
		for _, p := range pkgs {
			id := strings.ToLower(p.ChocolateyID)
			if p.NeedsUpdate && validChocolateyTarget(id) && !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	for _, id := range ids {
		it := r.upsert(models.PatchRunItem{Kind: WindowsTargetChocolatey, ID: id, Title: "chocolatey:" + id, Status: itemPending})
		r.post(it)
	}
	rebootPending := false
	for _, id := range ids {
		it := r.byKey[WindowsTargetChocolatey+"\x00"+id]
		if reason := r.interruptReason(runCtx); reason != "" {
			r.finish(it, itemSkipped, reason)
			continue
		}
		if rebootPending {
			r.finish(it, itemSkipped, "Restart required before continuing Chocolatey upgrades")
			continue
		}
		it.StartedAt = r.ts()
		pkgs, err := scan()
		if err != nil {
			r.finish(it, itemFailed, "Chocolatey scan failed: "+err.Error())
			continue
		}
		p, installed := findChocolateyPackage(pkgs, id)
		switch {
		case !installed:
			r.finish(it, itemNotApplicable, "Package is no longer installed")
		case p.ChocolateyPinned:
			r.finish(it, itemSkipped, "Pinned in Chocolatey; remove the pin to update it")
		case !p.NeedsUpdate:
			r.finish(it, itemNotApplicable, "No upgrade available")
		case r.req.DryRun:
			r.finish(it, itemWouldInstall, "Chocolatey upgrade available: "+p.AvailableVersion)
		default:
			if reason := r.interruptReason(runCtx); reason != "" {
				r.finish(it, itemSkipped, reason)
				continue
			}
			r.out("[Chocolatey] Upgrading %s to %s...\n", id, p.AvailableVersion)
			ctx, cancel := context.WithTimeout(runCtx, r.timeouts.wingetUpgrade)
			var code uint32
			r.withHeartbeat("[Chocolatey] Upgrading "+id, func() { code, err = r.chocolatey.Upgrade(ctx, id, p.AvailableVersion, r.log) })
			cancel()
			if err != nil {
				r.finish(it, itemFailed, "Chocolatey upgrade did not complete: "+err.Error())
				continue
			}
			ec := int64(int32(code))
			it.ExitCode = &ec
			status, message, reboot := chocolateyOutcome(code)
			it.RebootRequired = reboot
			rebootPending = reboot
			if status == itemInstalled || status == itemPendingReboot {
				// Exit 0 alone can also mean a pin was added after our scan. Verify the
				// selected package reached the requested version before claiming success.
				checkCtx, checkCancel := context.WithTimeout(runCtx, r.timeouts.scan)
				after, verifyErr := r.chocolatey.Scan(checkCtx)
				checkCancel()
				current, ok := findChocolateyPackage(after, id)
				if verifyErr != nil || !ok || current.CurrentVersion != p.AvailableVersion {
					status = itemFailed
					message = "Chocolatey exited successfully but the requested package version could not be verified"
				}
			}
			r.finish(it, status, message)
		}
		r.out("[Chocolatey] %s: %s\n", id, it.Message)
	}
}
