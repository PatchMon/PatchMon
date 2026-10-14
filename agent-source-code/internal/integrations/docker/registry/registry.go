// Package registry resolves the manifest digest a registry tag currently points at.
package registry

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DockerHubHost is the v2 API host behind docker.io.
const DockerHubHost = "registry-1.docker.io"

const (
	maxTokenBody    = 1 << 20
	maxManifestBody = 4 << 20
	maxRedirects    = 3
	requestTimeout  = 15 * time.Second
	manifestAccept  = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
)

// Errors returned by ManifestDigest, one per reported reason.
var (
	ErrRateLimited        = errors.New("registry rate limit reached")
	ErrAuthFailed         = errors.New("registry refused the credentials")
	ErrNotFound           = errors.New("tag not found in registry")
	ErrUnreachable        = errors.New("registry could not be reached")
	ErrUnsupportedAuth    = errors.New("registry asked for an unsupported authentication scheme")
	ErrInsecureDisallowed = errors.New("registry only speaks plain HTTP and is not marked insecure in the Docker daemon")
	ErrNoDigest           = errors.New("registry returned no usable digest")
)

var digestRe = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// Ref identifies a tag on a registry's v2 API.
type Ref struct {
	Host       string
	Repository string
	Tag        string
}

// ParseRef maps a repository as docker images prints it onto a registry host and path.
func ParseRef(repository, tag string) Ref {
	first, rest, hasSlash := strings.Cut(repository, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		switch first {
		case "docker.io", "index.docker.io", DockerHubHost:
			return hubRef(rest, tag)
		}
		return Ref{Host: first, Repository: rest, Tag: tag}
	}
	return hubRef(repository, tag)
}

func hubRef(path, tag string) Ref {
	if !strings.Contains(path, "/") {
		path = "library/" + path
	}
	return Ref{Host: DockerHubHost, Repository: path, Tag: tag}
}

// Client talks to registries on behalf of one check run. Tokens are cached for its lifetime.
type Client struct {
	Credentials func(ctx context.Context, host string) (Credential, bool)
	Insecure    func(host string) bool
	UserAgent   string
	Transport   http.RoundTripper

	mu         sync.Mutex
	tokens     map[string]string
	verified   *http.Transport
	unverified *http.Transport
}

// ManifestDigest returns the digest the registry serves for ref.
func (c *Client) ManifestDigest(ctx context.Context, ref Ref) (string, error) {
	if ref.Host == "" || ref.Repository == "" || ref.Tag == "" {
		return "", ErrNoDigest
	}
	insecure := c.Insecure != nil && c.Insecure(ref.Host)
	schemes := []string{"https"}
	if insecure {
		schemes = append(schemes, "http")
	}
	var lastErr error
	for _, scheme := range schemes {
		digest, err := c.manifestDigest(ctx, scheme, insecure, ref)
		if err == nil || !errors.Is(err, ErrUnreachable) {
			return digest, err
		}
		lastErr = err
	}
	return "", lastErr
}

func (c *Client) manifestDigest(ctx context.Context, scheme string, insecure bool, ref Ref) (string, error) {
	u := url.URL{Scheme: scheme, Host: ref.Host, Path: "/v2/" + ref.Repository + "/manifests/" + ref.Tag}
	manifestURL := u.String()
	hc := c.httpClient(insecure && scheme == "https")
	scope := "repository:" + ref.Repository + ":pull"
	tokenKey := ref.Host + " " + scope

	auth := c.cachedToken(tokenKey)
	resp, err := c.do(ctx, hc, http.MethodHead, manifestURL, auth)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		challenge := resp.Header.Get("WWW-Authenticate")
		drain(resp, maxTokenBody)
		auth, err = c.authorise(ctx, ref.Host, challenge, scope)
		if err != nil {
			return "", err
		}
		c.storeToken(tokenKey, auth)
		resp, err = c.do(ctx, hc, http.MethodHead, manifestURL, auth)
		if err != nil {
			return "", err
		}
	}

	if err := statusError(resp.StatusCode); err != nil {
		drain(resp, maxTokenBody)
		return "", err
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	drain(resp, maxTokenBody)
	if digestRe.MatchString(digest) {
		return digest, nil
	}

	resp, err = c.do(ctx, hc, http.MethodGet, manifestURL, auth)
	if err != nil {
		return "", err
	}
	defer drain(resp, maxManifestBody)
	if err := statusError(resp.StatusCode); err != nil {
		return "", err
	}
	if digest := resp.Header.Get("Docker-Content-Digest"); digestRe.MatchString(digest) {
		return digest, nil
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(resp.Body, maxManifestBody+1))
	if err != nil {
		return "", fmt.Errorf("%w: reading manifest: %s", ErrUnreachable, redact(err))
	}
	if n == 0 || n > maxManifestBody {
		return "", ErrNoDigest
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func statusError(code int) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		return ErrAuthFailed
	case code == http.StatusNotFound:
		return ErrNotFound
	case code == http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("%w: HTTP %d", ErrUnreachable, code)
	}
}

func (c *Client) authorise(ctx context.Context, host, challenge, scope string) (string, error) {
	scheme, params := parseChallenge(challenge)
	cred, hasCred := Credential{}, false
	if c.Credentials != nil {
		cred, hasCred = c.Credentials(ctx, host)
	}
	switch strings.ToLower(scheme) {
	case "bearer":
		return c.bearerToken(ctx, params, scope, cred, hasCred)
	case "basic":
		if !hasCred || cred.Username == "" || cred.Password == "" {
			return "", ErrAuthFailed
		}
		return basicAuth(cred.Username, cred.Password), nil
	default:
		return "", ErrUnsupportedAuth
	}
}

