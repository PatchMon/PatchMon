package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/oidc"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
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

func TestSetupAdminConcurrentCreatesExactlyOneUser(t *testing.T) {
	f := newSetupFixture(t)
	const n = 20
	codes := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i] = postSetupAdmin(f.auth, fmt.Sprintf("racer%02d", i)).Code
		}(i)
	}
	close(start)
	wg.Wait()
	created := 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusBadRequest:
		default:
			t.Fatalf("unexpected status %d in %v", c, codes)
		}
	}
	if created != 1 {
		t.Fatalf("201 responses = %d, want 1 (%v)", created, codes)
	}
	if got := f.count(t); got != 1 {
		t.Fatalf("users = %d, want 1", got)
	}
	var latched, audits int
	ctx := context.Background()
	if err := f.db.RawQueryRow(ctx, `SELECT COUNT(*) FROM install_state`).Scan(&latched); err != nil {
		t.Fatal(err)
	}
	if err := f.db.RawQueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE event = $1`, auditEventFirstUserCreated).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if latched != 1 || audits != 1 {
		t.Fatalf("install_state rows = %d, audit rows = %d; want 1 and 1", latched, audits)
	}
}

func TestOidcConcurrentFirstLoginsPromoteExactlyOne(t *testing.T) {
	f := newSetupFixture(t)
	h := &OidcHandler{
		cfg:   &config.Config{},
		users: f.users,
		entries: map[string]*oidcContextEntry{"": {
			resolved:        &config.ResolvedOidcConfig{IssuerURL: testIssuer, AutoCreateUsers: true, DefaultRole: "user"},
			configuredValid: true,
			expiresAt:       time.Now().Add(time.Hour),
		}},
	}
	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			id := fmt.Sprintf("%02d-%s", i, uuid.New().String()[:6])
			h.createOidcUser(context.Background(), &oidc.UserInfo{Sub: "sub-" + id, Email: "sso-" + id + "@example.com", Issuer: testIssuer})
		}(i)
	}
	close(start)
	wg.Wait()
	var supers int
	if err := f.db.RawQueryRow(context.Background(), `SELECT COUNT(*) FROM users WHERE role = 'superadmin'`).Scan(&supers); err != nil {
		t.Fatal(err)
	}
	if supers != 1 {
		t.Fatalf("superadmins = %d, want 1", supers)
	}
}

func TestSetupStaysClosedAfterAdminsAreGone(t *testing.T) {
	f := newSetupFixture(t)
	if rec := postSetupAdmin(f.auth, "owner"); rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := f.db.Exec(context.Background(), `UPDATE users SET is_active = false`); err != nil {
		t.Fatal(err)
	}
	if rec := postSetupAdmin(f.auth, "deactivated"); rec.Code != http.StatusBadRequest {
		t.Fatalf("after deactivation: %d, want 400", rec.Code)
	}
	if _, err := f.db.Exec(context.Background(), `DELETE FROM users`); err != nil {
		t.Fatal(err)
	}
	if rec := postSetupAdmin(f.auth, "deleted"); rec.Code != http.StatusBadRequest {
		t.Fatalf("after deleting every user: %d, want 400", rec.Code)
	}
	h := &SettingsHandler{settings: store.NewSettingsStore(f.db), users: f.users}
	rec := httptest.NewRecorder()
	h.GetLoginSettings(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings/login-settings", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["hasAdminUsers"] != true {
		t.Fatalf("hasAdminUsers = %v, want true once setup has run", body["hasAdminUsers"])
	}
}

func TestUsersUpdateRefusesLosingTheLastAdministrator(t *testing.T) {
	f := newSetupFixture(t)
	ctx := context.Background()
	mk := func(role string) *models.User {
		u := &models.User{ID: uuid.New().String(), Username: "u" + uuid.New().String()[:10], Email: uuid.New().String()[:8] + "@example.com", Role: role, IsActive: true}
		if err := f.users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	a := mk("superadmin")
	b := mk("superadmin")
	h := &UsersHandler{users: f.users, sessions: store.NewSessionsStore(f.db), permissions: store.NewPermissionsStore(f.db)}
	patch := func(callerID, targetID string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/admin/users/"+targetID, strings.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("userId", targetID)
		c := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		c = context.WithValue(c, middleware.UserIDKey, callerID)
		c = context.WithValue(c, middleware.UserRoleKey, "superadmin")
		rec := httptest.NewRecorder()
		h.Update(rec, req.WithContext(c))
		return rec
	}
	if rec := patch(a.ID, a.ID, `{"is_active":false}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("self-deactivation: %d, want 400", rec.Code)
	}
	if rec := patch(a.ID, b.ID, `{"is_active":false}`); rec.Code != http.StatusOK {
		t.Fatalf("deactivate other superadmin: %d %s", rec.Code, rec.Body.String())
	}
	if rec := patch(b.ID, a.ID, `{"is_active":false}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("deactivate last active superadmin: %d, want 400", rec.Code)
	}
	got, err := f.users.GetByID(ctx, a.ID)
	if err != nil || !got.IsActive {
		t.Fatalf("last superadmin was deactivated: %+v %v", got, err)
	}
}

func TestLoginSettingsHidesVersionWhenGithubPanelDisabled(t *testing.T) {
	f := newSetupFixture(t)
	h := &SettingsHandler{settings: store.NewSettingsStore(f.db), users: f.users, cfg: &config.Config{Version: "9.9.9"}}
	get := func() map[string]any {
		rec := httptest.NewRecorder()
		h.GetLoginSettings(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings/login-settings", nil))
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
		return body
	}
	setShow := func(show bool) {
		if _, err := f.db.Exec(context.Background(), "UPDATE settings SET show_github_version_on_login = $1", show); err != nil {
			t.Fatal(err)
		}
	}

	setShow(true)
	if got := get()["current_version"]; got != "9.9.9" {
		t.Fatalf("panel enabled: current_version = %v, want 9.9.9", got)
	}
	setShow(false)
	if got := get()["current_version"]; got != "" {
		t.Fatalf("panel disabled: current_version = %v, want empty", got)
	}

	h.cfg.RegistryDatabaseURL = "postgres://registry"
	setShow(true)
	if got := get()["current_version"]; got != "" {
		t.Fatalf("multi-context: current_version = %v, want empty", got)
	}
}
