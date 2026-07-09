package distro

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Debian source. Mirrors cve-patch-duration/fetch_debian.py: it reads the
// `linux` source-package entry from the Debian security tracker JSON dump and,
// per release codename, uses status + fixed_version ("0" means "not affected").
//
// The tracker dump is a single large document, so it is fetched once and cached
// (the `linux` subset only); per-CVE lookups are served from that cache. A
// future DB-sync would persist the same subset.
type Debian struct {
	url string

	mu      sync.Mutex
	linux   map[string]debianEntry
	expires time.Time
	ttl     time.Duration
}

// NewDebian creates a Debian source using the live security tracker.
func NewDebian() *Debian {
	return &Debian{url: "https://security-tracker.debian.org/tracker/data/json", ttl: defaultTTL}
}

func (d *Debian) Distro() string { return "debian" }

func (d *Debian) CompareVersions(a, b string) int { return CompareDpkg(a, b) }

var debianReleases = map[string]string{
	"10": "buster", "11": "bullseye", "12": "bookworm", "13": "trixie", "14": "forky",
}

var debianVerRe = regexp.MustCompile(`(\d{1,2})`)

func (d *Debian) ReleaseKey(osVersion string) string {
	v := strings.ToLower(strings.TrimSpace(osVersion))
	for _, code := range debianReleases {
		if strings.Contains(v, code) {
			return code
		}
	}
	if m := debianVerRe.FindStringSubmatch(v); m != nil {
		if code, ok := debianReleases[m[1]]; ok {
			return code
		}
	}
	return ""
}

type debianEntry struct {
	Releases map[string]struct {
		Status       string `json:"status"`
		FixedVersion string `json:"fixed_version"`
	} `json:"releases"`
}

func (d *Debian) Advisories(ctx context.Context, cve string) (*AdvisorySet, error) {
	cve = strings.ToUpper(strings.TrimSpace(cve))
	linux, err := d.linuxMap(ctx)
	if err != nil {
		return nil, err
	}
	set := &AdvisorySet{CVEID: cve, Distro: "debian", Source: d.url + " (linux)", ByRelease: map[string]Advisory{}}
	entry, ok := linux[cve]
	if !ok {
		set.Known = false
		return set, nil
	}
	set.Known = true
	for code, r := range entry.Releases {
		fv := strings.TrimSpace(r.FixedVersion)
		switch {
		case fv == "0":
			// Debian explicit: kernel never affected in this release.
			set.ByRelease[code] = Advisory{Release: code, Decision: DecisionNotAffected}
		case r.Status == "resolved" && fv != "":
			set.ByRelease[code] = Advisory{Release: code, Decision: DecisionFixed, FixedVersion: fv}
		case r.Status == "open":
			set.ByRelease[code] = Advisory{Release: code, Decision: DecisionAffected}
		default:
			set.ByRelease[code] = Advisory{Release: code, Decision: DecisionUnknown}
		}
	}
	return set, nil
}

// linuxMap returns the cached `linux` CVE map, refreshing it from the tracker
// dump when stale.
func (d *Debian) linuxMap(ctx context.Context) (map[string]debianEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.linux != nil && time.Now().Before(d.expires) {
		return d.linux, nil
	}
	linux, err := d.fetchLinuxMap(ctx)
	if err != nil {
		// Serve stale data rather than failing if we have any.
		if d.linux != nil {
			return d.linux, nil
		}
		return nil, err
	}
	d.linux = linux
	d.expires = time.Now().Add(d.ttl)
	return d.linux, nil
}

// fetchLinuxMap streams the tracker dump and decodes only the `linux` object,
// avoiding holding the whole multi-megabyte document.
func (d *Debian) fetchLinuxMap(ctx context.Context) (map[string]debianEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PatchMon-cve-distro/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching Debian tracker (server may lack internet access): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Debian tracker returned %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	if _, err := dec.Token(); err != nil { // opening '{'
		return nil, err
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := keyTok.(string)
		if key == "linux" {
			var m map[string]debianEntry
			if err := dec.Decode(&m); err != nil {
				return nil, err
			}
			return m, nil
		}
		// Skip this source package's value.
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return map[string]debianEntry{}, nil
}
