package packages

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// APKLockWaitSeconds is how long a patch command waits for another apk to release the database.
const APKLockWaitSeconds = "30"

// APKPlanStep is one "(i/n) Verb name (versions)" line from apk upgrade.
type APKPlanStep struct {
	Action string
	Name   string
	Detail string
}

func (s APKPlanStep) String() string {
	return strings.TrimSpace(s.Action + " " + s.Name + " " + s.Detail)
}

// APKPlan is the transaction apk upgrade --simulate printed.
type APKPlan struct {
	Steps []APKPlanStep
	// Unreliable: apk-tools has an update, which a real run applies first and then replans.
	Unreliable bool
	// Complete is set by apk's closing "OK:" line, which it prints even when there is nothing to do.
	Complete bool
}

var apkPlanLineRe = regexp.MustCompile(`^\(\s*\d+/\d+\)\s+(Updating pinning|\S+)\s+(\S+)\s*(.*)$`)

// ParseAPKPlan reads apk upgrade output. v3 pads the step counter, as in "( 1/14)".
func ParseAPKPlan(output string) APKPlan {
	var plan APKPlan
	for line := range strings.Lines(output) {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(line, "OK:") {
			plan.Complete = true
			continue
		}
		if strings.HasPrefix(lower, "warning:") && strings.Contains(lower, "simulation") && strings.Contains(lower, "reliable") {
			plan.Unreliable = true
			continue
		}
		m := apkPlanLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		plan.Steps = append(plan.Steps, APKPlanStep{Action: m[1], Name: apkBaseName(m[2]), Detail: m[3]})
	}
	return plan
}

// Refused returns every step that is not an install, upgrade, reinstall or pinning change:
// downgrades, purges, same-version replaces and any verb this agent does not know.
func (p APKPlan) Refused() []APKPlanStep {
	var out []APKPlanStep
	for _, s := range p.Steps {
		switch s.Action {
		case "Installing", "Upgrading", "Reinstalling", "Updating pinning":
		default:
			out = append(out, s)
		}
	}
	return out
}

// Names returns the package names the plan touches.
func (p APKPlan) Names() map[string]bool {
	names := make(map[string]bool, len(p.Steps))
	for _, s := range p.Steps {
		names[s.Name] = true
	}
	return names
}

func apkBaseName(name string) string {
	if at := strings.IndexByte(name, '@'); at > 0 {
		return name[:at]
	}
	return name
}

var apkToolsVersionRe = regexp.MustCompile(`^apk-tools (\d+)\.`)

