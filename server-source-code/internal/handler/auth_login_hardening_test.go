package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

const hardeningPassword = "Correct-Horse-9!"

type loginFixture struct {
	h        *AuthHandler
	active   string
	disabled string
	ssoOnly  string
}

func newLoginFixture(t *testing.T, maxAttempts int) *loginFixture {
	t.Helper()
	ctx := context.Background()
	users := store.NewUsersStore(pgtest.Open(t))

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	lockout := store.NewLoginLockoutStore(&hostctx.RedisResolver{Default: rdb}, nil, maxAttempts, 15)

	hash, err := bcrypt.GenerateFromPassword([]byte(hardeningPassword), 12)
	if err != nil {
		t.Fatal(err)
	}
	hashStr := string(hash)

	create := func(prefix string, active bool, pw *string) string {
		t.Helper()
		u := &models.User{
			ID:           uuid.New().String(),
			Username:     prefix + uuid.New().String()[:10],
			Email:        prefix + uuid.New().String()[:8] + "@example.com",
			Role:         "user",
			IsActive:     active,
			PasswordHash: pw,
		}
		if err := users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		return u.Username
	}

	return &loginFixture{
		h:        &AuthHandler{cfg: &config.Config{}, users: users, loginLockout: lockout},
		active:   create("act", true, &hashStr),
		disabled: create("dis", false, &hashStr),
		ssoOnly:  create("sso", true, nil),
	}
}

func postLogin(h *AuthHandler, username, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.7:40000"
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	return rec
}

func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %s", rec.Body.String())
	}
	msg, _ := body["error"].(string)
	return msg
}

func TestDummyPasswordHashMatchesRealCost(t *testing.T) {
	t.Parallel()
	cost, err := bcrypt.Cost([]byte(dummyPasswordHash))
	if err != nil {
		t.Fatalf("dummy hash is not a valid bcrypt hash: %v", err)
	}
	if cost != 12 {
		t.Fatalf("dummy hash cost = %d, want 12 to match the cost real passwords are hashed with", cost)
	}
}

// Every rejection before a session is issued must pay for one bcrypt
// comparison, or response time tells an attacker which usernames exist.
func TestLoginRejectionsCostTheSame(t *testing.T) {
	f := newLoginFixture(t, 1000)

	median := func(username string) time.Duration {
		var d []time.Duration
		for i := 0; i < 5; i++ {
			start := time.Now()
			rec := postLogin(f.h, username, "wrong-password")
			d = append(d, time.Since(start))
			if rec.Code != http.StatusUnauthorized || errorMessage(t, rec) != "Invalid credentials" {
				t.Fatalf("%s: got %d %s, want 401 Invalid credentials", username, rec.Code, rec.Body.String())
			}
		}
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		return d[len(d)/2]
	}

	reference := median(f.active)
	for name, username := range map[string]string{
		"unknown user":     "nobody-" + uuid.New().String()[:8],
		"disabled account": f.disabled,
		"SSO-only account": f.ssoOnly,
	} {
		if got := median(username); got < reference/2 {
			t.Errorf("%s answered in %v against %v for a real account: the timing still separates them", name, got, reference)
		}
	}
}

