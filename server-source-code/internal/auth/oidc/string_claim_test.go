package oidc

import "testing"

// A UserInfo response with no email but email_verified=true must not let an
// unsigned assertion verify an address the signed ID token denies.
func TestEmailVerificationFollowsTheEmailSource(t *testing.T) {
	t.Parallel()

	userInfo := map[string]interface{}{"email": "", "email_verified": true}
	idTok := map[string]interface{}{"email": "victim@corp.com", "email_verified": false}

	email, fromIDToken := lookupStringClaim(userInfo, idTok, "email")
	if email != "victim@corp.com" {
		t.Fatalf("email = %q, want the id token value", email)
	}
	if !fromIDToken {
		t.Fatal("email provenance not reported as the id token")
	}
	if verified, _ := resolveEmailVerified(userInfo, idTok, fromIDToken, "https://idp.example.com"); verified {
		t.Error("signed email_verified=false was overridden by the UserInfo assertion")
	}
}

func TestGetStringClaim(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		primary  map[string]interface{}
		fallback map[string]interface{}
		want     string
	}{
		{
			name:    "plain string",
			primary: map[string]interface{}{"email": "user@example.com"},
			want:    "user@example.com",
		},
		{
			name:    "single element array",
			primary: map[string]interface{}{"email": []interface{}{"user@example.com"}},
			want:    "user@example.com",
		},
		{
			name:     "array in the id token",
			primary:  map[string]interface{}{},
			fallback: map[string]interface{}{"email": []interface{}{"user@example.com"}},
			want:     "user@example.com",
		},
		{
			name:     "issue 805: empty userinfo email and an array in the id token",
			primary:  map[string]interface{}{"email": ""},
			fallback: map[string]interface{}{"email": []interface{}{"user@example.com"}},
			want:     "user@example.com",
		},
		{
			name:     "array in primary beats a string in fallback",
			primary:  map[string]interface{}{"email": []interface{}{"primary@example.com"}},
			fallback: map[string]interface{}{"email": "fallback@example.com"},
			want:     "primary@example.com",
		},
		{
			name:     "empty primary does not shadow fallback",
			primary:  map[string]interface{}{"email": ""},
			fallback: map[string]interface{}{"email": "user@example.com"},
			want:     "user@example.com",
		},
		{
			name:     "empty array does not shadow fallback",
			primary:  map[string]interface{}{"email": []interface{}{}},
			fallback: map[string]interface{}{"email": "user@example.com"},
			want:     "user@example.com",
		},
		{
			name:     "array of empty strings does not shadow fallback",
			primary:  map[string]interface{}{"email": []interface{}{"", "  "}},
			fallback: map[string]interface{}{"email": "user@example.com"},
			want:     "user@example.com",
		},
		{
			name:     "whitespace only is absent",
			primary:  map[string]interface{}{"email": "   "},
			fallback: map[string]interface{}{"email": "user@example.com"},
			want:     "user@example.com",
		},
		{
			name:    "padded value returned verbatim, not trimmed",
			primary: map[string]interface{}{"email": "  user@example.com  "},
			want:    "  user@example.com  ",
		},
		{
			name:    "first non-empty element wins",
			primary: map[string]interface{}{"email": []interface{}{"", "first@example.com", "second@example.com"}},
			want:    "first@example.com",
		},
		{
			name:    "primary still preferred over fallback",
			primary: map[string]interface{}{"email": "primary@example.com"},
			fallback: map[string]interface{}{
				"email": "fallback@example.com",
			},
			want: "primary@example.com",
		},
		{
			name:     "non-string element is skipped",
			primary:  map[string]interface{}{"email": []interface{}{42, true}},
			fallback: map[string]interface{}{"email": "user@example.com"},
			want:     "user@example.com",
		},
		{
			name:     "object value is not decodable",
			primary:  map[string]interface{}{"email": map[string]interface{}{"value": "user@example.com"}},
			fallback: map[string]interface{}{},
			want:     "",
		},
		{
			name:     "explicit null does not shadow fallback",
			primary:  map[string]interface{}{"email": nil},
			fallback: map[string]interface{}{"email": "user@example.com"},
			want:     "user@example.com",
		},
		{
			name:     "absent everywhere",
			primary:  map[string]interface{}{},
			fallback: map[string]interface{}{},
			want:     "",
		},
		{
			name:     "nil maps",
			primary:  nil,
			fallback: nil,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := getStringClaim(tt.primary, tt.fallback, "email"); got != tt.want {
				t.Errorf("getStringClaim() = %q, want %q", got, tt.want)
			}
		})
	}
}
