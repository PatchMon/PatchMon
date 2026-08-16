package handler

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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

func binaryServer(t *testing.T, path string) *httptest.Server {
	t.Helper()

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
		serveAgentBinary(w, r, filepath.Base(path), info, f)
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

func TestServeAgentBinaryFallsBackToIdentityWhenSlotsExhausted(t *testing.T) {
	path, want := compressibleBinary(t, 64*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	for i := 0; i < maxConcurrentAgentGzip; i++ {
		agentGzipSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < maxConcurrentAgentGzip; i++ {
			<-agentGzipSlots
		}
	}()

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

	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("Content-Encoding = %q, want empty once slots are exhausted", enc)
	}
	if resp.ContentLength != int64(len(want)) {
		t.Fatalf("Content-Length = %d, want %d", resp.ContentLength, len(want))
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Fatal("fallback response does not match the source file")
	}
}

func TestServeAgentBinarySlotsAreReleased(t *testing.T) {
	path, _ := compressibleBinary(t, 32*1024)
	srv := binaryServer(t, path)
	defer srv.Close()

	for i := 0; i < maxConcurrentAgentGzip+4; i++ {
		resp, err := srv.Client().Get(srv.URL)
		if err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	if len(agentGzipSlots) != 0 {
		t.Fatalf("%d slots still held after sequential requests", len(agentGzipSlots))
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
