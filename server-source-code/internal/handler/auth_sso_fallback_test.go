package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

type ssoFallbackFixture struct {
	*loginFixture
	logs       *bytes.Buffer
	superTFA   string
	superNoTFA string
	userTFA    string
	tfaSecret  string
}

func newSSOFallbackFixture(t *testing.T, maxAttempts int) *ssoFallbackFixture {
	t.Helper()
	f := newLoginFixture(t, maxAttempts)
	ctx := context.Background()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	f.h.pendingLogin = store.NewPendingLoginStore(&hostctx.RedisResolver{Default: rdb})
	f.h.trustedDevices = store.NewTrustedDevicesStore(f.db)
	logs := &bytes.Buffer{}
	f.h.log = slog.New(slog.NewTextHandler(logs, nil))
	f.h.WithLocalAuthPolicy(fixedLocalAuthPolicy{fallback: "discovery returned HTTP 503"})

	secret, _, err := util.GenerateTOTPSecret("PatchMon", "test")
	if err != nil {
		t.Fatal(err)
	}
	create := func(prefix, role string, tfa bool) string {
		t.Helper()
		hash := hardeningHash()
		u := &models.User{
			ID:           uuid.New().String(),
			Username:     prefix + uuid.New().String()[:10],
			Email:        prefix + uuid.New().String()[:8] + "@example.com",
			Role:         role,
			IsActive:     true,
			PasswordHash: &hash,
		}
		if err := f.h.users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		if tfa {
			if err := f.h.users.UpdateTfaSecret(ctx, u.ID, &secret); err != nil {
				t.Fatal(err)
			}
			codes := "[]"
			if err := f.h.users.UpdateTfaEnabled(ctx, u.ID, &codes); err != nil {
				t.Fatal(err)
			}
		}
		return u.Username
	}
	return &ssoFallbackFixture{
		loginFixture: f,
		logs:         logs,
		superTFA:     create("sat", "superadmin", true),
		superNoTFA:   create("san", "superadmin", false),
		userTFA:      create("ust", "admin", true),
		tfaSecret:    secret,
	}
}

func (f *ssoFallbackFixture) verifyTfa(t *testing.T, ticket string, rememberMe bool) *httptest.ResponseRecorder {
	t.Helper()
	code, err := totp.GenerateCode(f.tfaSecret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"token": code, "tfa_ticket": ticket, "remember_me": rememberMe})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-tfa", bytes.NewReader(body))
	req.RemoteAddr = "198.51.100.7:40000"
	rec := httptest.NewRecorder()
	f.h.VerifyTfa(rec, req)
	return rec
}

const fallbackUsedLog = "password login allowed because the SSO provider is unreachable"

func TestSSOFallbackRefusesAccountsThatAreNotSuperadmin(t *testing.T) {
	f := newSSOFallbackFixture(t, 1000)
	for _, username := range []string{f.active, f.userTFA} {
		rec := postLogin(f.h, username, hardeningPassword)
		if rec.Code != http.StatusUnauthorized || errorMessage(t, rec) != "Invalid credentials" {
			t.Fatalf("%s with the correct password: got %d %s, want the same 401 as a wrong password", username, rec.Code, rec.Body.String())
		}
	}
	if strings.Contains(f.logs.String(), fallbackUsedLog) {
		t.Fatal("the fallback was recorded as used for an account it refused")
	}
}

func TestSSOFallbackRefusalCountsTowardLockout(t *testing.T) {
	f := newSSOFallbackFixture(t, 3)
	for i := 1; i < 3; i++ {
		if rec := postLogin(f.h, f.active, hardeningPassword); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d, want 401", i, rec.Code)
		}
	}
	if rec := postLogin(f.h, f.active, hardeningPassword); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 3: got %d, want 429", rec.Code)
	}
}

func TestSSOFallbackRequiresSuperadminTFA(t *testing.T) {
	f := newSSOFallbackFixture(t, 1000)
	rec := postLogin(f.h, f.superNoTFA, hardeningPassword)
	if rec.Code != http.StatusForbidden || !strings.Contains(errorMessage(t, rec), "two-factor authentication") {
		t.Fatalf("superadmin without 2FA: got %d %s, want 403 explaining 2FA is required", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "token") {
		t.Fatal("a session was issued without a second factor")
	}
}

func TestSSOFallbackSuperadminSignsInThroughTFA(t *testing.T) {
	f := newSSOFallbackFixture(t, 1000)

	if rec := postLogin(f.h, f.superTFA, "wrong-password"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: got %d", rec.Code)
	}
	if strings.Contains(f.logs.String(), "sso fallback") || strings.Contains(f.logs.String(), fallbackUsedLog) {
		t.Fatal("a wrong password was logged as a use of the fallback")
	}

	rec := postLogin(f.h, f.superTFA, hardeningPassword)
	var step1 struct {
		RequiresTfa bool   `json:"requiresTfa"`
		Ticket      string `json:"tfaTicket"`
		Token       string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &step1); err != nil || !step1.RequiresTfa || step1.Ticket == "" || step1.Token != "" {
		t.Fatalf("password step: got %d %s, want a two-factor challenge and no session", rec.Code, rec.Body.String())
	}

	rec = f.verifyTfa(t, step1.Ticket, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token"`) {
		t.Fatalf("two-factor step: got %d %s, want a session", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == DeviceTrustCookieName && c.Value != "" {
			t.Fatal("remember-me must not mint a device-trust cookie on the fallback path")
		}
	}
	logs := f.logs.String()
	if !strings.Contains(logs, fallbackUsedLog) || !strings.Contains(logs, "audit_action=sso_fallback_login") || !strings.Contains(logs, "HTTP 503") {
		t.Fatalf("the fallback session was not recorded with its reason:\n%s", logs)
	}
}

func TestSSOFallbackTFAStepRefusesTicketsForOtherRoles(t *testing.T) {
	f := newSSOFallbackFixture(t, 1000)
	u, err := f.h.users.GetByUsernameOrEmail(context.Background(), f.userTFA)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := f.h.pendingLogin.Create(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec := f.verifyTfa(t, ticket, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("non-superadmin two-factor step under the fallback: got %d %s, want 401", rec.Code, rec.Body.String())
	}
}

// A remembered device skips the second factor normally; on the fallback path it must not.
func TestSSOFallbackIgnoresDeviceTrust(t *testing.T) {
	f := newSSOFallbackFixture(t, 1000)
	ctx := context.Background()
	u, err := f.h.users.GetByUsernameOrEmail(ctx, f.superTFA)
	if err != nil {
		t.Fatal(err)
	}
	raw, hash, err := store.GenerateTrustToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.trustedDevices.Create(ctx, store.CreateTrustedDeviceParams{UserID: u.ID, TokenHash: hash, DeviceID: "d1", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	login := func(h *AuthHandler) string {
		body, _ := json.Marshal(map[string]string{"username": f.superTFA, "password": hardeningPassword})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.RemoteAddr = "198.51.100.7:40000"
		req.AddCookie(&http.Cookie{Name: DeviceTrustCookieName, Value: raw})
		rec := httptest.NewRecorder()
		h.Login(rec, req)
		return rec.Body.String()
	}

	normal := *f.h
	normal.localAuth = fixedLocalAuthPolicy{}
	if body := login(&normal); strings.Contains(body, "requiresTfa") {
		t.Fatalf("precondition: the trusted device should skip 2FA outside the fallback: %s", body)
	}
	if body := login(f.h); !strings.Contains(body, `"requiresTfa":true`) {
		t.Fatalf("fallback sign-in skipped the second factor on a trusted device: %s", body)
	}
}
