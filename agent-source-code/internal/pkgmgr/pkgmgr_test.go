package pkgmgr

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeInfo struct{ mode fs.FileMode }

func (f fakeInfo) Name() string       { return "" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

type host struct {
	goos      string
	osRelease string
	bins      []string
	paths     map[string]fs.FileMode
	uname     string
}

func (h host) probe(t *testing.T) Probe {
	t.Helper()
	var releasePaths []string
	if h.osRelease != "" {
		path := filepath.Join(t.TempDir(), "os-release")
		if err := os.WriteFile(path, []byte(h.osRelease), 0o600); err != nil {
			t.Fatal(err)
		}
		releasePaths = []string{path}
	}
	bins := map[string]bool{}
	for _, b := range h.bins {
		bins[b] = true
	}
	goos := h.goos
	if goos == "" {
		goos = "linux"
	}
	uname := h.uname
	if uname == "" {
		uname = "Linux"
	}
	return Probe{
		GOOS:           goos,
		OSReleasePaths: append(releasePaths, filepath.Join(t.TempDir(), "absent")),
		LookPath: func(bin string) (string, error) {
			if bins[bin] {
				return "/usr/bin/" + bin, nil
			}
			return "", errors.New("not found")
		},
		Stat: func(path string) (fs.FileInfo, error) {
			if mode, ok := h.paths[path]; ok {
				return fakeInfo{mode: mode}, nil
			}
			return nil, fs.ErrNotExist
		},
		Uname: func() string { return uname },
	}
}

var (
	rpmDB  = map[string]fs.FileMode{"/var/lib/rpm": fs.ModeDir | 0o755}
	dpkgDB = map[string]fs.FileMode{"/var/lib/dpkg/status": 0o644}
)

const rhel79 = `NAME="Red Hat Enterprise Linux Server"
VERSION="7.9 (Maipo)"
ID="rhel"
ID_LIKE="fedora"
VARIANT="Server"
VARIANT_ID="server"
VERSION_ID="7.9"
PRETTY_NAME="Red Hat Enterprise Linux Server 7.9 (Maipo)"
`

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		host host
		want string
	}{
		{"issue 1101: RHEL 7.9 with an apt binary on PATH", host{osRelease: rhel79, bins: []string{"apt", "yum", "rpm"}, paths: rpmDB}, "yum"},
		{"CentOS 7 with apt on PATH", host{osRelease: "ID=\"centos\"\nID_LIKE=\"rhel fedora\"\n", bins: []string{"apt", "yum"}, paths: rpmDB}, "yum"},
		{"Rocky 9 with apt-get on PATH", host{osRelease: "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\n", bins: []string{"apt-get", "dnf", "yum"}, paths: rpmDB}, "dnf"},
		{"no os-release, rpm database, apt on PATH", host{bins: []string{"apt", "yum"}, paths: rpmDB}, "yum"},
		{"no os-release, dpkg database, yum on PATH", host{bins: []string{"apt", "yum"}, paths: dpkgDB}, "apt"},
		{"Debian with yum installed", host{osRelease: "ID=debian\n", bins: []string{"apt", "apt-get", "yum"}, paths: dpkgDB}, "apt"},
		{"Ubuntu with pacman installed", host{osRelease: "ID=ubuntu\nID_LIKE=debian\n", bins: []string{"apt", "pacman"}, paths: dpkgDB}, "apt"},
		{"Arch with apt on PATH", host{osRelease: "ID=arch\n", bins: []string{"apt", "pacman"}}, "pacman"},
		{"os-release family tool missing falls back to PATH", host{osRelease: "ID=fedora\n", bins: []string{"apk"}}, "apk"},
		{"Linux host with a non-FreeBSD pkg binary", host{osRelease: "ID=ubuntu\nID_LIKE=debian\n", bins: []string{"pkg", "apt"}}, "apt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.host.probe(t).Detect(); got != tc.want {
				t.Errorf("Detect() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Each supported platform with only its own tools must detect exactly what the
// PATH-only order did before os-release was consulted.
func TestDetectUnchangedForSupportedPlatforms(t *testing.T) {
	exec := fs.FileMode(0o755)
	cases := []struct {
		name string
		host host
		want string
	}{
		{"Debian 12", host{osRelease: "PRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\nID=debian\n", bins: []string{"apt", "apt-get", "dpkg"}, paths: dpkgDB}, "apt"},
		{"Ubuntu 24.04", host{osRelease: "ID=ubuntu\nID_LIKE=debian\n", bins: []string{"apt", "apt-get"}, paths: dpkgDB}, "apt"},
		{"Ubuntu with apt-get only", host{osRelease: "ID=ubuntu\nID_LIKE=debian\n", bins: []string{"apt-get"}, paths: dpkgDB}, "apt"},
		{"Linux Mint", host{osRelease: "ID=linuxmint\nID_LIKE=\"ubuntu debian\"\n", bins: []string{"apt", "apt-get"}, paths: dpkgDB}, "apt"},
		{"Raspberry Pi OS", host{osRelease: "ID=raspbian\nID_LIKE=debian\n", bins: []string{"apt", "apt-get"}, paths: dpkgDB}, "apt"},
		{"Proxmox VE", host{osRelease: "ID=debian\n", bins: []string{"apt", "apt-get"}, paths: dpkgDB}, "apt"},
		{"RHEL 9", host{osRelease: "ID=\"rhel\"\nID_LIKE=\"fedora\"\n", bins: []string{"dnf", "yum"}, paths: rpmDB}, "dnf"},
		{"RHEL 7", host{osRelease: rhel79, bins: []string{"yum"}, paths: rpmDB}, "yum"},
		{"CentOS Stream 9", host{osRelease: "ID=\"centos\"\nID_LIKE=\"rhel fedora\"\n", bins: []string{"dnf", "yum"}, paths: rpmDB}, "dnf"},
		{"AlmaLinux 9", host{osRelease: "ID=\"almalinux\"\nID_LIKE=\"rhel centos fedora\"\n", bins: []string{"dnf", "yum"}, paths: rpmDB}, "dnf"},
		{"Oracle Linux 8", host{osRelease: "ID=\"ol\"\nID_LIKE=\"fedora\"\n", bins: []string{"dnf", "yum"}, paths: rpmDB}, "dnf"},
		{"Fedora 41", host{osRelease: "ID=fedora\n", bins: []string{"dnf"}, paths: map[string]fs.FileMode{"/usr/lib/sysimage/rpm": fs.ModeDir}}, "dnf"},
		{"Amazon Linux 2", host{osRelease: "ID=\"amzn\"\nID_LIKE=\"centos rhel fedora\"\n", bins: []string{"yum"}, paths: rpmDB}, "yum"},
		{"Amazon Linux 2023", host{osRelease: "ID=\"amzn\"\nID_LIKE=\"fedora\"\n", bins: []string{"dnf"}, paths: rpmDB}, "dnf"},
		{"Alpine", host{osRelease: "ID=alpine\n", bins: []string{"apk"}}, "apk"},
		{"Arch", host{osRelease: "ID=arch\n", bins: []string{"pacman"}}, "pacman"},
		{"Manjaro", host{osRelease: "ID=manjaro\nID_LIKE=arch\n", bins: []string{"pacman"}}, "pacman"},
		{"no os-release", host{bins: []string{"apt-get"}}, "apt"},
		{"FreeBSD under rc.d with a minimal PATH", host{goos: "freebsd", uname: "FreeBSD", paths: map[string]fs.FileMode{"/usr/sbin/pkg": exec}}, "pkg"},
		{"FreeBSD with pkg on PATH", host{goos: "freebsd", uname: "FreeBSD", bins: []string{"pkg"}}, "pkg"},
		{"pfSense", host{goos: "freebsd", uname: "FreeBSD", paths: map[string]fs.FileMode{"/usr/local/sbin/pkg": exec}}, "pkg"},
		{"Windows", host{goos: "windows"}, "windows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.host.probe(t)
			if got := p.Detect(); got != tc.want {
				t.Errorf("Detect() = %q, want %q", got, tc.want)
			}
			if tc.host.goos == "" {
				legacy := legacyPathOrder(p)
				if legacy != tc.want {
					t.Errorf("fixture disagrees with the previous PATH-only order: %q", legacy)
				}
			}
		})
	}
}

func legacyPathOrder(p Probe) string {
	for _, pair := range [][2]string{{"apk", "apk"}, {"apt", "apt"}, {"apt-get", "apt"}, {"dnf", "dnf"}, {"yum", "yum"}, {"pacman", "pacman"}} {
		if p.has(pair[0]) {
			return pair[1]
		}
	}
	return "unknown"
}

func TestDetectSkipsAnOSReleaseWithoutID(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "etc-os-release")
	full := filepath.Join(dir, "usr-lib-os-release")
	if err := os.WriteFile(empty, []byte("# placeholder\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(rhel79), 0o600); err != nil {
		t.Fatal(err)
	}
	p := host{bins: []string{"apt", "yum"}}.probe(t)
	p.OSReleasePaths = []string{empty, full}
	if got := p.Detect(); got != "yum" {
		t.Errorf("Detect() = %q, want yum from the second os-release", got)
	}
}

func TestDetectSUSE(t *testing.T) {
	rpmDB := map[string]fs.FileMode{"/var/lib/rpm": fs.ModeDir}
	cases := []struct {
		name string
		host host
		want string
	}{
		{"openSUSE Leap 15.6", host{osRelease: "ID=\"opensuse-leap\"\nID_LIKE=\"suse opensuse\"\n", bins: []string{"zypper", "rpm"}, paths: map[string]fs.FileMode{"/usr/lib/sysimage/rpm": fs.ModeDir}}, "zypper"},
		{"openSUSE Tumbleweed", host{osRelease: "ID=\"opensuse-tumbleweed\"\nID_LIKE=\"opensuse suse\"\n", bins: []string{"zypper", "rpm"}, paths: map[string]fs.FileMode{"/usr/lib/sysimage/rpm": fs.ModeDir}}, "zypper"},
		{"SLES 15, ID_LIKE suse only", host{osRelease: "ID=\"sles\"\nID_LIKE=\"suse\"\n", bins: []string{"zypper", "rpm"}, paths: rpmDB}, "zypper"},
		{"SLES 15 with dnf installed from a module", host{osRelease: "ID=\"sles\"\nID_LIKE=\"suse\"\n", bins: []string{"zypper", "rpm", "dnf"}, paths: rpmDB}, "zypper"},
		{"rpm host without os-release and only zypper", host{bins: []string{"zypper", "rpm"}, paths: rpmDB}, "zypper"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.host.probe(t).Detect(); got != tc.want {
				t.Errorf("Detect() = %q, want %q", got, tc.want)
			}
		})
	}
}
