package packages

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
)

var (
	zypperOSReleasePath   = "/etc/os-release"
	zypperMountsPath      = "/proc/self/mounts"
	transactionalUpdateAt = "/usr/sbin/transactional-update"
)

// ZypperRolling reports whether the host is Tumbleweed or Slowroll, which are only
// supported when updated with zypper dist-upgrade.
func ZypperRolling() bool {
	f, err := os.Open(zypperOSReleasePath)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "ID="); ok {
			id := strings.Trim(v, `"'`)
			return id == "opensuse-tumbleweed" || id == "opensuse-slowroll"
		}
	}
	return false
}

// ZypperTransactional reports whether the root filesystem is read-only and updated
// through transactional-update, as on MicroOS, Leap Micro and SUSE Linux Micro.
func ZypperTransactional() bool {
	if _, err := os.Stat(transactionalUpdateAt); err != nil {
		return false
	}
	data, err := os.ReadFile(zypperMountsPath)
	if err != nil {
		return false
	}
	readOnly := false
	for line := range strings.Lines(string(data)) {
		f := strings.Fields(line)
		if len(f) >= 4 && f[1] == "/" {
			readOnly = slices.Contains(strings.Split(f[3], ","), "ro")
		}
	}
	return readOnly
}

func zypperGlobalArgs(xmlOut bool) []string {
	args := []string{"--non-interactive", "--non-interactive-include-reboot-patches", "--no-refresh"}
	if xmlOut {
		args = append(args, "--xmlout")
	}
	return args
}

// ZypperRefreshArgs refreshes repository metadata once at the start of a patch run.
func ZypperRefreshArgs() []string {
	return []string{"--non-interactive", "refresh"}
}

// ZypperTransactionArgs builds a patch run's zypper command. A nil targets list means
// Patch all: zypper patch on Leap and SLES, which updates the package manager first and
// exits 103, and dist-upgrade on rolling releases. Selected packages use update, which
// never installs a package that is not installed.
func ZypperTransactionArgs(rolling bool, targets []string, plan bool) []string {
	args := zypperGlobalArgs(plan)
	switch {
	case targets != nil:
		args = append(args, "update")
	case rolling:
		args = append(args, "dist-upgrade")
	default:
		args = append(args, "patch", "--with-update")
	}
	args = append(args, "--auto-agree-with-licenses")
	if plan {
		args = append(args, "--dry-run")
	}
	if targets == nil {
		return args
	}
	return append(append(args, "--"), targets...)
}

// ZypperPlan is the transaction a zypper --dry-run printed.
type ZypperPlan struct {
	Summary  ZypperInstallSummary
	Patches  []string
	Warnings []string
	Errors   []string
	// Problems are dependency problems zypper could not solve without a choice.
	Problems []string
	// Nothing is set when zypper said there was nothing to do.
	Nothing bool
}

// ParseZypperPlan reads the XML a zypper --xmlout --dry-run printed.
func ParseZypperPlan(out []byte) (ZypperPlan, error) {
	s, err := parseZypperStream(out)
	if err != nil {
		return ZypperPlan{}, err
	}
	plan := ZypperPlan{
		Warnings: s.messages("warning"),
		Errors:   s.messages("error"),
	}
	for _, pr := range s.Prompts {
		if d := strings.TrimSpace(pr.Description); strings.HasPrefix(d, "Problem") {
			plan.Problems = append(plan.Problems, d)
		}
	}
	if s.InstallSummary == nil {
		plan.Nothing = slices.ContainsFunc(s.messages("info"), func(m string) bool { return strings.Contains(m, "Nothing to do") })
		return plan, nil
	}
	plan.Summary = *s.InstallSummary
	plan.Nothing = plan.Summary.PackagesToChange == 0
	for _, sol := range slices.Concat(s.InstallSummary.Install, s.InstallSummary.Upgrade) {
		if sol.Type == "patch" {
			plan.Patches = append(plan.Patches, sol.Name)
		}
	}
	return plan, nil
}

func isZypperPackage(s ZypperSolvable) bool { return s.Type == "" || s.Type == "package" }

// Refused returns the steps PatchMon does not apply automatically: downgrades and
// architecture or vendor changes.
func (p ZypperPlan) Refused() []string {
	var out []string
	for _, s := range p.Summary.Downgrade {
		if isZypperPackage(s) {
			out = append(out, fmt.Sprintf("downgrade %s %s -> %s", s.Name, s.EditionOld, s.Edition))
		}
	}
	for _, s := range p.Summary.ChangeArch {
		if isZypperPackage(s) {
			out = append(out, fmt.Sprintf("change the architecture of %s from %s to %s", s.Name, s.ArchOld, s.Arch))
		}
	}
	for _, s := range p.Summary.ChangeVendor {
		if isZypperPackage(s) {
			out = append(out, "change the vendor of "+s.Name)
		}
	}
	return out
}

// Lines describes the planned package changes, one per line, in the form the server
// reads into packages_affected.
func (p ZypperPlan) Lines() []string {
	var out []string
	add := func(verb string, list []ZypperSolvable, withOld bool) {
		for _, s := range list {
			if !isZypperPackage(s) {
				continue
			}
			if withOld && s.EditionOld != "" {
				out = append(out, fmt.Sprintf("  %s %s %s -> %s", verb, s.Name, s.EditionOld, s.Edition))
			} else {
				out = append(out, fmt.Sprintf("  %s %s %s", verb, s.Name, s.Edition))
			}
		}
	}
	add("upgrade", p.Summary.Upgrade, true)
	add("install", p.Summary.Install, false)
	add("reinstall", p.Summary.Reinstall, false)
	add("remove", p.Summary.Remove, false)
	return out
}

// Removals returns the packages the plan removes.
func (p ZypperPlan) Removals() []string {
	var out []string
	for _, s := range p.Summary.Remove {
		if isZypperPackage(s) {
			out = append(out, s.Name)
		}
	}
	return out
}

// ZypperLockedTargets splits targets into those a zypper lock matches and the rest.
func ZypperLockedTargets(locks []ZypperLock, targets []string) (locked, free []string) {
	for _, t := range targets {
		if slices.ContainsFunc(locks, func(l ZypperLock) bool { return zypperLockMatches(l, t) }) {
			locked = append(locked, t)
		} else {
			free = append(free, t)
		}
	}
	return locked, free
}

func zypperLockMatches(l ZypperLock, name string) bool {
	if l.Type != "" && l.Type != "package" {
		return false
	}
	ok, err := path.Match(l.Name, name)
	return err == nil && ok
}
