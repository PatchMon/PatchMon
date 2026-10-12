package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
)

func newDiscoveryServer(t *testing.T, issuerSuffix string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 srv.URL + issuerSuffix,
			"authorization_endpoint": srv.URL + "/authorize",
			"token_endpoint":         srv.URL + "/token",
			"jwks_uri":               srv.URL + "/jwks",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestConnectAcceptsIssuerWithDiscoverySuffix(t *testing.T) {
	for _, tc := range []struct {
		name, issuerSuffix, configured string
	}{
		{"plain", "", ""},
		{"discovery suffix", "", "/.well-known/openid-configuration"},
		{"discovery suffix with slash", "", "/.well-known/openid-configuration/"},
		{"slash issuer plain", "/", "/"},
		{"slash issuer with discovery suffix", "/", "/.well-known/openid-configuration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newDiscoveryServer(t, tc.issuerSuffix)
			ctx := gooidc.ClientContext(context.Background(), srv.Client())
			issuer := srv.URL + tc.configured
			c, err := NewClient(ctx, Config{IssuerURL: issuer, ClientID: "id"})
			if err != nil {
				t.Fatal(err)
			}
			if err := c.connect(ctx); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestNormaliseIssuerURL(t *testing.T) {
	cases := map[string]string{
		"https://idp.example.com":                                                           "https://idp.example.com",
		"https://idp.example.com/":                                                          "https://idp.example.com/",
		"https://idp.example.com/application/o/patchmon/":                                   "https://idp.example.com/application/o/patchmon/",
		"https://idp.example.com/.well-known/openid-configuration":                          "https://idp.example.com",
		"https://idp.example.com/realms/x/.well-known/openid-configuration/":                "https://idp.example.com/realms/x",
		" https://idp.example.com/application/o/patchmon/.well-known/openid-configuration ": "https://idp.example.com/application/o/patchmon",
	}
	for in, want := range cases {
		if got, _ := normaliseIssuerURL(in); got != want {
			t.Errorf("normaliseIssuerURL(%q) = %q, want %q", in, got, want)
		}
	}
}
