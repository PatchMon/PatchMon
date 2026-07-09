package distro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Default cache lifetimes for live source lookups. A future DB-sync source can
// bypass these entirely by serving pre-fetched AdvisorySets.
const (
	defaultTTL    = 6 * time.Hour
	defaultNegTTL = 30 * time.Minute
	httpTimeout   = 15 * time.Second
)

// advisoryCache memoizes parsed AdvisorySets per CVE for a single source. It is
// the caching layer of the live seam; results are keyed by CVE id.
type advisoryCache struct {
	mu     sync.Mutex
	items  map[string]cachedAdvisory
	ttl    time.Duration
	negTTL time.Duration
}

type cachedAdvisory struct {
	set     *AdvisorySet
	err     error
	expires time.Time
}

func newAdvisoryCache() *advisoryCache {
	return &advisoryCache{items: make(map[string]cachedAdvisory), ttl: defaultTTL, negTTL: defaultNegTTL}
}

// get returns a cached entry if present and unexpired.
func (c *advisoryCache) get(key string) (*AdvisorySet, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok && time.Now().Before(e.expires) {
		return e.set, e.err, true
	}
	return nil, nil, false
}

// put stores an entry, using the negative TTL when err != nil.
func (c *advisoryCache) put(key string, set *AdvisorySet, err error) {
	ttl := c.ttl
	if err != nil {
		ttl = c.negTTL
	}
	c.mu.Lock()
	c.items[key] = cachedAdvisory{set: set, err: err, expires: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// httpClient is the shared client for live source lookups.
var httpClient = &http.Client{Timeout: httpTimeout}

// getJSON fetches url and decodes JSON into out. found is false (with nil error)
// on HTTP 404 so callers can represent "CVE absent from tracker" without an
// error. Transient statuses (429/5xx) are retried a few times with backoff; a
// final non-2xx/404 status or transport failure returns an error.
func getJSON(ctx context.Context, url string, headers map[string]string, out any) (found bool, err error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(time.Duration(attempt) * 750 * time.Millisecond):
			}
		}
		found, retry, e := getJSONOnce(ctx, url, headers, out)
		if e == nil {
			return found, nil
		}
		lastErr = e
		if !retry {
			return false, e
		}
	}
	return false, lastErr
}

// getJSONOnce performs a single request. retry is true when the failure is
// transient (429/5xx) and worth another attempt.
func getJSONOnce(ctx context.Context, url string, headers map[string]string, out any) (found, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PatchMon-cve-distro/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, true, fmt.Errorf("fetching %s (server may lack internet access): %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, false, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return false, true, fmt.Errorf("%s returned %d: %s", url, resp.StatusCode, body)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return false, false, fmt.Errorf("%s returned %d: %s", url, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return false, false, fmt.Errorf("decoding %s: %w", url, err)
	}
	return true, false, nil
}
