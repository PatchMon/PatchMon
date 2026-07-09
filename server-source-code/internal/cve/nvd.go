// Package cve resolves CVE identifiers to affected Linux-kernel version ranges
// using the public NVD (National Vulnerability Database) 2.0 REST API.
//
// The mapping is best-effort: NVD reports the affected *upstream* kernel
// version ranges declared in a CVE's CPE configuration. Distro-specific
// backports (Ubuntu/Debian/RHEL renumber the fix) are not covered by NVD and
// remain the operator's responsibility — combine the kernel filter with the OS
// filter for those cases. When the server has no outbound internet access the
// lookup fails gracefully and the caller surfaces a clear error.
package cve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

const (
	nvdBaseURL   = "https://services.nvd.nist.gov/rest/json/cves/2.0"
	successTTL   = 6 * time.Hour
	negativeTTL  = 30 * time.Minute
	httpTimeout  = 12 * time.Second
	linuxKernelC = ":o:linux:linux_kernel:"
)

// cveIDPattern matches a CVE identifier, e.g. CVE-2026-46331.
var cveIDPattern = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)

// IsCVEID reports whether s looks like a CVE identifier (case-insensitive).
func IsCVEID(s string) bool {
	return cveIDPattern.MatchString(strings.ToUpper(strings.TrimSpace(s)))
}

// Result is the outcome of a CVE lookup.
type Result struct {
	CVEID       string             `json:"cve_id"`
	Description string             `json:"description"`
	Ranges      []util.KernelRange `json:"ranges"`
	Filter      *util.KernelFilter `json:"-"`
}

type cacheEntry struct {
	result  *Result
	err     error
	expires time.Time
}

// Service resolves CVEs to kernel version ranges, with an in-memory TTL cache.
type Service struct {
	client *http.Client
	apiKey string

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewService creates a CVE lookup service. If the NVD_API_KEY environment
// variable is set it is sent with each request to raise NVD's rate limit.
func NewService() *Service {
	return &Service{
		client: &http.Client{Timeout: httpTimeout},
		apiKey: strings.TrimSpace(os.Getenv("NVD_API_KEY")),
		cache:  make(map[string]cacheEntry),
	}
}

// nvdResponse mirrors the subset of the NVD 2.0 schema we consume.
type nvdResponse struct {
	Vulnerabilities []struct {
		CVE struct {
			ID           string `json:"id"`
			Descriptions []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			} `json:"descriptions"`
			Configurations []struct {
				Nodes []struct {
					CPEMatch []cpeMatch `json:"cpeMatch"`
				} `json:"nodes"`
			} `json:"configurations"`
		} `json:"cve"`
	} `json:"vulnerabilities"`
}

type cpeMatch struct {
	Vulnerable            bool   `json:"vulnerable"`
	Criteria              string `json:"criteria"`
	VersionStartIncluding string `json:"versionStartIncluding"`
	VersionStartExcluding string `json:"versionStartExcluding"`
	VersionEndIncluding   string `json:"versionEndIncluding"`
	VersionEndExcluding   string `json:"versionEndExcluding"`
}

// Lookup resolves a CVE identifier to affected kernel version ranges, caching
// the result.
func (s *Service) Lookup(ctx context.Context, cveID string) (*Result, error) {
	cveID = strings.ToUpper(strings.TrimSpace(cveID))
	if !IsCVEID(cveID) {
		return nil, fmt.Errorf("invalid CVE identifier %q", cveID)
	}

	s.mu.Lock()
	if e, ok := s.cache[cveID]; ok && time.Now().Before(e.expires) {
		s.mu.Unlock()
		return e.result, e.err
	}
	s.mu.Unlock()

	result, err := s.fetch(ctx, cveID)

	s.mu.Lock()
	ttl := successTTL
	if err != nil {
		ttl = negativeTTL
	}
	s.cache[cveID] = cacheEntry{result: result, err: err, expires: time.Now().Add(ttl)}
	s.mu.Unlock()

	return result, err
}

func (s *Service) fetch(ctx context.Context, cveID string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nvdBaseURL+"?cveId="+cveID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if s.apiKey != "" {
		req.Header.Set("apiKey", s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach NVD (server may lack internet access): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("CVE %s not found in NVD", cveID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("NVD returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed nvdResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("could not parse NVD response: %w", err)
	}
	if len(parsed.Vulnerabilities) == 0 {
		return nil, fmt.Errorf("CVE %s not found in NVD", cveID)
	}

	v := parsed.Vulnerabilities[0].CVE
	res := &Result{CVEID: v.ID}
	if res.CVEID == "" {
		res.CVEID = cveID
	}
	for _, d := range v.Descriptions {
		if d.Lang == "en" {
			res.Description = d.Value
			break
		}
	}

	for _, cfg := range v.Configurations {
		for _, node := range cfg.Nodes {
			for _, m := range node.CPEMatch {
				if !m.Vulnerable || !strings.Contains(m.Criteria, linuxKernelC) {
					continue
				}
				r := cpeMatchToRange(m)
				if r != (util.KernelRange{}) {
					res.Ranges = append(res.Ranges, r)
				}
			}
		}
	}

	if len(res.Ranges) == 0 {
		return nil, fmt.Errorf("CVE %s has no Linux kernel version ranges in NVD; filter by kernel version manually", cveID)
	}

	res.Filter = util.FromKernelRanges(res.CVEID, res.Ranges)
	return res, nil
}

// cpeMatchToRange converts a single NVD cpeMatch entry into a KernelRange.
func cpeMatchToRange(m cpeMatch) util.KernelRange {
	var r util.KernelRange
	switch {
	case m.VersionStartIncluding != "":
		r.Lo, r.LoIncl = m.VersionStartIncluding, true
	case m.VersionStartExcluding != "":
		r.Lo, r.LoIncl = m.VersionStartExcluding, false
	}
	switch {
	case m.VersionEndIncluding != "":
		r.Hi, r.HiIncl = m.VersionEndIncluding, true
	case m.VersionEndExcluding != "":
		r.Hi, r.HiIncl = m.VersionEndExcluding, false
	}

	// No range bounds: fall back to an exact version embedded in the CPE
	// criteria (cpe:2.3:o:linux:linux_kernel:6.1.2:*:...).
	if r.Lo == "" && r.Hi == "" {
		if ver := versionFromCriteria(m.Criteria); ver != "" {
			r.Exact = ver
		}
	}
	return r
}

// versionFromCriteria extracts the version field of a linux_kernel CPE string,
// returning "" when it is a wildcard.
func versionFromCriteria(criteria string) string {
	idx := strings.Index(criteria, linuxKernelC)
	if idx < 0 {
		return ""
	}
	rest := criteria[idx+len(linuxKernelC):]
	ver := rest
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		ver = rest[:i]
	}
	if ver == "*" || ver == "-" || ver == "" {
		return ""
	}
	return ver
}
