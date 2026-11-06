package util

import (
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func hashKey(t testing.TB, key string, cost int) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(key), cost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

func clearVerifiedKeys() {
	verifiedKeys.Lock()
	clear(verifiedKeys.until)
	verifiedKeys.nextSweep = time.Time{}
	verifiedKeys.Unlock()
}

func cachedEntries() int {
	verifiedKeys.Lock()
	defer verifiedKeys.Unlock()
	return len(verifiedKeys.until)
}

func TestVerifyAPIKey(t *testing.T) {
	clearVerifiedKeys()
	stored := hashKey(t, "right-key", bcrypt.MinCost)

	if ok, _ := VerifyAPIKey("wrong-key", stored); ok {
		t.Fatal("wrong key verified")
	}
	if n := cachedEntries(); n != 0 {
		t.Fatalf("failed check was cached: %d entries", n)
	}

	for i := range 2 {
		if ok, _ := VerifyAPIKey("right-key", stored); !ok {
			t.Fatalf("right key rejected on call %d", i+1)
		}
	}
	if n := cachedEntries(); n != 1 {
		t.Fatalf("want 1 cached verification, got %d", n)
	}

	// A cached success for one key must not let a different key through.
	if ok, _ := VerifyAPIKey("wrong-key", stored); ok {
		t.Fatal("wrong key verified after the right key was cached")
	}

	// Rotating the key changes the stored hash, so the old key misses the cache
	// and fails bcrypt.
	rotated := hashKey(t, "new-key", bcrypt.MinCost)
	if ok, _ := VerifyAPIKey("right-key", rotated); ok {
		t.Fatal("old key verified against the rotated hash")
	}
	if ok, _ := VerifyAPIKey("new-key", rotated); !ok {
		t.Fatal("new key rejected")
	}
}

// synctest runs the test on a fake clock, so the sleeps return immediately.
func TestVerifyAPIKeyCacheExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clearVerifiedKeys()
		stored := hashKey(t, "right-key", bcrypt.MinCost)
		id := verificationID(stored, "right-key")

		if ok, _ := VerifyAPIKey("right-key", stored); !ok {
			t.Fatal("right key rejected")
		}
		time.Sleep(verifiedKeyTTL - time.Second)
		if !recentlyVerified(id) {
			t.Fatal("verification expired before the TTL")
		}
		time.Sleep(time.Second)
		if recentlyVerified(id) {
			t.Fatal("verification still trusted after the TTL")
		}

		// The first insert after a TTL sweeps the expired entry.
		if ok, _ := VerifyAPIKey("other-key", hashKey(t, "other-key", bcrypt.MinCost)); !ok {
			t.Fatal("other key rejected")
		}
		if n := cachedEntries(); n != 1 {
			t.Fatalf("expired entry not swept: %d entries", n)
		}

		// After expiry the key is checked with bcrypt again and still verifies.
		if ok, _ := VerifyAPIKey("right-key", stored); !ok {
			t.Fatal("right key rejected after the cache expired")
		}
	})
}

func TestVerifyAPIKeyLegacyPlaintext(t *testing.T) {
	clearVerifiedKeys()
	if ok, _ := VerifyAPIKey("plain", "plain"); !ok {
		t.Fatal("legacy plaintext key rejected")
	}
	if ok, _ := VerifyAPIKey("plain", "other"); ok {
		t.Fatal("wrong legacy plaintext key verified")
	}
	if ok, _ := VerifyAPIKey("", ""); ok {
		t.Fatal("empty key verified")
	}
	if n := cachedEntries(); n != 0 {
		t.Fatalf("plaintext checks were cached: %d entries", n)
	}
}

// BenchmarkVerifyAPIKey compares a bcrypt check at the cost agents' keys are
// hashed with (10, see auto_enrollment.go) against a cached repeat check.
func BenchmarkVerifyAPIKey(b *testing.B) {
	stored := hashKey(b, "right-key", 10)
	b.Run("bcrypt", func(b *testing.B) {
		for b.Loop() {
			clearVerifiedKeys()
			if ok, _ := VerifyAPIKey("right-key", stored); !ok {
				b.Fatal("right key rejected")
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		clearVerifiedKeys()
		if ok, _ := VerifyAPIKey("right-key", stored); !ok {
			b.Fatal("right key rejected")
		}
		for b.Loop() {
			if ok, _ := VerifyAPIKey("right-key", stored); !ok {
				b.Fatal("right key rejected")
			}
		}
	})
}