func TestLoginDisabledAccountRevealedOnlyAfterCorrectPassword(t *testing.T) {
	f := newLoginFixture(t, 1000)

	rec := postLogin(f.h, f.disabled, "wrong-password")
	if rec.Code != http.StatusUnauthorized || errorMessage(t, rec) != "Invalid credentials" {
		t.Fatalf("wrong password on a disabled account: got %d %s, want 401 Invalid credentials", rec.Code, rec.Body.String())
	}

	rec = postLogin(f.h, f.disabled, hardeningPassword)
	if rec.Code != http.StatusUnauthorized || errorMessage(t, rec) != "Account is disabled" {
		t.Fatalf("correct password on a disabled account: got %d %s, want 401 Account is disabled", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "token") || len(rec.Result().Cookies()) > 0 {
		t.Fatalf("a disabled account must not receive a session: %s", rec.Body.String())
	}
}

// A disabled account must lock out on the same attempt as an unknown username,
// whether or not the password is right, or the lockout becomes the oracle.
func TestLoginDisabledAccountLocksOutLikeUnknownUser(t *testing.T) {
	const maxAttempts = 3
	cases := []struct {
		name     string
		username func(f *loginFixture) string
		password string
	}{
		{"unknown user", func(*loginFixture) string { return "ghost-" + uuid.New().String()[:8] }, "x"},
		{"disabled, wrong password", func(f *loginFixture) string { return f.disabled }, "x"},
		{"disabled, correct password", func(f *loginFixture) string { return f.disabled }, hardeningPassword},
		{"SSO-only", func(f *loginFixture) string { return f.ssoOnly }, "x"},
		{"active, wrong password", func(f *loginFixture) string { return f.active }, "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLoginFixture(t, maxAttempts)
			username := tc.username(f)
			for i := 1; i < maxAttempts; i++ {
				if rec := postLogin(f.h, username, tc.password); rec.Code != http.StatusUnauthorized {
					t.Fatalf("attempt %d: got %d, want 401", i, rec.Code)
				}
			}
			if rec := postLogin(f.h, username, tc.password); rec.Code != http.StatusTooManyRequests {
				t.Fatalf("attempt %d: got %d %s, want 429", maxAttempts, rec.Code, rec.Body.String())
			}
		})
	}
}

type fixedLocalAuthPolicy bool

func (p fixedLocalAuthPolicy) LocalAuthDisabled(context.Context) bool { return bool(p) }

func TestLocalAuthDisabledBlocksPasswordEndpoints(t *testing.T) {
	t.Parallel()

	endpoints := map[string]func(h *AuthHandler, w http.ResponseWriter, r *http.Request){
		"login":      func(h *AuthHandler, w http.ResponseWriter, r *http.Request) { h.Login(w, r) },
		"verify-tfa": func(h *AuthHandler, w http.ResponseWriter, r *http.Request) { h.VerifyTfa(w, r) },
		"signup":     func(h *AuthHandler, w http.ResponseWriter, r *http.Request) { h.Signup(w, r) },
	}
	handlers := map[string]*AuthHandler{
		"database setting": (&AuthHandler{cfg: &config.Config{}}).WithLocalAuthPolicy(fixedLocalAuthPolicy(true)),
		"environment":      {cfg: &config.Config{OidcEnabled: true, OidcDisableLocalAuth: true}},
	}
	for hname, h := range handlers {
		for ename, call := range endpoints {
			t.Run(hname+"/"+ename, func(t *testing.T) {
				t.Parallel()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/"+ename, strings.NewReader(`{"username":"admin","password":"pw","token":"123456"}`))
				rec := httptest.NewRecorder()
				call(h, rec, req)
				if rec.Code != http.StatusForbidden || !strings.Contains(errorMessage(t, rec), "Local authentication is disabled") {
					t.Fatalf("got %d %s, want 403 local authentication disabled", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestLocalAuthAllowedWhenPolicyOff(t *testing.T) {
	t.Parallel()
	h := (&AuthHandler{cfg: &config.Config{}}).WithLocalAuthPolicy(fixedLocalAuthPolicy(false))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d %s, want the request to reach input validation (400)", rec.Code, rec.Body.String())
	}
}

func TestOidcLocalAuthDisabledRequiresWorkingSSOConfig(t *testing.T) {
	t.Parallel()

	entry := func(valid, disable bool) *OidcHandler {
		return &OidcHandler{
			cfg: &config.Config{},
			entries: map[string]*oidcContextEntry{"": {
				resolved:        &config.ResolvedOidcConfig{DisableLocalAuth: disable},
				configuredValid: valid,
				expiresAt:       time.Now().Add(time.Hour),
			}},
		}
	}

	cases := []struct {
		name string
		h    *OidcHandler
		want bool
	}{
		{"SSO configured, local auth disabled", entry(true, true), true},
		{"SSO configured, local auth allowed", entry(true, false), false},
		{"SSO incomplete, setting left on", entry(false, true), false},
		{"no OIDC handler", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.h.LocalAuthDisabled(context.Background()); got != tc.want {
				t.Fatalf("LocalAuthDisabled = %v, want %v", got, tc.want)
			}
		})
	}
}
