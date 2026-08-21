package handler

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	agentGzipIdleTTL       = 24 * time.Hour
	agentGzipSweepInterval = time.Hour
)

// singleflight is what bounds concurrent compressors: one per binary version,
// over a servable set fixed at compile time in util.agentBinaryNames.
var (
	agentGzipCache   sync.Map
	agentGzipGroup   singleflight.Group
	agentGzipSweeper sync.Once
	agentGzipFills   atomic.Int64
)

var agentGzipWriterPool = sync.Pool{
	New: func() interface{} {
		return gzip.NewWriter(io.Discard)
	},
}

type agentGzipEntry struct {
	data       []byte
	modTime    time.Time
	size       int64
	lastAccess atomic.Int64
}

// acceptsGzip reports whether the client offered gzip with a non-zero q value.
// A plain substring test would compress for "gzip;q=0", which means the opposite.
func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), "gzip") {
			continue
		}
		for _, param := range fields[1:] {
			param = strings.TrimSpace(param)
			if !strings.HasPrefix(strings.ToLower(param), "q=") {
				continue
			}
			if q, err := strconv.ParseFloat(param[2:], 64); err == nil && q == 0 {
				return false
			}
		}
		return true
	}
	return false
}

// Byte offsets only mean anything against the identity encoding, and only
// ServeContent knows how to answer a conditional request, so both stay raw.
func compressible(r *http.Request) bool {
	if r.Header.Get("Range") != "" || r.Header.Get("If-Range") != "" {
		return false
	}
	if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
		return false
	}
	return acceptsGzip(r.Header.Get("Accept-Encoding"))
}

// A cached entry is only valid for the exact file it was built from: an upgrade
// that replaced the binary would otherwise serve old bytes against a new hash.
func lookupAgentGzip(binaryPath string, info os.FileInfo) ([]byte, bool) {
	v, ok := agentGzipCache.Load(binaryPath)
	if !ok {
		return nil, false
	}
	entry, ok := v.(*agentGzipEntry)
	if !ok || !entry.modTime.Equal(info.ModTime()) || entry.size != info.Size() {
		return nil, false
	}
	entry.lastAccess.Store(time.Now().UnixNano())
	return entry.data, true
}

func buildAgentGzip(binaryPath string, info os.FileInfo, f *os.File) ([]byte, error) {
	// Keying on the path alone would let a follower that stat'd a replaced binary
	// receive the leader's pre-replacement bytes.
	key := fmt.Sprintf("%s|%d|%d", binaryPath, info.ModTime().UnixNano(), info.Size())
	v, err, _ := agentGzipGroup.Do(key, func() (interface{}, error) {
		if data, ok := lookupAgentGzip(binaryPath, info); ok {
			return data, nil
		}

		started := time.Now()
		var buf bytes.Buffer
		buf.Grow(int(info.Size() / 2))

		zw := agentGzipWriterPool.Get().(*gzip.Writer)
		zw.Reset(&buf)
		_, copyErr := io.Copy(zw, f)
		closeErr := zw.Close()
		zw.Reset(io.Discard)
		agentGzipWriterPool.Put(zw)
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}

		compressed := make([]byte, buf.Len())
		copy(compressed, buf.Bytes())

		entry := &agentGzipEntry{data: compressed, modTime: info.ModTime(), size: info.Size()}
		entry.lastAccess.Store(time.Now().UnixNano())
		agentGzipCache.Store(binaryPath, entry)
		agentGzipFills.Add(1)
		startAgentGzipSweeper()

		slog.Info("compressed agent binary",
			"name", filepath.Base(binaryPath),
			"raw_bytes", info.Size(),
			"gzip_bytes", len(entry.data),
			"elapsed_ms", time.Since(started).Milliseconds())

		return entry.data, nil
	})
	if err != nil {
		return nil, err
	}
	data, ok := v.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected cache value for %s", binaryPath)
	}
	return data, nil
}

func startAgentGzipSweeper() {
	agentGzipSweeper.Do(func() {
		go func() {
			ticker := time.NewTicker(agentGzipSweepInterval)
			for range ticker.C {
				evictIdleAgentGzip(time.Now())
			}
		}()
	})
}

func evictIdleAgentGzip(now time.Time) int {
	evicted := 0
	agentGzipCache.Range(func(key, value interface{}) bool {
		entry, ok := value.(*agentGzipEntry)
		if !ok || now.Sub(time.Unix(0, entry.lastAccess.Load())) >= agentGzipIdleTTL {
			agentGzipCache.Delete(key)
			evicted++
		}
		return true
	})
	return evicted
}

// serveAgentBinary writes an agent binary, serving a cached gzip copy when the
// client accepts one.
func serveAgentBinary(w http.ResponseWriter, r *http.Request, binaryPath string, info os.FileInfo, f *os.File) {
	binaryName := filepath.Base(binaryPath)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, binaryName))
	w.Header().Add("Vary", "Accept-Encoding")

	if !compressible(r) {
		http.ServeContent(w, r, binaryName, info.ModTime(), f)
		return
	}

	data, ok := lookupAgentGzip(binaryPath, info)
	if !ok {
		built, err := buildAgentGzip(binaryPath, info, f)
		if err != nil {
			slog.Warn("agent binary compression failed", "name", binaryName, "error", err)
			http.ServeContent(w, r, binaryName, info.ModTime(), f)
			return
		}
		data = built
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)

	// A client hanging up mid-download is a client-side event, and during a release
	// stampede it would otherwise flood the log every context shares.
	if _, err := w.Write(data); err != nil {
		slog.Debug("agent binary download interrupted", "name", binaryName, "error", err)
	}
}
