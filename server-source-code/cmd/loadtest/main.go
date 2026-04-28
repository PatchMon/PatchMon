//go:build loadtest

// loadtest drives concurrent host reports at a running PatchMon server to
// reproduce — or, after the deadlock fix lands, fail to reproduce — the
// 40P01 deadlocks observed in production at 100+ hosts.
//
// Build with the loadtest tag so it never ends up in a normal binary:
//
//	go build -tags loadtest -o /tmp/loadtest ./cmd/loadtest
//
// See cmd/loadtest/README.md for usage.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// productionHostBlocklist is a list of substrings that, if found in --server,
// cause loadtest to refuse to run. This is a cheap safety net to prevent
// pointing the driver at production by accident — the tool is designed to
// hammer an endpoint as hard as it can and there is no scenario where that
// is appropriate against the live SaaS.
var productionHostBlocklist = []string{
	"patchmon.cloud",
	"patchmon.net",
}

type creds struct {
	APIID  string
	APIKey string
}

type reportPackage struct {
	Name             string  `json:"name"`
	Description      string  `json:"description,omitempty"`
	Category         string  `json:"category,omitempty"`
	CurrentVersion   string  `json:"currentVersion"`
	AvailableVersion *string `json:"availableVersion,omitempty"`
	NeedsUpdate      bool    `json:"needsUpdate"`
	IsSecurityUpdate bool    `json:"isSecurityUpdate"`
	SourceRepository string  `json:"sourceRepository,omitempty"`
}

type reportPayload struct {
	Packages       []reportPackage `json:"packages"`
	Repositories   []any           `json:"repositories"`
	OSType         string          `json:"osType"`
	OSVersion      string          `json:"osVersion"`
	Hostname       string          `json:"hostname"`
	IP             string          `json:"ip"`
	Architecture   string          `json:"architecture"`
	AgentVersion   string          `json:"agentVersion"`
	MachineID      string          `json:"machineId"`
	PackageManager string          `json:"packageManager"`
}

type stats struct {
	mu        sync.Mutex
	latencies []time.Duration

	requests        atomic.Int64
	success         atomic.Int64
	clientErrors    atomic.Int64 // 4xx
	serverErrors    atomic.Int64 // 5xx
	contextTimeouts atomic.Int64
	otherErrors     atomic.Int64
}

func (s *stats) record(latency time.Duration) {
	s.mu.Lock()
	s.latencies = append(s.latencies, latency)
	s.mu.Unlock()
}

func (s *stats) percentile(p float64) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.latencies)
	if n == 0 {
		return 0
	}
	sorted := make([]time.Duration, n)
	copy(sorted, s.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(n-1) * p)
	return sorted[idx]
}

func main() {
	server := flag.String("server", "http://localhost:3001", "PatchMon server base URL")
	hostsCount := flag.Int("hosts", 100, "number of concurrent hosts")
	pkgsPerReport := flag.Int("packages", 200, "packages per report")
	pkgPool := flag.Int("pool", 5000, "total pool of unique package names (shared across hosts)")
	duration := flag.Duration("duration", 30*time.Second, "test duration")
	credsFile := flag.String("creds", "", "CSV file with api_id,api_key per host (required)")
	requestTimeout := flag.Duration("request-timeout", 30*time.Second, "per-request HTTP timeout")
	flag.Parse()

	if *credsFile == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --creds is required (CSV: api_id,api_key)")
		os.Exit(2)
	}

	// Refuse to run against any host that looks like production.
	serverLower := strings.ToLower(*server)
	for _, banned := range productionHostBlocklist {
		if strings.Contains(serverLower, banned) {
			fmt.Fprintf(os.Stderr, "ERROR: refusing to load-test production host %q (matched %q in blocklist)\n",
				*server, banned)
			os.Exit(2)
		}
	}

	allCreds, err := loadCreds(*credsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR loading creds: %v\n", err)
		os.Exit(2)
	}
	if len(allCreds) < *hostsCount {
		fmt.Fprintf(os.Stderr, "ERROR: --hosts=%d but creds file only has %d entries\n", *hostsCount, len(allCreds))
		os.Exit(2)
	}

	pkgNames := make([]string, *pkgPool)
	for i := range pkgNames {
		pkgNames[i] = fmt.Sprintf("loadtest-pkg-%05d", i)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	st := &stats{}
	var wg sync.WaitGroup
	wg.Add(*hostsCount)

	httpClient := &http.Client{Timeout: *requestTimeout}

	start := time.Now()
	for i := 0; i < *hostsCount; i++ {
		c := allCreds[i]
		go func(c creds) {
			defer wg.Done()
			runHost(ctx, httpClient, *server, c, pkgNames, *pkgsPerReport, st)
		}(c)
	}
	wg.Wait()
	elapsed := time.Since(start)

	reqs := st.requests.Load()
	ok := st.success.Load()
	c4xx := st.clientErrors.Load()
	c5xx := st.serverErrors.Load()
	tos := st.contextTimeouts.Load()
	other := st.otherErrors.Load()

	fmt.Println("==== load test results ====")
	fmt.Printf("server:           %s\n", *server)
	fmt.Printf("hosts:            %d\n", *hostsCount)
	fmt.Printf("packages/report:  %d\n", *pkgsPerReport)
	fmt.Printf("package pool:     %d\n", *pkgPool)
	fmt.Printf("elapsed:          %v\n", elapsed)
	fmt.Printf("requests:         %d\n", reqs)
	fmt.Printf("success (2xx):    %d\n", ok)
	fmt.Printf("client err (4xx): %d\n", c4xx)
	fmt.Printf("server err (5xx): %d\n", c5xx)
	fmt.Printf("ctx timeouts:     %d\n", tos)
	fmt.Printf("other errors:     %d\n", other)
	if elapsed.Seconds() > 0 {
		fmt.Printf("throughput:       %.1f req/s\n", float64(reqs)/elapsed.Seconds())
	}
	fmt.Printf("p50 latency:      %v\n", st.percentile(0.50))
	fmt.Printf("p95 latency:      %v\n", st.percentile(0.95))
	fmt.Printf("p99 latency:      %v\n", st.percentile(0.99))

	if c5xx == 0 && tos == 0 {
		fmt.Println("RESULT: PASS")
		os.Exit(0)
	}
	fmt.Println("RESULT: FAIL")
	os.Exit(1)
}

