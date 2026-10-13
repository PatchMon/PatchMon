package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const testDigest = "sha256:b997b9ab1bcbd44e4c2eb3ffbd3a8b9a8c8a6b4d0c8e3f2a1b0c9d8e7f6a5b4c"

func TestParseRef(t *testing.T) {
	tests := []struct {
		repo, tag string
		want      Ref
	}{
		{"nginx", "1.25", Ref{DockerHubHost, "library/nginx", "1.25"}},
		{"portainer/portainer-ce", "lts", Ref{DockerHubHost, "portainer/portainer-ce", "lts"}},
		{"docker.io/library/nginx", "1", Ref{DockerHubHost, "library/nginx", "1"}},
		{"index.docker.io/nginx", "1", Ref{DockerHubHost, "library/nginx", "1"}},
		{"ghcr.io/x/y", "1", Ref{"ghcr.io", "x/y", "1"}},
		{"host:5000/a/b", "tag", Ref{"host:5000", "a/b", "tag"}},
		{"localhost/app", "dev", Ref{"localhost", "app", "dev"}},
		{"localhost:5000/team/app", "stable", Ref{"localhost:5000", "team/app", "stable"}},
	}
	for _, tt := range tests {
		if got := ParseRef(tt.repo, tt.tag); got != tt.want {
			t.Errorf("ParseRef(%q, %q) = %+v, want %+v", tt.repo, tt.tag, got, tt.want)
		}
	}
}

func TestParseChallenge(t *testing.T) {
	scheme, params := parseChallenge(`Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:a/b:pull,push"`)
	if scheme != "Bearer" {
		t.Fatalf("scheme = %q", scheme)
	}
	want := map[string]string{
		"realm":   "https://auth.example.com/token",
		"service": "registry.example.com",
		"scope":   "repository:a/b:pull,push",
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("%s = %q, want %q", k, params[k], v)
		}
	}
}

func refFor(srv *httptest.Server, repo, tag string) Ref {
	u, _ := url.Parse(srv.URL)
	return Ref{Host: u.Host, Repository: repo, Tag: tag}
}

func newClient(srv *httptest.Server, cred *Credential) *Client {
	c := &Client{Transport: srv.Client().Transport}
	if cred != nil {
		c.Credentials = func(context.Context, string) (Credential, bool) { return *cred, true }
	}
	return c
}

func TestBearerChallengeWithBasicCredentials(t *testing.T) {
	var tokenHits atomic.Int32
	tokenSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHits.Add(1)
		user, pass, ok := r.BasicAuth()
		if !ok || user != "alice" || pass != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.URL.Query().Get("scope"); got != "repository:a/b:pull,push" {
			t.Errorf("scope = %q", got)
		}
		if got := r.URL.Query().Get("service"); got != "reg" {
			t.Errorf("service = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"access_token":"T0K"}`)
	}))
	defer tokenSrv.Close()

	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Path != "/v2/a/b/manifests/1.0" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Accept"), "application/vnd.oci.image.index.v1+json") {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("Authorization") != "Bearer T0K" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`/token",service="reg",scope="repository:a/b:pull,push"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer reg.Close()

	c := newClient(reg, &Credential{Username: "alice", Password: "s3cret"})
	for range 2 {
		got, err := c.ManifestDigest(context.Background(), refFor(reg, "a/b", "1.0"))
		if err != nil || got != testDigest {
			t.Fatalf("digest = %q, err = %v", got, err)
		}
	}
	if n := tokenHits.Load(); n != 1 {
		t.Errorf("token endpoint hit %d times, want 1 (cached)", n)
	}
}

func TestBearerIdentityToken(t *testing.T) {
	tokenSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("grant_type") != "refresh_token" || r.PostForm.Get("refresh_token") != "idtok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprint(w, `{"token":"T"}`)
	}))
	defer tokenSrv.Close()
	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer T" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`/token",service="reg"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer reg.Close()

	got, err := newClient(reg, &Credential{IdentityToken: "idtok"}).ManifestDigest(context.Background(), refFor(reg, "a", "1"))
	if err != nil || got != testDigest {
		t.Fatalf("digest = %q, err = %v", got, err)
	}
}

