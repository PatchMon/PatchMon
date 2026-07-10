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
	"time"
)

// DataEntry describes the freshness of one cached data set (e.g. a distro's
// per-release OVAL index): when it was last refreshed, whether it succeeded,
// how many CVEs it holds and the newest CVE it knows about.
type DataEntry struct {
	Source      string     `json:"source"`
	Kind        string     `json:"kind"`
	LastAttempt *time.Time `json:"last_attempt,omitempty"`
	LastSuccess *time.Time `json:"last_success,omitempty"`
	OK          bool       `json:"ok"`
	Error       string     `json:"error,omitempty"`
	Count       int        `json:"count"`
	Newest      string     `json:"newest_cve,omitempty"`
	NewestDate  string     `json:"newest_cve_date,omitempty"`
	FromDisk    bool       `json:"from_disk"`
}

// newestCVEKey returns the highest CVE id (by year then number) among a map's
// keys, used to report how fresh a bulk source's data is.
func newestCVEKey[V any](m map[string]V) string {
	best := ""
	var by, bn int
	for id := range m {
		p := strings.Split(id, "-")
		if len(p) != 3 {
			continue
		}
		y, n := atoiSafe(p[1]), atoiSafe(p[2])
		if y > by || (y == by && n > bn) {
			by, bn, best = y, n, id
		}
	}
	return best
}

// StatusReporter is an optional Source capability exposing data-freshness info.
type StatusReporter interface {
	DataStatus() []DataEntry
}

// freshness records the outcome of the most recent load of a bulk source
// (Debian tracker, CentOS/AlmaLinux errata) so it can report DataStatus without
// each source re-implementing the bookkeeping.
type freshness struct {
	lastAttempt *time.Time
	lastSuccess *time.Time
	ok          bool
	err         string
	count       int
	newest      string
	fromDisk    bool
}

// attempt stamps the start of a load. Caller must hold the source's lock.
func (f *freshness) attempt(now time.Time) { f.lastAttempt = &now }

// fail records a load error. Caller must hold the source's lock.
func (f *freshness) fail(err error) {
	f.ok = false
	if err != nil {
		f.err = err.Error()
	}
}

// success records a completed load (fromDisk true when served from the on-disk
// cache rather than a fresh network fetch). Caller must hold the source's lock.
func (f *freshness) success(now time.Time, count int, newest string, fromDisk bool) {
	f.lastSuccess = &now
	f.ok = true
	f.err = ""
	f.count = count
	f.newest = newest
	f.fromDisk = fromDisk
}

// entry builds the DataEntry snapshot for this source. Caller must hold the
// source's lock.
func (f *freshness) entry(source, kind string) DataEntry {
	return DataEntry{
		Source:      source,
		Kind:        kind,
		LastAttempt: f.lastAttempt,
		LastSuccess: f.lastSuccess,
		OK:          f.ok,
		Error:       f.err,
		Count:       f.count,
		Newest:      f.newest,
		FromDisk:    f.fromDisk,
	}
}

// SourcesStatus aggregates freshness across all sources that report it.
func (e *Evaluator) SourcesStatus() []DataEntry {
	var out []DataEntry
	for _, s := range e.sources {
		if sr, ok := s.(StatusReporter); ok {
			out = append(out, sr.DataStatus()...)
		}
	}
	return out
}

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

// HostResolver is an optional Source capability: when the primary (bulk) source
// yields no verdict for a specific host's kernel, provide a richer per-host
// AdvisorySet (e.g. by fetching the live per-CVE API). Used to catch cases the
// bulk feed omits — notably Ubuntu OVAL, which lists fixed versions but not
// "affected, no fix yet" series like an EOL HWE kernel.
type HostResolver interface {
	ResolveForHost(ctx context.Context, cve string, h Host) (*AdvisorySet, bool)
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
	KernelVersion    string // running uname
	KernelPkgName    string // running kernel package name (may be empty)
	KernelPkgVersion string // running kernel package version (may be empty)
	// InstalledKernels are all installed kernel packages (name+version). Used to
	// tell "vulnerable, reboot required" (a fixed kernel is installed but not
	// booted) from "vulnerable, update required" (no fixed kernel installed).
	InstalledKernels []KernelPackage
}

// Result is the per-host evaluation outcome. Status reflects the RUNNING kernel;
// RebootRequired is set when the host is vulnerable now but a non-vulnerable
// kernel is already installed (just needs a reboot).
type Result struct {
	Status         Status `json:"status"`
	FixedVersion   string `json:"fixed_version,omitempty"`
	Release        string `json:"release,omitempty"`
	Source         string `json:"source,omitempty"`
	Distro         string `json:"distro,omitempty"`
	RebootRequired bool   `json:"reboot_required,omitempty"`
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

	// Status of the currently RUNNING kernel — this is the host's live exposure.
	res := e.resolveKernel(src, set, family, h.OSVersion, h.KernelVersion, h.KernelPkgVersion)

	// If the bulk source had no verdict for this host's kernel (e.g. Ubuntu OVAL
	// lists only fixed series, not an affected-no-fix HWE kernel), ask the source
	// for richer per-host data (live per-CVE API) and re-evaluate against it.
	if res.Status == StatusUnknown {
		if hr, ok := src.(HostResolver); ok {
			if jset, ok2 := hr.ResolveForHost(ctx, cve, h); ok2 && jset != nil && jset.Known {
				set = jset
				res = e.resolveKernel(src, set, family, h.OSVersion, h.KernelVersion, h.KernelPkgVersion)
			}
		}
	}

	// If the running kernel is vulnerable, check whether any already-installed
	// kernel is safe (fixed/not-affected). If so, the fix is on disk and the
	// host only needs a reboot; otherwise a package update is required first.
	if res.Status == StatusVulnerable {
		for _, k := range h.InstalledKernels {
			if k.Version == "" {
				continue
			}
			st := e.resolveKernel(src, set, family, h.OSVersion, k.Version, k.Version)
			if st.Status == StatusPatched || st.Status == StatusNotAffected {
				res.RebootRequired = true
				break
			}
		}
	}
	return res
}

// resolveKernel evaluates a single kernel against the advisory set. kernelVersion
// determines the kernel series (for series-aware sources); pkgVersion is the
// installed package version compared against the distro's fixed version.
func (e *Evaluator) resolveKernel(src Source, set *AdvisorySet, family, osVersion, kernelVersion, pkgVersion string) Result {
	probe := Host{OSType: family, OSVersion: osVersion, KernelVersion: kernelVersion, KernelPkgVersion: pkgVersion}
	var adv Advisory
	var ok bool
	if hm, isHM := src.(HostMatcher); isHM {
		adv, ok = hm.SelectAdvisory(set, probe)
	} else {
		rel := src.ReleaseKey(osVersion)
		if rel == "" {
			return Result{Status: StatusUnknown, Distro: family, Source: set.Source}
		}
		adv, ok = set.ByRelease[rel]
	}
	if !ok {
		return Result{Status: StatusUnknown, Distro: family, Release: src.ReleaseKey(osVersion), Source: set.Source}
	}

	res := Result{Distro: family, Release: adv.Release, Source: set.Source, FixedVersion: adv.FixedVersion}
	switch adv.Decision {
	case DecisionNotAffected:
		res.Status = StatusNotAffected
	case DecisionAffected:
		res.Status = StatusVulnerable
	case DecisionFixed:
		if adv.FixedVersion == "" || pkgVersion == "" {
			res.Status = StatusUnknown
			return res
		}
		if src.CompareVersions(pkgVersion, adv.FixedVersion) >= 0 {
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
