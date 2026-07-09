package distro

import (
	"context"
	"regexp"
	"strings"
)

// Proxmox source. Mirrors cve-patch-duration/fetch_proxmox.py + fetch_proxmox_psa.py.
//
// Proxmox VE ships an Ubuntu-derived kernel (historically pve-kernel, now
// proxmox-kernel). The Python project does not have a first-class Proxmox CVE
// tracker; instead it derives the Proxmox fix from the *underlying Ubuntu
// kernel* the pve-kernel changelog imports: each changelog entry carries an
// "update sources to Ubuntu-X.Y.Z-N.M" marker, and that Ubuntu package version
// is matched against Ubuntu's own tracker (the `linux` / `linux-hwe-*` LTS
// packages) to learn which CVEs the imported base already fixes. A forum PSA
// (Proxmox Security Advisory) feed is used as a higher-priority override.
//
// Rather than re-scraping the pve-kernel changelog and the forum here, this Go
// source takes the faithful shortcut the semantics imply: it DELEGATES to an
// Ubuntu Source for the CVE, then remaps Ubuntu's codename-keyed verdicts onto
// Proxmox VE major versions using the documented VE-major -> Ubuntu-base
// mapping below. This is a best-effort proxy:
//
//   - The verdict (fixed / affected / not_affected) is inherited directly from
//     the Ubuntu base kernel that the corresponding Proxmox VE series imports.
//   - The FixedVersion is the *Ubuntu* kernel package version, NOT the Proxmox
//     package version (e.g. proxmox-kernel-6.8.x-pve). Proxmox rebases the
//     Ubuntu kernel roughly in lockstep and uses dpkg-comparable versions, so
//     comparing an installed Proxmox kernel version against the Ubuntu fixed
//     version is directionally correct but not exact. This mirrors the
//     ubuntu_base match_type in fetch_proxmox.py.
//   - The forum-PSA override and the upstream stable-tag fallback from the
//     Python are intentionally omitted; the Ubuntu-base signal is the dominant
//     one and keeps this source dependency-free and offline-testable.
type Proxmox struct {
	ubuntu *Ubuntu
	cache  *advisoryCache
}

// NewProxmox creates a Proxmox source that derives its verdicts from the given
// Ubuntu source. The Ubuntu source is consulted (and cached by Ubuntu) for the
// actual CVE data; Proxmox only remaps the result onto VE major keys.
func NewProxmox(ub *Ubuntu) *Proxmox {
	return &Proxmox{ubuntu: ub, cache: newAdvisoryCache()}
}

func (p *Proxmox) Distro() string { return "proxmox" }

// CompareVersions uses dpkg semantics: Proxmox kernels are Debian/Ubuntu-based
// and versioned with dpkg-comparable strings.
func (p *Proxmox) CompareVersions(a, b string) int { return CompareDpkg(a, b) }

// proxmoxToUbuntu maps a Proxmox VE major version to the Ubuntu kernel
// codename(s) whose kernel that VE series is derived from. When a VE series has
// shipped more than one base over its life (e.g. VE 8 started on the jammy 6.2
// HWE kernel and moved to the noble 6.8 kernel), all relevant bases are listed
// and merged with the same "most actionable verdict wins" rule Ubuntu uses.
//
// Sources: pve-kernel changelog branches in fetch_proxmox.py (bullseye-6.2,
// bookworm-6.2/6.5/6.8/6.11/6.14, trixie-6.14/6.17) and the Proxmox VE roadmap:
//
//	VE 9  (Debian 13 trixie)  -> noble  (6.14/6.17 base)
//	VE 8  (Debian 12 bookworm)-> noble (6.8/6.11/6.14), jammy (6.2 HWE)
//	VE 7  (Debian 11 bullseye)-> focal  (5.4/5.15 base)
//	VE 6  (Debian 10 buster)  -> bionic (5.4 base)
var proxmoxToUbuntu = map[string][]string{
	"9": {"noble"},
	"8": {"noble", "jammy"},
	"7": {"focal"},
	"6": {"bionic"},
}

var proxmoxVerRe = regexp.MustCompile(`(\d+)`)

// ReleaseKey extracts the leading Proxmox VE major version ("8.2" -> "8",
// "7" -> "7"). It returns "" when no integer is present.
func (p *Proxmox) ReleaseKey(osVersion string) string {
	if m := proxmoxVerRe.FindStringSubmatch(strings.TrimSpace(osVersion)); m != nil {
		return m[1]
	}
	return ""
}

func (p *Proxmox) Advisories(ctx context.Context, cve string) (*AdvisorySet, error) {
	cve = strings.ToUpper(strings.TrimSpace(cve))
	return p.cache.do(cve, func(fctx context.Context) (*AdvisorySet, error) {
		return p.build(fctx, cve)
	})
}

func (p *Proxmox) build(ctx context.Context, cve string) (*AdvisorySet, error) {
	set := &AdvisorySet{
		CVEID:     cve,
		Distro:    "proxmox",
		Source:    "derived from Ubuntu kernel base",
		ByRelease: map[string]Advisory{},
	}
	if p.ubuntu == nil {
		return set, nil
	}
	ub, err := p.ubuntu.Advisories(ctx, cve)
	if err != nil {
		return nil, err
	}
	if ub == nil || !ub.Known {
		return set, nil
	}
	// Pass the Ubuntu advisories through unchanged (keyed by Ubuntu
	// codename/series); SelectAdvisory maps a Proxmox host onto them.
	set.Known = true
	set.Source = ub.Source + " (Proxmox kernel derived from Ubuntu base)"
	for k, adv := range ub.ByRelease {
		set.ByRelease[k] = adv
	}
	return set, nil
}

// SelectAdvisory maps a Proxmox host to the underlying Ubuntu kernel advisory:
// the host's VE major picks the Ubuntu base codename(s), and the host's running
// kernel series picks the series. Verdicts across candidate bases are merged
// (affected > fixed > lowest-version).
func (p *Proxmox) SelectAdvisory(set *AdvisorySet, h Host) (Advisory, bool) {
	veMajor := p.ReleaseKey(h.OSVersion)
	bases := proxmoxToUbuntu[veMajor]
	if len(bases) == 0 {
		return Advisory{}, false
	}
	series := kernelSeries(h.KernelVersion)
	if series == "" {
		series = kernelSeries(h.KernelPkgVersion)
	}
	if series == "" {
		return Advisory{}, false
	}
	var cur ubuntuCand
	have := false
	for _, code := range bases {
		adv, ok := set.ByRelease[code+"/"+series]
		if !ok {
			continue
		}
		cand := ubuntuCand{decision: adv.Decision, fixed: adv.FixedVersion}
		if !have {
			cur, have = cand, true
		} else {
			cur = mergeCand(cur, cand)
		}
	}
	if !have {
		return Advisory{}, false
	}
	return Advisory{Release: veMajor + "/" + series, Decision: cur.decision, FixedVersion: cur.fixed}, true
}
