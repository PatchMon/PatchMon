package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

const testJWTSecret = "test-secret-for-token-type-assertions"

func signToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return s
}

func baseClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":       "user-1",
		"role":      "admin",
		"sessionId": "session-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
	}
}

// serveWithAuth runs the Auth middleware over a handler that records whether it
// was reached. sessionsStore is nil so the session-validity block is skipped;
// these tests are only about which tokens are admitted at all.
func serveWithAuth(t *testing.T, token string) (status int, reached bool) {
	t.Helper()
	cfg := &config.Config{JWTSecret: testJWTSecret}
	h := Auth(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, reached
}

// TestAuth_RejectsRefreshTokenAsBearer is the regression guard for the
// revocation bypass.
func TestAuth_RejectsRefreshTokenAsBearer(t *testing.T) {
	t.Parallel()

	claims := baseClaims()
	claims["typ"] = "refresh"
	delete(claims, "sessionId") // exactly how refresh tokens were minted

	status, reached := serveWithAuth(t, signToken(t, claims))
	if status != http.StatusUnauthorized {
		t.Errorf("expected 401 for a refresh token, got %d", status)
	}
	if reached {
		t.Error("a refresh token must never reach the handler")
	}
}

// TestAuth_RejectsTokenWithoutTypeClaim covers tokens minted before the typ
// claim existed.
func TestAuth_RejectsTokenWithoutTypeClaim(t *testing.T) {
	t.Parallel()

	status, reached := serveWithAuth(t, signToken(t, baseClaims()))
	if status != http.StatusUnauthorized {
		t.Errorf("expected 401 for a token with no typ claim, got %d", status)
	}
	if reached {
		t.Error("a token with no typ claim must not reach the handler")
	}
}

// TestAuth_AcceptsAccessToken guards against over-correcting.
func TestAuth_AcceptsAccessToken(t *testing.T) {
	t.Parallel()

	claims := baseClaims()
	claims["typ"] = "access"

	status, reached := serveWithAuth(t, signToken(t, claims))
	if status != http.StatusOK {
		t.Errorf("expected 200 for a valid access token, got %d", status)
	}
	if !reached {
		t.Error("a valid access token must reach the handler")
	}
}

// TestAuth_RejectsUnexpectedTypeValues covers anything else that might be
// smuggled into the claim.
func TestAuth_RejectsUnexpectedTypeValues(t *testing.T) {
	t.Parallel()

	for _, typ := range []any{"", "ACCESS", "Access", "id", "bearer", 1, true, nil} {
		claims := baseClaims()
		claims["typ"] = typ

		status, reached := serveWithAuth(t, signToken(t, claims))
		if status != http.StatusUnauthorized || reached {
			t.Errorf("typ=%v must be rejected, got status %d reached %v", typ, status, reached)
		}
	}
}

// TestAuth_PinsSigningAlgorithm confirms the parser will not accept a token
// signed with an algorithm other than HS256.
func TestAuth_PinsSigningAlgorithm(t *testing.T) {
	t.Parallel()

	claims := baseClaims()
	claims["typ"] = "access"

	// HS384 with the same secret: valid signature, wrong algorithm.
	tok := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	status, reached := serveWithAuth(t, signed)
	if status != http.StatusUnauthorized || reached {
		t.Errorf("a non-HS256 token must be rejected, got status %d reached %v", status, reached)
	}
}

// TestOptionalAuth_DoesNotEstablishIdentityFromRefreshToken mirrors the Auth
// rule on the optional path, which populates the user context when a token is
// present and would otherwise accept a refresh token.
func TestOptionalAuth_DoesNotEstablishIdentityFromRefreshToken(t *testing.T) {
	t.Parallel()

	claims := baseClaims()
	claims["typ"] = "refresh"
	delete(claims, "sessionId")

	cfg := &config.Config{JWTSecret: testJWTSecret}
	var sawUser bool
	h := OptionalAuth(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if v, _ := r.Context().Value(UserIDKey).(string); v != "" {
			sawUser = true
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/logout", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, claims))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if sawUser {
		t.Error("OptionalAuth must not establish identity from a refresh token")
	}
}
