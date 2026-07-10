package distro

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Fedora source. Mirrors cve-patch-duration/fetch_fedora.py, which drives the
// Bodhi update system (https://bodhi.fedoraproject.org). The Python fetches the
// kernel updates and maps a CVE to the Fedora kernel build that carries the fix;
// here we query Bodhi directly for the updates that reference the CVE and ship
// the `kernel` package, then derive a per-release verdict from each update's
// status.
//
// Endpoint (Bodhi updates API, returns JSON):
//
//	https://bodhi.fedoraproject.org/updates/?packages=kernel&search={CVE}
//
// A kernel update in status "stable" gives the fixed kernel NVR for its Fedora
// release (e.g. F39 -> 6.8.9-100.fc39 => DecisionFixed). An update still in
// "testing"/"pending" means work is in flight => DecisionAffected. If no Fedora
// kernel update references the CVE at all, the set is Known=false.
type Fedora struct {
	cache   *advisoryCache
	baseURL string
}

// NewFedora creates a Fedora source using the live Bodhi API.
func NewFedora() *Fedora {
	return &Fedora{cache: newAdvisoryCache("fedora"), baseURL: "https://bodhi.fedoraproject.org"}
}

func (f *Fedora) Distro() string { return "fedora" }

// DataStatus reports how many CVEs this on-demand source has cached so far.
func (f *Fedora) DataStatus() []DataEntry {
	return []DataEntry{{
		Source: "fedora-bodhi",
		Kind:   "on-demand",
		OK:     true,
		Count:  f.cache.count(),
		Newest: f.cache.newest(),
	}}
}

func (f *Fedora) CompareVersions(a, b string) int { return CompareRPM(a, b) }

// fedoraRelNameRe extracts the Fedora major from a Bodhi release name such as
// "F39" -> "39". Non-Fedora releases (e.g. "EPEL-9") do not match.
var fedoraRelNameRe = regexp.MustCompile(`^F(\d+)`)

// fedoraLeadIntRe extracts the leading integer of a host OS version string.
var fedoraLeadIntRe = regexp.MustCompile(`\d+`)

// kernelNVRRe matches the base `kernel` build NVR and captures the version-
// release (e.g. "kernel-6.8.9-100.fc39" -> "6.8.9-100.fc39"). Requiring a digit
// right after "kernel-" excludes subpackages like kernel-headers/kernel-tools.
var kernelNVRRe = regexp.MustCompile(`^kernel-(\d+\.\d+\.\d+-\S+)$`)

// ReleaseKey maps a host OS version to the Fedora major used as the ByRelease
// key ("40" -> "40", "39 (Rawhide)" -> "39").
func (f *Fedora) ReleaseKey(osVersion string) string {
	return fedoraLeadIntRe.FindString(strings.TrimSpace(osVersion))
}

// fedoraReleaseKey maps a Bodhi release name (e.g. "F39") to a major ("39").
func fedoraReleaseKey(name string) string {
	if m := fedoraRelNameRe.FindStringSubmatch(strings.TrimSpace(name)); m != nil {
		return m[1]
	}
	return ""
}

type bodhiResp struct {
	Updates []bodhiUpdate `json:"updates"`
	Page    int           `json:"page"`
	Pages   int           `json:"pages"`
	Total   int           `json:"total"`
}

type bodhiUpdate struct {
	Alias   string `json:"alias"`
	Status  string `json:"status"`
	Release struct {
		Name string `json:"name"`
	} `json:"release"`
	Builds []struct {
		NVR string `json:"nvr"`
	} `json:"builds"`
}

func (f *Fedora) Advisories(ctx context.Context, cve string) (*AdvisorySet, error) {
	cve = strings.ToUpper(strings.TrimSpace(cve))
	return f.cache.do(cve, func(fctx context.Context) (*AdvisorySet, error) {
		return f.fetch(fctx, cve)
	})
}

func (f *Fedora) fetch(ctx context.Context, cve string) (*AdvisorySet, error) {
	source := fmt.Sprintf("%s/updates/?packages=kernel&search=%s", f.baseURL, cve)
	set := &AdvisorySet{CVEID: cve, Distro: "fedora", Source: source, ByRelease: map[string]Advisory{}}

	var all []bodhiUpdate
	for page := 1; page <= 20; page++ {
		u := fmt.Sprintf("%s/updates/?packages=kernel&search=%s&rows_per_page=50&page=%d",
			f.baseURL, url.QueryEscape(cve), page)
		var resp bodhiResp
		found, err := getJSON(ctx, u, nil, &resp)
		if err != nil {
			return nil, err
		}
		if !found {
			break
		}
		all = append(all, resp.Updates...)
		if len(resp.Updates) == 0 || resp.Pages <= page {
			break
		}
	}

	f.parse(all, set)
	// Known reflects whether any Fedora kernel update references the CVE.
	set.Known = len(set.ByRelease) > 0
	return set, nil
}

// fedoraCand is a per-release candidate verdict during aggregation.
type fedoraCand struct {
	dec   Decision
	fixed string
}

// parse fills set.ByRelease from the Bodhi updates. Per Fedora release it keeps
// the most conclusive verdict: a "stable" update (DecisionFixed) beats a
// pending/testing one (DecisionAffected); among several stable updates it keeps
// the lowest kernel NVR (the earliest fix).
func (f *Fedora) parse(updates []bodhiUpdate, set *AdvisorySet) {
	best := map[string]fedoraCand{}
	for _, u := range updates {
		rel := fedoraReleaseKey(u.Release.Name)
		if rel == "" {
			continue // not a Fedora release (e.g. EPEL)
		}
		var dec Decision
		switch strings.ToLower(strings.TrimSpace(u.Status)) {
		case "stable":
			dec = DecisionFixed
		case "testing", "pending":
			dec = DecisionAffected
		default:
			continue // obsolete/unpushed: not actionable
		}
		ver := ""
		for _, b := range u.Builds {
			if m := kernelNVRRe.FindStringSubmatch(b.NVR); m != nil {
				ver = m[1]
				break
			}
		}
		// A "fixed" verdict is only meaningful with a concrete kernel NVR; an
		// in-flight (affected) update carries no released fix version.
		if dec == DecisionFixed && ver == "" {
			continue
		}
		if dec != DecisionFixed {
			ver = ""
		}
		cand := fedoraCand{dec: dec, fixed: ver}
		if cur, ok := best[rel]; ok {
			best[rel] = mergeFedora(cur, cand)
		} else {
			best[rel] = cand
		}
	}
	for rel, c := range best {
		set.ByRelease[rel] = Advisory{Release: rel, Decision: c.dec, FixedVersion: c.fixed}
	}
}

// mergeFedora combines two candidates for the same release. A shipped fix
// (DecisionFixed) is authoritative over an in-flight update; between two fixes
// the lower NVR (earliest fix) is kept.
func mergeFedora(a, b fedoraCand) fedoraCand {
	rank := func(d Decision) int {
		if d == DecisionFixed {
			return 2
		}
		return 1
	}
	if rank(b.dec) > rank(a.dec) {
		return b
	}
	if rank(b.dec) < rank(a.dec) {
		return a
	}
	if a.dec == DecisionFixed && b.fixed != "" && (a.fixed == "" || CompareRPM(b.fixed, a.fixed) < 0) {
		return b
	}
	return a
}
