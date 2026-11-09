package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"patchmon-agent/internal/winexec"
	"patchmon-agent/pkg/models"
)

var chocolateyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

// Chocolatey packages have their own identities and versions (including meta
// packages and packaging revisions). Do not guess a registry/WinGet match or
// attach a WingetID: these rows report availability, not WinGet patch targets.
func collectChocolateyPackages() ([]models.Package, error) {
	exe, err := findChocolatey()
	if err != nil {
		return nil, err
	}
	if exe == "" {
		return nil, nil
	}
	return chocolateyPackages(chocolateyCommandRunner(context.Background(), exe))
}

func chocolateyCommandRunner(ctx context.Context, exe string) chocolateyRunner {
	return func(timeout time.Duration, args ...string) (string, int, error) {
		childCtx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(childCtx, exe, args...)
		cmd.WaitDelay = commandKillDelay
		defer cancel()
		hideWindow(cmd)
		output, err := cmd.Output()
		code := 0
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				return "", 0, fmt.Errorf("run Chocolatey: %w", err)
			}
			code = exitErr.ExitCode()
		}
		return string(winexec.TrimBOM(output)), code, nil
	}
}

func findChocolatey() (string, error) {
	root := os.Getenv("ChocolateyInstall")
	if root == "" {
		if programData := os.Getenv("ProgramData"); programData != "" {
			root = filepath.Join(programData, "chocolatey")
		}
	}
	if root != "" {
		candidate := filepath.Join(root, "bin", "choco.exe")
		info, err := os.Stat(candidate)
		if err == nil {
			if !info.Mode().IsRegular() {
				return "", errors.New("chocolatey executable is not a regular file")
			}
			return candidate, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("locate Chocolatey: %w", err)
		}
	}
	exe, err := exec.LookPath("choco.exe")
	if errors.Is(err, exec.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("locate Chocolatey: %w", err)
	}
	return exe, nil
}

type chocolateyRunner func(time.Duration, ...string) (string, int, error)

func chocolateyPackages(run chocolateyRunner) ([]models.Package, error) {
	version, code, err := run(collectorTimeout, "--version")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("chocolatey version query failed (exit %d)", code)
	}
	majorText, _, ok := strings.Cut(strings.TrimSpace(version), ".")
	major, err := strconv.Atoi(majorText)
	if !ok || err != nil || major < 1 {
		return nil, errors.New("unrecognized Chocolatey CLI version")
	}
	args := []string{"list", "--limit-output", "--no-color"}
	// Before CLI 2, list searched remote feeds unless --local-only was given.
	if major == 1 {
		args = append(args, "--local-only")
	}
	installed, code, err := run(collectorTimeout, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 && code != 2 {
		return nil, fmt.Errorf("chocolatey installed-package scan failed (exit %d)", code)
	}
	pkgs, err := parseChocolateyInstalled(installed)
	if err != nil {
		return nil, err
	}
	if code == 2 && len(pkgs) != 0 {
		return nil, errors.New("chocolatey list returned packages with a no-results exit code")
	}
	if len(pkgs) == 0 {
		return pkgs, nil
	}
	// No --source override: use the administrator's configured feeds/credentials.
	outdated, code, err := run(networkCollectorTimeout, "outdated", "--limit-output", "--no-color")
	if err != nil {
		return nil, err
	}
	// With useEnhancedExitCodes, 2 means updates were found, not a failure.
	if code != 0 && code != 2 {
		return nil, fmt.Errorf("chocolatey outdated scan failed (exit %d)", code)
	}
	if err := applyChocolateyOutdated(pkgs, outdated); err != nil {
		return nil, err
	}
	if code == 2 && strings.TrimSpace(outdated) == "" {
		return nil, errors.New("chocolatey reported updates without package results")
	}
	return pkgs, nil
}

func parseChocolateyInstalled(output string) ([]models.Package, error) {
	var pkgs []models.Package
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) != 2 || !chocolateyIDPattern.MatchString(fields[0]) || strings.TrimSpace(fields[1]) == "" {
			return nil, errors.New("invalid Chocolatey installed-package output")
		}
		id := strings.ToLower(fields[0])
		if seen[id] {
			return nil, errors.New("duplicate Chocolatey package ID")
		}
		seen[id] = true
		pkgs = append(pkgs, models.Package{ChocolateyID: id, Name: "chocolatey:" + id, CurrentVersion: fields[1], SourceRepository: "chocolatey", Category: "Applications", Description: "Chocolatey-managed package"})
	}
	return pkgs, nil
}

func applyChocolateyOutdated(pkgs []models.Package, output string) error {
	byID := make(map[string]int, len(pkgs))
	for i, p := range pkgs {
		byID[strings.TrimPrefix(p.Name, "chocolatey:")] = i
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// --limit-output: id|installed version|available version|pinned
		fields := strings.Split(line, "|")
		if len(fields) != 4 || !chocolateyIDPattern.MatchString(fields[0]) || fields[1] == "" || fields[2] == "" || (fields[3] != "true" && fields[3] != "false") {
			return errors.New("invalid Chocolatey outdated output")
		}
		id := strings.ToLower(fields[0])
		i, ok := byID[id]
		if !ok || seen[id] || pkgs[i].CurrentVersion != fields[1] {
			return errors.New("chocolatey inventory changed between list and outdated scans")
		}
		seen[id] = true
		pkgs[i].AvailableVersion = fields[2]
		pkgs[i].NeedsUpdate = true
		if fields[3] == "true" {
			pkgs[i].Description += "; pinned in Chocolatey"
			pkgs[i].ChocolateyPinned = true
		}
	}
	return nil
}
