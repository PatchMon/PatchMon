package handler

import (
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Live compressors hold flate state on a heap shared by every context.
const maxConcurrentAgentGzip = 16

var agentGzipSlots = make(chan struct{}, maxConcurrentAgentGzip)

var agentGzipWriterPool = sync.Pool{
	New: func() interface{} {
		return gzip.NewWriter(io.Discard)
	},
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

// serveAgentBinary writes an agent binary, compressing it when the client
// accepts gzip and a compressor slot is free.
func serveAgentBinary(w http.ResponseWriter, r *http.Request, binaryName string, info os.FileInfo, f *os.File) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, binaryName))
	w.Header().Add("Vary", "Accept-Encoding")

	if !compressible(r) {
		http.ServeContent(w, r, binaryName, info.ModTime(), f)
		return
	}

	select {
	case agentGzipSlots <- struct{}{}:
		defer func() { <-agentGzipSlots }()
	default:
		http.ServeContent(w, r, binaryName, info.ModTime(), f)
		return
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(http.StatusOK)

	gzw := agentGzipWriterPool.Get().(*gzip.Writer)
	gzw.Reset(w)
	defer func() {
		// Without this the pooled writer keeps the ResponseWriter, and with it the
		// connection and request, reachable until the next Get.
		gzw.Reset(io.Discard)
		agentGzipWriterPool.Put(gzw)
	}()

	// A client hanging up mid-download is a client-side event, and during a release
	// stampede it would otherwise flood the log every context shares.
	if _, err := io.Copy(gzw, f); err != nil {
		slog.Debug("agent binary download interrupted", "name", binaryName, "error", err)
		_ = gzw.Close()
		return
	}
	if err := gzw.Close(); err != nil {
		slog.Debug("agent binary download failed to flush", "name", binaryName, "error", err)
	}
}
