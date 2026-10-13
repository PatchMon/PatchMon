package handler

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const jtiTestSecret = "jti-test-secret-0123456789abcdef"

func parseTestToken(t *testing.T, tok string) jwt.MapClaims {
	t.Helper()
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(tok, &claims, func(*jwt.Token) (interface{}, error) {
		return []byte(jtiTestSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"})); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestTokensMintedInTheSameSecondDiffer(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{cfg: &config.Config{JWTSecret: jtiTestSecret}}
	mint := map[string]func() (string, error){
		"refresh": func() (string, error) { return h.createRefreshToken("user-1", "admin", 3600) },
		"access":  func() (string, error) { return h.createAccessToken("user-1", "admin", "session-1", 900) },
	}
	for name, fn := range mint {
		seen := map[string]bool{}
		for i := 0; i < 64; i++ {
			tok, err := fn()
			if err != nil {
				t.Fatal(err)
			}
			if seen[tok] {
				t.Fatalf("%s token minted twice for the same user: %s", name, tok)
			}
			seen[tok] = true
		}
	}
}

func TestTokenClaimsCarryRandomJTI(t *testing.T) {
	t.Parallel()
	h := &AuthHandler{cfg: &config.Config{JWTSecret: jtiTestSecret}}

	refresh, err := h.createRefreshToken("user-1", "admin", 3600)
	if err != nil {
		t.Fatal(err)
	}
	access, err := h.createAccessToken("user-1", "admin", "session-1", 900)
	if err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		tok, typ, sessionID string
	}{
		"refresh": {refresh, tokenTypeRefresh, ""},
		"access":  {access, tokenTypeAccess, "session-1"},
	} {
		c := parseTestToken(t, tc.tok)
		jti, _ := c["jti"].(string)
		raw, err := base64.RawURLEncoding.DecodeString(jti)
		if err != nil || len(raw) != 16 {
			t.Errorf("%s: jti %q is not 128 bits of base64url", name, jti)
		}
		if c["sub"] != "user-1" || c["role"] != "admin" || c["typ"] != tc.typ {
			t.Errorf("%s: claims changed: %v", name, c)
		}
		if sid, ok := c["sessionId"]; tc.sessionID == "" && ok || tc.sessionID != "" && sid != tc.sessionID {
			t.Errorf("%s: sessionId = %v, want %q", name, sid, tc.sessionID)
		}
		if _, ok := c["exp"]; !ok {
			t.Errorf("%s: no exp", name)
		}
	}
}

func TestTwoLoginsInOneSecondGetSeparateSessions(t *testing.T) {
	d := pgtest.Open(t)
	ctx := context.Background()
	users := store.NewUsersStore(d)
	sessions := store.NewSessionsStore(d)
	h := &AuthHandler{cfg: &config.Config{JWTSecret: jtiTestSecret}, users: users, sessions: sessions}

	u := &models.User{
		ID:       uuid.New().String(),
		Username: "jti" + uuid.New().String()[:12],
		Email:    "jti-" + uuid.New().String()[:8] + "@example.com",
		Role:     "admin",
		IsActive: true,
	}
	if err := users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	var refresh [2]string
	var sessionIDs [2]string
	for i, ua := range []string{"browser-a/1.0", "browser-b/1.0"} {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		r.Header.Set("User-Agent", ua)
		access, rt, err := h.issueSessionTokens(r, u, 900, 3600)
		if err != nil {
			t.Fatalf("login %d: %v", i+1, err)
		}
		refresh[i] = rt
		sessionIDs[i], _ = parseTestToken(t, access)["sessionId"].(string)
	}
	if refresh[0] == refresh[1] || sessionIDs[0] == sessionIDs[1] {
		t.Fatalf("logins shared a refresh token or session: %v %v", refresh, sessionIDs)
	}
	for i, rt := range refresh {
		s, err := sessions.GetByRefreshToken(ctx, rt)
		if err != nil || s == nil || s.ID != sessionIDs[i] {
			t.Fatalf("refresh token %d does not resolve to its session: %v %v", i+1, s, err)
		}
	}
}
