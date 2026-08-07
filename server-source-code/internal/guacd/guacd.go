package guacd

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
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
	done   chan struct{} // closed when wait() returns
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
		if h, p, err := net.SplitHostPort(listenAddr); err == nil {
			if h != "" {
				host = h
			}
			if p != "" {
				port = p
			}
		}
	}

	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, binary, "-b", host, "-l", port)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		if log != nil {
			log.Warn("guacd stdout pipe failed", "error", err)
		}
		cancel()
		return nil
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		if log != nil {
			log.Warn("guacd stderr pipe failed", "error", err)
		}
		cancel()
		return nil
	}

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

	// Pipe subprocess output to the logger.
	if log != nil {
		go pipeToLogger(log, "stdout", stdoutPipe)
		go pipeToLogger(log, "stderr", stderrPipe)
	}

	p := &Process{cmd: cmd, cancel: cancel, done: make(chan struct{}), log: log}
	go p.wait()
	return p
}

func (p *Process) wait() {
	err := p.cmd.Wait()
	p.mu.Lock()
	p.cmd = nil
	p.mu.Unlock()
	close(p.done)
	if p.log != nil && err != nil {
		p.log.Debug("guacd exited", "error", err)
	}
}

// Stop stops the guacd process. It signals the process via context cancellation
// and waits for the existing wait() goroutine to finish, avoiding a double Wait race.
func (p *Process) Stop() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.mu.Unlock()

	// Wait for the existing wait() goroutine to observe the exit.
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		// Force kill if context cancellation wasn't enough.
		p.mu.Lock()
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		p.mu.Unlock()
		<-p.done
	}
}

// Running returns true if guacd is still running.
func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && p.cmd.Process != nil
}

// pipeToLogger reads lines from r and logs them at Debug level.
func pipeToLogger(log *slog.Logger, stream string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Debug("guacd", "stream", stream, "line", scanner.Text())
	}
}