func TestBasicChallenge(t *testing.T) {
	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "bob" || p != "pw" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer reg.Close()

	got, err := newClient(reg, &Credential{Username: "bob", Password: "pw"}).ManifestDigest(context.Background(), refFor(reg, "a", "1"))
	if err != nil || got != testDigest {
		t.Fatalf("digest = %q, err = %v", got, err)
	}

	_, err = newClient(reg, nil).ManifestDigest(context.Background(), refFor(reg, "a", "1"))
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("anonymous Basic challenge err = %v, want ErrAuthFailed", err)
	}
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{"401 after auth", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="r"`)
			w.WriteHeader(http.StatusUnauthorized)
		}, ErrAuthFailed},
		{"403", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }, ErrAuthFailed},
		{"404", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, ErrNotFound},
		{"429", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }, ErrRateLimited},
		{"500", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }, ErrUnreachable},
		{"unknown scheme", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Negotiate`)
			w.WriteHeader(http.StatusUnauthorized)
		}, ErrUnsupportedAuth},
		{"http realm", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="http://auth.example.com/token"`)
			w.WriteHeader(http.StatusUnauthorized)
		}, ErrUnsupportedAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := httptest.NewTLSServer(tt.handler)
			defer reg.Close()
			_, err := newClient(reg, &Credential{Username: "u", Password: "p"}).ManifestDigest(context.Background(), refFor(reg, "a", "1"))
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestHeadWithoutDigestFallsBackToGet(t *testing.T) {
	index := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
	sum := sha256.Sum256(index)
	want := "sha256:" + hex.EncodeToString(sum[:])
	var gets atomic.Int32
	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		if r.Method == http.MethodGet {
			gets.Add(1)
			_, _ = w.Write(index)
		}
	}))
	defer reg.Close()

	got, err := newClient(reg, nil).ManifestDigest(context.Background(), refFor(reg, "a", "multi"))
	if err != nil || got != want {
		t.Fatalf("digest = %q, err = %v, want %q", got, err, want)
	}
	if gets.Load() != 1 {
		t.Errorf("GET fallback ran %d times", gets.Load())
	}
}

func TestRedirects(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("credentials reached another host")
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer other.Close()

	authed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="r"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer authed.Close()

	_, err := newClient(authed, &Credential{Username: "u", Password: "s3cret"}).ManifestDigest(context.Background(), refFor(authed, "a", "1"))
	if !errors.Is(err, ErrUnreachable) {
		t.Errorf("cross-host redirect with credentials err = %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "s3cret") {
		t.Error("error leaks the password")
	}
	if otherHits.Load() != 0 {
		t.Errorf("other host received %d requests", otherHits.Load())
	}

	anon := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer anon.Close()
	got, err := newClient(anon, nil).ManifestDigest(context.Background(), refFor(anon, "a", "1"))
	if err != nil || got != testDigest {
		t.Errorf("anonymous redirect digest = %q, err = %v", got, err)
	}

	var loops atomic.Int32
	var loop *httptest.Server
	loop = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loops.Add(1)
		http.Redirect(w, r, loop.URL+r.URL.Path, http.StatusFound)
	}))
	defer loop.Close()
	if _, err := newClient(loop, nil).ManifestDigest(context.Background(), refFor(loop, "a", "1")); !errors.Is(err, ErrUnreachable) {
		t.Errorf("redirect loop err = %v", err)
	}
	if n := loops.Load(); n > maxRedirects+1 {
		t.Errorf("followed %d redirects", n-1)
	}
}

func TestPlainHTTPRegistry(t *testing.T) {
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer reg.Close()
	ref := refFor(reg, "team/app", "stable")

	_, err := (&Client{}).ManifestDigest(context.Background(), ref)
	if !errors.Is(err, ErrInsecureDisallowed) {
		t.Errorf("secure registry over plain HTTP err = %v", err)
	}

	c := &Client{Insecure: func(host string) bool { return host == ref.Host }}
	got, err := c.ManifestDigest(context.Background(), ref)
	if err != nil || got != testDigest {
		t.Errorf("insecure registry digest = %q, err = %v", got, err)
	}
}

func TestRedactStripsUserinfo(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://user:pw@reg.example.com/v2/?token=abc", Err: errors.New("boom")}
	got := redact(err)
	if strings.Contains(got, "pw") || strings.Contains(got, "abc") {
		t.Errorf("redact = %q", got)
	}
}

func TestCredentialedTokenRequestNeverRedirects(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		otherHits.Add(1)
		_, _ = fmt.Fprint(w, `{"token":"T"}`)
	}))
	defer other.Close()

	tokenSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/token", http.StatusTemporaryRedirect)
	}))
	defer tokenSrv.Close()

	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer T" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`/token",service="reg"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
	}))
	defer reg.Close()

	for _, cred := range []Credential{{IdentityToken: "idtok"}, {Username: "u", Password: "s3cret"}} {
		_, err := newClient(reg, &cred).ManifestDigest(context.Background(), refFor(reg, "a", "1"))
		if !errors.Is(err, ErrUnreachable) {
			t.Errorf("err = %v, want ErrUnreachable", err)
		}
	}
	if n := otherHits.Load(); n != 0 {
		t.Errorf("credentialed token request was redirected to another host %d times", n)
	}
}

func TestClientReusesTransports(t *testing.T) {
	c := &Client{}
	if c.transport(false) != c.transport(false) || c.transport(true) != c.transport(true) {
		t.Error("transports must be built once per client")
	}
	if c.transport(false) == c.transport(true) {
		t.Error("verified and unverified transports must differ")
	}
	tr, ok := c.transport(false).(*http.Transport)
	if !ok || tr.IdleConnTimeout == 0 || tr.MaxIdleConnsPerHost == 0 {
		t.Errorf("transport lacks idle limits: %+v", tr)
	}
	c.CloseIdleConnections()
}
