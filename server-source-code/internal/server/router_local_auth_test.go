package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// Proves the router wires the per-context "Disable local auth" policy into the
// login handler, and that saving the setting takes effect on the next request.
func TestRouterEnforcesDatabaseDisableLocalAuth(t *testing.T) {
	t.Setenv("ENV_FILE", "/nonexistent")
	t.Setenv("DATABASE_URL", pgtest.URL(t))
	t.Setenv("JWT_SECRET", "router-test-secret-0123456789abcdef")
	t.Setenv("AI_ENCRYPTION_KEY", "router-test-encryption-key-0123456")
	t.Setenv("APP_ENV", "development")
	t.Setenv("CORS_ORIGIN", "http://example.com")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	d := pgtest.Open(t)
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	const password = "Router-Test-Pass-9!"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		t.Fatal(err)
	}
	hashStr := string(hash)
	admin := &models.User{
		ID:           uuid.New().String(),
		Username:     "routeradmin" + uuid.New().String()[:6],
		Email:        "router-" + uuid.New().String()[:8] + "@example.com",
		Role:         "superadmin",
		IsActive:     true,
		PasswordHash: &hashStr,
	}
	if err := store.NewUsersStore(d).Create(ctx, admin); err != nil {
		t.Fatal(err)
	}

	h, _ := NewRouter(ctx, cfg, d, rdb, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	login := func() *httptest.ResponseRecorder {
		body := `{"username":"` + admin.Username + `","password":"` + password + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := login()
	if rec.Code != http.StatusOK {
		t.Fatalf("baseline login: got %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Token == "" {
		t.Fatalf("no token in login response: %s", rec.Body.String())
	}

	putSettings := func(body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/oidc/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+resp.Token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT /auth/oidc/settings: got %d %s", rec.Code, rec.Body.String())
		}
	}

	putSettings(`{"oidc_enabled":true,"oidc_issuer_url":"https://idp.example.invalid","oidc_client_id":"patchmon","oidc_client_secret":"s3cret","oidc_redirect_uri":"http://127.0.0.1/api/v1/auth/oidc/callback","oidc_disable_local_auth":true}`)
	if rec := login(); rec.Code != http.StatusForbidden {
		t.Fatalf("login right after enabling Disable local auth: got %d %s, want 403", rec.Code, rec.Body.String())
	}

	putSettings(`{"oidc_disable_local_auth":false}`)
	if rec := login(); rec.Code != http.StatusOK {
		t.Fatalf("login right after clearing Disable local auth: got %d %s, want 200", rec.Code, rec.Body.String())
	}
}
