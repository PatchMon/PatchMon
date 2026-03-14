// Package discord provides Discord OAuth2 authentication client support.
package discord

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

const (
	discordAPIBase = "https://discord.com/api"
	discordCDNBase = "https://cdn.discordapp.com"
	discordScopes  = "identify email"
)

// Config holds Discord OAuth2 client configuration.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// DiscordUser holds the Discord user profile from /users/@me.
type DiscordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
	Verified bool   `json:"verified"`
}

// tokenResponse holds the OAuth2 token exchange response.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// GenerateState creates a random 32-byte hex state for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// GeneratePKCE creates a code verifier and S256 code challenge.
func GeneratePKCE() (codeVerifier, codeChallenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	codeVerifier = base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge = base64.RawURLEncoding.EncodeToString(hash[:])
	return codeVerifier, codeChallenge, nil
}

// GenerateAuthURL builds the Discord OAuth2 authorization URL with PKCE.
func (c *Config) GenerateAuthURL(state, codeVerifier string) (string, error) {
	codeChallenge, err := pkceChallenge(codeVerifier)
	if err != nil {
		return "", err
	}
	params := url.Values{
		"client_id":             {c.ClientID},
		"redirect_uri":          {c.RedirectURI},
		"response_type":         {"code"},
		"scope":                 {discordScopes},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return discordAPIBase + "/oauth2/authorize?" + params.Encode(), nil
}

func pkceChallenge(codeVerifier string) (string, error) {
	hash := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(hash[:]), nil
}

// ExchangeCode exchanges the authorization code for an access token.
func (c *Config) ExchangeCode(ctx context.Context, code, codeVerifier string) (accessToken string, err error) {
	params := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.RedirectURI},
		"code_verifier": {codeVerifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordAPIBase+"/oauth2/token", strings.NewReader(params.Encode()))
	if err != nil {
		return "", fmt.Errorf("discord: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("discord: token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("discord: read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discord: token exchange failed: %s", string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("discord: parse token response: %w", err)
	}
	return tr.AccessToken, nil
}

// GetUser fetches the Discord user profile using the access token.
func GetUser(ctx context.Context, accessToken string) (*DiscordUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discordAPIBase+"/users/@me", nil)
	if err != nil {
		return nil, fmt.Errorf("discord: create user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord: fetch user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("discord: read user response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord: fetch user failed: %s", string(body))
	}

	var u DiscordUser
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("discord: parse user response: %w", err)
	}
	return &u, nil
}

// AvatarURL returns the CDN URL for a Discord user's avatar.
// If avatarHash is empty, returns the default avatar based on user ID.
func AvatarURL(userID, avatarHash string) string {
	if avatarHash == "" {
		// Default avatar: (userID >> 22) % 6
		var id big.Int
		_, _ = id.SetString(userID, 10)
		shifted := new(big.Int).Rsh(&id, 22)
		mod := new(big.Int).Mod(shifted, big.NewInt(6))
		idx := mod.Int64()
		if idx < 0 {
			idx = 0
		}
		return fmt.Sprintf("%s/embed/avatars/%d.png", discordCDNBase, idx)
	}
	ext := "png"
	if strings.HasPrefix(avatarHash, "a_") {
		ext = "gif"
	}
	return fmt.Sprintf("%s/avatars/%s/%s.%s?size=256", discordCDNBase, userID, avatarHash, ext)
}
