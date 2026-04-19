package packages

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// APTManager handles APT package information collection
type APTManager struct {
	logger       *logrus.Logger
	cacheRefresh CacheRefreshConfig
}

// NewAPTManager creates a new APT package manager
func NewAPTManager(logger *logrus.Logger, cacheRefresh CacheRefreshConfig) *APTManager {
	return &APTManager{
		logger:       logger,
		cacheRefresh: cacheRefresh,
	}
}

// detectPackageManager detects whether to use apt or apt-get
func (m *APTManager) detectPackageManager() string {
	// Prefer /usr/bin/apt (upstream binary) to avoid wrapper scripts (like on Linux Mint)
	if _, err := exec.LookPath("/usr/bin/apt"); err == nil {
		return "/usr/bin/apt"
	}
	// Fallback to checking for "apt" in PATH
	if _, err := exec.LookPath("apt"); err == nil {
		return "apt"
	}
	// As a last resort, try "apt-get"
	return "apt-get"
}

// GetPackages gets package information for APT-based systems
func (m *APTManager) GetPackages() []models.Package {
	// Determine package manager
	packageManager := m.detectPackageManager()

	// Conditionally refresh the package cache based on configuration
	shouldRefresh := m.cacheRefresh.Mode == "always" ||
		(m.cacheRefresh.Mode == "if_stale" && m.isCacheStale(m.cacheRefresh.MaxAge))
	if shouldRefresh {
		m.logger.WithField("mode", m.cacheRefresh.Mode).Debug("Refreshing package cache")
		updateCmd := exec.Command(packageManager, "update", "-qq")
		if err := updateCmd.Run(); err != nil {
			m.logger.WithError(err).WithField("manager", packageManager).Warn("Failed to update package lists")
		}
	} else {
		m.logger.WithField("mode", m.cacheRefresh.Mode).Debug("Skipping package cache refresh")
	}

	// OPTIMIZATION: dpkg-query and the apt upgrade simulation are independent
	// read-only operations. Run them concurrently so their wall times overlap
	// instead of stacking.
	var (
		installedPackages  map[string]models.Package
		upgradablePackages []models.Package
	)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		m.logger.Debug("Getting installed packages...")
		installedCmd := exec.Command("dpkg-query", "-W", "-f", "${Package} ${Version} ${Description}\n")
		installedCmd.Env = append(os.Environ(), "LANG=C")
		out, err := installedCmd.Output()
		if err != nil {
			m.logger.WithError(err).Warn("Failed to get installed packages")
			installedPackages = make(map[string]models.Package)
			return
		}
		installedPackages = m.parseInstalledPackages(string(out))
		m.logger.WithField("count", len(installedPackages)).Debug("Found installed packages")
	}()

	go func() {
		defer wg.Done()
		m.logger.Debug("Getting upgradable packages...")
		upgradeCmd := exec.Command(packageManager, "-s", "-o", "Debug::NoLocking=1", "upgrade")
		upgradeCmd.Env = append(os.Environ(), "LANG=C")
		out, err := upgradeCmd.Output()
		if err != nil {
			m.logger.WithError(err).Warn("Failed to get upgrade simulation")
			upgradablePackages = []models.Package{}
			return
		}
		upgradablePackages = m.parseAPTUpgrade(string(out))
		m.logger.WithField("count", len(upgradablePackages)).Debug("Found upgradable packages")
	}()

	wg.Wait()

	// Merge and deduplicate packages (pass full installed packages to preserve descriptions)
	packages := CombinePackageData(installedPackages, upgradablePackages)

	// Enrich packages with repository attribution
	m.enrichWithRepoAttribution(packages)

	return packages
}

