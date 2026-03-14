package util

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// GCM requires 12-byte nonce (96 bits); 16-byte causes "incorrect nonce length" panic
	ivLength      = 12
	authTagLength = 16
)

// Encryption provides AES-256-GCM encrypt/decrypt.
// Key derivation matches Node's encryption.js: AI_ENCRYPTION_KEY > SESSION_SECRET > DATABASE_URL.
type Encryption struct {
	key []byte
}

// NewEncryption derives encryption key from environment.
// Order: AI_ENCRYPTION_KEY (64 hex chars) > SESSION_SECRET > DATABASE_URL.
func NewEncryption() (*Encryption, error) {
	key := deriveKey()
	if key == nil {
		return nil, fmt.Errorf("encryption: could not derive key from environment")
	}
	return &Encryption{key: key}, nil
}

func deriveKey() []byte {
	if v := os.Getenv("AI_ENCRYPTION_KEY"); v != "" {
		return keyFromHexOrHash(v)
	}
	if v := os.Getenv("SESSION_SECRET"); v != "" {
		h := sha256.Sum256([]byte(v))
		return h[:]
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		h := sha256.Sum256([]byte("patchmon-enc-" + v))
		return h[:]
	}
	return nil
}

func keyFromHexOrHash(s string) []byte {
	s = strings.TrimSpace(s)
	if len(s) == 64 {
		b, err := hex.DecodeString(s)
		if err == nil {
			return b
		}
	}
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// Encrypt encrypts plaintext using AES-256-GCM.
// Format: iv:authTag:ciphertext (all hex encoded), matching Node.
func (e *Encryption) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, ivLength)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, iv, []byte(plaintext), nil)
	authTag := ciphertext[len(ciphertext)-authTagLength:]
	ct := ciphertext[:len(ciphertext)-authTagLength]
	return hex.EncodeToString(iv) + ":" + hex.EncodeToString(authTag) + ":" + hex.EncodeToString(ct), nil
}

// IsEncrypted returns true if the string appears to be in encrypted format (iv:authTag:ciphertext).
func IsEncrypted(s string) bool {
	if s == "" {
		return false
	}
	parts := strings.Split(s, ":")
	return len(parts) == 3 &&
		len(parts[0]) == ivLength*2 &&
		len(parts[1]) == authTagLength*2
}

// Decrypt decrypts data encrypted with Encrypt.
func (e *Encryption) Decrypt(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	parts := strings.Split(encrypted, ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("encryption: invalid format")
	}
	iv, err := hex.DecodeString(parts[0])
	if err != nil || len(iv) != ivLength {
		return "", fmt.Errorf("encryption: invalid iv")
	}
	authTag, err := hex.DecodeString(parts[1])
	if err != nil || len(authTag) != authTagLength {
		return "", fmt.Errorf("encryption: invalid auth tag")
	}
	ciphertext, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	combined := append(ciphertext, authTag...)
	plaintext, err := aead.Open(nil, iv, combined, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
