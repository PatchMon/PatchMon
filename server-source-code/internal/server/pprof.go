package server

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"
)

// NewPprofServer builds the profiling listener.
//
// Not mounted on the main router: a heap dump spans every context, so
// reachability is the control rather than a per-context permission. Reach it
// over an SSH tunnel. See docs PROFILING.md.
func NewPprofServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		// Loopback only.
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: profile and trace stream for their full duration.
	}
}