func (c *Client) bearerToken(ctx context.Context, params map[string]string, scope string, cred Credential, hasCred bool) (string, error) {
	realm, err := url.Parse(params["realm"])
	if err != nil || realm.Scheme != "https" || realm.Host == "" || realm.User != nil {
		return "", ErrUnsupportedAuth
	}
	if s := params["scope"]; s != "" {
		scope = s
	}
	hc := c.httpClient(false)

	var req *http.Request
	if hasCred && cred.IdentityToken != "" {
		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", cred.IdentityToken)
		form.Set("client_id", "patchmon-agent")
		form.Set("scope", scope)
		if s := params["service"]; s != "" {
			form.Set("service", s)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, realm.String(), strings.NewReader(form.Encode()))
		if err != nil {
			return "", ErrUnsupportedAuth
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		q := realm.Query()
		if s := params["service"]; s != "" {
			q.Set("service", s)
		}
		q.Set("scope", scope)
		realm.RawQuery = q.Encode()
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
		if err != nil {
			return "", ErrUnsupportedAuth
		}
		if hasCred && cred.Username != "" && cred.Password != "" {
			req.SetBasicAuth(cred.Username, cred.Password)
		}
	}
	c.setCommonHeaders(req)
	if hasCred {
		hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}

	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: token endpoint: %s", ErrUnreachable, redact(err))
	}
	defer drain(resp, maxTokenBody)
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", ErrRateLimited
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return "", ErrAuthFailed
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("%w: token endpoint returned HTTP %d", ErrUnreachable, resp.StatusCode)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTokenBody)).Decode(&body); err != nil {
		return "", fmt.Errorf("%w: token endpoint returned an unreadable response", ErrUnreachable)
	}
	token := body.Token
	if token == "" {
		token = body.AccessToken
	}
	if token == "" {
		return "", ErrAuthFailed
	}
	return "Bearer " + token, nil
}

func (c *Client) do(ctx context.Context, hc *http.Client, method, rawURL, auth string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, redact(err))
	}
	req.Header.Set("Accept", manifestAccept)
	c.setCommonHeaders(req)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, http.ErrSchemeMismatch) || strings.Contains(err.Error(), "server gave HTTP response to HTTPS client") {
			if c.Insecure == nil || !c.Insecure(req.URL.Host) {
				return nil, ErrInsecureDisallowed
			}
		}
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, redact(err))
	}
	return resp, nil
}

func (c *Client) setCommonHeaders(req *http.Request) {
	ua := c.UserAgent
	if ua == "" {
		ua = "patchmon-agent"
	}
	req.Header.Set("User-Agent", ua)
}

func (c *Client) httpClient(skipVerify bool) *http.Client {
	return &http.Client{
		Transport:     c.transport(skipVerify),
		Timeout:       requestTimeout,
		CheckRedirect: checkRedirect,
	}
}

func (c *Client) transport(skipVerify bool) http.RoundTripper {
	if c.Transport != nil && !skipVerify {
		return c.Transport
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	slot := &c.verified
	if skipVerify {
		slot = &c.unverified
	}
	if *slot == nil {
		*slot = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			// skipVerify is only ever true for registries the Docker daemon itself marks insecure.
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: skipVerify},
			TLSHandshakeTimeout: 10 * time.Second,
			IdleConnTimeout:     30 * time.Second,
			MaxIdleConns:        16,
			MaxIdleConnsPerHost: 2,
		}
	}
	return *slot
}

// CloseIdleConnections releases pooled connections; call it when a check run ends.
func (c *Client) CloseIdleConnections() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range []*http.Transport{c.verified, c.unverified} {
		if t != nil {
			t.CloseIdleConnections()
		}
	}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	orig := via[0]
	if orig.URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errors.New("refusing a redirect from HTTPS to plain HTTP")
	}
	if req.URL.Host != orig.URL.Host && orig.Header.Get("Authorization") != "" {
		return errors.New("refusing to follow a redirect to another host with credentials attached")
	}
	return nil
}

func (c *Client) cachedToken(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens[key]
}

func (c *Client) storeToken(key, auth string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		c.tokens = make(map[string]string)
	}
	c.tokens[key] = auth
}

func basicAuth(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func drain(resp *http.Response, limit int64) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, limit))
	_ = resp.Body.Close()
}

func parseChallenge(header string) (string, map[string]string) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", nil
	}
	scheme, rest, _ := strings.Cut(header, " ")
	params := make(map[string]string)
	for _, part := range splitParams(rest) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		params[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return scheme, params
}

func splitParams(s string) []string {
	var (
		out    []string
		buf    strings.Builder
		quoted bool
	)
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			buf.WriteRune(r)
		case r == ',' && !quoted:
			out = append(out, buf.String())
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}
	if strings.TrimSpace(buf.String()) != "" {
		out = append(out, buf.String())
	}
	return out
}

func redact(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		if u, perr := url.Parse(ue.URL); perr == nil {
			u.User = nil
			u.RawQuery = ""
			return fmt.Sprintf("%s %s: %v", ue.Op, u.String(), ue.Err)
		}
	}
	return err.Error()
}
