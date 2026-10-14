package sshproxy

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var appendMu sync.Mutex

// TOFUHostKeyCallback returns a trust-on-first-use host key callback backed by
// knownHostsPath. An unknown host is pinned (its key appended) and accepted; a
// host whose presented key no longer matches the pinned key is rejected, since
// that is the signature of a man-in-the-middle. The file and its directory are
// created if missing. It never falls back to accepting arbitrary keys.
func TOFUHostKeyCallback(knownHostsPath string, log *slog.Logger) (ssh.HostKeyCallback, error) {
	if err := ensureKnownHostsFile(knownHostsPath); err != nil {
		return nil, err
	}
	inner, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts %s: %w", knownHostsPath, err)
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := inner(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			if pinErr := appendKnownHost(knownHostsPath, hostname, key); pinErr != nil {
				return fmt.Errorf("pin host key for %s: %w", hostname, pinErr)
			}
			if log != nil {
				log.Info("SSH: pinned new host key (trust on first use)", "host", hostname)
			}
			return nil
		}
		if log != nil {
			log.Warn("SSH: host key verification failed", "host", hostname, "error", err)
		}
		return fmt.Errorf("host key verification failed for %s (possible man-in-the-middle): %w", hostname, err)
	}, nil
}

func ensureKnownHostsFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create known_hosts dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create known_hosts file: %w", err)
	}
	return f.Close()
}

func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	appendMu.Lock()
	defer appendMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key) + "\n")
	return err
}

// DefaultKnownHostsPath resolves the base PatchMon-managed known_hosts path. It
// honours PATCHMON_SSH_KNOWN_HOSTS when set, otherwise it uses the first
// writable location among the working directory and the OS temp directory. The
// stock container mounts no volume for these, so pinned keys do not survive
// container recreation unless the override points at a mounted path.
func DefaultKnownHostsPath() string {
	if p := strings.TrimSpace(os.Getenv("PATCHMON_SSH_KNOWN_HOSTS")); p != "" {
		return p
	}
	bases := []string{}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		bases = append(bases, wd)
	}
	bases = append(bases, os.TempDir())
	for _, base := range bases {
		dir := filepath.Join(base, "patchmon-ssh")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			return filepath.Join(dir, "known_hosts")
		}
	}
	return filepath.Join(os.TempDir(), "patchmon_known_hosts")
}

// KnownHostsPath returns the managed known_hosts path for a context. One shared
// process serves every context, so an empty contextKey (single-context) uses
// the base file while each context gets its own sibling file. Without this, one
// context pinning a key for an address that another context legitimately uses
// elsewhere would make the second context's host fail verification.
func KnownHostsPath(contextKey string) string {
	base := DefaultKnownHostsPath()
	if contextKey == "" {
		return base
	}
	return filepath.Join(filepath.Dir(base), "known_hosts_"+sanitizeContextKey(contextKey))
}

func sanitizeContextKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.TrimLeft(b.String(), ".")
	if out == "" {
		return "ctx"
	}
	return out
}
