package packages

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	wingetSigMu      sync.Mutex
	wingetSigChecked = map[string]bool{}
)

const (
	wingetPackagePrefix = "Microsoft.DesktopAppInstaller_"
	vclibsPackagePrefix = "Microsoft.VCLibs.140.00.UWPDesktop_"
	vclibsAppPrefix     = "Microsoft.VCLibs.140.00_"
	microsoftPublisher  = "8wekyb3d8bbwe"
)

type appxDir struct {
	path    string
	version string
	arch    string
}

func parseAppxDir(path, prefix string) (appxDir, bool) {
	name := filepath.Base(path)
	if !strings.HasPrefix(name, prefix) {
		return appxDir{}, false
	}
	parts := strings.Split(strings.TrimPrefix(name, prefix), "_")
	if len(parts) != 4 || parts[2] != "" || !strings.EqualFold(parts[3], microsoftPublisher) {
		return appxDir{}, false
	}
	if _, ok := parseDottedVersion(parts[0]); !ok {
		return appxDir{}, false
	}
	return appxDir{path: path, version: parts[0], arch: strings.ToLower(parts[1])}, true
}

func hostAppxArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	}
	return runtime.GOARCH
}

func newestAppxDir(paths []string, prefix string, archs ...string) (appxDir, bool) {
	var best appxDir
	found := false
	for _, p := range paths {
		d, ok := parseAppxDir(p, prefix)
		if !ok {
			continue
		}
		match := len(archs) == 0
		for _, a := range archs {
			if d.arch == a {
				match = true
			}
		}
		if !match {
			continue
		}
		if !found {
			best, found = d, true
			continue
		}
		if c, ok := compareDottedVersions(d.version, best.version); ok && c > 0 {
			best = d
		}
	}
	return best, found
}

func wingetCandidates(dirs []string, arch string) []appxDir {
	var preferred, other []appxDir
	for _, p := range dirs {
		d, ok := parseAppxDir(p, wingetPackagePrefix)
		if !ok {
			continue
		}
		if d.arch == arch || d.arch == "neutral" {
			preferred = append(preferred, d)
		} else {
			other = append(other, d)
		}
	}
	newestFirst := func(a, b appxDir) int {
		c, _ := compareDottedVersions(b.version, a.version)
		return c
	}
	slices.SortStableFunc(preferred, newestFirst)
	slices.SortStableFunc(other, newestFirst)
	return append(preferred, other...)
}

func firstTrustedWinget(cands []appxDir, verify func(exe string) error) (appxDir, error) {
	if len(cands) == 0 {
		return appxDir{}, errWingetNotFound
	}
	var errs []error
	for _, d := range cands {
		if err := verify(filepath.Join(d.path, "winget.exe")); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(d.path), err))
			continue
		}
		return d, nil
	}
	return appxDir{}, errors.Join(errs...)
}

func wingetChildEnv(environ []string, libDirs ...string) []string {
	var dirs []string
	for _, d := range libDirs {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	vclibsDir := strings.Join(dirs, ";")
	out := make([]string, 0, len(environ)+1)
	pathSet := false
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		switch {
		case strings.EqualFold(k, "TERM"):
			continue
		case strings.EqualFold(k, "PATH") && !pathSet:
			pathSet = true
			if vclibsDir != "" {
				v = vclibsDir + ";" + v
			}
			out = append(out, k+"="+v)
			continue
		case strings.EqualFold(k, "PATH"):
			continue
		}
		out = append(out, kv)
	}
	if !pathSet && vclibsDir != "" {
		out = append(out, "PATH="+vclibsDir)
	}
	return append(out, "TERM=dumb")
}

var errWingetNotFound = errors.New("winget.exe not found under WindowsApps")

const (
	wingetPackageFamily    = "Microsoft.DesktopAppInstaller_8wekyb3d8bbwe"
	microsoftPublisherName = "CN=Microsoft Corporation, O=Microsoft Corporation, L=Redmond, S=Washington, C=US"
)

type wingetAppxPackage struct {
	Publisher       string `json:"publisher"`
	SignatureKind   string `json:"signature_kind"`
	Family          string `json:"family"`
	InstallLocation string `json:"install_location"`
}

type wingetAppxPackages []wingetAppxPackage

func (p *wingetAppxPackages) UnmarshalJSON(b []byte) error {
	var one wingetAppxPackage
	if len(b) > 0 && b[0] == '{' {
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*p = wingetAppxPackages{one}
		return nil
	}
	var many []wingetAppxPackage
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*p = many
	return nil
}

