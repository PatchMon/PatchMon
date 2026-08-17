package store

import (
	"strings"
	"testing"
)

// The identifier becomes a Redis key, and a failed attempt for a username that
// does not exist creates one. Interpolating the raw username would let an
// unauthenticated caller store a key as large as the request body limit allows.
func TestIdentifierIsBoundedRegardlessOfUsernameLength(t *testing.T) {
	t.Parallel()

	s := &LoginLockoutStore{}
	const ip = "203.0.113.9"

	short := s.Identifier(ip, "admin")
	huge := s.Identifier(ip, strings.Repeat("A", 5<<20))

	if len(short) != len(huge) {
		t.Errorf("identifier length varies with username: %d vs %d", len(short), len(huge))
	}
	if strings.Contains(huge, "AAAA") {
		t.Error("identifier embeds the raw username; an oversized username would become an oversized Redis key")
	}
	if !strings.HasPrefix(huge, ip+"|") {
		t.Errorf("identifier lost its IP prefix: %q", huge)
	}
}

// GetUserByUsernameOrEmail matches with LOWER(), so these are all one account
// and must share one attempt counter. Keying on the raw string would hand an
// attacker a fresh allowance per capitalisation.
func TestIdentifierIsCaseAndWhitespaceInsensitive(t *testing.T) {
	t.Parallel()

	s := &LoginLockoutStore{}
	const ip = "203.0.113.9"

	want := s.Identifier(ip, "admin")
	for _, variant := range []string{"Admin", "ADMIN", "aDmIn", " admin", "admin ", "  Admin  "} {
		if got := s.Identifier(ip, variant); got != want {
			t.Errorf("Identifier(%q) = %q, want the same key as %q", variant, got, "admin")
		}
	}
}

func TestIdentifierSeparatesDifferentUsersAndIPs(t *testing.T) {
	t.Parallel()

	s := &LoginLockoutStore{}

	if s.Identifier("203.0.113.9", "alice") == s.Identifier("203.0.113.9", "bob") {
		t.Error("different usernames share a lockout counter")
	}
	if s.Identifier("203.0.113.9", "alice") == s.Identifier("198.51.100.4", "alice") {
		t.Error("different client IPs share a lockout counter")
	}
}
