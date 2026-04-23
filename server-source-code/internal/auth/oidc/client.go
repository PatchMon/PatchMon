// Package oidc provides OpenID Connect authentication client support.
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const maxMicrosoftGraphPhotoBytes = 1 << 20

var microsoftGraphPhotoURL = "https://graph.microsoft.com/v1.0/me/photo/$value"

// SessionData holds PKCE and state data for the OIDC flow.
type SessionData struct {
	State        string
	CodeVerifier string
	Nonce        string
}

// UserInfo holds normalized user claims from OIDC.
type UserInfo struct {
	Sub           string
	Email         string
	Name          string
	GivenName     string
	FamilyName    string
	EmailVerified bool
	Groups        []string
	Picture       string
	IDToken       string
}

// Client wraps the OIDC provider and OAuth2 config.
// Provider discovery is lazy: the first login attempt triggers the HTTP call to
// the issuer's discovery endpoint, so startup succeeds even when the provider is
// temporarily unreachable.
type Client struct {
	cfg      Config
	scopes   []string
	mu       sync.Mutex
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth2   *oauth2.Config
}

// Config holds OIDC client configuration.
type Config struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       string
}

// NewClient creates a new OIDC client. Provider discovery is deferred until the
// first login attempt, so this never fails due to network issues at startup.
func NewClient(_ context.Context, cfg Config) (*Client, error) {
	return &Client{
		cfg:    cfg,
		scopes: parseScopes(cfg.Scopes),
	}, nil
}

// connect performs provider discovery and populates the oauth2 config and token
// verifier. It is idempotent and safe for concurrent callers.
func (c *Client) connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return nil
	}
	provider, err := oidc.NewProvider(ctx, c.cfg.IssuerURL)
	if err != nil {
		return fmt.Errorf("oidc: discover provider: %w", err)
	}
	c.provider = provider
	c.verifier = provider.Verifier(&oidc.Config{ClientID: c.cfg.ClientID})
	c.oauth2 = &oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		RedirectURL:  c.cfg.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       c.scopes,
	}
	return nil
}

func parseScopes(s string) []string {
	if s == "" {
		return []string{oidc.ScopeOpenID, "email", "profile", "groups"}
	}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return []string{oidc.ScopeOpenID, "email", "profile", "groups"}
	}
	hasOpenID := false
	for _, p := range parts {
		if p == oidc.ScopeOpenID {
			hasOpenID = true
			break
		}
	}
	if !hasOpenID {
		parts = append([]string{oidc.ScopeOpenID}, parts...)
	}
	return parts
}

// AuthCodeURL generates the authorization URL with PKCE and returns session data.
func (c *Client) AuthCodeURL(ctx context.Context, state string) (authURL string, session *SessionData, err error) {
	if err := c.connect(ctx); err != nil {
		return "", nil, err
	}
	verifier := oauth2.GenerateVerifier()
	nonce, err := generateNonce()
	if err != nil {
		return "", nil, fmt.Errorf("oidc: generate nonce: %w", err)
	}

	opts := []oauth2.AuthCodeOption{
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("nonce", nonce),
	}

	authURL = c.oauth2.AuthCodeURL(state, opts...)

	session = &SessionData{
		State:        state,
		CodeVerifier: verifier,
		Nonce:        nonce,
	}
	return authURL, session, nil
}

func generateNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Exchange exchanges the authorization code for tokens and fetches UserInfo.
func (c *Client) Exchange(ctx context.Context, code, codeVerifier, expectedState, expectedNonce string, callbackParams url.Values) (*UserInfo, error) {
	if err := c.connect(ctx); err != nil {
		return nil, err
	}
	if code == "" {
		return nil, errors.New("oidc: missing code parameter")
	}
	if expectedState == "" {
		return nil, errors.New("oidc: missing state parameter")
	}

	state := callbackParams.Get("state")
	if state != expectedState {
		return nil, errors.New("oidc: state mismatch")
	}

	opts := []oauth2.AuthCodeOption{
		oauth2.VerifierOption(codeVerifier),
	}

	token, err := c.oauth2.Exchange(ctx, code, opts...)
	if err != nil {
		return nil, fmt.Errorf("oidc: token exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("oidc: no id_token in response")
	}

	idToken, err := c.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: verify id_token: %w", err)
	}

	if idToken.Subject == "" {
		return nil, errors.New("oidc: id_token missing sub claim")
	}

	if expectedNonce != "" {
		if idToken.Nonce != expectedNonce {
			return nil, errors.New("oidc: nonce mismatch")
		}
	}

	userInfo := &UserInfo{
		Sub:     idToken.Subject,
		IDToken: rawIDToken,
	}

	userInfoClaims := make(map[string]interface{})
	if token.AccessToken != "" {
		oidcUserInfo, err := c.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
		if err != nil {
			if !isMicrosoftIdentityIssuer(c.cfg.IssuerURL) {
				return nil, fmt.Errorf("oidc: fetch userinfo: %w", err)
			}
		} else {
			if oidcUserInfo.Subject != "" && oidcUserInfo.Subject != idToken.Subject {
				return nil, errors.New("oidc: UserInfo sub does not match id_token sub")
			}
			userInfoClaims["sub"] = oidcUserInfo.Subject
			userInfoClaims["email"] = oidcUserInfo.Email
			userInfoClaims["email_verified"] = oidcUserInfo.EmailVerified
			userInfoClaims["profile"] = oidcUserInfo.Profile
			var extraClaims map[string]interface{}
			if err := oidcUserInfo.Claims(&extraClaims); err == nil {
				for k, v := range extraClaims {
					userInfoClaims[k] = v
				}
			}
		}
	}

	idClaims := make(map[string]interface{})
	_ = idToken.Claims(&idClaims)

	userInfo.Email = getStringClaim(userInfoClaims, idClaims, "email")
	if userInfo.Email == "" {
		return nil, errors.New("oidc: no email in UserInfo or id_token")
	}

	userInfo.EmailVerified = getBoolClaim(userInfoClaims, idClaims, "email_verified")
	userInfo.Name = getStringClaim(userInfoClaims, idClaims, "name")
	if userInfo.Name == "" {
		userInfo.Name = getStringClaim(userInfoClaims, idClaims, "preferred_username")
	}
	if userInfo.Name == "" {
		userInfo.Name = strings.Split(userInfo.Email, "@")[0]
	}
	userInfo.GivenName = getStringClaim(userInfoClaims, idClaims, "given_name")
	userInfo.FamilyName = getStringClaim(userInfoClaims, idClaims, "family_name")
	userInfo.Picture = getStringClaim(userInfoClaims, idClaims, "picture")
	// Always honour the resolved value, even on error. resolveProviderPicture is
	// responsible for producing a browser-renderable value (data: URL, external
	// https URL, or ""); keeping a raw claim like
	// https://graph.microsoft.com/v1.0/me/photo/$value on failure would result in
	// the browser trying to load an unauthenticated Graph endpoint and logging
	// 401s in the console, which is exactly what we're trying to avoid. Errors
	// from the Graph fetch are swallowed here — "no photo" is a normal state
	// (user hasn't uploaded one, tenant lacks Graph scope, etc.) and should not
	// block login.
	resolvedPicture, _ := resolveProviderPicture(ctx, c.cfg.IssuerURL, token, userInfo.Picture)
	userInfo.Picture = resolvedPicture
	userInfo.Groups = extractGroups(userInfoClaims, idClaims)

	return userInfo, nil
}

func resolveProviderPicture(ctx context.Context, issuerURL string, token *oauth2.Token, rawPicture string) (string, error) {
	if !isMicrosoftIdentityIssuer(issuerURL) {
		return rawPicture, nil
	}
	// For Microsoft Entra, the `picture` claim is NOT directly renderable:
	//   - Often empty (Entra doesn't include it unless configured as an optional claim).
	//   - A bare user GUID.
	//   - Or `https://graph.microsoft.com/v1.0/me/photo/$value` — a Graph API endpoint
	//     that requires a bearer token; the browser can't fetch it directly.
	// So regardless of what the claim contains, the right thing to do is call Graph
	// with our access token and embed the returned image bytes as a data: URL.
	// If Graph fetch succeeds, prefer it; otherwise fall back to the raw claim only
	// if it's a renderable non-Graph URL (rare but possible for hybrid setups).
	if token != nil && token.AccessToken != "" {
		picture, err := fetchMicrosoftGraphPhotoDataURL(ctx, token)
		if err == nil && picture != "" {
			return picture, nil
		}
		// Graph fetch failed or returned no photo — log-by-returning-err semantics are
		// preserved below only when the claim is unusable, so we don't surface transient
		// Graph errors to the caller when we have a usable fallback.
		if err != nil && (rawPicture == "" || isMicrosoftGraphURL(rawPicture)) {
			return "", err
		}
	}
	// No token or Graph failed: fall back to the claim ONLY if it's renderable AND
	// not a Graph API URL (which the browser can never load without auth).
	if isRenderableImageSrc(rawPicture) && !isMicrosoftGraphURL(rawPicture) {
		return rawPicture, nil
	}
	return "", nil
}