// APKToolsMajor returns 2 or 3 from apk --version output, or 0 when it cannot tell.
func APKToolsMajor(versionOutput string) int {
	m := apkToolsVersionRe.FindStringSubmatch(strings.TrimSpace(versionOutput))
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// APKPreupgradeFlag returns the flag that upgrades only apk-tools and its libraries.
func APKPreupgradeFlag(apkMajor int) string {
	switch apkMajor {
	case 2:
		return "--self-upgrade-only"
	case 3:
		return "--preupgrade-only"
	}
	return ""
}

// APKUpgradeArgs builds apk upgrade arguments. Named packages are upgraded with their
// dependencies and, unlike apk add -u, are not written into /etc/apk/world.
func APKUpgradeArgs(simulate bool, packageNames []string) []string {
	args := []string{"upgrade", "--no-progress"}
	if simulate {
		args = append(args, "--simulate")
	} else {
		args = append(args, "--wait", APKLockWaitSeconds)
	}
	return append(args, packageNames...)
}

// APKUpdateArgs refreshes the indexes, waiting for a concurrent apk like a real upgrade does.
func APKUpdateArgs() []string {
	return []string{"update", "--wait", APKLockWaitSeconds}
}

// APKPreupgradeArgs builds the step that brings apk-tools up to date first.
func APKPreupgradeArgs(apkMajor int) []string {
	flag := APKPreupgradeFlag(apkMajor)
	if flag == "" {
		return nil
	}
	return []string{"upgrade", "--no-progress", "--wait", APKLockWaitSeconds, flag}
}

// APKInventory is what a patch run needs to know before it plans.
type APKInventory struct {
	Major     int
	Installed map[string]string
	Available map[string]string
	Origin    map[string]string
	World     []APKWorldConstraint
}

// LoadAPKInventory reads the apk version, installed and upgradable packages, and world.
func LoadAPKInventory(ctx context.Context) (APKInventory, error) {
	inv := APKInventory{}
	if out, err := apkOutput(ctx, "--version"); err == nil {
		inv.Major = APKToolsMajor(out)
	}
	installedOut, err := apkOutput(ctx, "list", "--installed")
	if err != nil {
		return inv, commandError("apk list --installed", err)
	}
	inv.Installed = make(map[string]string)
	inv.Origin = make(map[string]string)
	for _, p := range parseAPKListLines(installedOut) {
		inv.Installed[p.Name] = p.Version
		inv.Origin[p.Name] = p.Origin
	}
	upgradableOut, err := apkOutput(ctx, "list", "--upgradable")
	if err != nil {
		return inv, commandError("apk list --upgradable", err)
	}
	inv.Available = make(map[string]string)
	for _, p := range parseAPKListLines(upgradableOut) {
		if p.From != "" {
			inv.Available[p.Name] = p.Version
			inv.Origin[p.Name] = p.Origin
		}
	}
	inv.World, err = ReadAPKWorld()
	return inv, err
}

func apkOutput(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, collectorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apk", args...)
	cmd.WaitDelay = commandKillDelay
	out, err := cmd.Output()
	return string(out), err
}

// Holds returns the world holds and conflicts among the upgradable packages.
func (inv APKInventory) Holds() (holds, conflicts []APKHold) {
	return apkWorldHolds(inv.World, inv.Installed, inv.Available, apkVersionComparer(inv.Major))
}

// ExpandTargets adds upgradable packages built from the same origin as a target: apk
// upgrade libssl3 alone plans nothing, as the new libssl3 needs the new libcrypto3.
func (inv APKInventory) ExpandTargets(targets []string, holds []APKHold) []string {
	origins := map[string]bool{}
	for _, t := range targets {
		if o := inv.Origin[t]; o != "" {
			origins[o] = true
		}
	}
	held := map[string]bool{}
	for _, h := range holds {
		held[h.Name] = true
	}
	out := slices.Clone(targets)
	var extra []string
	for name := range inv.Available {
		if origins[inv.Origin[name]] && !held[name] && !slices.Contains(out, name) {
			extra = append(extra, name)
		}
	}
	slices.Sort(extra)
	return append(out, extra...)
}

// NotUpgraded describes the upgradable packages, limited to targets when given, that the
// plan leaves alone, apart from the world holds themselves.
func (inv APKInventory) NotUpgraded(plan APKPlan, holds []APKHold, targets []string) []string {
	planned := plan.Names()
	heldOrigin := map[string]string{}
	direct := map[string]bool{}
	for _, h := range holds {
		direct[h.Name] = true
		if o := inv.Origin[h.Name]; o != "" {
			heldOrigin[o] = h.Name
		}
	}
	var names []string
	for name := range inv.Available {
		if planned[name] || direct[name] || (targets != nil && !slices.Contains(targets, name)) {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]string, 0, len(names))
	for _, name := range names {
		msg := fmt.Sprintf("Not upgraded by apk: %s %s (%s available)", name, inv.Installed[name], inv.Available[name])
		switch {
		case heldOrigin[inv.Origin[name]] != "":
			msg += ", held back with " + heldOrigin[inv.Origin[name]] + " by /etc/apk/world"
		case targets != nil:
			msg += ". Its new version needs other packages upgraded too; select those as well or use Patch all"
		}
		out = append(out, msg)
	}
	return out
}

// APKConflictMessage explains a world entry the installed package already breaks.
// apk refuses every transaction until it is fixed, and apk fix fails the same way.
func APKConflictMessage(c APKHold) string {
	suggestion := c.Name + "=" + c.Installed
	return fmt.Sprintf("/etc/apk/world requires %s but %s %s is installed, so apk refuses every upgrade. "+
		"Correct that line in /etc/apk/world (for example change it to %s, or remove the version constraint), then retry. Nothing was changed.",
		c.Constraint, c.Name, c.Installed, suggestion)
}

var apkBreaksWorldRe = regexp.MustCompile(`(?m)^\s*(\S+):\s*\n\s*breaks: world\[([^\]]+)\]`)

// APKConflictFromError recovers the conflict from apk's own "breaks: world[...]" error,
// for the case the pre-check could not see.
func APKConflictFromError(output string) (APKHold, bool) {
	m := apkBreaksWorldRe.FindStringSubmatch(output)
	if m == nil {
		return APKHold{}, false
	}
	name, version := splitAPKNameVersion(m[1])
	if name == "" {
		return APKHold{}, false
	}
	return APKHold{Name: name, Installed: version, Constraint: m[2]}, true
}
