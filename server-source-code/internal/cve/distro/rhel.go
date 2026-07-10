package distro

import (
	"context"
	"regexp"
	"strings"
)

// RHEL source. Mirrors cve-patch-duration/fetch_rhel_vex.py: it reads Red Hat's
// CSAF VEX (Vulnerability Exploitability eXchange) statement for a CVE and, per
// RHEL major (8/9/...), derives a verdict from the vulnerability's
// `product_status` map.
//
// Data source (one JSON document per CVE):
//
//	https://access.redhat.com/security/data/csaf/v2/vex/{year}/{cve-lowercase}.json
//	e.g. https://access.redhat.com/security/data/csaf/v2/vex/2024/cve-2024-1086.json
//
// CSAF `product_status` buckets a full product id under one category. The
// categories we act on (mapped to Decision) are:
//
//	fixed               -> DecisionFixed       (remediated; FixedVersion = kernel NVR)
//	first_fixed         -> DecisionFixed
//	known_affected      -> DecisionAffected    (affected, no fix yet)
//	first_affected      -> DecisionAffected
//	last_affected       -> DecisionAffected
//	known_not_affected  -> DecisionNotAffected
//	under_investigation -> DecisionUnknown
//	recommended         -> DecisionUnknown
//
// A full product id looks like
//
//	red_hat_enterprise_linux_9:kernel-0:5.14.0-427.13.1.el9_4.x86_64
//	└── product_ref ────────┘ └── component (name-epoch:version-release.arch) ┘
//
// so we split on the first ':' to get the product ref (mapped to a RHEL major
// via CPE from the product_tree, or by regex on the ref itself) and parse the
// remaining component to recover the kernel package name and NVR.
//
// Per major, when several products fall in different categories, we choose the
// dominant one using the same status priority as fetch_rhel_vex.py
// (fixed > known_affected > ... > known_not_affected). We focus on the mainline
// `kernel` source/product for FixedVersion and ignore kernel-rt / kernel-debug /
// kpatch, falling back to kernel-rt only when that is the sole fixed kernel.
type RHEL struct {
	cache   *advisoryCache
	baseURL string
}

// NewRHEL creates a RHEL source using the live CSAF VEX data feed.
func NewRHEL() *RHEL {
	return &RHEL{cache: newAdvisoryCache("rhel"), baseURL: "https://access.redhat.com/security/data/csaf/v2/vex"}
}

func (r *RHEL) Distro() string { return "rhel" }

// DataStatus reports how many CVEs this on-demand source has cached so far and
// whether the last fetch attempt errored.
func (r *RHEL) DataStatus() []DataEntry {
	return []DataEntry{r.cache.dataEntry("redhat-csaf-vex", "on-demand")}
}

func (r *RHEL) CompareVersions(a, b string) int { return CompareRPM(a, b) }

// rhelOSVerRe pulls the leading major number out of a host os_version so
// "9.4" -> "9", "8" -> "8", "Red Hat Enterprise Linux 9.4" -> "9".
var rhelOSVerRe = regexp.MustCompile(`(\d+)`)

func (r *RHEL) ReleaseKey(osVersion string) string {
	if m := rhelOSVerRe.FindStringSubmatch(strings.TrimSpace(osVersion)); m != nil {
		return m[1]
	}
	return ""
}

// CSAF VEX document (only the fields we consume). See the CSAF 2.0 spec and
// Red Hat's data feed at access.redhat.com/security/data/csaf/v2/vex/.
type csafVEX struct {
	Document struct {
		Tracking struct {
			ID                 string `json:"id"`
			InitialReleaseDate string `json:"initial_release_date"`
		} `json:"tracking"`
	} `json:"document"`
	ProductTree struct {
		Branches []csafBranch `json:"branches"`
	} `json:"product_tree"`
	Vulnerabilities []struct {
		CVE           string              `json:"cve"`
		ProductStatus map[string][]string `json:"product_status"`
	} `json:"vulnerabilities"`
}

// csafBranch is a (recursive) node of the product_tree. product_name nodes carry
// the CPE we use to map a product id to a RHEL major.
type csafBranch struct {
	Category string       `json:"category"`
	Name     string       `json:"name"`
	Branches []csafBranch `json:"branches"`
	Product  *struct {
		ProductID string `json:"product_id"`
		Helper    struct {
			CPE  string `json:"cpe"`
			PURL string `json:"purl"`
		} `json:"product_identification_helper"`
	} `json:"product"`
}

