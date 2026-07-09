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
	defaultTTL = 6 * time.Hour
	// Transient upstream failures (e.g. ubuntu.com 503) are cached only briefly
	// so a blip does not poison results, and we prefer serving the last known
	// good result over caching the error at all (see do()).
	defaultNegTTL = 90 * time.Second
	httpTimeout   = 15 * time.Second
)

// advisoryCache memoizes parsed AdvisorySets per CVE for a single source. It is
// the caching layer of the live seam; results are keyed by CVE id. It keeps the
// last successful result per key ("good") and serves it when a later refresh
// fails, so a flaky upstream degrades gracefully instead of flipping hosts to
// "unknown".
type advisoryCache struct {
	mu     sync.Mutex
	items  map[string]cachedAdvisory
	good   map[string]*AdvisorySet
	ttl    time.Duration
	negTTL time.Duration
}

type cachedAdvisory struct {
	set     *AdvisorySet
	err     error
	expires time.Time
}

func newAdvisoryCache() *advisoryCache {
	return &advisoryCache{
		items:  make(map[string]cachedAdvisory),
		good:   make(map[string]*AdvisorySet),
		ttl:    defaultTTL,
		negTTL: defaultNegTTL,
	}
}

// put stores a result directly (used by tests to seed the cache). A successful
// result also becomes the last-known-good.
func (c *advisoryCache) put(key string, set *AdvisorySet, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.good[key] = set
		c.items[key] = cachedAdvisory{set: set, expires: time.Now().Add(c.ttl)}
		return
	}
	c.items[key] = cachedAdvisory{err: err, expires: time.Now().Add(c.negTTL)}
}

// do returns a cached result if fresh, otherwise runs fetch and caches it. On a
// fetch error it serves the last known good result if one exists (with a short
// re-check window), rather than surfacing the transient failure.
func (c *advisoryCache) do(key string, fetch func() (*AdvisorySet, error)) (*AdvisorySet, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.set, e.err
	}
	good := c.good[key]
	c.mu.Unlock()

	set, err := fetch()

	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		if good != nil {
			// Serve stale-but-good; re-check again after negTTL.
			c.items[key] = cachedAdvisory{set: good, expires: time.Now().Add(c.negTTL)}
			return good, nil
		}
		c.items[key] = cachedAdvisory{err: err, expires: time.Now().Add(c.negTTL)}
		return nil, err
	}
	c.good[key] = set
	c.items[key] = cachedAdvisory{set: set, expires: time.Now().Add(c.ttl)}
	return set, nil
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
