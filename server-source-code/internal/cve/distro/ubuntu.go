package distro

import (
	"context"
	"regexp"
	"strings"
)

// Ubuntu source. Mirrors cve-patch-duration/fetch_ubuntu.py: it reads
// ubuntu.com/security/cves/{CVE}.json and, for kernel packages, derives a
// per-release verdict from the `statuses` array (released → fixed at the
// version in `description`; needed/needs-triage/pending/deferred/ignored →
// affected; not-affected/DNE → not affected).
type Ubuntu struct {
	cache   *advisoryCache
	baseURL string
}

// NewUbuntu creates an Ubuntu source using the live security API.
func NewUbuntu() *Ubuntu {
	return &Ubuntu{cache: newAdvisoryCache("ubuntu"), baseURL: "https://ubuntu.com/security/cves"}
}

func (u *Ubuntu) Distro() string { return "ubuntu" }

func (u *Ubuntu) CompareVersions(a, b string) int { return CompareDpkg(a, b) }

// ubuntuReleases maps Ubuntu version numbers to codenames used by the tracker.
var ubuntuReleases = map[string]string{
	"14.04": "trusty", "16.04": "xenial", "18.04": "bionic", "20.04": "focal",
	"22.04": "jammy", "23.10": "mantic", "24.04": "noble", "24.10": "oracular",
	"25.04": "plucky", "25.10": "questing",
}

var ubuntuVerRe = regexp.MustCompile(`(\d{2}\.\d{2})`)

func (u *Ubuntu) ReleaseKey(osVersion string) string {
	v := strings.ToLower(strings.TrimSpace(osVersion))
	// Already a codename?
	for _, code := range ubuntuReleases {
		if strings.Contains(v, code) {
			return code
		}
	}
	if m := ubuntuVerRe.FindStringSubmatch(v); m != nil {
		if code, ok := ubuntuReleases[m[1]]; ok {
			return code
		}
	}
	return ""
}

// hweRe matches the generic HWE kernel source package, e.g. "linux-hwe-6.8".
var hweRe = regexp.MustCompile(`^linux-hwe-\d+\.\d+$`)

// isGenericUbuntuKernelPkg matches only the source packages that build the
// *generic* kernel a typical host runs: the GA "linux" and the HWE
// "linux-hwe-<M.N>". It deliberately excludes flavour/cloud variants
// (linux-aws, linux-azure*, linux-gcp, linux-lowlatency*, linux-realtime,
// linux-oem, linux-nvidia, linux-oracle, linux-ibm, linux-riscv, ...), because
// merging their statuses into one series would let, say, a "needed"
// linux-azure-fde-6.8 wrongly mark a fixed generic linux-hwe-6.8 as affected.
// Non-generic hosts are handled as "unknown" (a known limitation).
func isGenericUbuntuKernelPkg(name string) bool {
	return name == "linux" || hweRe.MatchString(name)
}

type ubuntuCVE struct {
	Packages []struct {
		Name     string `json:"name"`
		Statuses []struct {
			Status          string `json:"status"`
			ReleaseCodename string `json:"release_codename"`
			Description     string `json:"description"`
		} `json:"statuses"`
	} `json:"packages"`
}

func (u *Ubuntu) Advisories(ctx context.Context, cve string) (*AdvisorySet, error) {
	cve = strings.ToUpper(strings.TrimSpace(cve))
	return u.cache.do(cve, func(fctx context.Context) (*AdvisorySet, error) {
		return u.fetch(fctx, cve)
	})
}

func (u *Ubuntu) fetch(ctx context.Context, cve string) (*AdvisorySet, error) {
	url := u.baseURL + "/" + cve + ".json"
	var data ubuntuCVE
	found, err := getJSON(ctx, url, nil, &data)
	if err != nil {
		return nil, err
	}
	set := &AdvisorySet{CVEID: cve, Distro: "ubuntu", Source: url, Known: found, ByRelease: map[string]Advisory{}}
	if !found {
		return set, nil
	}
	u.parse(&data, set)
	return set, nil
}

// ubuntuGASeries is the GA (default) kernel series shipped with each Ubuntu
// release. A host running the GA kernel uses the base `linux` package, whose
// name carries no series, so we resolve it from the codename.
var ubuntuGASeries = map[string]string{
	"trusty": "3.13", "xenial": "4.4", "bionic": "4.15", "focal": "5.4",
	"jammy": "5.15", "mantic": "6.5", "noble": "6.8", "oracular": "6.11",
	"plucky": "6.14",
}

