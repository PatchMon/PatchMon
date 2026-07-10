package distro

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CentOS source. Mirrors cve-patch-duration/fetch_centos.py, which uses
// AlmaLinux errata as a RHEL/CentOS(-Stream) proxy: AlmaLinux is a bit-for-bit
// RHEL clone and publishes the errata that CentOS Stream itself does not, so an
// AlmaLinux Security Advisory (ALSA) for the `kernel` package stands in for the
// corresponding RHSA/CentOS fix (typically 1-5 days behind RHEL).
//
// Data source (one large JSON document per RHEL major, ~20MB each):
//
//	https://errata.almalinux.org/8/errata.full.json
//	https://errata.almalinux.org/9/errata.full.json
//	https://errata.almalinux.org/10/errata.full.json
//
// Each document is {"data": [<erratum>, ...]}. For every kernel erratum
// (title contains "kernel" and it ships the main `kernel` package, excluding
// kernel-rt / kpatch, exactly as fetch_centos.py's is_kernel_erratum) that
// references the CVE, the fixed kernel RPM NVR (version-release) becomes the
// DecisionFixed FixedVersion, keyed by the AlmaLinux major ("8"/"9"/"10").
// When a major has several kernel errata for the CVE (e.g. EUS sub-streams) the
// earliest-issued one wins, matching the Python's per-major min(issued_date)
// selection. If no erratum references the CVE the set is Known=false: AlmaLinux
// coverage is intentionally partial (notably weak for RHEL EUS/AUS sub-streams).
//
// Like the Debian source, the full documents are fetched once and cached; the
// per-CVE index is served from that cache. A future DB-sync would persist the
// same CVE->major->fix index.
type CentOS struct {
	baseURL string
	majors  []string

	mu       sync.Mutex
	index    map[string]map[string]Advisory // CVE id -> major -> advisory
	expires  time.Time
	ttl      time.Duration
	inflight bool
	stat     freshness
}

// DataStatus reports the freshness of the cached AlmaLinux errata index. If the
// index is not loaded (or stale) it triggers a background rebuild so a
// subsequent view shows populated data.
func (c *CentOS) DataStatus() []DataEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if (c.index == nil || !time.Now().Before(c.expires)) && !c.inflight {
		c.inflight = true
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), backgroundFetchTimeout)
			defer cancel()
			_, _ = c.indexMap(ctx)
			c.mu.Lock()
			c.inflight = false
			c.mu.Unlock()
		}()
	}
	return []DataEntry{c.stat.entry("almalinux-errata (RHEL/CentOS proxy)", "errata")}
}

// NewCentOS creates a CentOS source backed by the live AlmaLinux errata API.
func NewCentOS() *CentOS {
	return &CentOS{
		baseURL: "https://errata.almalinux.org",
		majors:  []string{"8", "9", "10"},
		ttl:     defaultTTL,
	}
}

func (c *CentOS) Distro() string { return "centos" }

func (c *CentOS) CompareVersions(a, b string) int { return CompareRPM(a, b) }

// centosVerRe extracts the leading integer (the RHEL major) of a host OS
// version string: "9.4" -> "9", "9" -> "9", "AlmaLinux 8.10" -> "8".
var centosVerRe = regexp.MustCompile(`\d+`)

// ReleaseKey maps a host OS version to the AlmaLinux major used as the
// ByRelease key.
func (c *CentOS) ReleaseKey(osVersion string) string {
	return centosVerRe.FindString(strings.TrimSpace(osVersion))
}

// almaErrataFile is the top-level errata.full.json document.
type almaErrataFile struct {
	Data []almaErratum `json:"data"`
}

// almaErratum is one advisory (ALSA/ALBA/ALEA) in the errata document.
type almaErratum struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	IssuedDate json.Number     `json:"issued_date"` // unix seconds
	References []almaReference `json:"references"`
	Packages   []almaPackage   `json:"packages"`
}

type almaReference struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type almaPackage struct {
	Name     string `json:"name"`
	Epoch    string `json:"epoch"`
	Version  string `json:"version"`
	Release  string `json:"release"`
	Arch     string `json:"arch"`
	Filename string `json:"filename"`
}

// issued returns the erratum's issue time as unix seconds (0 when absent).
func (e almaErratum) issued() int64 {
	if e.IssuedDate == "" {
		return 0
	}
	if n, err := e.IssuedDate.Int64(); err == nil {
		return n
	}
	if f, err := e.IssuedDate.Float64(); err == nil {
		return int64(f)
	}
	return 0
}

func (c *CentOS) Advisories(ctx context.Context, cve string) (*AdvisorySet, error) {
	cve = strings.ToUpper(strings.TrimSpace(cve))
	idx, err := c.indexMap(ctx)
	if err != nil {
		return nil, err
	}
	set := &AdvisorySet{
		CVEID:     cve,
		Distro:    "centos",
		Source:    c.baseURL + " (AlmaLinux errata as RHEL/CentOS proxy)",
		ByRelease: map[string]Advisory{},
	}
	byMajor, ok := idx[cve]
	if !ok {
		// No AlmaLinux kernel erratum references this CVE. Coverage is partial
		// by design, so this is "not known", not an error.
		set.Known = false
		return set, nil
	}
	set.Known = true
	for major, adv := range byMajor {
		set.ByRelease[major] = adv
	}
	return set, nil
}

