package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/oidc"
)

func TestOidcBaseUsername(t *testing.T) {
	cases := []struct {
		name, preferred, email, want string
	}{
		{"preferred username wins", "brambo", "brant.rambo@example.com", "brambo"},
		{"missing preferred falls back to email", "", "brant.rambo@example.com", "brant.rambo"},
		{"UPN-style preferred keeps the local part", "alice@contoso.com", "alice.smith@contoso.com", "alice"},
		{"unsafe characters are stripped", "b rambo!", "x@example.com", "brambo"},
		{"too short after sanitising falls back", "!", "brant.rambo@example.com", "brant.rambo"},
		{"two-character preferred falls back", "jd", "john.doe@example.com", "john.doe"},
		{"long values are capped at 32", "abcdefghijklmnopqrstuvwxyz0123456789", "x@example.com", "abcdefghijklmnopqrstuvwxyz012345"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := oidcBaseUsername(&oidc.UserInfo{PreferredUsername: tc.preferred, Email: tc.email})
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
