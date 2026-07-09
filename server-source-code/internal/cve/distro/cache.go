package distro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	// Short per-attempt timeout so a hung connection (ubuntu.com sometimes
	// stalls) fails fast and we can retry again within the background window,
	// rather than burning the whole budget on one stall.
	httpTimeout = 8 * time.Second
)

// backgroundFetchTimeout bounds a background refresh (which runs off the
// request path, so it can afford generous retries against a rate-limited
// tracker without blocking the Hosts page).
const backgroundFetchTimeout = 150 * time.Second

// advisoryCache memoizes parsed AdvisorySets per CVE for a single source. It is
// the caching layer of the live seam; results are keyed by CVE id.
//
// Lookups never block on the network: on a cache miss the fetch runs in a
// background goroutine (detached from the request context) and the caller gets
// the last known good result if any, or a nil "pending" result that surfaces as
// StatusUnknown until the refresh lands. This keeps the Hosts request fast and
// resilient to a flaky upstream; a completed background fetch is cached for
// hours and served instantly thereafter.
type advisoryCache struct {
	mu       sync.Mutex
	items    map[string]cachedAdvisory
	good     map[string]*AdvisorySet
	inflight map[string]bool
	ttl      time.Duration
	negTTL   time.Duration
}

type cachedAdvisory struct {
	set     *AdvisorySet
	err     error
	expires time.Time
}

func newAdvisoryCache() *advisoryCache {
	return &advisoryCache{
		items:    make(map[string]cachedAdvisory),
		good:     make(map[string]*AdvisorySet),
		inflight: make(map[string]bool),
		ttl:      defaultTTL,
		negTTL:   defaultNegTTL,
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

// do returns a fresh cached result if present; otherwise it kicks off a
// background refresh and returns the last known good result (or nil "pending").
// It never blocks on fetch.
func (c *advisoryCache) do(key string, fetch func(context.Context) (*AdvisorySet, error)) (*AdvisorySet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok && time.Now().Before(e.expires) {
		if e.err != nil {
			if g := c.good[key]; g != nil {
				return g, nil
			}
		}
		return e.set, e.err
	}
	if !c.inflight[key] {
		c.inflight[key] = true
		go c.refresh(key, fetch)
	}
	if g := c.good[key]; g != nil {
		return g, nil // serve stale while refreshing
	}
	return nil, nil // pending -> StatusUnknown for now
}

// refresh performs a background fetch with generous retries and caches it. A
// recover guards against a parser panic wedging the in-flight flag (which would
// permanently block future refreshes for this key).
func (c *advisoryCache) refresh(key string, fetch func(context.Context) (*AdvisorySet, error)) {
	var set *AdvisorySet
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic in distro fetch: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), backgroundFetchTimeout)
		defer cancel()
		set, err = fetch(ctx)
	}()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.inflight[key] = false
	if err != nil {
		log.Printf("[cve/distro] refresh %s FAILED: %v", key, err)
		if g := c.good[key]; g != nil {
			// Keep serving good; re-check after negTTL.
			c.items[key] = cachedAdvisory{set: g, expires: time.Now().Add(c.negTTL)}
			return
		}
		c.items[key] = cachedAdvisory{err: err, expires: time.Now().Add(c.negTTL)}
		return
	}
	n := 0
	if set != nil {
		n = len(set.ByRelease)
	}
	log.Printf("[cve/distro] refresh %s OK: known=%v releases=%d", key, set != nil && set.Known, n)
	c.good[key] = set
	c.items[key] = cachedAdvisory{set: set, expires: time.Now().Add(c.ttl)}
}

// httpClient is the shared client for live source lookups.
var httpClient = &http.Client{Timeout: httpTimeout}

// getJSON fetches url and decodes JSON into out. found is false (with nil error)
// on HTTP 404 so callers can represent "CVE absent from tracker" without an
// error. Transient statuses (429/5xx) are retried a few times with backoff; a
// final non-2xx/404 status or transport failure returns an error.
func getJSON(ctx context.Context, url string, headers map[string]string, out any) (found bool, err error) {
	// Distro trackers (notably ubuntu.com via Cloudflare) rate-limit a shared
	// egress IP with bursty 503s, so retry generously with growing backoff to
	// step outside the throttle window. The result is cached for hours once a
	// single fetch lands, so the extra latency is paid at most once.
	// Space retries ~8s apart rather than in a tight burst: rate limiters like
	// Cloudflare (ubuntu.com) throttle bursts hardest, so spaced requests slip
	// through where a burst gets a wall of 503s. This runs in the background so
	// the spacing costs no request latency. A successful source returns on the
	// first attempt with no wait.
	const maxAttempts = 8
	const retryGap = 8 * time.Second
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(retryGap):
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
