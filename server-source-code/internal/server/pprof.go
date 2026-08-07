package server

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"
)

// NewPprofServer builds the profiling listener.
//
// pprof is deliberately not mounted on the main router. One process serves every
// context, so a heap dump contains every context's data. Gating it on a
// permission would mean any single context's admin could obtain all of it, and
// the endpoints are expensive enough (/debug/pprof/profile blocks for 30s by
// default) to be a denial-of-service lever on the port serving users.
//
// Instead the listener binds to loopback only, so reachability is the control:
// you need shell access on the host, typically via an SSH tunnel:
//
//	ssh -L 6060:127.0.0.1:6060 <host>
//	go tool pprof http://localhost:6060/debug/pprof/heap
func NewPprofServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		// Loopback only. Binding to :port would expose heap and goroutine dumps
		// on every interface.
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: /debug/pprof/profile and /trace stream for their full
		// duration and would be cut short by one.
	}
}