// rhelStatusPriority orders product_status categories from "most decisive" to
// least, matching STATUS_PRIORITY in fetch_rhel_vex.py (with last_affected added
// alongside the other *_affected buckets). The dominant category present for a
// major becomes that major's decision.
var rhelStatusPriority = []string{
	"fixed",
	"known_affected",
	"first_affected",
	"last_affected",
	"under_investigation",
	"first_fixed",
	"recommended",
	"known_not_affected",
}

var (
	// red_hat_enterprise_linux_9 (newer VEX product ids)
	rhelRefNewRe = regexp.MustCompile(`^red_hat_enterprise_linux_(\d+)`)
	// BaseOS-9.4.0.GA, AppStream-9.4.0.GA
	rhelRefDotRe = regexp.MustCompile(`-(\d+)\.\d+`)
	// 8Base-..., 9Base-...
	rhelRefBaseRe = regexp.MustCompile(`^(\d+)Base-`)
	// cpe:/o:redhat:enterprise_linux:9 or cpe:/a:redhat:enterprise_linux:9
	rhelCPERe = regexp.MustCompile(`cpe:/[oa]:redhat:enterprise_linux:(\d+)`)
)

// rhelArches are the RPM architecture suffixes we strip from a NEVRA's
// version-release.arch tail to recover the bare NVR (e.g. ".x86_64").
var rhelArches = []string{
	".x86_64", ".noarch", ".aarch64", ".ppc64le", ".ppc64", ".s390x", ".i686", ".i386", ".src",
}

func (r *RHEL) Advisories(ctx context.Context, cve string) (*AdvisorySet, error) {
	cve = strings.ToUpper(strings.TrimSpace(cve))
	return r.cache.do(cve, func(fctx context.Context) (*AdvisorySet, error) {
		return r.fetch(fctx, cve)
	})
}

func (r *RHEL) fetch(ctx context.Context, cve string) (*AdvisorySet, error) {
	url := r.baseURL + "/" + rhelYear(cve) + "/" + strings.ToLower(cve) + ".json"
	var data csafVEX
	found, err := getJSON(ctx, url, nil, &data)
	if err != nil {
		return nil, err
	}
	set := &AdvisorySet{CVEID: cve, Distro: "rhel", Source: url, Known: found, ByRelease: map[string]Advisory{}}
	if !found {
		// CVE has no VEX statement (HTTP 404): unknown to the RHEL tracker.
		return set, nil
	}
	r.parse(&data, set)
	return set, nil
}

// rhelMajorAgg accumulates, for one RHEL major, which product_status categories
// were seen and the fixed kernel NVRs (mainline vs kernel-rt fallback).
type rhelMajorAgg struct {
	statuses    map[string]bool
	kernelFixed []string // mainline `kernel` versions found under "fixed"
	rtFixed     []string // kernel-rt versions found under "fixed" (fallback)
}

// parse fills set.ByRelease from the decoded CSAF VEX document. It is separated
// from fetch so it can be unit-tested against a fixture without any network.
func (r *RHEL) parse(data *csafVEX, set *AdvisorySet) {
	// Map product id -> RHEL major via the CPE on each product_name node.
	cpeMajor := map[string]string{}
	var walk func(bs []csafBranch)
	walk = func(bs []csafBranch) {
		for i := range bs {
			b := &bs[i]
			if b.Product != nil {
				if m := rhelCPERe.FindStringSubmatch(b.Product.Helper.CPE); m != nil {
					cpeMajor[b.Product.ProductID] = m[1]
				}
			}
			if len(b.Branches) > 0 {
				walk(b.Branches)
			}
		}
	}
	walk(data.ProductTree.Branches)

	// Use the first vulnerability that carries a product_status (as the Python
	// does: the VEX document is per-CVE, so there is normally exactly one).
	var ps map[string][]string
	for _, v := range data.Vulnerabilities {
		if len(v.ProductStatus) > 0 {
			ps = v.ProductStatus
			break
		}
	}
	if ps == nil {
		return
	}

	agg := map[string]*rhelMajorAgg{}
	ensure := func(major string) *rhelMajorAgg {
		a := agg[major]
		if a == nil {
			a = &rhelMajorAgg{statuses: map[string]bool{}}
			agg[major] = a
		}
		return a
	}

	for status, ids := range ps {
		for _, id := range ids {
			productRef, component := splitProductID(id)
			major := rhelMajor(productRef, cpeMajor)
			if major == "" {
				continue
			}
			a := ensure(major)
			a.statuses[status] = true
			if status == "fixed" {
				if name, ver, ok := rhelKernelNVR(component); ok {
					switch name {
					case "kernel":
						a.kernelFixed = append(a.kernelFixed, ver)
					case "kernel-rt":
						a.rtFixed = append(a.rtFixed, ver)
					}
				}
			}
		}
	}

	for major, a := range agg {
		status := dominantRHELStatus(a.statuses)
		dec := rhelDecision(status)
		adv := Advisory{Release: major, Decision: dec}
		if dec == DecisionFixed {
			// Prefer the mainline kernel; fall back to kernel-rt only when it is
			// the sole fixed kernel. Keep the lowest NVR: with major-only
			// granularity a host at or above the earliest fix is patched.
			fixed := lowestRPM(a.kernelFixed)
			if fixed == "" {
				fixed = lowestRPM(a.rtFixed)
			}
			adv.FixedVersion = fixed
		}
		set.ByRelease[major] = adv
	}
}