// enrichWithRepoAttribution populates SourceRepository for each package by running
// apt-cache policy in batches and parsing the output.
//
// OPTIMIZATION: Batches are dispatched concurrently across a small worker pool
// (bounded by GOMAXPROCS) because each apt-cache invocation spends most of its
// time in process startup + apt database reads — work the kernel and apt can
// pipeline happily in parallel. Each worker writes into its own local repoMap
// and the results are merged serially at the end to avoid lock contention
// inside the hot loop.
func (m *APTManager) enrichWithRepoAttribution(packages []models.Package) {
	if len(packages) == 0 {
		return
	}

	// Build name list
	names := make([]string, len(packages))
	for i := range packages {
		names[i] = packages[i].Name
	}

	// Build lookup: name -> index in packages slice (multiple entries possible for same name)
	pkgIdx := make(map[string][]int, len(packages))
	for i, p := range packages {
		pkgIdx[p.Name] = append(pkgIdx[p.Name], i)
	}

	const batchSize = 200

	// Collect batch ranges upfront so the worker loop is just "run the i-th range"
	type batchRange struct{ start, end int }
	var batches []batchRange
	for start := 0; start < len(names); start += batchSize {
		end := start + batchSize
		if end > len(names) {
			end = len(names)
		}
		batches = append(batches, batchRange{start, end})
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > len(batches) {
		workers = len(batches)
	}
	if workers < 1 {
		workers = 1
	}

	// workCh is buffered to the full batch count so the feeder goroutine never
	// blocks on send. This matters because if a worker dies (panic in the
	// parser, for example), a later send on an unbuffered channel would hang
	// forever and close(workCh) would never be reached, deadlocking the whole
	// report.
	workCh := make(chan batchRange, len(batches))
	resultCh := make(chan map[string]string, len(batches))

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			env := append(os.Environ(), "LANG=C")
			for br := range workCh {
				// Per-batch recover: a parser panic takes out the batch, not
				// the worker. resultCh still gets a value per batch so the
				// merger loop eventually terminates.
				func(br batchRange) {
					defer func() {
						if r := recover(); r != nil {
							m.logger.WithField("panic", r).Error("apt-cache batch panicked, dropping result")
							resultCh <- nil
						}
					}()
					batch := names[br.start:br.end]
					args := append([]string{"policy"}, batch...)
					cmd := exec.Command("apt-cache", args...)
					cmd.Env = env
					output, err := cmd.Output()
					if err != nil {
						m.logger.WithError(err).Warn("apt-cache policy failed, skipping repo attribution for batch")
						resultCh <- nil
						return
					}
					local := make(map[string]string, br.end-br.start)
					m.parseAptCachePolicy(string(output), local)
					resultCh <- local
				}(br)
			}
		}()
	}

	go func() {
		for _, br := range batches {
			workCh <- br
		}
		close(workCh)
		wg.Wait()
		close(resultCh)
	}()

	repoMap := make(map[string]string, len(packages))
	for local := range resultCh {
		for name, repo := range local {
			// Last-writer-wins is fine: each package only appears once in the
			// flattened name list, so there's no key collision between batches.
			repoMap[name] = repo
		}
	}

	// Apply results
	for name, repo := range repoMap {
		for _, idx := range pkgIdx[name] {
			packages[idx].SourceRepository = repo
		}
	}

	m.logger.WithField("attributed", len(repoMap)).Debug("Enriched packages with repository attribution")
}

// parseAptCachePolicy parses apt-cache policy output and populates repoMap with
// package name -> repository string mappings.
//
// apt-cache policy output format per package:
//
//	bash:
//	  Installed: 5.2.15-2+b7
//	  Candidate: 5.2.15-2+b7
//	  Version table:
//	 *** 5.2.15-2+b7 500
//	        500 http://deb.debian.org/debian bookworm/main amd64 Packages
//	        100 /var/lib/dpkg/status
//	     5.1.4-2 500
//	        500 http://archive.ubuntu.com/ubuntu focal/main amd64 Packages
//
// The *** marker may appear at any position in the version table (not necessarily first).
// When the installed version has only /var/lib/dpkg/status as its source (common for
// packages with pending updates — the repo has the newer version, not the installed one),
// we fall back to the first remote source from any version entry in the table.
func (m *APTManager) parseAptCachePolicy(output string, repoMap map[string]string) {
	lines := strings.Split(output, "\n")

	var currentPkg string
	var inVersionTable bool
	var inInstalledBlock bool
	var installedRepo string // best source from the *** block
	var fallbackRepo string  // first remote source from any version block

	finalizePkg := func() {
		if currentPkg == "" {
			return
		}
		if installedRepo != "" {
			repoMap[currentPkg] = installedRepo
		} else if fallbackRepo != "" {
			repoMap[currentPkg] = fallbackRepo
		}
		// If neither found, don't set anything (package stays unattributed)
	}

	for _, line := range lines {
		// Package header line: "packagename:" (no leading whitespace)
		if len(line) > 0 && line[0] != ' ' && strings.HasSuffix(strings.TrimSpace(line), ":") {
			finalizePkg()
			currentPkg = strings.TrimSuffix(strings.TrimSpace(line), ":")
			inVersionTable = false
			inInstalledBlock = false
			installedRepo = ""
			fallbackRepo = ""
			continue
		}

		if currentPkg == "" {
			continue
		}

		// Already resolved this package with a real repo
		if repoMap[currentPkg] != "" {
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Detect "Version table:" section
		if trimmed == "Version table:" {
			inVersionTable = true
			inInstalledBlock = false
			continue
		}

		if !inVersionTable {
			continue
		}

		// Version entry lines start with " *** " (installed) or "     " (other versions)
		// followed by version and priority. They have ~5 spaces indent (not 8+).
		// Source lines below them have 8+ spaces indent.
		isSourceLine := strings.HasPrefix(line, "        ") // 8+ spaces
		isVersionLine := !isSourceLine && trimmed != ""

		if isVersionLine {
			if strings.HasPrefix(trimmed, "*** ") {
				inInstalledBlock = true
			} else {
				inInstalledBlock = false
			}
			continue
		}

		// Parse source lines (8+ spaces indent)
		if isSourceLine && trimmed != "" {
			// Skip /var/lib/dpkg/status
			if strings.Contains(trimmed, "/var/lib/dpkg/status") {
				continue
			}

			// Parse: <priority> <url> <suite>/<component> <arch> Packages
			fields := strings.Fields(trimmed)
			if len(fields) >= 3 {
				url := fields[1]
				suiteComponent := fields[2] // e.g. "bookworm/main"
				repoStr := url + " " + suiteComponent

				if inInstalledBlock && installedRepo == "" {
					installedRepo = repoStr
				}
				if fallbackRepo == "" {
					fallbackRepo = repoStr
				}
			}
		}
	}

	finalizePkg()
}

// isCacheStale checks if the APT package cache is older than maxAgeMinutes.
func (m *APTManager) isCacheStale(maxAgeMinutes int) bool {
	// Check standard cache file locations
	paths := []string{"/var/cache/apt/pkgcache.bin", "/var/lib/apt/lists"}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		age := time.Since(info.ModTime())
		return age > time.Duration(maxAgeMinutes)*time.Minute
	}
	// If we can't determine age, assume stale
	return true
}

