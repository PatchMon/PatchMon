package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/oidc"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

type setupFixture struct {
	db    *database.DB
	users *store.UsersStore
	auth  *AuthHandler
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()
	d := pgtest.OpenEmpty(t)
	users := store.NewUsersStore(d)
	return &setupFixture{
		db:    d,
		users: users,
		auth: &AuthHandler{
			cfg:      &config.Config{JWTSecret: "setup-admin-test-secret-0123456789"},
			users:    users,
			sessions: store.NewSessionsStore(d),
		},
	}
}

func (f *setupFixture) seed(t *testing.T, role string, active bool) {
	t.Helper()
	u := &models.User{
		ID:       uuid.New().String(),
		Username: "seed" + uuid.New().String()[:10],
		Email:    "seed" + uuid.New().String()[:8] + "@example.com",
		Role:     role,
		IsActive: active,
	}
	if err := f.users.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func (f *setupFixture) count(t *testing.T) int {
	t.Helper()
	n, err := f.users.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func postSetupAdmin(h *AuthHandler, username string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{
		"firstName": "First",
		"lastName":  "Admin",
		"username":  username,
		"email":     username + "@example.com",
		"password":  hardeningPassword,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup-admin", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.9:40000"
	rec := httptest.NewRecorder()
	h.SetupAdmin(rec, req)
	return rec
}

func TestSetupAdminCreatesFirstSuperadminOnce(t *testing.T) {
	f := newSetupFixture(t)
	if rec := postSetupAdmin(f.auth, "first"); rec.Code != http.StatusCreated {
		t.Fatalf("first setup: status %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := postSetupAdmin(f.auth, "second"); rec.Code != http.StatusBadRequest {
		t.Fatalf("second setup: status %d, want 400", rec.Code)
	}
	if n := f.count(t); n != 1 {
		t.Fatalf("users = %d, want 1", n)
	}
}

func TestSetupAdminRefusedWhenAnyUserExists(t *testing.T) {
	cases := []struct {
		name   string
		role   string
		active bool
	}{
		{"inactive superadmin", "superadmin", false},
		{"inactive admin", "admin", false},
		{"custom role only", "custom-operator", true},
		{"inactive ordinary user", "user", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSetupFixture(t)
			f.seed(t, tc.role, tc.active)
			rec := postSetupAdmin(f.auth, "intruder")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400; body %s", rec.Code, rec.Body.String())
			}
			if n := f.count(t); n != 1 {
				t.Fatalf("users = %d, want 1", n)
			}
		})
	}
}

func TestLoginSettingsHasAdminUsersWhenOnlyInactiveAdminExists(t *testing.T) {
	f := newSetupFixture(t)
	h := &SettingsHandler{settings: store.NewSettingsStore(f.db), users: f.users}
	get := func() map[string]any {
		rec := httptest.NewRecorder()
		h.GetLoginSettings(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings/login-settings", nil))
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
		return body
	}
	if got := get()["hasAdminUsers"]; got != false {
		t.Fatalf("empty install: hasAdminUsers = %v, want false", got)
	}
	f.seed(t, "superadmin", false)
	if got := get()["hasAdminUsers"]; got != true {
		t.Fatalf("inactive superadmin: hasAdminUsers = %v, want true", got)
	}
}

func TestOidcFirstUserPromotionOnlyOnEmptyInstall(t *testing.T) {
	newOidc := func(users *store.UsersStore) *OidcHandler {
		return &OidcHandler{
			cfg:   &config.Config{},
			users: users,
			entries: map[string]*oidcContextEntry{"": {
				resolved: &config.ResolvedOidcConfig{
					IssuerURL:       testIssuer,
					AutoCreateUsers: true,
					DefaultRole:     "user",
				},
				configuredValid: true,
				expiresAt:       time.Now().Add(time.Hour),
			}},
		}
	}
	info := func() *oidc.UserInfo {
		id := uuid.New().String()[:8]
		return &oidc.UserInfo{Sub: "sub-" + id, Email: "sso-" + id + "@example.com", Issuer: testIssuer}
	}

	t.Run("empty install promotes", func(t *testing.T) {
		f := newSetupFixture(t)
		u := newOidc(f.users).createOidcUser(context.Background(), info())
		if u == nil || u.Role != "superadmin" {
			t.Fatalf("got %+v, want superadmin", u)
		}
	})
	t.Run("inactive admin present does not promote", func(t *testing.T) {
		f := newSetupFixture(t)
		f.seed(t, "superadmin", false)
		u := newOidc(f.users).createOidcUser(context.Background(), info())
		if u == nil || u.Role != "user" {
			t.Fatalf("got %+v, want role user", u)
		}
	})
}
