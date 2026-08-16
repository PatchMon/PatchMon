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

var agentGzipWriterPool = sync.Pool{
	New: func() interface{} {
		w, err := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		if err != nil {
			return gzip.NewWriter(io.Discard)
		}
		return w
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

// serveAgentBinary writes an agent binary, compressing it when the client
// accepts gzip. The binaries are ~12MB raw and ~5MB gzipped, which decides
// whether a high-latency agent finishes its self-update before timing out.
func serveAgentBinary(w http.ResponseWriter, r *http.Request, binaryName string, info os.FileInfo, f *os.File) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, binaryName))
	w.Header().Add("Vary", "Accept-Encoding")

	// Byte offsets only mean anything against the identity encoding, and only
	// ServeContent knows how to answer a conditional request, so both stay raw.
	conditional := r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" || r.Header.Get("If-Range") != ""
	if r.Header.Get("Range") != "" || conditional || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
		http.ServeContent(w, r, binaryName, info.ModTime(), f)
		return
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(http.StatusOK)

	gzw, ok := agentGzipWriterPool.Get().(*gzip.Writer)
	if !ok {
		gzw = gzip.NewWriter(io.Discard)
	}
	gzw.Reset(w)
	defer agentGzipWriterPool.Put(gzw)

	if _, err := io.Copy(gzw, f); err != nil {
		slog.Warn("agent binary download interrupted", "name", binaryName, "error", err)
		_ = gzw.Close()
		return
	}
	if err := gzw.Close(); err != nil {
		slog.Warn("agent binary download failed to flush", "name", binaryName, "error", err)
	}
}
