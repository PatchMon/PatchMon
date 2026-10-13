package packages

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// chooseWingetDir picks the newest App Installer directory for the host's architecture
// (or neutral), falling back to any architecture. Dirs must already contain winget.exe.
func chooseWingetDir(dirs []string, arch string) (appxDir, bool) {
	if d, ok := newestAppxDir(dirs, wingetPackagePrefix, arch, "neutral"); ok {
		return d, true
	}
	return newestAppxDir(dirs, wingetPackagePrefix)
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

func windowsAppsDir() string {
	pf := os.Getenv("ProgramFiles")
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
	d, ok := chooseWingetDir(dirs, arch)
	if !ok {
		return "", nil, errors.New("winget.exe not found under WindowsApps")
	}
	exe := filepath.Join(d.path, "winget.exe")
	if err := verifyWingetSignature(exe); err != nil {
		return "", nil, err
	}
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
	return exe, wingetChildEnv(os.Environ(), libDirs...), nil
}

func verifyWingetSignature(exe string) error {
	wingetSigMu.Lock()
	defer wingetSigMu.Unlock()
	if wingetSigChecked[exe] {
		return nil
	}
	err := checkAuthenticode(exe)
	if err == nil {
		wingetSigChecked[exe] = true
	}
	return err
}
