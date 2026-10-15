// Package pkgmgr decides which package manager the host uses.
package pkgmgr

import (
	"bufio"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Probe is the view of the host that Detect reads.
type Probe struct {
	GOOS           string
	OSReleasePaths []string
	LookPath       func(string) (string, error)
	Stat           func(string) (fs.FileInfo, error)
	Uname          func() string
}

// SystemProbe returns a Probe backed by the running host.
func SystemProbe() Probe {
	return Probe{
		GOOS:           runtime.GOOS,
		OSReleasePaths: []string{"/etc/os-release", "/usr/lib/os-release"},
		LookPath:       exec.LookPath,
		Stat:           os.Stat,
		Uname: func() string {
			out, err := exec.Command("uname", "-s").Output()
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(out))
		},
	}
}

// Detect returns apt, dnf, yum, apk, pacman, zypper, pkg, windows or unknown.
func Detect() string {
	return SystemProbe().Detect()
}

var familyByOSID = map[string]string{
	"debian":              "apt",
	"ubuntu":              "apt",
	"rhel":                "rpm",
	"fedora":              "rpm",
	"centos":              "rpm",
	"rocky":               "rpm",
	"almalinux":           "rpm",
	"ol":                  "rpm",
	"amzn":                "rpm",
	"alpine":              "apk",
	"arch":                "pacman",
	"suse":                "zypper",
	"opensuse":            "zypper",
	"opensuse-leap":       "zypper",
	"opensuse-tumbleweed": "zypper",
	"opensuse-slowroll":   "zypper",
	"sles":                "zypper",
	"sled":                "zypper",
}

// Detect works through os-release, then the installed package database, then
// PATH, so a stray binary from another family cannot outvote the distribution.
func (p Probe) Detect() string {
	if p.GOOS == "windows" {
		return "windows"
	}
	if p.GOOS == "freebsd" {
		for _, path := range []string{"/usr/sbin/pkg", "/usr/local/sbin/pkg"} {
			if info, err := p.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
				return "pkg"
			}
		}
	}
	if p.has("pkg") && p.Uname() == "FreeBSD" {
		return "pkg"
	}

	for _, id := range p.osReleaseIDs() {
		if m := p.resolveFamily(familyByOSID[id]); m != "" {
			return m
		}
	}

	dpkg := p.exists("/var/lib/dpkg/status")
	rpm := p.exists("/var/lib/rpm") || p.exists("/usr/lib/sysimage/rpm")
	switch {
	case dpkg && !rpm:
		if m := p.resolveFamily("apt"); m != "" {
			return m
		}
	case rpm && !dpkg:
		if m := p.resolveFamily("rpm"); m != "" {
			return m
		}
	}

	for _, family := range []string{"apk", "apt", "rpm", "pacman", "zypper"} {
		if m := p.resolveFamily(family); m != "" {
			return m
		}
	}
	return "unknown"
}

func (p Probe) resolveFamily(family string) string {
	switch family {
	case "apt":
		if p.has("apt") || p.has("apt-get") {
			return "apt"
		}
	case "rpm":
		if p.has("dnf") {
			return "dnf"
		}
		if p.has("yum") {
			return "yum"
		}
		if p.has("zypper") {
			return "zypper"
		}
	case "apk", "pacman", "zypper":
		if p.has(family) {
			return family
		}
	}
	return ""
}

func (p Probe) has(bin string) bool {
	_, err := p.LookPath(bin)
	return err == nil
}

func (p Probe) exists(path string) bool {
	_, err := p.Stat(path)
	return err == nil
}

func (p Probe) osReleaseIDs() []string {
	for _, path := range p.OSReleasePaths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		var id, idLike string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
			if !ok {
				continue
			}
			value = strings.ToLower(strings.Trim(strings.TrimSpace(value), `"'`))
			switch key {
			case "ID":
				id = value
			case "ID_LIKE":
				idLike = value
			}
		}
		_ = f.Close()
		if id == "" && idLike == "" {
			continue
		}
		return append([]string{id}, strings.Fields(idLike)...)
	}
	return nil
}
