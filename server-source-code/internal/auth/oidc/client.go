// Package oidc provides OpenID Connect authentication client support.
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

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
type Client struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	config   *oauth2.Config
	scopes   []string
}

// Config holds OIDC client configuration.
type Config struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       string
}

// NewClient discovers the OIDC provider and creates a new client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc: discover provider: %w", err)
	}

	scopes := parseScopes(cfg.Scopes)

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	return &Client{
		provider: provider,
		verifier: verifier,
		config:   oauth2Config,
		scopes:   scopes,
	}, nil
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
func (c *Client) AuthCodeURL(state string) (authURL string, session *SessionData, err error) {
	verifier := oauth2.GenerateVerifier()
	nonce, err := generateNonce()
	if err != nil {
		return "", nil, fmt.Errorf("oidc: generate nonce: %w", err)
	}

	opts := []oauth2.AuthCodeOption{
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("nonce", nonce),
	}

	authURL = c.config.AuthCodeURL(state, opts...)

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

	token, err := c.config.Exchange(ctx, code, opts...)
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
			return nil, fmt.Errorf("oidc: fetch userinfo: %w", err)
		}
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
	userInfo.Groups = extractGroups(userInfoClaims, idClaims)

	return userInfo, nil
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
func (c *Client) LogoutURL(postLogoutRedirectURI, idTokenHint, clientID string) string {
	var meta struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := c.provider.Claims(&meta); err != nil || meta.EndSessionEndpoint == "" {
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
