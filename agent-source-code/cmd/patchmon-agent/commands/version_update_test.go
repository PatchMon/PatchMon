package commands

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/blake2b"

	"patchmon-agent/internal/pkgversion"
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
	sigRaw := append([]byte("ED"), keyID...)
	sigRaw = append(sigRaw, sig...)
	sigB64 := base64.StdEncoding.EncodeToString(sigRaw)

	// The global signature covers the signature followed by the comment text.
	tc := fmt.Sprintf("trusted comment: %s", trustedComment)
	globalSig := ed25519.Sign(priv, append(append([]byte{}, sig...), trustedComment...))
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

	t.Run("altered trusted comment is rejected", func(t *testing.T) {
		// A server replaying an old valid signature with a raised version must
		// not get past the downgrade check.
		sigFile := signData(t, priv, keyID, data, "PatchMon Agent 1.0.0")
		forged := strings.Replace(string(sigFile), "PatchMon Agent 1.0.0", "PatchMon Agent 9.9.9", 1)
		if _, err := verifyMinisignSignature(pubKeyB64, data, []byte(forged)); err == nil {
			t.Fatal("expected a rewritten trusted comment to fail verification")
		}
	})

	t.Run("signature that is not an agent signature is rejected", func(t *testing.T) {
		sigFile := signData(t, priv, keyID, data, "PatchMon SSG 0.1.80 ssg-debian12-ds.xml")
		if _, err := verifyMinisignSignature(pubKeyB64, data, sigFile); err == nil {
			t.Fatal("expected a non-agent trusted comment to fail")
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

// TestVerifyMinisignRealSignature checks the parser against a signature made by
// the minisign tool itself (throwaway key, no password), so the format above
// cannot drift from what sign-all produces.
func TestVerifyMinisignRealSignature(t *testing.T) {
	const pub = "RWSSkLb7h5RYIT5eo40pL9u86VAKYqZ0oNvRPSnIYEdjVvVEEruicMaz"
	data := []byte("PatchMon minisign test vector\n")
	sig := []byte("untrusted comment: signature from minisign secret key\n" +
		"RUSSkLb7h5RYIbTfQDXJsSsIdSIcZgQ02/EL/J92MaydRwHdGUkAxC10S2T9WnLq+VljLVNcIQW58J2mVMP0XThFaYNQvvoYdww=\n" +
		"trusted comment: PatchMon Agent 2.1.9-fork.3\n" +
		"S9zhrUBYXI3UfIYZgc5Lmh4ZQpMWMqUXNU2m0K6wPWIkvowOTWfpcdlUq2dIYMgNYcMlrI6y88t/OI9J3tcPAg==\n")
	version, err := verifyMinisignSignature(pub, data, sig)
	if err != nil {
		t.Fatalf("real minisign signature rejected: %v", err)
	}
	if version != "2.1.9-fork.3" {
		t.Fatalf("version = %q", version)
	}
	forged := strings.Replace(string(sig), "2.1.9-fork.3", "9.9.9", 1)
	if _, err := verifyMinisignSignature(pub, data, []byte(forged)); err == nil {
		t.Fatal("rewritten trusted comment accepted")
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
		{"next edge build is accepted", "2.0.3-rc.61", "2.0.3-rc.60", false},
		{"release is accepted over its edge build", "2.0.3", "2.0.3-rc.61", false},
		{"edge build is rejected over its release", "2.0.3-rc.61", "2.0.3", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sigFile := signData(t, priv, keyID, data, "PatchMon Agent "+c.signedVersion)
			signedVersion, err := verifyMinisignSignature(pubKeyB64, data, sigFile)
			if err != nil {
				t.Fatalf("signature verification failed unexpectedly: %v", err)
			}
			rejected := pkgversion.Compare(signedVersion, c.currentVersion) <= 0
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