func loadCreds(path string) ([]creds, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := make([]creds, 0, len(rows))
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		out = append(out, creds{APIID: row[0], APIKey: row[1]})
	}
	return out, nil
}

func runHost(ctx context.Context, hc *http.Client, baseURL string, c creds, pool []string, perReport int, st *stats) {
	// Each host uses its own RNG seeded by APIID hash to keep results
	// reproducible across runs with the same creds.
	r := rand.New(rand.NewPCG(uint64(hashString(c.APIID)), uint64(hashString(c.APIKey))))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		payload := buildPayload(r, c, pool, perReport)
		body, err := json.Marshal(payload)
		if err != nil {
			st.otherErrors.Add(1)
			continue
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/hosts/update", bytes.NewReader(body))
		if err != nil {
			st.otherErrors.Add(1)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-ID", c.APIID)
		req.Header.Set("X-API-KEY", c.APIKey)

		t0 := time.Now()
		resp, err := hc.Do(req)
		latency := time.Since(t0)

		st.requests.Add(1)
		if err != nil {
			if ctx.Err() != nil {
				st.contextTimeouts.Add(1)
			} else {
				st.otherErrors.Add(1)
			}
			st.record(latency)
			continue
		}
		_ = resp.Body.Close()

		st.record(latency)
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			st.success.Add(1)
		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			st.clientErrors.Add(1)
		case resp.StatusCode >= 500:
			st.serverErrors.Add(1)
		default:
			st.otherErrors.Add(1)
		}
	}
}

func buildPayload(r *rand.Rand, c creds, pool []string, perReport int) reportPayload {
	pkgs := make([]reportPackage, 0, perReport)
	for i := 0; i < perReport; i++ {
		name := pool[r.IntN(len(pool))]
		needs := r.IntN(10) < 3 // ~30% need update
		sec := needs && r.IntN(10) < 2
		var avail *string
		if needs {
			v := fmt.Sprintf("2.0.%d", r.IntN(50))
			avail = &v
		}
		pkgs = append(pkgs, reportPackage{
			Name:             name,
			CurrentVersion:   fmt.Sprintf("1.0.%d", r.IntN(50)),
			AvailableVersion: avail,
			NeedsUpdate:      needs,
			IsSecurityUpdate: sec,
		})
	}
	return reportPayload{
		Packages:       pkgs,
		Repositories:   []any{},
		OSType:         "ubuntu",
		OSVersion:      "22.04",
		Hostname:       "loadtest-" + c.APIID,
		Architecture:   "x86_64",
		AgentVersion:   "loadtest",
		PackageManager: "apt",
	}
}

// hashString is a tiny FNV-1a hash for deterministic per-host RNG seeding.
func hashString(s string) uint64 {
	const (
		offset64 uint64 = 1469598103934665603
		prime64  uint64 = 1099511628211
	)
	h := offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}
