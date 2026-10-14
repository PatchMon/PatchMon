package oidc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeOP struct {
	srv          *httptest.Server
	key          *rsa.PrivateKey
	sub          string
	userInfo     func(w http.ResponseWriter)
	jwksPadding  int
	discoveryPad int
	tokenPad     int
}

func newFakeOP(t *testing.T, sub string) *fakeOP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	op := &fakeOP{key: key, sub: sub}
	op.userInfo = func(w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": sub, "email": "alice@example.com", "email_verified": true})
	}
	op.srv = httptest.NewServer(http.HandlerFunc(op.serve))
	t.Cleanup(op.srv.Close)
	return op
}

func (op *fakeOP) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case discoveryPath:
		_, _ = w.Write([]byte(strings.Repeat(" ", op.discoveryPad)))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 op.srv.URL,
			"authorization_endpoint": op.srv.URL + "/authorize",
			"token_endpoint":         op.srv.URL + "/token",
			"userinfo_endpoint":      op.srv.URL + "/userinfo",
			"jwks_uri":               op.srv.URL + "/jwks",
		})
	case "/jwks":
		_, _ = w.Write([]byte(strings.Repeat(" ", op.jwksPadding)))
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": b64(op.key.N.Bytes()),
			"e": b64(big.NewInt(int64(op.key.E)).Bytes()),
		}}})
	case "/token":
		_, _ = w.Write([]byte(strings.Repeat(" ", op.tokenPad)))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "Bearer", "expires_in": 300,
			"id_token": op.idToken(),
		})
	case "/userinfo":
		op.userInfo(w)
	default:
		http.NotFound(w, r)
	}
}

func (op *fakeOP) idToken() string {
	now := time.Now().Unix()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	payload, _ := json.Marshal(map[string]any{
		"iss": op.srv.URL, "aud": "client", "sub": op.sub, "iat": now, "exp": now + 300,
		"nonce": "nonce", "email": "alice@example.com", "email_verified": true,
	})
	signing := b64(header) + "." + b64(payload)
	sum := sha256.Sum256([]byte(signing))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, op.key, crypto.SHA256, sum[:])
	return signing + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (op *fakeOP) exchange(t *testing.T) (*UserInfo, error) {
	t.Helper()
	c, err := NewClient(context.Background(), Config{
		IssuerURL: op.srv.URL, ClientID: "client", ClientSecret: "secret",
		RedirectURI: "http://localhost/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c.Exchange(context.Background(), "code", "verifier", "state", "nonce", url.Values{"state": {"state"}})
}

func TestExchangeAcceptsMatchingUserInfoSub(t *testing.T) {
	op := newFakeOP(t, "user-1")
	info, err := op.exchange(t)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if info.Sub != "user-1" || info.Issuer != op.srv.URL || info.Email != "alice@example.com" || !info.EmailVerified {
		t.Fatalf("unexpected user info: %+v", info)
	}
}

func TestExchangeRejectsUserInfoSubMismatchOrAbsence(t *testing.T) {
	for name, body := range map[string]map[string]any{
		"different sub": {"sub": "user-2", "email": "alice@example.com"},
		"empty sub":     {"sub": "", "email": "alice@example.com"},
		"no sub":        {"email": "alice@example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			op := newFakeOP(t, "user-1")
			op.userInfo = func(w http.ResponseWriter) { _ = json.NewEncoder(w).Encode(body) }
			if _, err := op.exchange(t); err == nil || !strings.Contains(err.Error(), "UserInfo") {
				t.Fatalf("expected a UserInfo sub error, got %v", err)
			}
		})
	}
}
