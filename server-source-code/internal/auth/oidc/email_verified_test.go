package oidc

import "testing"

// A representative Entra issuer; isMicrosoftIdentityIssuer gates the fallback.
const msIssuer = "https://login.microsoftonline.com/tenant-id/v2.0"

// resolveEmailVerified gates account linking and auto-creation, so the ordering
// between email_verified and xms_edov is a security property, not a preference.
func TestResolveEmailVerified(t *testing.T) {
	tests := []struct {
		name            string
		userInfo, idTok map[string]interface{}
		issuer          string
		want            bool
	}{
		{
			name:     "no claims at all fails closed",
			userInfo: map[string]interface{}{},
			idTok:    map[string]interface{}{},
			want:     false,
		},
		{
			name:     "email_verified true is honoured",
			userInfo: map[string]interface{}{"email_verified": true},
			idTok:    map[string]interface{}{},
			want:     true,
		},
		{
			name:     "Entra: xms_edov in the id_token is used when email_verified absent",
			userInfo: map[string]interface{}{},
			idTok:    map[string]interface{}{"xms_edov": true},
			issuer:   msIssuer,
			want:     true,
		},
		{
			name:     "Entra: xms_edov false is still false",
			userInfo: map[string]interface{}{},
			idTok:    map[string]interface{}{"xms_edov": false},
			issuer:   msIssuer,
			want:     false,
		},
		{
			// The security-critical case. Stock Authentik sends an explicit
			// false. A provider denying verification must not be overridden by
			// a second claim.
			name:     "explicit email_verified false is NOT overridden by xms_edov",
			userInfo: map[string]interface{}{"email_verified": false},
			idTok:    map[string]interface{}{"xms_edov": true},
			issuer:   msIssuer,
			want:     false,
		},
		{
			name:     "explicit false in id_token is not overridden either",
			userInfo: map[string]interface{}{},
			idTok:    map[string]interface{}{"email_verified": false, "xms_edov": true},
			issuer:   msIssuer,
			want:     false,
		},
		{
			name:     "string encodings still tolerated",
			userInfo: map[string]interface{}{"email_verified": "true"},
			idTok:    map[string]interface{}{},
			want:     true,
		},
		{
			name:     "xms_edov string encoding tolerated",
			userInfo: map[string]interface{}{},
			idTok:    map[string]interface{}{"xms_edov": "1"},
			issuer:   msIssuer,
			want:     true,
		},
		{
			name:     "userinfo takes precedence over id_token for email_verified",
			userInfo: map[string]interface{}{"email_verified": true},
			idTok:    map[string]interface{}{"email_verified": false},
			want:     true,
		},
		{
			// A nil value is not an assertion, so resolution must continue.
			name:     "nil email_verified falls through to xms_edov",
			userInfo: map[string]interface{}{"email_verified": nil},
			idTok:    map[string]interface{}{"xms_edov": true},
			issuer:   msIssuer,
			want:     true,
		},
		{
			// An undecodable value must not be read as a denial.
			name:     "garbage email_verified falls through to xms_edov",
			userInfo: map[string]interface{}{"email_verified": "banana"},
			idTok:    map[string]interface{}{"xms_edov": true},
			issuer:   msIssuer,
			want:     true,
		},
		{
			// xms_edov is Microsoft-proprietary. An IdP that maps arbitrary
			// user attributes into claims could otherwise manufacture it.
			name:     "xms_edov is ignored for a non-Microsoft issuer",
			userInfo: map[string]interface{}{},
			idTok:    map[string]interface{}{"xms_edov": true},
			issuer:   "https://authentik.example.com/application/o/patchmon/",
			want:     false,
		},
		{
			name:     "xms_edov is ignored when the issuer is empty",
			userInfo: map[string]interface{}{},
			idTok:    map[string]interface{}{"xms_edov": true},
			issuer:   "",
			want:     false,
		},
		{
			// The UserInfo body is not signature-verified; Microsoft only ever
			// issues xms_edov in the ID token.
			name:     "xms_edov from UserInfo is ignored even for Microsoft",
			userInfo: map[string]interface{}{"xms_edov": true},
			idTok:    map[string]interface{}{},
			issuer:   msIssuer,
			want:     false,
		},
		{
			name:     "other Microsoft issuer hosts are accepted",
			userInfo: map[string]interface{}{},
			idTok:    map[string]interface{}{"xms_edov": true},
			issuer:   "https://sts.windows.net/tenant-id/",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := resolveEmailVerified(tt.userInfo, tt.idTok, tt.issuer)
			if got != tt.want {
				t.Errorf("resolveEmailVerified() = %v, want %v (reason %q)", got, tt.want, reason)
			}
			// The reason is what an operator reads in the log, so a rejection
			// must always explain itself and a success must never add noise.
			if !got && reason == "" {
				t.Error("rejection returned an empty reason")
			}
			if got && reason != "" {
				t.Errorf("success returned reason %q, want empty", reason)
			}
		})
	}
}

func TestLookupBoolClaimReportsPresence(t *testing.T) {
	tests := []struct {
		name      string
		claims    map[string]interface{}
		wantValue bool
		wantFound bool
	}{
		{"absent", map[string]interface{}{}, false, false},
		{"nil is absent", map[string]interface{}{"k": nil}, false, false},
		{"undecodable is absent", map[string]interface{}{"k": "banana"}, false, false},
		{"struct is absent", map[string]interface{}{"k": struct{}{}}, false, false},
		{"explicit false is present", map[string]interface{}{"k": false}, false, true},
		{"explicit true is present", map[string]interface{}{"k": true}, true, true},
		{"empty string reads as false, present", map[string]interface{}{"k": ""}, false, true},
		{"zero number is present", map[string]interface{}{"k": float64(0)}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, found := lookupBoolClaim(tt.claims, map[string]interface{}{}, "k")
			if v != tt.wantValue || found != tt.wantFound {
				t.Errorf("lookupBoolClaim() = (%v, %v), want (%v, %v)",
					v, found, tt.wantValue, tt.wantFound)
			}
		})
	}
}
