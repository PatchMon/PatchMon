// Package distro determines, per Linux distribution, whether a host is affected
// by a kernel CVE by consulting the same security sources and applying the same
// logic as the cve-patch-duration project (github.com/WMP/cve-patch-duration):
//
//   - Ubuntu:  ubuntu.com/security/cves/{CVE}.json      (pkg statuses per release)
//   - Debian:  security-tracker.debian.org/.../data/json (linux fixed_version)
//   - RHEL:    access.redhat.com CSAF VEX               (fixed / (not_)affected)
//   - Fedora:  bodhi.fedoraproject.org                  (kernel updates)
//   - Proxmox: derived from the Ubuntu kernel it ships
//   - CentOS:  AlmaLinux errata as a RHEL proxy
//
// The design separates two concerns so the runtime can evolve from live lookups
// to a periodic DB sync without touching evaluation logic:
//
//   - Source     — fetches+parses a distro's advisory for a CVE. Host-independent
//     and cacheable. This is the seam: a live (HTTP) implementation
//     today; a DB-backed one reading a synced table later.
//   - Evaluator  — compares the affected host's installed kernel *package* version
//     against the distro's fixed version (dpkg/rpm semantics) to
//     produce a per-host Status. Shared across all distros.
package distro

import (
	"context"
	"strings"
)

// Status is the per-host verdict for a CVE.
type Status string

const (
	// StatusVulnerable: the host's distro is affected and the host's installed
	// kernel package is older than the fix (or no fix exists yet).
	StatusVulnerable Status = "vulnerable"
	// StatusPatched: the distro released a fix and the host's kernel package is
	// at or above the fixed version.
	StatusPatched Status = "patched"
	// StatusNotAffected: the distro declared this CVE does not affect its kernel.
	StatusNotAffected Status = "not_affected"
	// StatusUnknown: no data (CVE not tracked by the distro, unknown release,
	// missing package version, or the source could not be reached).
	StatusUnknown Status = "unknown"
)

// Decision is a distro's host-independent verdict for a CVE in one release.
type Decision string

const (
	DecisionFixed       Decision = "fixed"        // fix released at FixedVersion
	DecisionAffected    Decision = "affected"     // affected, no fix available yet
	DecisionNotAffected Decision = "not_affected" // declared not affected
	DecisionUnknown     Decision = "unknown"
)

// Advisory is a distro's verdict for one CVE in one release.
type Advisory struct {
	Release      string   `json:"release"`
	Decision     Decision `json:"decision"`
	FixedVersion string   `json:"fixed_version,omitempty"`
}

// AdvisorySet is the host-independent result of consulting a distro's security
// source for one CVE. It is the cacheable unit and the seam for a future
// DB-backed sync (persist AdvisorySet per (distro, cve); serve it from the DB
// instead of the network).
type AdvisorySet struct {
	CVEID     string              `json:"cve_id"`
	Distro    string              `json:"distro"`
	Source    string              `json:"source"`
	Known     bool                `json:"known"` // false: CVE absent from this distro's tracker
	ByRelease map[string]Advisory `json:"by_release"`
}

// Source consults a distro's security data for a CVE. Implementations MUST be
// host-independent and safe for concurrent use; results should be cached.
type Source interface {
	// Distro returns the canonical distro family key this source handles
	// (e.g. "ubuntu", "debian", "rhel", "fedora", "proxmox", "centos").
	Distro() string
	// Advisories fetches and parses the distro's verdict for cve. It returns a
	// set with Known=false (not an error) when the CVE is absent from the
	// distro's tracker.
	Advisories(ctx context.Context, cve string) (*AdvisorySet, error)
	// ReleaseKey maps a host OS version string to the key used in
	// AdvisorySet.ByRelease (e.g. Ubuntu "24.04"->"noble", Debian "12"->
	// "bookworm", RHEL "9.4"->"9"). It returns "" when the release is unknown.
	ReleaseKey(osVersion string) string
	// CompareVersions compares two kernel package versions using this distro's
	// package-manager semantics (dpkg or rpm). Returns -1, 0 or 1.
	CompareVersions(a, b string) int
}

// HostMatcher is an optional Source capability for distributions where a single
// release ships multiple kernel series simultaneously (e.g. Ubuntu HWE: jammy
// carries both the 5.15 GA kernel and rolling 5.19/6.2/6.5/6.8 HWE kernels).
// Such a source keys its advisories per kernel series and selects the one that
// matches the host's running kernel. Sources that don't implement it fall back
// to a plain ByRelease[ReleaseKey(osVersion)] lookup.
type HostMatcher interface {
	SelectAdvisory(set *AdvisorySet, h Host) (Advisory, bool)
}