type wingetTrustReport struct {
	Authenticode string             `json:"authenticode"`
	Signer       string             `json:"signer"`
	Packages     wingetAppxPackages `json:"packages"`
}

func normaliseWindowsPath(p string) (string, bool) {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	for strings.Contains(p, `\\`) {
		p = strings.ReplaceAll(p, `\\`, `\`)
	}
	p = strings.TrimRight(p, `\`)
	for _, seg := range strings.Split(p, `\`) {
		if seg == "." || seg == ".." {
			return "", false
		}
	}
	return strings.ToLower(p), p != ""
}

func pathInside(child, parent string) bool {
	c, ok1 := normaliseWindowsPath(child)
	d, ok2 := normaliseWindowsPath(parent)
	return ok1 && ok2 && strings.HasPrefix(c, d+`\`)
}

// A file inside an MSIX package is covered by the package signature and is often not
// Authenticode-signed itself, so the package is the primary check.
func evaluateWingetTrust(exe string, r wingetTrustReport) error {
	for _, pkg := range r.Packages {
		if pkg.Publisher == microsoftPublisherName &&
			pkg.Family == wingetPackageFamily &&
			(pkg.SignatureKind == "Store" || pkg.SignatureKind == "System") &&
			pathInside(exe, pkg.InstallLocation) {
			return nil
		}
	}
	if r.Authenticode == "Valid" && r.Signer == microsoftPublisherName {
		return nil
	}
	return fmt.Errorf("winget.exe is not inside a trusted Microsoft App Installer package (%d package(s) checked, Authenticode %s)", len(r.Packages), r.Authenticode)
}

func windowsAppsDir() string {
	pf := os.Getenv("ProgramW6432")
	if pf == "" {
		pf = os.Getenv("ProgramFiles")
	}
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "WindowsApps")
}

// resolveWinget returns the signed winget.exe under WindowsApps and the environment to run it
// with as SYSTEM. PATH is never searched: a writable PATH entry would run code as SYSTEM.
func resolveWinget() (string, []string, error) {
	root := windowsAppsDir()
	exes, _ := filepath.Glob(filepath.Join(root, wingetPackagePrefix+"*__"+microsoftPublisher, "winget.exe"))
	dirs := make([]string, 0, len(exes))
	for _, e := range exes {
		dirs = append(dirs, filepath.Dir(e))
	}
	arch := hostAppxArch()
	d, err := firstTrustedWinget(wingetCandidates(dirs, arch), verifyWingetSignature)
	if err != nil {
		return "", nil, err
	}
	exe := filepath.Join(d.path, "winget.exe")
	libArch := d.arch
	if libArch == "neutral" {
		libArch = arch
	}
	var libDirs []string
	for _, prefix := range []string{vclibsPackagePrefix, vclibsAppPrefix} {
		libs, _ := filepath.Glob(filepath.Join(root, prefix+"*__"+microsoftPublisher))
		if v, ok := newestAppxDir(libs, prefix, libArch); ok {
			libDirs = append(libDirs, v.path)
		}
	}
	tmp, err := wingetTempDir()
	if err != nil {
		return "", nil, fmt.Errorf("prepare a private temp directory for winget: %w", err)
	}
	return exe, withTempDir(wingetChildEnv(os.Environ(), libDirs...), tmp), nil
}

func verifyWingetSignature(exe string) error {
	wingetSigMu.Lock()
	defer wingetSigMu.Unlock()
	if wingetSigChecked[exe] {
		return nil
	}
	err := checkWingetTrust(exe)
	if err == nil {
		wingetSigChecked[exe] = true
	}
	return err
}

const wingetRetryAfter = 10 * time.Minute

var errWingetRecentlyFailed = errors.New("winget resolution failed recently")

type wingetResolver struct {
	mu       sync.Mutex
	resolve  func() (string, []string, error)
	now      func() time.Time
	failedAt time.Time
	lastErr  error
}

var sharedWingetResolver = &wingetResolver{resolve: resolveWinget, now: time.Now}

func (r *wingetResolver) get() (string, []string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastErr != nil && r.now().Sub(r.failedAt) < wingetRetryAfter {
		return "", nil, fmt.Errorf("%w: %w", errWingetRecentlyFailed, r.lastErr)
	}
	exe, env, err := r.resolve()
	if err != nil {
		r.failedAt, r.lastErr = r.now(), err
		return "", nil, err
	}
	r.lastErr = nil
	return exe, env, nil
}