// isMicrosoftGraphURL detects URLs pointing at Microsoft Graph, which require a
// bearer token and therefore cannot be used as a browser-renderable <img src>.
func isMicrosoftGraphURL(value string) bool {
	if value == "" {
		return false
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "graph.microsoft.com" ||
		strings.HasSuffix(host, ".graph.microsoft.com") ||
		host == "graph.microsoft.us" ||
		host == "graph.microsoft.de" ||
		host == "microsoftgraph.chinacloudapi.cn"
}

func isMicrosoftIdentityIssuer(issuerURL string) bool {
	u, err := url.Parse(issuerURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "login.microsoftonline.com" ||
		strings.HasSuffix(host, ".microsoftonline.com") ||
		strings.HasSuffix(host, ".microsoftonline.us") ||
		strings.HasSuffix(host, ".microsoftonline.de") ||
		strings.HasSuffix(host, ".chinacloudapi.cn") ||
		host == "sts.windows.net"
}

func isRenderableImageSrc(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(value), "data:image/") {
		return true
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != ""
}

func fetchMicrosoftGraphPhotoDataURL(ctx context.Context, token *oauth2.Token) (picture string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, microsoftGraphPhotoURL, nil)
	if err != nil {
		return "", fmt.Errorf("oidc: create microsoft graph photo request: %w", err)
	}
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("oidc: fetch microsoft graph photo: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("oidc: close microsoft graph photo response body: %w", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oidc: fetch microsoft graph photo: unexpected status %d", resp.StatusCode)
	}

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return "", fmt.Errorf("oidc: parse microsoft graph photo content type: %w", err)
	}
	if !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		return "", fmt.Errorf("oidc: unexpected microsoft graph photo content type %q", mediaType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMicrosoftGraphPhotoBytes+1))
	if err != nil {
		return "", fmt.Errorf("oidc: read microsoft graph photo: %w", err)
	}
	if len(body) == 0 {
		return "", nil
	}
	if len(body) > maxMicrosoftGraphPhotoBytes {
		return "", errors.New("oidc: microsoft graph photo exceeds max size")
	}

	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(body), nil
}

func getStringClaim(primary, fallback map[string]interface{}, key string) string {
	if v, ok := primary[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	if v, ok := fallback[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getBoolClaim(primary, fallback map[string]interface{}, key string) bool {
	if v, ok := primary[key]; ok && v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	if v, ok := fallback[key]; ok && v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// extractGroups extracts group names from claims (groups or ak_groups for Authentik).
// Supports: array of strings, array of objects with "name" key, or single string.
func extractGroups(primary, fallback map[string]interface{}) []string {
	for _, key := range []string{"groups", "ak_groups"} {
		for _, claims := range []map[string]interface{}{primary, fallback} {
			if v, ok := claims[key]; ok && v != nil {
				switch val := v.(type) {
				case []interface{}:
					out := make([]string, 0, len(val))
					for _, item := range val {
						switch t := item.(type) {
						case string:
							out = append(out, t)
						case map[string]interface{}:
							if n, ok := t["name"].(string); ok && n != "" {
								out = append(out, n)
							}
						}
					}
					return out
				case string:
					return []string{val}
				}
			}
		}
	}
	return nil
}

// LogoutURL builds the RP-initiated logout URL if the provider supports it.
// Returns empty string if the provider has not yet been discovered or does not
// advertise an end_session_endpoint.
func (c *Client) LogoutURL(postLogoutRedirectURI, idTokenHint, clientID string) string {
	c.mu.Lock()
	provider := c.provider
	c.mu.Unlock()
	if provider == nil {
		return ""
	}
	var meta struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&meta); err != nil || meta.EndSessionEndpoint == "" {
		return ""
	}

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("post_logout_redirect_uri", postLogoutRedirectURI)
	if idTokenHint != "" {
		params.Set("id_token_hint", idTokenHint)
	}
	return meta.EndSessionEndpoint + "?" + params.Encode()
}