// Host carries what the evaluator needs about a single host.
type Host struct {
	OSType           string // raw os_type from the host record
	OSVersion        string // raw os_version (e.g. "24.04", "12", "9.4")
	KernelVersion    string // running uname (fallback when no package version)
	KernelPkgName    string // installed kernel package name (may be empty)
	KernelPkgVersion string // installed kernel package current_version (may be empty)
}

// Result is the per-host evaluation outcome.
type Result struct {
	Status       Status `json:"status"`
	FixedVersion string `json:"fixed_version,omitempty"`
	Release      string `json:"release,omitempty"`
	Source       string `json:"source,omitempty"`
	Distro       string `json:"distro,omitempty"`
}

// Evaluator resolves per-host CVE status across the registered distro sources.
type Evaluator struct {
	sources map[string]Source
}

// NewEvaluator builds an evaluator from the given sources, keyed by Distro().
func NewEvaluator(sources ...Source) *Evaluator {
	m := make(map[string]Source, len(sources))
	for _, s := range sources {
		if s != nil {
			m[s.Distro()] = s
		}
	}
	return &Evaluator{sources: m}
}

// NormalizeDistro maps a raw host os_type to a canonical distro family key.
func NormalizeDistro(osType string) string {
	t := strings.ToLower(strings.TrimSpace(osType))
	switch {
	case strings.Contains(t, "ubuntu"):
		return "ubuntu"
	case strings.Contains(t, "debian"):
		return "debian"
	case strings.Contains(t, "proxmox"), strings.Contains(t, "pve"):
		return "proxmox"
	case strings.Contains(t, "alma"), strings.Contains(t, "rocky"), strings.Contains(t, "centos"):
		return "centos"
	case strings.Contains(t, "fedora"):
		return "fedora"
	case strings.Contains(t, "rhel"), strings.Contains(t, "red hat"), strings.Contains(t, "redhat"):
		return "rhel"
	default:
		return t
	}
}

// SupportsDistro reports whether a source is registered for the host's distro.
func (e *Evaluator) SupportsDistro(osType string) bool {
	_, ok := e.sources[NormalizeDistro(osType)]
	return ok
}

// Evaluate determines the CVE status for a single host. It never returns an
// error for "no data" cases — those map to StatusUnknown — so a filter can
// treat a failed/absent lookup as inconclusive rather than aborting.
func (e *Evaluator) Evaluate(ctx context.Context, cve string, h Host) Result {
	family := NormalizeDistro(h.OSType)
	src := e.sources[family]
	if src == nil {
		return Result{Status: StatusUnknown, Distro: family}
	}
	set, err := src.Advisories(ctx, cve)
	if err != nil || set == nil || !set.Known {
		return Result{Status: StatusUnknown, Distro: family, Source: sourceURL(set)}
	}

	// Select the advisory for this host. Series-aware sources (Ubuntu, Proxmox)
	// pick by the host's running kernel series; the rest key by release.
	var adv Advisory
	var ok bool
	if hm, isHM := src.(HostMatcher); isHM {
		adv, ok = hm.SelectAdvisory(set, h)
	} else {
		rel := src.ReleaseKey(h.OSVersion)
		if rel == "" {
			return Result{Status: StatusUnknown, Distro: family, Source: set.Source}
		}
		adv, ok = set.ByRelease[rel]
	}
	if !ok {
		return Result{Status: StatusUnknown, Distro: family, Release: src.ReleaseKey(h.OSVersion), Source: set.Source}
	}

	res := Result{Distro: family, Release: adv.Release, Source: set.Source, FixedVersion: adv.FixedVersion}
	switch adv.Decision {
	case DecisionNotAffected:
		res.Status = StatusNotAffected
	case DecisionAffected:
		// Affected with no fix available: the host is vulnerable regardless of
		// its installed version.
		res.Status = StatusVulnerable
	case DecisionFixed:
		if adv.FixedVersion == "" || h.KernelPkgVersion == "" {
			res.Status = StatusUnknown
			return res
		}
		if src.CompareVersions(h.KernelPkgVersion, adv.FixedVersion) >= 0 {
			res.Status = StatusPatched
		} else {
			res.Status = StatusVulnerable
		}
	default:
		res.Status = StatusUnknown
	}
	return res
}

func sourceURL(set *AdvisorySet) string {
	if set == nil {
		return ""
	}
	return set.Source
}
