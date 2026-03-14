package util

import (
	"crypto/subtle"
	"regexp"

	"golang.org/x/crypto/bcrypt"
)

// bcrypt hash pattern: $2a$, $2b$, or $2y$
var bcryptHashPattern = regexp.MustCompile(`^\$2[aby]\$`)

// VerifyAPIKey verifies the provided API key against the stored key.
// Supports both bcrypt hashed keys (new) and plaintext keys (legacy).
// Returns true if the key matches, false otherwise.
func VerifyAPIKey(providedKey, storedKey string) (bool, error) {
	if providedKey == "" || storedKey == "" {
		return false, nil
	}
	if bcryptHashPattern.MatchString(storedKey) {
		err := bcrypt.CompareHashAndPassword([]byte(storedKey), []byte(providedKey))
		return err == nil, nil
	}
	// Legacy plaintext key - use timing-safe comparison
	if len(storedKey) != len(providedKey) {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(storedKey), []byte(providedKey)) == 1, nil
}