// ubuntuPkgSeries derives the kernel series (MAJOR.MINOR) an advisory package
// applies to. The base `linux` package is the codename's GA kernel; HWE and
// flavour packages carry the series in their name (linux-hwe-5.19, linux-6.8);
// otherwise fall back to the released version's series.
func ubuntuPkgSeries(name, codename, description string) string {
	if name == "linux" {
		if ga := ubuntuGASeries[codename]; ga != "" {
			return ga
		}
		return kernelSeries(description)
	}
	if s := kernelSeries(name); s != "" {
		return s
	}
	return kernelSeries(description)
}

// parse fills set.ByRelease keyed by "codename/series" so a host can be matched
// to the exact kernel series it runs (GA vs HWE). Ubuntu LTS releases ship
// several kernel series at once and a fix in one series does not imply a fix in
// another; collapsing them would wrongly compare, say, a 5.19 HWE kernel
// against the 5.15 GA fixed version.
func (u *Ubuntu) parse(data *ubuntuCVE, set *AdvisorySet) {
	best := map[string]ubuntuCand{}

	consider := func(key string, dec Decision, fixed string) {
		if cur, ok := best[key]; ok {
			best[key] = mergeCand(cur, ubuntuCand{dec, fixed})
		} else {
			best[key] = ubuntuCand{dec, fixed}
		}
	}

	for _, pkg := range data.Packages {
		if !isGenericUbuntuKernelPkg(pkg.Name) {
			continue
		}
		for _, st := range pkg.Statuses {
			code := st.ReleaseCodename
			if code == "" {
				continue
			}
			series := ubuntuPkgSeries(pkg.Name, code, st.Description)
			if series == "" {
				continue
			}
			key := code + "/" + series
			fields := strings.Fields(st.Status)
			if len(fields) == 0 {
				continue
			}
			base := strings.ToLower(fields[0])
			switch base {
			case "released":
				consider(key, DecisionFixed, st.Description)
			case "not-affected", "dne":
				consider(key, DecisionNotAffected, "")
			case "needed", "needs-triage", "pending", "deferred", "active", "ignored":
				consider(key, DecisionAffected, "")
			}
		}
	}

	for key, c := range best {
		set.ByRelease[key] = Advisory{Release: key, Decision: c.decision, FixedVersion: c.fixed}
	}
}

// SelectAdvisory picks the advisory for the host's own kernel series, so an HWE
// kernel is compared against its own fix, not the GA one.
func (u *Ubuntu) SelectAdvisory(set *AdvisorySet, h Host) (Advisory, bool) {
	code := u.ReleaseKey(h.OSVersion)
	if code == "" {
		return Advisory{}, false
	}
	series := kernelSeries(h.KernelVersion)
	if series == "" {
		series = kernelSeries(h.KernelPkgVersion)
	}
	if series != "" {
		if adv, ok := set.ByRelease[code+"/"+series]; ok {
			return adv, true
		}
	}
	// Fallback: if the codename has exactly one series, use it.
	var only Advisory
	n := 0
	for key, adv := range set.ByRelease {
		if strings.HasPrefix(key, code+"/") {
			only = adv
			n++
		}
	}
	if n == 1 {
		return only, true
	}
	return Advisory{}, false
}

// ubuntuCand is a per-key candidate verdict during aggregation.
type ubuntuCand struct {
	decision Decision
	fixed    string
}

// mergeCand combines two candidates for the same key. Affected (no fix)
// dominates so we never mark a host patched when the distro still lists work to
// do; between two fixes we keep the lower version.
func mergeCand(a, b ubuntuCand) ubuntuCand {
	rank := func(d Decision) int {
		switch d {
		case DecisionAffected:
			return 3
		case DecisionFixed:
			return 2
		case DecisionNotAffected:
			return 1
		default:
			return 0
		}
	}
	if rank(b.decision) > rank(a.decision) {
		return b
	}
	if rank(b.decision) < rank(a.decision) {
		return a
	}
	if a.decision == DecisionFixed && b.fixed != "" && (a.fixed == "" || CompareDpkg(b.fixed, a.fixed) < 0) {
		return b
	}
	return a
}
