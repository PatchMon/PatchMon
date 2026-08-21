package handler

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   bool
	}{
		{"empty", "", false},
		{"bare gzip", "gzip", true},
		{"go transport default", "gzip", true},
		{"browser list", "gzip, deflate, br", true},
		{"with quality", "gzip;q=0.8, deflate", true},
		{"explicitly refused", "gzip;q=0", false},
		{"refused with spaces", "gzip; q=0", false},
		{"refused among others", "deflate, gzip;q=0", false},
		{"zero point zero", "gzip;q=0.0", false},
		{"other codings only", "deflate, br", false},
		{"not a prefix match", "x-gzip", false},
		{"uppercase", "GZIP", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := acceptsGzip(tc.header); got != tc.want {
				t.Fatalf("acceptsGzip(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

func compressibleBinary(t *testing.T, size int) (string, []byte) {
	t.Helper()

	data := make([]byte, size)
	seed := make([]byte, 512)
	if _, err := rand.Read(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for i := range data {
		data[i] = seed[i%len(seed)]
	}

	path := filepath.Join(t.TempDir(), "patchmon-agent-linux-amd64")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path, data
}

func resetAgentGzipCache(t *testing.T) {
	t.Helper()

	agentGzipCache.Range(func(k, _ interface{}) bool {
		agentGzipCache.Delete(k)
		return true
	})
	agentGzipFills.Store(0)
}

func binaryServer(t *testing.T, path string) *httptest.Server {
	t.Helper()
	resetAgentGzipCache(t)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open(path)
		if err != nil {
			t.Errorf("open: %v", err)
			return
		}
		defer func() { _ = f.Close() }()

		info, err := f.Stat()
		if err != nil {
			t.Errorf("stat: %v", err)
			return
		}
		serveAgentBinary(w, r, path, info, f)
	}))
}

func TestServeAgentBinaryGzipRoundTripPreservesHash(t *testing.T) {
	path, want := compressibleBinary(t, 512*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !resp.Uncompressed {
		t.Fatal("expected the transport to have negotiated and unwrapped gzip")
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Fatalf("hash mismatch: got %d bytes, want %d bytes", len(got), len(want))
	}
}

func TestServeAgentBinaryCompressesTheWire(t *testing.T) {
	path, want := compressibleBinary(t, 512*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip")

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}
	if vary := resp.Header.Get("Vary"); vary != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", vary)
	}

	wire, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read wire: %v", err)
	}
	if len(wire) >= len(want) {
		t.Fatalf("wire form %d bytes is not smaller than the raw %d bytes", len(wire), len(want))
	}

	zr, err := gzip.NewReader(bytes.NewReader(wire))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = zr.Close() }()

	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Fatal("gunzipped payload does not match the source file")
	}
}

func TestServeAgentBinaryIdentityWhenGzipNotAccepted(t *testing.T) {
	path, want := compressibleBinary(t, 64*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "identity")

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding = %q, want empty", enc)
	}
	if resp.ContentLength != int64(len(want)) {
		t.Fatalf("Content-Length = %d, want %d", resp.ContentLength, len(want))
	}
}

func TestServeAgentBinaryRangeStaysUncompressed(t *testing.T) {
	path, want := compressibleBinary(t, 64*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=100-199")

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding = %q, want empty for a range request", enc)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("got %d bytes, want 100", len(got))
	}
	for i, b := range got {
		if b != want[100+i] {
			t.Fatalf("byte %d of the range does not match the source file", i)
		}
	}
}

func TestServeAgentBinaryConditionalRequestStillAnswers304(t *testing.T) {
	path, _ := compressibleBinary(t, 64*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-Modified-Since", info.ModTime().UTC().Format(http.TimeFormat))

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", resp.StatusCode)
	}
}

