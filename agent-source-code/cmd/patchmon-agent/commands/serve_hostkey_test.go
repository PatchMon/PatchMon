package commands

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func agentTestPublicKey(t *testing.T) ssh.PublicKey {
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

func agentMustCallback(t *testing.T, path string) ssh.HostKeyCallback {
	t.Helper()
	cb, err := agentHostKeyCallback(path)
	if err != nil {
		t.Fatalf("build callback: %v", err)
	}
	return cb
}

func TestAgentHostKeyCallbackTOFU(t *testing.T) {
	t.Parallel()

	const hostname = "198.51.100.7:22"
	remote := &net.TCPAddr{IP: net.ParseIP("198.51.100.7"), Port: 22}
	first := agentTestPublicKey(t)
	second := agentTestPublicKey(t)

	tests := []struct {
		name    string
		run     func(t *testing.T, path string) error
		wantErr bool
	}{
		{
			name: "unknown host pinned and accepted",
			run: func(t *testing.T, path string) error {
				return agentMustCallback(t, path)(hostname, remote, first)
			},
			wantErr: false,
		},
		{
			name: "same key accepted later",
			run: func(t *testing.T, path string) error {
				if err := agentMustCallback(t, path)(hostname, remote, first); err != nil {
					t.Fatalf("pin: %v", err)
				}
				return agentMustCallback(t, path)(hostname, remote, first)
			},
			wantErr: false,
		},
		{
			name: "changed key rejected",
			run: func(t *testing.T, path string) error {
				if err := agentMustCallback(t, path)(hostname, remote, first); err != nil {
					t.Fatalf("pin: %v", err)
				}
				return agentMustCallback(t, path)(hostname, remote, second)
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

func TestValidateSSHProxyHostLinkLocal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"cloud metadata", "169.254.169.254", true},
		{"link-local v4", "169.254.0.1", true},
		{"loopback", "127.0.0.1", false},
		{"localhost", "localhost", false},
		{"private 10", "10.0.0.5", false},
		{"private 192", "192.168.1.1", false},
		{"hostname", "db.internal.example.com", false},
		{"empty", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateSSHProxyHost(tc.host)
			if tc.wantErr && err == nil {
				t.Fatalf("validateSSHProxyHost(%q) = nil, want error", tc.host)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateSSHProxyHost(%q) = %v, want nil", tc.host, err)
			}
		})
	}
}
