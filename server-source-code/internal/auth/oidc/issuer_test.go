package oidc

import "testing"

func TestSameIssuer(t *testing.T) {
	const tid = "11111111-2222-3333-4444-555555555555"
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"https://idp.example.com", "https://idp.example.com", true},
		{"https://idp.example.com/", "https://IDP.example.com", true},
		{"https://idp.example.com/application/o/patchmon/", "https://idp.example.com/application/o/patchmon", true},
		{"https://idp.example.com/realms/a", "https://idp.example.com/realms/b", false},
		{"https://idp.example.com", "http://idp.example.com", false},
		{"https://idp.example.com:8443", "https://idp.example.com", false},
		{"https://idp.example.com", "https://evil.example.net", false},
		{"https://login.microsoftonline.com/" + tid + "/v2.0", "https://sts.windows.net/" + tid + "/", true},
		{"https://login.microsoftonline.com/" + tid + "/v2.0", "https://login.microsoftonline.com/99999999-2222-3333-4444-555555555555/v2.0", false},
		{"https://login.microsoftonline.com/common/v2.0", "https://login.microsoftonline.com/common/v2.0", false},
		{"https://login.microsoftonline.com/{tenantid}/v2.0", "https://login.microsoftonline.com/{tenantid}/v2.0", false},
		{"https://login.microsoftonline.us/" + tid + "/v2.0", "https://sts.windows.net/" + tid + "/", true},
		{"https://login.chinacloudapi.cn/" + tid + "/v2.0", "https://login.partner.microsoftonline.cn/" + tid + "/v2.0", true},
		{"https://evil.chinacloudapi.cn/" + tid + "/v2.0", "https://login.chinacloudapi.cn/" + tid + "/v2.0", false},
		{"https://evil.chinacloudapi.cn/" + tid + "/v2.0", "https://evil.chinacloudapi.cn/" + tid + "/v2.0", true},
		{"https://tenant.b2clogin.microsoftonline.com/" + tid + "/v2.0", "https://login.microsoftonline.com/" + tid + "/v2.0", false},
		{"idp.example.com", "idp.example.com", false},
		{"", "", false},
	} {
		if got := SameIssuer(tc.a, tc.b); got != tc.want {
			t.Errorf("SameIssuer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCanonicalIssuer(t *testing.T) {
	const tid = "11111111-2222-3333-4444-555555555555"
	for _, tc := range []struct{ iss, tid, want string }{
		{"https://idp.example.com/", "", "https://idp.example.com"},
		{"https://idp.example.com", tid, "https://idp.example.com"},
		{"https://login.microsoftonline.com/" + tid + "/v2.0", tid, "https://login.microsoftonline.com/" + tid + "/v2.0"},
		{"https://login.microsoftonline.com/{tenantid}/v2.0", tid, "https://login.microsoftonline.com/" + tid + "/v2.0"},
		{"https://login.microsoftonline.com/common/v2.0", tid, "https://login.microsoftonline.com/" + tid + "/v2.0"},
		{"https://login.microsoftonline.com/common/v2.0", "", "https://login.microsoftonline.com/common/v2.0"},
		{"https://evil.chinacloudapi.cn/common/v2.0", tid, "https://evil.chinacloudapi.cn/common/v2.0"},
	} {
		if got := canonicalIssuer(tc.iss, tc.tid); got != tc.want {
			t.Errorf("canonicalIssuer(%q, %q) = %q, want %q", tc.iss, tc.tid, got, tc.want)
		}
	}
}