// parseAPTUpgrade parses apt/apt-get upgrade simulation output
func (m *APTManager) parseAPTUpgrade(output string) []models.Package {
	var packages []models.Package

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Look for lines starting with "Inst"
		if !strings.HasPrefix(line, "Inst ") {
			continue
		}

		// Parse the line: Inst package [current_version] (new_version source)
		fields := slices.Collect(strings.FieldsSeq(line))
		if len(fields) < 4 {
			m.logger.WithField("line", line).Debug("Skipping 'Inst' line due to insufficient fields")
			continue
		}

		packageName := fields[1]

		// Extract current version (in brackets)
		var currentVersion string
		for i, field := range fields {
			if strings.HasPrefix(field, "[") && strings.HasSuffix(field, "]") {
				currentVersion = strings.Trim(field, "[]")
				break
			} else if after, found := strings.CutPrefix(field, "["); found {
				// Multi-word version, continue until we find the closing bracket
				versionParts := []string{after}
				for j := i + 1; j < len(fields); j++ {
					if strings.HasSuffix(fields[j], "]") {
						versionParts = append(versionParts, strings.TrimSuffix(fields[j], "]"))
						break
					}
					versionParts = append(versionParts, fields[j])
				}
				currentVersion = strings.Join(versionParts, " ")
				break
			}
		}

		// Extract available version (in parentheses)
		var availableVersion string
		for _, field := range fields {
			if after, found := strings.CutPrefix(field, "("); found {
				availableVersion = after
				break
			}
		}

		// Check if it's a security update
		isSecurityUpdate := strings.Contains(strings.ToLower(line), "security")

		if packageName != "" && currentVersion != "" && availableVersion != "" {
			packages = append(packages, models.Package{
				Name:             packageName,
				CurrentVersion:   currentVersion,
				AvailableVersion: availableVersion,
				NeedsUpdate:      true,
				IsSecurityUpdate: isSecurityUpdate,
			})
		}
	}

	return packages
}

// parseInstalledPackages parses dpkg-query output and returns a map of package name to version
func (m *APTManager) parseInstalledPackages(output string) map[string]models.Package {
	installedPackages := make(map[string]models.Package)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentPkg *models.Package

	for scanner.Scan() {
		line := scanner.Text() // Preserve whitespace for description continuation detection
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		// Check if this line is a continuation of the description (starts with space)
		if strings.HasPrefix(line, " ") && currentPkg != nil {
			// It's a description continuation
			// For now, we can append it or just skip if we only want the summary.
			// Let's append it to have full description, joining with newline
			currentPkg.Description += "\n" + trimmedLine
			installedPackages[currentPkg.Name] = *currentPkg // Update map
			continue
		}

		// New package line: Package Version Description
		// We use SplitN with 3 parts. Description is the rest.
		parts := strings.SplitN(trimmedLine, " ", 3)
		if len(parts) < 2 {
			m.logger.WithField("line", line).Debug("Skipping malformed installed package line")
			currentPkg = nil
			continue
		}

		packageName := parts[0]
		version := parts[1]
		description := ""
		if len(parts) == 3 {
			description = parts[2]
		}

		pkg := models.Package{
			Name:           packageName,
			CurrentVersion: version,
			Description:    description,
			NeedsUpdate:    false,
		}
		installedPackages[packageName] = pkg
		currentPkg = &pkg
	}

	return installedPackages
}
