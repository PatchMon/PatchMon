package models

import "testing"

func TestTfaEnrolmentPendingMatchesTfaSetupSSORule(t *testing.T) {
	hash, sub, provider := "h", "sub", "idp.example.com"
	cases := []struct {
		name string
		u    User
		want bool
	}{
		{"local account", User{TfaEnrolmentRequired: true, PasswordHash: &hash}, true},
		{"not flagged", User{PasswordHash: &hash}, false},
		{"no password", User{TfaEnrolmentRequired: true}, false},
		{"linked subject", User{TfaEnrolmentRequired: true, PasswordHash: &hash, OidcSub: &sub}, false},
		{"legacy provider-only link", User{TfaEnrolmentRequired: true, PasswordHash: &hash, OidcProvider: &provider}, false},
	}
	for _, c := range cases {
		if got := c.u.TfaEnrolmentPending(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