// indexMap returns the cached CVE->major->advisory index, rebuilding it from the
// AlmaLinux errata documents when stale. On a refresh failure it serves stale
// data if any is present.
func (c *CentOS) indexMap(ctx context.Context) (map[string]map[string]Advisory, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index != nil && time.Now().Before(c.expires) {
		return c.index, nil
	}
	// On a cold start, serve a fresh on-disk copy instantly (survives restarts).
	if c.index == nil {
		var idx map[string]map[string]Advisory
		if loadDiskJSON(centosDiskFile, c.ttl, &idx) && len(idx) > 0 {
			now := time.Now()
			c.index = idx
			c.expires = now.Add(c.ttl)
			c.stat.success(now, len(idx), newestCVEKey(idx), true)
			return c.index, nil
		}
	}
	c.stat.attempt(time.Now())
	idx, err := c.buildIndex(ctx)
	if err != nil {
		c.stat.fail(err)
		if c.index != nil {
			return c.index, nil
		}
		return nil, err
	}
	now := time.Now()
	c.index = idx
	c.expires = now.Add(c.ttl)
	c.stat.success(now, len(idx), newestCVEKey(idx), false)
	saveDiskJSON(centosDiskFile, idx)
	return c.index, nil
}

// centosDiskFile is the on-disk cache name for the parsed CVE->major index.
const centosDiskFile = "centos_alma_index.json"

// buildIndex fetches each major's errata document and merges the kernel errata
// into a single CVE->major->advisory index.
func (c *CentOS) buildIndex(ctx context.Context) (map[string]map[string]Advisory, error) {
	idx := map[string]map[string]Advisory{}
	for _, major := range c.majors {
		url := fmt.Sprintf("%s/%s/errata.full.json", c.baseURL, major)
		var file almaErrataFile
		// ~25 MB per major — use the bulk client, not the per-CVE getJSON.
		found, err := getJSONBulk(ctx, url, &file)
		if err != nil {
			return nil, err
		}
		if !found {
			continue // this major has no published errata document
		}
		for cve, adv := range c.parse(&file, major) {
			m := idx[cve]
			if m == nil {
				m = map[string]Advisory{}
				idx[cve] = m
			}
			m[major] = adv
		}
	}
	return idx, nil
}

// parse builds a CVE -> Advisory map for a single AlmaLinux major from one
// errata document. It considers only kernel errata (per isKernelErratum) that
// carry a concrete kernel NVR and an issue date, and — when a CVE has several
// such errata in the major — keeps the earliest-issued fix (the lowest fixed
// version), matching fetch_centos.py's per-major min(issued_date) selection.
func (c *CentOS) parse(file *almaErrataFile, major string) map[string]Advisory {
	type cand struct {
		fixed  string
		issued int64
	}
	best := map[string]cand{}

	for _, e := range file.Data {
		if !isKernelErratum(&e) {
			continue
		}
		issued := e.issued()
		if issued == 0 {
			continue // fetch_centos.py skips errata without an issued_date
		}
		fixed := kernelNVR(&e)
		if fixed == "" {
			continue
		}
		for _, ref := range e.References {
			if !strings.EqualFold(ref.Type, "cve") {
				continue
			}
			id := strings.ToUpper(strings.TrimSpace(ref.ID))
			if id == "" {
				continue
			}
			if cur, ok := best[id]; !ok || issued < cur.issued {
				best[id] = cand{fixed: fixed, issued: issued}
			}
		}
	}

	out := make(map[string]Advisory, len(best))
	for id, cd := range best {
		out[id] = Advisory{Release: major, Decision: DecisionFixed, FixedVersion: cd.fixed}
	}
	return out
}

// isKernelErratum reports whether an erratum patches the main kernel, mirroring
// fetch_centos.py: the title must mention "kernel" and it must ship the main
// `kernel` package (by name or a "kernel-" filename) while excluding the
// kernel-rt and kpatch variants.
func isKernelErratum(e *almaErratum) bool {
	if !strings.Contains(strings.ToLower(e.Title), "kernel") {
		return false
	}
	for i := range e.Packages {
		p := &e.Packages[i]
		fn := p.Filename
		if (strings.HasPrefix(fn, "kernel-") || p.Name == "kernel") &&
			!strings.Contains(fn, "kernel-rt") &&
			!strings.Contains(fn, "kpatch") {
			return true
		}
	}
	return false
}

// kernelNVR returns the fixed kernel version-release ([epoch:]version-release)
// for an erratum. It prefers the exact `kernel` package; otherwise it falls back
// to the first kernel-* subpackage (all share the same version-release),
// skipping kernel-rt and kpatch.
func kernelNVR(e *almaErratum) string {
	var chosen *almaPackage
	for i := range e.Packages {
		p := &e.Packages[i]
		fn := p.Filename
		if strings.Contains(fn, "kernel-rt") || strings.Contains(fn, "kpatch") {
			continue
		}
		if p.Name == "kernel" {
			chosen = p
			break
		}
		if chosen == nil && strings.HasPrefix(fn, "kernel-") {
			chosen = p
		}
	}
	if chosen == nil || chosen.Version == "" || chosen.Release == "" {
		return ""
	}
	nvr := chosen.Version + "-" + chosen.Release
	if ep := strings.TrimSpace(chosen.Epoch); ep != "" && ep != "0" {
		nvr = ep + ":" + nvr
	}
	return nvr
}
