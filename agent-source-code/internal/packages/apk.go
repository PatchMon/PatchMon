// Package packages provides package management functionality for APK package manager
package packages

import (
	"context"
	"os"
	"regexp"
	"strings"
	"sync"

	"patchmon-agent/internal/logutil"
	"patchmon-agent/internal/repositories"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// APKManager handles APK package information collection
type APKManager struct {
	logger       *logrus.Logger
	secdb        *apkSecDB
	secdbEnabled bool
}

// NewAPKManager creates a new APK package manager
func NewAPKManager(logger *logrus.Logger) *APKManager {
	return &APKManager{
		logger:       logger,
		secdb:        newAPKSecDB(logger),
		secdbEnabled: true,
	}
}

// apkBusy is held by a patch run for its whole apk transaction, so a report in the
// same process skips its apk update instead of racing the run for the database lock.
var apkBusy = make(chan struct{}, 1)

// AcquireAPK waits for any in-process apk work to finish and keeps reports from starting
// another until release is called. onWait, if set, runs once when it has to wait.
func AcquireAPK(ctx context.Context, onWait func()) (release func(), err error) {
	var once sync.Once
	release = func() { once.Do(func() { <-apkBusy }) }
	select {
	case apkBusy <- struct{}{}:
		return release, nil
	default:
	}
	if onWait != nil {
		onWait()
	}
	select {
	case apkBusy <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetPackages gets package information for APK-based systems
func (m *APKManager) GetPackages() ([]models.Package, error) {
	ctx := context.Background()
	m.refreshIndexes()

	major := 0
	if out, err := apkOutput(ctx, "--version"); err == nil {
		major = APKToolsMajor(out)
	}
	cmp := apkVersionComparer(major)

	installedOutput, err := apkOutput(ctx, "list", "--installed")
	if err != nil {
		// See apt.go: an empty inventory reads as "fully patched".
		return nil, commandError("apk list --installed", err)
	}
	installedPackages := m.parseInstalledPackages(installedOutput)
	m.logger.WithField("count", len(installedPackages)).Debug("Found installed packages")

	upgradableOutput, err := apkOutput(ctx, "list", "--upgradable")
	if err != nil {
		return nil, commandError("apk list --upgradable", err)
	}
	upgradable := parseAPKListLines(upgradableOutput)

	held := m.heldPackages(ctx, upgradable, installedPackages, cmp)

	var fixes map[string][]string
	if m.secdbEnabled && len(upgradable) > len(held) {
		if branch, repos := apkSecDBSources(repositories.ReadAPKRepositoryLines()); branch != "" {
			fixes = m.secdb.fixes(ctx, branch, repos)
		}
	}

	upgradablePackages := make([]models.Package, 0, len(upgradable))
	for _, e := range upgradable {
		if e.From == "" || held[e.Name] {
			continue
		}
		current := e.From
		if p, ok := installedPackages[e.Name]; ok {
			current = p.CurrentVersion
		}
		upgradablePackages = append(upgradablePackages, models.Package{
			Name:             e.Name,
			CurrentVersion:   current,
			AvailableVersion: e.Version,
			NeedsUpdate:      true,
			IsSecurityUpdate: apkFixesSecurity(fixes[e.Origin], current, e.Version, cmp),
		})
	}
	m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")

	packages := CombinePackageData(installedPackages, upgradablePackages)
	m.enrichWithRepoAttribution(packages)
	m.logger.WithField("total", len(packages)).Debug("Total packages collected")
	return packages, nil
}

// SetSecDBEnabled turns the Alpine secdb lookup for security updates on or off.
func (m *APKManager) SetSecDBEnabled(enabled bool) {
	m.secdbEnabled = enabled
}

func (m *APKManager) refreshIndexes() {
	select {
	case apkBusy <- struct{}{}:
		defer func() { <-apkBusy }()
	default:
		m.logger.Debug("Patch run in progress, using the indexes it refreshed")
		return
	}
	updateCmd, cancel := boundedCommand(networkCollectorTimeout, "apk", "update", "-q")
	defer cancel()
	if err := updateCmd.Run(); err != nil {
		m.logger.WithError(err).Warn("Failed to update package index")
	}
}

// Held packages are reported as current rather than offered for patching.
func (m *APKManager) heldPackages(ctx context.Context, upgradable []apkListEntry, installed map[string]models.Package, cmp func(a, b string) int) map[string]bool {
	world, err := ReadAPKWorld()
	if err != nil || len(world) == 0 {
		return nil
	}
	installedVersions := make(map[string]string, len(installed))
	for name, p := range installed {
		installedVersions[name] = p.CurrentVersion
	}
	available := make(map[string]string, len(upgradable))
	for _, e := range upgradable {
		if e.From != "" {
			available[e.Name] = e.Version
		}
	}
	holds, conflicts := apkWorldHolds(world, installedVersions, available, cmp)
	held := make(map[string]bool)
	for _, h := range append(holds, conflicts...) {
		if h.Available != "" {
			held[h.Name] = true
			m.logger.WithField("hold", logutil.Sanitize(h.String())).Info("Held by /etc/apk/world")
		}
	}
	for _, c := range conflicts {
		m.logger.Warn(logutil.Sanitize(APKConflictMessage(c)))
	}
	if len(holds) == 0 || len(conflicts) > 0 {
		return held
	}
	out, err := apkOutput(ctx, "upgrade", "--simulate", "--no-progress")
	if err != nil {
		return held
	}
	planned := ParseAPKPlan(out).Names()
	for name := range available {
		if !planned[name] && !held[name] {
			held[name] = true
			m.logger.WithField("package", logutil.Sanitize(name)).Info("Held back with a package pinned in /etc/apk/world")
		}
	}
	return held
}

// enrichWithRepoAttribution populates SourceRepository for each package by running
// apk policy in batches and extracting the logical repo name from the URL.
func (m *APKManager) enrichWithRepoAttribution(packages []models.Package) {
	if len(packages) == 0 {
		return
	}

	names := make([]string, len(packages))
	for i := range packages {
		names[i] = packages[i].Name
	}

	pkgIdx := make(map[string][]int, len(packages))
	for i, p := range packages {
		pkgIdx[p.Name] = append(pkgIdx[p.Name], i)
	}

	const batchSize = 200
	repoMap := make(map[string]string, len(packages))

	for start := 0; start < len(names); start += batchSize {
		end := min(start+batchSize, len(names))
		args := append([]string{"policy"}, names[start:end]...)
		// Scoped so each batch releases its context.
		output, err := func() ([]byte, error) {
			cmd, cancel := boundedCommand(collectorTimeout, "apk", args...)
			defer cancel()
			cmd.Env = append(os.Environ(), "LANG=C")
			return cmd.Output()
		}()
		if err != nil {
			m.logger.WithError(err).Warn("apk policy failed, skipping repo attribution for batch")
			continue
		}

		m.parseApkPolicy(string(output), repoMap)
	}

	for name, repo := range repoMap {
		for _, idx := range pkgIdx[name] {
			packages[idx].SourceRepository = repo
		}
	}

	m.logger.WithField("attributed", len(repoMap)).Debug("Enriched packages with repository attribution")
}

// parseApkPolicy parses apk policy output and populates repoMap.
// Output format per package:
//
//	busybox policy:
//	  1.37.0-r29:
//	    lib/apk/db/installed
//	  1.37.0-r30:
//	    https://dl-cdn.alpinelinux.org/alpine/v3.23/main
func (m *APKManager) parseApkPolicy(output string, repoMap map[string]string) {
	var currentPkg string
	for line := range strings.Lines(output) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasSuffix(trimmed, " policy:") {
			currentPkg = strings.TrimSuffix(trimmed, " policy:")
			continue
		}
		if currentPkg == "" || strings.HasSuffix(trimmed, ":") || strings.Contains(trimmed, "lib/apk/db/installed") {
			continue
		}
		if _, ok := repoMap[currentPkg]; ok {
			continue
		}
		if repo := m.extractRepoNameFromURL(trimmed); repo != "" {
			repoMap[currentPkg] = repo
		}
	}
}

var apkArchSegments = map[string]bool{
	"x86_64": true, "x86": true, "aarch64": true, "armhf": true, "armv7": true,
	"ppc64le": true, "s390x": true, "riscv64": true, "loongarch64": true,
}

func (m *APKManager) extractRepoNameFromURL(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	url := fields[len(fields)-1]
	if !strings.Contains(url, "://") {
		return ""
	}
	if _, comp := repositories.APKBranchAndComponent(url); comp != "" {
		return comp
	}
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	for i, part := range parts {
		if apkArchSegments[part] && i > 0 {
			return parts[i-1]
		}
	}
	if n := len(parts); n > 3 && !strings.HasSuffix(parts[n-1], ".tar.gz") {
		return parts[n-1]
	}
	return ""
}

type apkListEntry struct {
	Name    string
	Version string
	Origin  string
	From    string
}

// The licence can contain spaces and parentheses, so origin and status are found by their brackets.
func parseAPKListLines(output string) []apkListEntry {
	var out []apkListEntry
	for line := range strings.Lines(output) {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name, version := splitAPKNameVersion(fields[0])
		if name == "" || version == "" {
			continue
		}
		e := apkListEntry{Name: name, Version: version}
		if i := strings.IndexByte(line, '{'); i >= 0 {
			if j := strings.IndexByte(line[i:], '}'); j > 0 {
				e.Origin = line[i+1 : i+j]
			}
		}
		if i := strings.LastIndex(line, "[upgradable from: "); i >= 0 {
			old := strings.TrimSuffix(strings.TrimSpace(line[i+len("[upgradable from: "):]), "]")
			_, e.From = splitAPKNameVersion(old)
			if e.From == "" {
				continue
			}
		} else if !strings.HasSuffix(line, "[installed]") {
			continue
		}
		out = append(out, e)
	}
	return out
}

var apkRevisionRe = regexp.MustCompile(`^r\d+$`)

// Split from the right: names can contain "-digit" (font-adobe-100dpi), and virtual
// packages from apk add -t have no -rN.
func splitAPKNameVersion(s string) (name, version string) {
	i := strings.LastIndexByte(s, '-')
	if i <= 0 || i+1 >= len(s) {
		return "", ""
	}
	if apkRevisionRe.MatchString(s[i+1:]) {
		j := strings.LastIndexByte(s[:i], '-')
		if j <= 0 {
			return "", ""
		}
		i = j
	}
	if c := s[i+1]; c < '0' || c > '9' {
		return "", ""
	}
	return s[:i], s[i+1:]
}

// parseInstalledPackages parses apk list --installed output
// Format: package-name-version-release arch {origin} (license) [installed]
func (m *APKManager) parseInstalledPackages(output string) map[string]models.Package {
	installedPackages := make(map[string]models.Package)
	for _, e := range parseAPKListLines(output) {
		if e.From != "" {
			continue
		}
		installedPackages[e.Name] = models.Package{
			Name:           e.Name,
			CurrentVersion: e.Version,
		}
	}
	return installedPackages
}
