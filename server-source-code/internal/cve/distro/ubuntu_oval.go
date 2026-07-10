package distro

import (
	"bufio"
	"compress/bzip2"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ubuntuOVAL is the bulk Ubuntu source: Canonical's per-release OVAL feed
// (security-metadata.canonical.com/oval/com.ubuntu.<codename>.cve.oval.xml.bz2).
// One ~9 MB download per release yields fixed kernel versions for (almost) every
// CVE, sidestepping the per-CVE ubuntu.com API that Cloudflare rate-limits.
//
// We extract, per CVE, the fixed version of the *generic* kernel per series
// (e.g. jammy 5.15 -> 5.15.0-101.111, jammy 6.8 -> 6.8.0-124.124~22.04.1) from
// each definition's "Update Instructions" description. The parsed index is
// cached on disk (CVE_CACHE_DIR) and refreshed on a TTL. The Ubuntu source
// falls back to the per-CVE JSON API when OVAL lacks a CVE (or has no generic
// data for it).
type ubuntuOVAL struct {
	baseURL  string
	releases []string
	ttl      time.Duration

	mu       sync.Mutex
	idx      map[string]map[string]map[string]string // codename -> cve -> series -> fixedVersion
	expires  map[string]time.Time
	inflight map[string]bool
	stats    map[string]*ovalStat // codename -> load status
}

// ovalStat records the freshness/outcome of a release's OVAL load.
type ovalStat struct {
	Release     string     `json:"release"`
	LastAttempt *time.Time `json:"last_attempt,omitempty"`
	LastSuccess *time.Time `json:"last_success,omitempty"`
	OK          bool       `json:"ok"`
	Error       string     `json:"error,omitempty"`
	CVECount    int        `json:"cve_count"`
	NewestCVE   string     `json:"newest_cve,omitempty"`
	FromDisk    bool       `json:"from_disk"`
}

func newUbuntuOVAL() *ubuntuOVAL {
	releases := []string{"focal", "jammy", "noble"}
	if env := strings.TrimSpace(os.Getenv("UBUNTU_OVAL_RELEASES")); env != "" {
		releases = nil
		for _, r := range strings.Split(env, ",") {
			if r = strings.TrimSpace(r); r != "" {
				releases = append(releases, r)
			}
		}
	}
	return &ubuntuOVAL{
		baseURL:  "https://security-metadata.canonical.com/oval",
		releases: releases,
		ttl:      24 * time.Hour,
		idx:      map[string]map[string]map[string]string{},
		expires:  map[string]time.Time{},
		inflight: map[string]bool{},
		stats:    map[string]*ovalStat{},
	}
}

func (o *ubuntuOVAL) stat(code string) *ovalStat {
	s := o.stats[code]
	if s == nil {
		s = &ovalStat{Release: code}
		o.stats[code] = s
	}
	return s
}

// status returns a freshness snapshot for every configured release. It also
// warms any release that isn't loaded yet (from disk instantly, or a background
// fetch) so viewing the report populates the indexes.
func (o *ubuntuOVAL) status() []DataEntry {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, code := range o.releases {
		if idx, ok := o.idx[code]; !ok || len(idx) == 0 || !time.Now().Before(o.expires[code]) {
			o.ensureLoadLocked(code)
		}
	}
	out := make([]DataEntry, 0, len(o.releases))
	for _, code := range o.releases {
		s := o.stat(code)
		out = append(out, DataEntry{
			Source:      "ubuntu-oval:" + code,
			Kind:        "oval",
			LastAttempt: s.LastAttempt,
			LastSuccess: s.LastSuccess,
			OK:          s.OK,
			Error:       s.Error,
			Count:       s.CVECount,
			Newest:      s.NewestCVE,
			FromDisk:    s.FromDisk,
		})
	}
	return out
}

// newestCVE returns the highest CVE id (by year then number) in an index.
func newestCVE(idx map[string]map[string]string) string {
	best := ""
	var by, bn int
	for id := range idx {
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

// advisorySet returns the OVAL-derived per-(codename/series) fixed versions for
// a CVE. covered is true when at least one release has generic fix data for it;
// ready is true when every configured release index is loaded (so a "not
// covered" answer is authoritative rather than "still warming up").
func (o *ubuntuOVAL) advisorySet(cve string) (byRelease map[string]Advisory, covered, ready bool) {
	cve = strings.ToUpper(strings.TrimSpace(cve))
	byRelease = map[string]Advisory{}
	ready = true
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, code := range o.releases {
		if idx, ok := o.idx[code]; ok && time.Now().Before(o.expires[code]) {
			if m, ok := idx[cve]; ok {
				for series, ver := range m {
					covered = true
					byRelease[code+"/"+series] = Advisory{Release: code + "/" + series, Decision: DecisionFixed, FixedVersion: ver}
				}
			}
		} else {
			ready = false
			o.ensureLoadLocked(code)
		}
	}
	return byRelease, covered, ready
}

// ensureLoadLocked triggers a background load for a release if none is running.
// Caller must hold o.mu.
func (o *ubuntuOVAL) ensureLoadLocked(codename string) {
	if o.inflight[codename] {
		return
	}
	now := time.Now()
	st := o.stat(codename)
	st.LastAttempt = &now
	// Serve a disk-cached index immediately if present.
	if idx := o.loadDisk(codename); idx != nil {
		o.idx[codename] = idx
		o.expires[codename] = now.Add(o.ttl)
		st.LastSuccess = &now
		st.OK = true
		st.Error = ""
		st.CVECount = len(idx)
		st.NewestCVE = newestCVE(idx)
		st.FromDisk = true
		return
	}
	o.inflight[codename] = true
	go o.load(codename)
}

func (o *ubuntuOVAL) load(codename string) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	idx, err := o.fetchAndParse(ctx, codename)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.inflight[codename] = false
	now := time.Now()
	st := o.stat(codename)
	if err != nil {
		st.OK = false
		st.Error = err.Error()
		log.Printf("[cve/distro] ubuntu OVAL %s load failed: %v", codename, err)
		return
	}
	o.idx[codename] = idx
	o.expires[codename] = now.Add(o.ttl)
	st.LastSuccess = &now
	st.OK = true
	st.Error = ""
	st.CVECount = len(idx)
	st.NewestCVE = newestCVE(idx)
	st.FromDisk = false
	o.saveDisk(codename, idx)
	log.Printf("[cve/distro] ubuntu OVAL %s loaded: %d CVEs (newest %s)", codename, len(idx), st.NewestCVE)
}

// ovalDefinition mirrors the subset of a <definition> we consume. Field tags
// match local element names (namespace-independent).
type ovalDefinition struct {
	Metadata struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
		References  []struct {
			Source string `xml:"source,attr"`
			RefID  string `xml:"ref_id,attr"`
		} `xml:"reference"`
	} `xml:"metadata"`
}

func (o *ubuntuOVAL) fetchAndParse(ctx context.Context, codename string) (map[string]map[string]string, error) {
	url := fmt.Sprintf("%s/com.ubuntu.%s.cve.oval.xml.bz2", o.baseURL, codename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PatchMon-cve-distro/1.0")
	resp, err := (&http.Client{Timeout: 4 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OVAL %s returned %d", codename, resp.StatusCode)
	}

	dec := xml.NewDecoder(bzip2.NewReader(bufio.NewReaderSize(resp.Body, 1<<20)))
	out := map[string]map[string]string{}
	for {
		tok, err := dec.Token()
		if err != nil {
			break // EOF or parse end
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "definition" {
			continue
		}
		var def ovalDefinition
		if err := dec.DecodeElement(&def, &se); err != nil {
			continue
		}
		fixed := parseOVALFixed(def.Metadata.Description)
		if len(fixed) == 0 {
			continue
		}
		for _, ref := range def.Metadata.References {
			if !strings.EqualFold(ref.Source, "CVE") {
				continue
			}
			cve := strings.ToUpper(strings.TrimSpace(ref.RefID))
			if cve == "" {
				continue
			}
			m := out[cve]
			if m == nil {
				m = map[string]string{}
				out[cve] = m
			}
			for series, ver := range fixed {
				// Keep the lowest fixed version if several map to a series.
				if cur, ok := m[series]; !ok || CompareDpkg(ver, cur) < 0 {
					m[series] = ver
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("OVAL %s parsed 0 definitions", codename)
	}
	return out, nil
}

// parseOVALFixed extracts generic-kernel fixed versions from a definition's
// description "Update Instructions" block. Lines look like:
//
//	linux-image-5.15.0-101-generic - 5.15.0-101.111
//
// It keeps only generic packages (suffix "-generic", excluding "-generic-64k"
// and cloud/lowlatency flavours) and keys them by kernel series.
func parseOVALFixed(desc string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(desc, "\n") {
		line = strings.TrimSpace(line)
		i := strings.Index(line, " - ")
		if i <= 0 {
			continue
		}
		pkg := strings.TrimSpace(line[:i])
		ver := strings.TrimSpace(line[i+3:])
		if !strings.HasPrefix(pkg, "linux-image-") {
			continue
		}
		if !strings.HasSuffix(pkg, "-generic") { // excludes -64k, -aws, -lowlatency, ...
			continue
		}
		if strings.ContainsAny(ver, " \t") || ver == "" {
			continue
		}
		series := kernelSeries(ver)
		if series == "" {
			continue
		}
		if cur, ok := out[series]; !ok || CompareDpkg(ver, cur) < 0 {
			out[series] = ver
		}
	}
	return out
}

func (o *ubuntuOVAL) diskPath(codename string) string {
	if diskCacheDir == "" {
		return ""
	}
	return filepath.Join(diskCacheDir, "oval_ubuntu_"+codename+".json")
}

func (o *ubuntuOVAL) loadDisk(codename string) map[string]map[string]string {
	p := o.diskPath(codename)
	if p == "" {
		return nil
	}
	info, err := os.Stat(p)
	if err != nil || time.Since(info.ModTime()) > o.ttl {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var idx map[string]map[string]string
	if json.Unmarshal(b, &idx) != nil || len(idx) == 0 {
		return nil
	}
	return idx
}

func (o *ubuntuOVAL) saveDisk(codename string, idx map[string]map[string]string) {
	p := o.diskPath(codename)
	if p == "" {
		return
	}
	b, err := json.Marshal(idx)
	if err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.tmp.%d", p, os.Getpid())
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, p)
	}
}
