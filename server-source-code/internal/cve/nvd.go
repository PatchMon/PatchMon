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
	CVEID        string             `json:"cve_id"`
	Description  string             `json:"description"`
	CVSSScore    float64            `json:"cvss_score,omitempty"`
	CVSSSeverity string             `json:"cvss_severity,omitempty"`
	CVSSVector   string             `json:"cvss_vector,omitempty"`
	Published    string             `json:"published,omitempty"`
	LastModified string             `json:"last_modified,omitempty"`
	References   []string           `json:"references,omitempty"`
	Weaknesses   []string           `json:"weaknesses,omitempty"` // CWE ids
	Labels       []string           `json:"labels,omitempty"`     // derived: RCE/LPE/DoS/…
	Ranges       []util.KernelRange `json:"ranges"`
	Filter       *util.KernelFilter `json:"-"`
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
			Published    string `json:"published"`
			LastModified string `json:"lastModified"`
			Descriptions []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			} `json:"descriptions"`
			Metrics struct {
				V31 []cvssMetric `json:"cvssMetricV31"`
				V30 []cvssMetric `json:"cvssMetricV30"`
			} `json:"metrics"`
			References []struct {
				URL string `json:"url"`
			} `json:"references"`
			Weaknesses []struct {
				Description []struct {
					Value string `json:"value"`
				} `json:"description"`
			} `json:"weaknesses"`
			Configurations []struct {
				Nodes []struct {
					CPEMatch []cpeMatch `json:"cpeMatch"`
				} `json:"nodes"`
			} `json:"configurations"`
		} `json:"cve"`
	} `json:"vulnerabilities"`
}

type cvssMetric struct {
	CVSSData struct {
		BaseScore    float64 `json:"baseScore"`
		BaseSeverity string  `json:"baseSeverity"`
		VectorString string  `json:"vectorString"`
	} `json:"cvssData"`
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
	res.Published = v.Published
	res.LastModified = v.LastModified
	metrics := v.Metrics.V31
	if len(metrics) == 0 {
		metrics = v.Metrics.V30
	}
	if len(metrics) > 0 {
		res.CVSSScore = metrics[0].CVSSData.BaseScore
		res.CVSSSeverity = metrics[0].CVSSData.BaseSeverity
		res.CVSSVector = metrics[0].CVSSData.VectorString
	}
	for i, ref := range v.References {
		if i >= 6 {
			break
		}
		res.References = append(res.References, ref.URL)
	}
	seenCWE := map[string]bool{}
	for _, w := range v.Weaknesses {
		for _, d := range w.Description {
			if strings.HasPrefix(d.Value, "CWE-") && !seenCWE[d.Value] {
				seenCWE[d.Value] = true
				res.Weaknesses = append(res.Weaknesses, d.Value)
			}
		}
	}
	res.Labels = deriveLabels(res.CVSSVector, res.Description, res.Weaknesses)

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

// cweNames maps common CWE ids to a short weakness label.
var cweNames = map[string]string{
	"CWE-416": "use-after-free",
	"CWE-415": "double-free",
	"CWE-787": "out-of-bounds write",
	"CWE-125": "out-of-bounds read",
	"CWE-476": "NULL deref",
	"CWE-362": "race condition",
	"CWE-190": "integer overflow",
	"CWE-401": "memory leak",
	"CWE-119": "buffer overflow",
	"CWE-120": "buffer overflow",
	"CWE-200": "info exposure",
	"CWE-269": "privilege management",
	"CWE-770": "resource exhaustion",
	"CWE-667": "improper locking",
}

// vectorField returns the value of a CVSS vector field (e.g. "AV" -> "L").
func vectorField(vector, key string) string {
	for _, part := range strings.Split(strings.ToUpper(vector), "/") {
		if kv := strings.SplitN(part, ":", 2); len(kv) == 2 && kv[0] == key {
			return kv[1]
		}
	}
	return ""
}

// deriveLabels produces heuristic impact labels (RCE/LPE/DoS/…) from the CVSS
// vector, description keywords and CWE ids. Heuristic, not authoritative.
func deriveLabels(vector, description string, cwes []string) []string {
	var out []string
	add := func(s string) {
		for _, x := range out {
			if x == s {
				return
			}
		}
		out = append(out, s)
	}
	av := vectorField(vector, "AV")
	cI := vectorField(vector, "C")
	iI := vectorField(vector, "I")
	aI := vectorField(vector, "A")
	prI := vectorField(vector, "PR")
	d := strings.ToLower(description)
	highImpact := cI == "H" || iI == "H"

	switch av {
	case "N":
		add("Remote")
	case "A":
		add("Adjacent")
	case "L":
		add("Local")
	case "P":
		add("Physical")
	}

	switch {
	case strings.Contains(d, "remote code execution"), av == "N" && iI == "H" && (strings.Contains(d, "code execution") || strings.Contains(d, "overflow") || strings.Contains(d, "use-after-free")):
		add("RCE")
	}
	if strings.Contains(d, "privilege escalation") || strings.Contains(d, "local privilege") ||
		(av == "L" && iI == "H" && cI == "H" && prI != "N") {
		add("LPE")
	}
	if aI == "H" && cI == "N" && iI == "N" {
		add("DoS")
	}
	if cI == "H" && iI != "H" && !containsLabel(out, "RCE") && !containsLabel(out, "LPE") {
		add("Info leak")
	}
	if !highImpact && aI != "H" && (strings.Contains(d, "denial of service") || strings.Contains(d, "crash")) {
		add("DoS")
	}
	for _, cwe := range cwes {
		if name, ok := cweNames[cwe]; ok {
			add(name)
		}
	}
	return out
}

func containsLabel(labels []string, s string) bool {
	for _, x := range labels {
		if x == s {
			return true
		}
	}
	return false
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
