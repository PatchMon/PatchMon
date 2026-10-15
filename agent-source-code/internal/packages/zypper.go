package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"patchmon-agent/internal/logutil"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// ZypperLockWaitSeconds is how long zypper waits for another process to release the zypp lock.
const ZypperLockWaitSeconds = "30"

// Zypper exit codes from zypper's src/main.h.
const (
	ZypperExitLocked        = 7
	ZypperExitRebootNeeded  = 102
	ZypperExitRestartNeeded = 103
	ZypperExitCapNotFound   = 104
	ZypperExitReposSkipped  = 106
	ZypperExitRPMScript     = 107
)

// ZypperManager handles zypper package information collection.
type ZypperManager struct {
	logger *logrus.Logger
}

// NewZypperManager creates a new zypper package manager.
func NewZypperManager(logger *logrus.Logger) *ZypperManager {
	return &ZypperManager{logger: logger}
}

var zypperBusy = make(chan struct{}, 1)

// AcquireZypper waits for any in-process zypper work to finish and keeps reports from
// starting another until release is called. onWait, if set, runs once when it has to wait.
func AcquireZypper(ctx context.Context, onWait func()) (release func(), err error) {
	var once sync.Once
	release = func() { once.Do(func() { <-zypperBusy }) }
	select {
	case zypperBusy <- struct{}{}:
		return release, nil
	default:
	}
	if onWait != nil {
		onWait()
	}
	select {
	case zypperBusy <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ZypperEnv returns the environment every zypper invocation runs with.
func ZypperEnv(src []string) []string {
	return append(messagesCEnv(src), "ZYPP_LOCK_TIMEOUT="+ZypperLockWaitSeconds)
}

// ZypperExitCode returns the exit status of a zypper run, 0 for success and -1 when it did not run.
func ZypperExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func zypperXML(timeout time.Duration, okCodes []int, args ...string) (zypperStream, error) {
	cmd, cancel := boundedCommand(timeout, "zypper", append([]string{"--non-interactive", "--xmlout"}, args...)...)
	defer cancel()
	cmd.Env = ZypperEnv(os.Environ())
	out, runErr := cmd.Output()
	stream, parseErr := parseZypperStream(out)
	name := "zypper " + strings.Join(args, " ")
	if code := ZypperExitCode(runErr); code != 0 && !slices.Contains(okCodes, code) {
		if msgs := stream.messages("error"); len(msgs) > 0 {
			return stream, fmt.Errorf("%s: %w: %s", name, runErr, logutil.Sanitize(redactURLCredentials(strings.Join(msgs, "; "))))
		}
		return stream, commandError(name, runErr)
	}
	if parseErr != nil {
		return stream, fmt.Errorf("%s: %w", name, parseErr)
	}
	return stream, nil
}

// GetPackages gets package information for zypper-based systems.
func (m *ZypperManager) GetPackages() ([]models.Package, error) {
	ctx, cancel := context.WithTimeout(context.Background(), networkCollectorTimeout)
	defer cancel()
	release, err := AcquireZypper(ctx, func() {
		m.logger.Info("Waiting for a zypper patch run by this agent to finish before collecting packages")
	})
	if err != nil {
		return nil, fmt.Errorf("a zypper patch run is still in progress: %w", err)
	}
	defer release()

	installed, err := m.installedPackages()
	if err != nil {
		return nil, err
	}

	lu, err := zypperXML(networkCollectorTimeout, []int{ZypperExitReposSkipped}, "list-updates")
	if err != nil {
		return nil, err
	}
	for _, w := range lu.messages("warning") {
		m.logger.WithField("message", logutil.Sanitize(w)).Warn("zypper warning")
	}
	upgradable := zypperUpgradablePackages(lu.Updates, installed)

	if len(upgradable) > 0 {
		fixes := m.securityFixes()
		for i, p := range upgradable {
			upgradable[i].IsSecurityUpdate = zypperFixesSecurity(fixes[p.Name], p.CurrentVersion, p.AvailableVersion)
		}
	}

	packages := CombinePackageData(installed, upgradable)
	m.logger.WithFields(logrus.Fields{
		"total":      len(packages),
		"installed":  len(installed),
		"upgradable": len(upgradable),
	}).Info("Package collection completed")
	return packages, nil
}

const rpmQueryFormat = "%{NAME}\\t%{EPOCH}\\t%{VERSION}\\t%{RELEASE}\\t%{INSTALLTIME}\\t%{SUMMARY}\\n"

func (m *ZypperManager) installedPackages() (map[string]models.Package, error) {
	cmd, cancel := boundedCommand(collectorTimeout, "rpm", "-qa", "--qf", rpmQueryFormat)
	defer cancel()
	cmd.Env = messagesCEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return nil, commandError("rpm -qa", err)
	}
	installed := parseRPMQueryOutput(string(out))
	if len(installed) == 0 {
		return nil, errors.New("rpm -qa listed no installed packages")
	}
	return installed, nil
}

// parseRPMQueryOutput keeps the most recently installed version when a name is
// installed more than once, as kernels are.
func parseRPMQueryOutput(output string) map[string]models.Package {
	installed := make(map[string]models.Package)
	installTime := make(map[string]int64)
	for line := range strings.Lines(output) {
		f := strings.SplitN(strings.TrimRight(line, "\r\n"), "\t", 6)
		if len(f) < 5 || f[0] == "" || f[0] == "gpg-pubkey" {
			continue
		}
		t, _ := strconv.ParseInt(f[4], 10, 64)
		if prev, seen := installTime[f[0]]; seen && prev > t {
			continue
		}
		summary := ""
		if len(f) == 6 {
			summary = f[5]
		}
		installTime[f[0]] = t
		installed[f[0]] = models.Package{
			Name:           f[0],
			Description:    summary,
			CurrentVersion: rpmEdition(f[1], f[2], f[3]),
		}
	}
	return installed
}

// rpmEdition formats an EVR the way zypper prints an edition.
func rpmEdition(epoch, version, release string) string {
	evr := version + "-" + release
	if epoch != "" && epoch != "(none)" && epoch != "0" {
		evr = epoch + ":" + evr
	}
	return evr
}

func zypperUpgradablePackages(updates []zypperUpdate, installed map[string]models.Package) []models.Package {
	out := make([]models.Package, 0, len(updates))
	for _, u := range updates {
		if u.Kind != "package" || u.Name == "" {
			continue
		}
		current := u.EditionOld
		if current == "" {
			if p, ok := installed[u.Name]; ok {
				current = p.CurrentVersion
			}
		}
		out = append(out, models.Package{
			Name:             u.Name,
			CurrentVersion:   current,
			AvailableVersion: u.Edition,
			NeedsUpdate:      true,
			SourceRepository: u.Source.Alias,
		})
	}
	return out
}

// securityFixes returns, per package name, the versions that needed security patches
// require. zypper names them only in the text of info -t patch, as "name.arch < edition"
// under Conflicts; a dry run of the patches cannot see past a pending update-stack patch.
func (m *ZypperManager) securityFixes() map[string][]string {
	lp, err := zypperXML(collectorTimeout, []int{ZypperExitReposSkipped}, "--no-refresh", "list-patches", "--category", "security")
	if err != nil {
		m.logger.WithError(err).Warn("zypper list-patches failed, security updates are not flagged")
		return nil
	}
	if lp.noUpdateRepositories() {
		m.logger.Info("zypper has no update repositories with patch data, security updates are not flagged")
		return nil
	}
	names := neededSecurityPatches(lp)
	if len(names) == 0 {
		return nil
	}
	fixes := make(map[string][]string)
	for batch := range slices.Chunk(names, 100) {
		cmd, cancel := boundedCommand(collectorTimeout, "zypper", append([]string{"--non-interactive", "--no-refresh", "info", "-t", "patch"}, batch...)...)
		cmd.Env = ZypperEnv(os.Environ())
		out, err := cmd.Output()
		cancel()
		if err != nil {
			m.logger.WithError(commandError("zypper info -t patch", err)).Warn("zypper info failed, security updates are not flagged")
			return nil
		}
		for name, eds := range parseZypperPatchConflicts(string(out)) {
			fixes[name] = append(fixes[name], eds...)
		}
	}
	m.logger.WithFields(logrus.Fields{"patches": len(names), "packages": len(fixes)}).Debug("zypper security patches")
	return fixes
}

// parseZypperPatchConflicts reads the Conflicts lists of zypper info -t patch.
func parseZypperPatchConflicts(out string) map[string][]string {
	fixes := make(map[string][]string)
	inConflicts := false
	for line := range strings.Lines(out) {
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "Conflicts") {
			inConflicts = true
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inConflicts = false
			continue
		}
		if !inConflicts {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 || f[1] != "<" {
			continue
		}
		name := stripRPMArchSuffix(f[0])
		if !slices.Contains(fixes[name], f[2]) {
			fixes[name] = append(fixes[name], f[2])
		}
	}
	return fixes
}

// zypperFixesSecurity reports whether moving from current to available crosses a
// version a needed security patch requires.
func zypperFixesSecurity(fixes []string, current, available string) bool {
	for _, fix := range fixes {
		if compareRPMEVR(current, fix) < 0 && compareRPMEVR(fix, available) <= 0 {
			return true
		}
	}
	return false
}

// ZypperLocks returns the zypper package locks, sorted by name.
func ZypperLocks() ([]ZypperLock, error) {
	s, err := zypperXML(collectorTimeout, nil, "--no-refresh", "locks")
	if err != nil {
		return nil, err
	}
	locks := s.Locks
	sort.Slice(locks, func(i, j int) bool { return locks[i].Name < locks[j].Name })
	return locks, nil
}

// Old zypper ignores --category on list-patches, and while the update stack is pending
// it lists every other patch under blocked-update-list.
func neededSecurityPatches(lp zypperStream) []string {
	var names []string
	for _, u := range slices.Concat(lp.Updates, lp.BlockedUpdates) {
		if u.Kind == "patch" && u.Status == "needed" && u.Category == "security" && validZypperName(u.Name) {
			names = append(names, u.Name)
		}
	}
	return names
}

func validZypperName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "-") && !strings.ContainsAny(name, " \t\n")
}