// splitProductID splits a CSAF full product id on the first ':' into the product
// reference and the component (the rest, itself "name-epoch:version-release.arch").
func splitProductID(id string) (productRef, component string) {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[:i], id[i+1:]
	}
	return id, ""
}

// rhelMajor resolves a product reference to a RHEL major, preferring the CPE map
// built from the product_tree and falling back to the ref-name regexes used by
// fetch_rhel_vex.py.
func rhelMajor(productRef string, cpeMajor map[string]string) string {
	if m, ok := cpeMajor[productRef]; ok {
		return m
	}
	if m := rhelRefNewRe.FindStringSubmatch(productRef); m != nil {
		return m[1]
	}
	// Check the "<major>Base-" prefix before the "-<major>.<minor>" pattern:
	// a ref like "8Base-RHOSE-4.12" is RHEL 8, but the dot regex would otherwise
	// match the trailing "-4.12".
	if m := rhelRefBaseRe.FindStringSubmatch(productRef); m != nil {
		return m[1]
	}
	if m := rhelRefDotRe.FindStringSubmatch(productRef); m != nil {
		return m[1]
	}
	return ""
}

// rhelKernelNVR parses a component "name-epoch:version-release.arch" and returns the
// package name (epoch stripped) and the bare NVR (epoch and arch stripped), e.g.
// "kernel-0:5.14.0-427.13.1.el9_4.x86_64" -> ("kernel", "5.14.0-427.13.1.el9_4").
func rhelKernelNVR(component string) (name, version string, ok bool) {
	ci := strings.IndexByte(component, ':')
	if ci < 0 {
		return "", "", false
	}
	nameEpoch := component[:ci]
	verArch := component[ci+1:]
	if di := strings.LastIndexByte(nameEpoch, '-'); di >= 0 {
		name = nameEpoch[:di]
	} else {
		name = nameEpoch
	}
	version = rhelStripArch(verArch)
	if name == "" || version == "" {
		return name, version, false
	}
	return name, version, true
}

func rhelStripArch(v string) string {
	for _, a := range rhelArches {
		if strings.HasSuffix(v, a) {
			return v[:len(v)-len(a)]
		}
	}
	return v
}

// dominantRHELStatus returns the highest-priority category present, or any
// present category if none is in the priority list.
func dominantRHELStatus(statuses map[string]bool) string {
	for _, s := range rhelStatusPriority {
		if statuses[s] {
			return s
		}
	}
	for s := range statuses {
		return s
	}
	return ""
}

func rhelDecision(status string) Decision {
	switch status {
	case "fixed", "first_fixed":
		return DecisionFixed
	case "known_affected", "first_affected", "last_affected":
		return DecisionAffected
	case "known_not_affected":
		return DecisionNotAffected
	default: // under_investigation, recommended, empty, ...
		return DecisionUnknown
	}
}

// lowestRPM returns the lowest (by rpm semantics) non-empty version in vers.
func lowestRPM(vers []string) string {
	best := ""
	for _, v := range vers {
		if v == "" {
			continue
		}
		if best == "" || CompareRPM(v, best) < 0 {
			best = v
		}
	}
	return best
}

// rhelYear extracts the year from a CVE id ("CVE-2024-1086" -> "2024") for the
// year-partitioned VEX path.
func rhelYear(cve string) string {
	parts := strings.Split(cve, "-")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}
