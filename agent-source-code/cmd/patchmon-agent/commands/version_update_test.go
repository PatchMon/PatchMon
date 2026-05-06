package commands

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"testing"

	"golang.org/x/crypto/blake2b"
)

// generateTestKeypair creates a minisign-compatible Ed25519 keypair for testing.
// Returns (publicKeyB64, privateKey, keyID).
func generateTestKeypair(t *testing.T) (string, ed25519.PrivateKey, []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}
	keyID := make([]byte, 8)
	if _, err := rand.Read(keyID); err != nil {
		t.Fatalf("failed to generate key ID: %v", err)
	}
	// minisign public key: algorithm(2) + keyID(8) + ed25519PublicKey(32)
	raw := append([]byte("Ed"), keyID...)
	raw = append(raw, pub...)
	return base64.StdEncoding.EncodeToString(raw), priv, keyID
}

// signData creates a minisign signature file for data using the given private key.
func signData(t *testing.T, priv ed25519.PrivateKey, keyID, data []byte, trustedComment string) []byte {
	t.Helper()
	h := blake2b.Sum512(data)
	sig := ed25519.Sign(priv, h[:])

	// minisign signature: algorithm(2) + keyID(8) + signature(64)
	sigRaw := append([]byte("Ed"), keyID...)
	sigRaw = append(sigRaw, sig...)
	sigB64 := base64.StdEncoding.EncodeToString(sigRaw)

	// Global signature covers "trusted comment: <comment>\n"
	tc := fmt.Sprintf("trusted comment: %s", trustedComment)
	globalSig := ed25519.Sign(priv, []byte(tc+"\n"))
	globalB64 := base64.StdEncoding.EncodeToString(globalSig)

	sigFile := fmt.Sprintf("untrusted comment: minisign signature\n%s\n%s\n%s\n", sigB64, tc, globalB64)
	return []byte(sigFile)
}

func TestVerifyMinisignSignature(t *testing.T) {
	pubKeyB64, priv, keyID := generateTestKeypair(t)
	data := []byte("fake agent binary content")

	t.Run("valid signature is accepted", func(t *testing.T) {
		sigFile := signData(t, priv, keyID, data, "PatchMon Agent 2.0.0")
		version, err := verifyMinisignSignature(pubKeyB64, data, sigFile)
		if err != nil {
			t.Fatalf("expected valid signature to pass, got error: %v", err)
		}
		if version != "2.0.0" {
			t.Errorf("expected version 2.0.0, got %q", version)
		}
	})

	t.Run("tampered binary is rejected", func(t *testing.T) {
		sigFile := signData(t, priv, keyID, data, "PatchMon Agent 2.0.0")
		tampered := append([]byte(nil), data...)
		tampered[0] ^= 0xFF
		_, err := verifyMinisignSignature(pubKeyB64, tampered, sigFile)
		if err == nil {
			t.Fatal("expected tampered binary to fail verification")
		}
	})

	t.Run("signature from different key is rejected", func(t *testing.T) {
		_, otherPriv, otherKeyID := generateTestKeypair(t)
		sigFile := signData(t, otherPriv, otherKeyID, data, "PatchMon Agent 2.0.0")
		_, err := verifyMinisignSignature(pubKeyB64, data, sigFile)
		if err == nil {
			t.Fatal("expected signature from different key to fail verification")
		}
	})

	t.Run("invalid public key encoding is rejected", func(t *testing.T) {
		_, err := verifyMinisignSignature("not-valid-base64!!!", data, []byte("sig"))
		if err == nil {
			t.Fatal("expected invalid public key to fail")
		}
	})

	t.Run("malformed signature file is rejected", func(t *testing.T) {
		_, err := verifyMinisignSignature(pubKeyB64, data, []byte("only one line\n"))
		if err == nil {
			t.Fatal("expected malformed signature file to fail")
		}
	})

	t.Run("version is extracted from trusted comment", func(t *testing.T) {
		sigFile := signData(t, priv, keyID, data, "PatchMon Agent 1.9.3")
		version, err := verifyMinisignSignature(pubKeyB64, data, sigFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version != "1.9.3" {
			t.Errorf("expected version 1.9.3, got %q", version)
		}
	})
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		v1, v2 string
		want   int
	}{
		{"2.0.0", "1.9.9", 1},
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"1.10.0", "1.9.0", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"2.0.1", "2.0.0", 1},
		{"2.0.0", "2.0.1", -1},
	}
	for _, c := range cases {
		got := compareVersions(c.v1, c.v2)
		if got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.v1, c.v2, got, c.want)
		}
	}
}

func TestDowngradeProtection(t *testing.T) {
	pubKeyB64, priv, keyID := generateTestKeypair(t)
	data := []byte("fake agent binary content")

	cases := []struct {
		name           string
		signedVersion  string
		currentVersion string
		wantRejected   bool
	}{
		{"newer version is accepted", "2.1.0", "2.0.0", false},
		{"same version is rejected", "2.0.0", "2.0.0", true},
		{"older version is rejected", "1.9.0", "2.0.0", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sigFile := signData(t, priv, keyID, data, "PatchMon Agent "+c.signedVersion)
			signedVersion, err := verifyMinisignSignature(pubKeyB64, data, sigFile)
			if err != nil {
				t.Fatalf("signature verification failed unexpectedly: %v", err)
			}
			rejected := compareVersions(signedVersion, c.currentVersion) <= 0
			if rejected != c.wantRejected {
				t.Errorf("downgrade check for signed=%s current=%s: rejected=%v, want %v",
					c.signedVersion, c.currentVersion, rejected, c.wantRejected)
			}
		})
	}
}

// Ensure keyID mismatch is caught even when signature is otherwise valid.
func TestKeyIDMismatch(t *testing.T) {
	pubKeyB64, priv, _ := generateTestKeypair(t)
	data := []byte("fake agent binary content")

	// Sign with a different keyID than what's in the public key
	wrongKeyID := make([]byte, 8)
	if _, err := rand.Read(wrongKeyID); err != nil {
		t.Fatalf("failed to generate wrong key ID: %v", err)
	}
	sigFile := signData(t, priv, wrongKeyID, data, "PatchMon Agent 2.0.0")

	_, err := verifyMinisignSignature(pubKeyB64, data, sigFile)
	if err == nil {
		t.Fatal("expected key ID mismatch to fail verification")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("key ID")) {
		t.Errorf("expected key ID error, got: %v", err)
	}
}