func TestServeAgentBinaryPooledWritersDoNotBleed(t *testing.T) {
	path, want := compressibleBinary(t, 128*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	for i := 0; i < 8; i++ {
		resp, err := srv.Client().Get(srv.URL)
		if err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
		got, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if sha256.Sum256(got) != sha256.Sum256(want) {
			t.Fatalf("response %d does not match the source file", i)
		}
	}
}

func TestServeAgentBinaryCompressesOnlyOncePerBinary(t *testing.T) {
	path, want := compressibleBinary(t, 256*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	for i := 0; i < 12; i++ {
		resp, err := srv.Client().Get(srv.URL)
		if err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
		got, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if sha256.Sum256(got) != sha256.Sum256(want) {
			t.Fatalf("response %d does not match the source file", i)
		}
	}

	if n := agentGzipFills.Load(); n != 1 {
		t.Fatalf("compressed %d times across 12 requests, want 1", n)
	}
}

func TestServeAgentBinaryConcurrentColdRequestsCompressOnce(t *testing.T) {
	path, want := compressibleBinary(t, 512*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	const callers = 32
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	start := make(chan struct{})

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := srv.Client().Get(srv.URL)
			if err != nil {
				errs <- err
				return
			}
			got, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				errs <- err
				return
			}
			if sha256.Sum256(got) != sha256.Sum256(want) {
				errs <- fmt.Errorf("payload mismatch")
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent caller: %v", err)
	}

	if n := agentGzipFills.Load(); n != 1 {
		t.Fatalf("compressed %d times across %d concurrent cold requests, want 1", n, callers)
	}
}

func TestServeAgentBinaryRecompressesWhenBinaryReplaced(t *testing.T) {
	path, _ := compressibleBinary(t, 128*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	replacement := make([]byte, 128*1024)
	for i := range replacement {
		replacement[i] = byte('a' + i%23)
	}
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := os.Chtimes(path, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	resp2, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("second get: %v", err)
	}
	got, err := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if sha256.Sum256(got) != sha256.Sum256(replacement) {
		t.Fatal("served the stale cached copy after the binary was replaced")
	}
	if n := agentGzipFills.Load(); n != 2 {
		t.Fatalf("compressed %d times, want 2 after a replacement", n)
	}
}

func TestEvictIdleAgentGzipDropsOnlyStaleEntries(t *testing.T) {
	resetAgentGzipCache(t)

	fresh := &agentGzipEntry{data: []byte("fresh"), size: 5}
	fresh.lastAccess.Store(time.Now().UnixNano())
	agentGzipCache.Store("/fresh", fresh)

	stale := &agentGzipEntry{data: []byte("stale"), size: 5}
	stale.lastAccess.Store(time.Now().Add(-agentGzipIdleTTL - time.Minute).UnixNano())
	agentGzipCache.Store("/stale", stale)

	if evicted := evictIdleAgentGzip(time.Now()); evicted != 1 {
		t.Fatalf("evicted %d entries, want 1", evicted)
	}
	if _, ok := agentGzipCache.Load("/fresh"); !ok {
		t.Fatal("evicted an entry that was still in use")
	}
	if _, ok := agentGzipCache.Load("/stale"); ok {
		t.Fatal("kept an entry idle past the TTL")
	}
}

func TestServeAgentBinaryCachedResponseCarriesContentLength(t *testing.T) {
	path, _ := compressibleBinary(t, 256*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip")

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.ContentLength <= 0 {
		t.Fatalf("ContentLength = %d, want a positive length on the gzip path", resp.ContentLength)
	}
	wire, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if int64(len(wire)) != resp.ContentLength {
		t.Fatalf("read %d bytes, Content-Length said %d", len(wire), resp.ContentLength)
	}
}

func TestServeAgentBinaryGzipRefusesRangeResumption(t *testing.T) {
	path, _ := compressibleBinary(t, 128*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip")

	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := resp.Header.Get("Accept-Ranges"); got != "none" {
		t.Fatalf("Accept-Ranges = %q, want none so a resume cannot splice identity bytes into a gzip stream", got)
	}
}

func TestCachedGzipDoesNotRetainSpareCapacity(t *testing.T) {
	path, _ := compressibleBinary(t, 1024*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	v, ok := agentGzipCache.Load(path)
	if !ok {
		t.Fatal("entry was not cached")
	}
	entry, ok := v.(*agentGzipEntry)
	if !ok {
		t.Fatal("unexpected cache value type")
	}
	if cap(entry.data) != len(entry.data) {
		t.Fatalf("cached slice retains %d bytes of spare capacity above its %d byte length",
			cap(entry.data)-len(entry.data), len(entry.data))
	}
}
