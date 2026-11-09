package util

import (
	"crypto/sha256"
	"crypto/subtle"
	"maps"
	"regexp"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// bcrypt hash pattern: $2a$, $2b$, or $2y$
var bcryptHashPattern = regexp.MustCompile(`^\$2[aby]\$`)

// Agents report at least daily, so each pays at most one bcrypt per day.
const verifiedKeyTTL = 24 * time.Hour

// verifiedKeys caches successful bcrypt checks (~100 ms each) until they expire.
// Failures are never cached, so a wrong key always pays the full cost.
var verifiedKeys = struct {
	sync.Mutex
	until     map[[sha256.Size]byte]time.Time
	nextSweep time.Time
}{until: make(map[[sha256.Size]byte]time.Time)}

// VerifyAPIKey verifies the provided API key against the stored key.
// Supports both bcrypt hashed keys (new) and plaintext keys (legacy).
// Returns true if the key matches, false otherwise.
func VerifyAPIKey(providedKey, storedKey string) (bool, error) {
	if providedKey == "" || storedKey == "" {
		return false, nil
	}
	if bcryptHashPattern.MatchString(storedKey) {
		id := verificationID(storedKey, providedKey)
		if recentlyVerified(id) {
			return true, nil
		}
		if bcrypt.CompareHashAndPassword([]byte(storedKey), []byte(providedKey)) != nil {
			return false, nil
		}
		rememberVerified(id)
		return true, nil
	}
	// Legacy plaintext key - use timing-safe comparison
	if len(storedKey) != len(providedKey) {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(storedKey), []byte(providedKey)) == 1, nil
}

func verificationID(storedKey, providedKey string) [sha256.Size]byte {
	return sha256.Sum256([]byte(storedKey + "\x00" + providedKey))
}

func recentlyVerified(id [sha256.Size]byte) bool {
	verifiedKeys.Lock()
	defer verifiedKeys.Unlock()
	// A missing entry yields the zero time, which is never after now.
	return time.Now().Before(verifiedKeys.until[id])
}

func rememberVerified(id [sha256.Size]byte) {
	now := time.Now()
	verifiedKeys.Lock()
	defer verifiedKeys.Unlock()
	// Sweep at most once per TTL: one pass a day whatever the fleet size, and
	// the map never holds more than two TTLs' worth of verified keys.
	if !now.Before(verifiedKeys.nextSweep) {
		maps.DeleteFunc(verifiedKeys.until, func(_ [sha256.Size]byte, until time.Time) bool {
			return !now.Before(until)
		})
		verifiedKeys.nextSweep = now.Add(verifiedKeyTTL)
	}
	verifiedKeys.until[id] = now.Add(verifiedKeyTTL)
}
