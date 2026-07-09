package distro

import "strings"

// KernelPackage is an installed kernel-related package on a host.
type KernelPackage struct {
	Name    string
	Version string
}

// SelectRunningKernel picks the installed kernel package that corresponds to
// the host's running kernel, so its version can be compared against a distro's
// fixed version. It matches the package to the uname where possible and falls
// back to the highest-versioned kernel package for the distro family.
//
// Debian/Ubuntu/Proxmox: the booted kernel is the linux-image-<uname> package
// (its version, e.g. 6.8.0-51.52, matches the distro's fixed version format).
// RHEL/Fedora/CentOS: the kernel/kernel-core package whose version is the
// uname minus the architecture suffix.
func SelectRunningKernel(osType, uname string, pkgs []KernelPackage) KernelPackage {
	family := NormalizeDistro(osType)
	uname = strings.TrimSpace(uname)

	switch family {
	case "ubuntu", "debian", "proxmox":
		return selectDebKernel(uname, pkgs)
	case "rhel", "fedora", "centos":
		return selectRPMKernel(uname, pkgs)
	default:
		return selectDebKernel(uname, pkgs)
	}
}

func selectDebKernel(uname string, pkgs []KernelPackage) KernelPackage {
	// Exact match: linux-image-<uname>.
	if uname != "" {
		want := "linux-image-" + uname
		for _, p := range pkgs {
			if p.Name == want {
				return p
			}
		}
	}
	// Highest-versioned image/kernel package, preferring versioned images.
	var best KernelPackage
	for _, p := range pkgs {
		if !isDebKernelName(p.Name) {
			continue
		}
		if best.Version == "" || CompareDpkg(p.Version, best.Version) > 0 {
			best = p
		}
	}
	return best
}

func isDebKernelName(name string) bool {
	return name == "linux" ||
		strings.HasPrefix(name, "linux-image-") ||
		strings.HasPrefix(name, "pve-kernel-") ||
		strings.HasPrefix(name, "proxmox-kernel-")
}

func selectRPMKernel(uname string, pkgs []KernelPackage) KernelPackage {
	// uname is like "5.14.0-427.el9.x86_64"; the package version is the same
	// without the trailing ".<arch>". Prefer the package whose version is a
	// prefix of the uname.
	unameVer := stripArch(uname)
	var prefixMatch, best KernelPackage
	for _, p := range pkgs {
		if p.Name != "kernel" && p.Name != "kernel-core" {
			continue
		}
		if unameVer != "" && (p.Version == unameVer || strings.HasPrefix(unameVer, p.Version)) {
			if prefixMatch.Version == "" || CompareRPM(p.Version, prefixMatch.Version) > 0 {
				prefixMatch = p
			}
		}
		if best.Version == "" || CompareRPM(p.Version, best.Version) > 0 {
			best = p
		}
	}
	if prefixMatch.Version != "" {
		return prefixMatch
	}
	return best
}

// stripArch removes a trailing ".x86_64"/".aarch64"/... architecture suffix.
func stripArch(uname string) string {
	for _, arch := range []string{".x86_64", ".aarch64", ".ppc64le", ".s390x", ".armv7hl", ".i686"} {
		if strings.HasSuffix(uname, arch) {
			return strings.TrimSuffix(uname, arch)
		}
	}
	return uname
}
