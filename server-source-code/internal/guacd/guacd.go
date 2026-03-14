package guacd

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// IsRemoteAddress returns true if addr points to a remote host (not localhost).
// Used when guacd runs as a sidecar (e.g. Docker) instead of a subprocess.
func IsRemoteAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port; treat as hostname
		host = strings.TrimSpace(addr)
	}
	host = strings.ToLower(host)
	return host != "" && host != "127.0.0.1" && host != "localhost" && host != "::1"
}

// Process manages the guacd subprocess lifecycle.
type Process struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	mu     sync.Mutex
	log    *slog.Logger
}

// Start attempts to start guacd. Returns nil if guacd binary is not found or fails to start.
// guacdPath: path to guacd binary, or empty to use PATH.
// listenAddr: e.g. "127.0.0.1:4822"
func Start(ctx context.Context, guacdPath string, listenAddr string, log *slog.Logger) *Process {
	binary := guacdPath
	if binary == "" {
		binary = "guacd"
	}
	if _, err := exec.LookPath(binary); err != nil {
		if log != nil {
			log.Debug("guacd not found, RDP disabled", "path", binary, "error", err)
		}
		return nil
	}

	// Parse listenAddr into host and port for guacd -b and -l
	// guacd -b 127.0.0.1 -l 4822
	host, port := "127.0.0.1", "4822"
	if listenAddr != "" {
		// Simple parse: "host:port"
		for i, c := range listenAddr {
			if c == ':' {
				if i > 0 {
					host = listenAddr[:i]
				}
				if i+1 < len(listenAddr) {
					port = listenAddr[i+1:]
				}
				break
			}
		}
	}

	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, binary, "-b", host, "-l", port)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		if log != nil {
			log.Warn("guacd failed to start", "error", err)
		}
		cancel()
		return nil
	}

	if log != nil {
		log.Info("guacd started", "addr", listenAddr, "pid", cmd.Process.Pid)
	}

	p := &Process{cmd: cmd, cancel: cancel, log: log}
	go p.wait()
	return p
}

func (p *Process) wait() {
	err := p.cmd.Wait()
	p.mu.Lock()
	p.cmd = nil
	p.mu.Unlock()
	if p.log != nil && err != nil {
		p.log.Debug("guacd exited", "error", err)
	}
}

// Stop stops the guacd process.
func (p *Process) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = p.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = p.cmd.Process.Kill()
		}
	}
}

// Running returns true if guacd is still running.
func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && p.cmd.Process != nil
}
