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
	return &Ubuntu{cache: newAdvisoryCache(), baseURL: "https://ubuntu.com/security/cves"}
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

// isUbuntuKernelPkg matches the generic/flavour kernel source packages while
// excluding linux-firmware (as fetch_ubuntu.py does).
func isUbuntuKernelPkg(name string) bool {
	if name == "linux" {
		return true
	}
	return strings.HasPrefix(name, "linux-") && !strings.Contains(name, "firmware")
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
	if set, err, ok := u.cache.get(cve); ok {
		return set, err
	}
	set, err := u.fetch(ctx, cve)
	u.cache.put(cve, set, err)
	return set, err
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

// parse fills set.ByRelease. Per codename it prefers the exact "linux" package
// (the generic kernel); otherwise it aggregates across kernel flavour packages.
func (u *Ubuntu) parse(data *ubuntuCVE, set *AdvisorySet) {
	best := map[string]ubuntuCand{}

	consider := func(codename string, dec Decision, fixed string, fromBase bool) {
		cur, ok := best[codename]
		c := ubuntuCand{dec, fixed, fromBase}
		if !ok {
			best[codename] = c
			return
		}
		// The exact "linux" package always wins.
		if fromBase && !cur.fromBase {
			best[codename] = c
			return
		}
		if !fromBase && cur.fromBase {
			return
		}
		// Same tier: prefer the more actionable verdict (affected > fixed >
		// not_affected), and among fixed keep the lowest fixed version.
		best[codename] = mergeCand(cur, c)
	}

	for _, pkg := range data.Packages {
		if !isUbuntuKernelPkg(pkg.Name) {
			continue
		}
		fromBase := pkg.Name == "linux"
		for _, st := range pkg.Statuses {
			code := st.ReleaseCodename
			if code == "" {
				continue
			}
			base := strings.ToLower(strings.Fields(st.Status)[0])
			switch base {
			case "released":
				consider(code, DecisionFixed, st.Description, fromBase)
			case "not-affected", "dne":
				consider(code, DecisionNotAffected, "", fromBase)
			case "needed", "needs-triage", "pending", "deferred", "active", "ignored":
				consider(code, DecisionAffected, "", fromBase)
			}
		}
	}

	for code, c := range best {
		set.ByRelease[code] = Advisory{Release: code, Decision: c.decision, FixedVersion: c.fixed}
	}
}

// ubuntuCand is a per-codename candidate verdict during aggregation.
type ubuntuCand struct {
	decision Decision
	fixed    string
	fromBase bool // came from the exact "linux" package
}

// mergeCand combines two candidates of the same package tier. Affected (no fix)
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
