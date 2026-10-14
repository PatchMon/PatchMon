package sshproxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return signer.PublicKey()
}

func TestTOFUHostKeyCallback(t *testing.T) {
	t.Parallel()

	const hostname = "192.0.2.10:22"
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 22}
	first := testPublicKey(t)
	second := testPublicKey(t)

	tests := []struct {
		name string
		// run receives a freshly built callback and the shared path; it returns
		// the error the handshake would see.
		run     func(t *testing.T, path string) error
		wantErr bool
	}{
		{
			name: "unknown host is pinned and accepted",
			run: func(t *testing.T, path string) error {
				cb := mustCallback(t, path)
				return cb(hostname, remote, first)
			},
			wantErr: false,
		},
		{
			name: "same key accepted on a later connection",
			run: func(t *testing.T, path string) error {
				if err := mustCallback(t, path)(hostname, remote, first); err != nil {
					t.Fatalf("pin: %v", err)
				}
				// Fresh callback re-reads the file, as each connection does.
				return mustCallback(t, path)(hostname, remote, first)
			},
			wantErr: false,
		},
		{
			name: "changed key is rejected as mismatch",
			run: func(t *testing.T, path string) error {
				if err := mustCallback(t, path)(hostname, remote, first); err != nil {
					t.Fatalf("pin: %v", err)
				}
				return mustCallback(t, path)(hostname, remote, second)
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "sub", "known_hosts")
			err := tc.run(t, path)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func mustCallback(t *testing.T, path string) ssh.HostKeyCallback {
	t.Helper()
	cb, err := TOFUHostKeyCallback(path, nil)
	if err != nil {
		t.Fatalf("build callback: %v", err)
	}
	return cb
}

func TestTOFUHostKeyCallbackCreatesFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "dir", "known_hosts")
	if _, err := TOFUHostKeyCallback(path, nil); err != nil {
		t.Fatalf("build callback: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("known_hosts file not created: %v", err)
	}
}

func TestDefaultKnownHostsPathHonoursOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom_known_hosts")
	t.Setenv("PATCHMON_SSH_KNOWN_HOSTS", want)
	if got := DefaultKnownHostsPath(); got != want {
		t.Fatalf("DefaultKnownHostsPath() = %q, want %q", got, want)
	}
}

func TestKnownHostsPathPerContext(t *testing.T) {
	base := filepath.Join(t.TempDir(), "known_hosts")
	t.Setenv("PATCHMON_SSH_KNOWN_HOSTS", base)

	if got := KnownHostsPath(""); got != base {
		t.Fatalf("KnownHostsPath(\"\") = %q, want base %q", got, base)
	}

	a := KnownHostsPath("ctx1.patchmon.cloud")
	b := KnownHostsPath("ctx2.patchmon.cloud")
	if a == b {
		t.Fatalf("expected distinct per-context paths, both = %q", a)
	}
	if a == base || b == base {
		t.Fatalf("per-context path collided with base: a=%q b=%q base=%q", a, b, base)
	}
	if filepath.Dir(a) != filepath.Dir(base) {
		t.Fatalf("per-context file should sit beside base: dir(a)=%q dir(base)=%q", filepath.Dir(a), filepath.Dir(base))
	}

	// A traversal attempt in the context key must not escape the base directory.
	evil := KnownHostsPath("../../etc/evil")
	if filepath.Dir(evil) != filepath.Dir(base) {
		t.Fatalf("sanitised context key escaped base dir: %q", evil)
	}
}
