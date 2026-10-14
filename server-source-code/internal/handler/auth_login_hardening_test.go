package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/oidc"
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

type fixedLocalAuthPolicy struct {
	disabled bool
	fallback string
}

func (p fixedLocalAuthPolicy) LocalAuthDecision(context.Context) (bool, string) {
	return p.disabled, p.fallback
}

func callAuthEndpoint(h *AuthHandler, endpoint string) *httptest.ResponseRecorder {
	// No password, so a request let through stops at input validation.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/"+endpoint, strings.NewReader(`{"username":"admin","token":"123456"}`))
	rec := httptest.NewRecorder()
	switch endpoint {
	case "login":
		h.Login(rec, req)
	case "verify-tfa":
		h.VerifyTfa(rec, req)
	case "signup":
		h.Signup(rec, req)
	}
	return rec
}

func isLocalAuthRefusal(t *testing.T, rec *httptest.ResponseRecorder) bool {
	t.Helper()
	return rec.Code == http.StatusForbidden && strings.Contains(errorMessage(t, rec), "Local authentication is disabled")
}

func TestLocalAuthDisabledBlocksPasswordEndpoints(t *testing.T) {
	t.Parallel()

	handlers := map[string]*AuthHandler{
		"database setting": (&AuthHandler{cfg: &config.Config{}}).WithLocalAuthPolicy(fixedLocalAuthPolicy{disabled: true}),
		"environment":      {cfg: &config.Config{OidcEnabled: true, OidcDisableLocalAuth: true}},
	}
	for hname, h := range handlers {
		for _, endpoint := range []string{"login", "verify-tfa", "signup"} {
			t.Run(hname+"/"+endpoint, func(t *testing.T) {
				t.Parallel()
				if rec := callAuthEndpoint(h, endpoint); !isLocalAuthRefusal(t, rec) {
					t.Fatalf("got %d %s, want 403 local authentication disabled", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestLocalAuthAllowedWhenPolicyOff(t *testing.T) {
	t.Parallel()
	h := (&AuthHandler{cfg: &config.Config{}}).WithLocalAuthPolicy(fixedLocalAuthPolicy{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d %s, want the request to reach input validation (400)", rec.Code, rec.Body.String())
	}
}

// The fallback reopens sign-in for existing users only; it must not reopen signup.
func TestLocalAuthFallbackAllowsSignInButNotSignup(t *testing.T) {
	t.Parallel()
	fallback := fixedLocalAuthPolicy{fallback: "discovery returned HTTP 503"}
	handlers := map[string]*AuthHandler{
		"database setting": (&AuthHandler{cfg: &config.Config{}}).WithLocalAuthPolicy(fallback),
		"environment":      (&AuthHandler{cfg: &config.Config{OidcEnabled: true, OidcDisableLocalAuth: true}}).WithLocalAuthPolicy(fallback),
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, endpoint := range []string{"login", "verify-tfa"} {
				if rec := callAuthEndpoint(h, endpoint); isLocalAuthRefusal(t, rec) {
					t.Errorf("%s refused despite the fallback: %s", endpoint, rec.Body.String())
				}
			}
			if rec := callAuthEndpoint(h, "signup"); !isLocalAuthRefusal(t, rec) {
				t.Errorf("signup: got %d %s, want 403", rec.Code, rec.Body.String())
			}
		})
	}
}

func newProbeHandler(t *testing.T, issuer string, fallback bool, hosts ...string) *OidcHandler {
	t.Helper()
	client, err := oidc.NewClient(context.Background(), oidc.Config{IssuerURL: issuer, ClientID: "c", ClientSecret: "s", RedirectURI: "http://127.0.0.1/cb"})
	if err != nil {
		t.Fatal(err)
	}
	h := &OidcHandler{cfg: &config.Config{}, entries: map[string]*oidcContextEntry{}}
	for _, host := range append([]string{""}, hosts...) {
		h.entries[host] = &oidcContextEntry{
			client:          client,
			resolved:        &config.ResolvedOidcConfig{IssuerURL: issuer, DisableLocalAuth: true, LocalAuthFallback: fallback},
			configuredValid: true,
			expiresAt:       time.Now().Add(time.Hour),
		}
	}
	return h
}

type countingIdP struct {
	srv    *httptest.Server
	hits   atomic.Int32
	status atomic.Int32
}

func newCountingIdP(t *testing.T) *countingIdP {
	t.Helper()
	idp := &countingIdP{}
	idp.status.Store(http.StatusOK)
	idp.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idp.hits.Add(1)
		w.WriteHeader(int(idp.status.Load()))
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(idp.srv.Close)
	return idp
}

func TestOidcLocalAuthDecision(t *testing.T) {
	t.Parallel()

	stopped := httptest.NewServer(http.NotFoundHandler())
	stoppedURL := stopped.URL
	stopped.Close()

	cases := []struct {
		name         string
		status       int
		down         bool
		fallback     bool
		wantDisabled bool
		wantReason   string
	}{
		{"fallback off, provider down", 0, true, false, true, ""},
		{"fallback on, provider down", 0, true, true, false, "discovery request failed"},
		{"fallback on, provider 503", http.StatusServiceUnavailable, false, true, false, "HTTP 503"},
		{"fallback on, provider up", http.StatusOK, false, true, true, ""},
		{"fallback on, provider answers 404 (misconfigured issuer)", http.StatusNotFound, false, true, true, ""},
		{"fallback on, provider answers 401", http.StatusUnauthorized, false, true, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			issuer := stoppedURL
			if !tc.down {
				idp := newCountingIdP(t)
				idp.status.Store(int32(tc.status))
				issuer = idp.srv.URL
			}
			disabled, reason := newProbeHandler(t, issuer, tc.fallback).LocalAuthDecision(context.Background())
			if disabled != tc.wantDisabled || (tc.wantReason == "") != (reason == "") || !strings.Contains(reason, tc.wantReason) {
				t.Fatalf("LocalAuthDecision = (%v, %q), want (%v, containing %q)", disabled, reason, tc.wantDisabled, tc.wantReason)
			}
		})
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
			if got, _ := tc.h.LocalAuthDecision(context.Background()); got != tc.want {
				t.Fatalf("LocalAuthDecision disabled = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOidcProviderProbeIsCachedPerContext(t *testing.T) {
	t.Parallel()
	idp := newCountingIdP(t)
	idp.status.Store(http.StatusBadGateway)
	h := newProbeHandler(t, idp.srv.URL, true, "a.example.com", "b.example.com")
	ctxA := hostctx.WithEntry(context.Background(), &hostctx.Entry{Host: "a.example.com"})
	ctxB := hostctx.WithEntry(context.Background(), &hostctx.Entry{Host: "b.example.com"})

	for i := 0; i < 20; i++ {
		if disabled, _ := h.LocalAuthDecision(ctxA); disabled {
			t.Fatal("context A should fall back while its provider returns 502")
		}
	}
	if got := idp.hits.Load(); got != 1 {
		t.Fatalf("20 decisions in one context probed the provider %d times, want 1", got)
	}

	idp.status.Store(http.StatusOK)
	if disabled, _ := h.LocalAuthDecision(ctxA); disabled {
		t.Fatal("context A should keep its cached outage until the TTL passes")
	}
	if disabled, _ := h.LocalAuthDecision(ctxB); !disabled {
		t.Fatal("context B must probe for itself, not reuse context A's outage")
	}
	if got := idp.hits.Load(); got != 2 {
		t.Fatalf("provider probed %d times, want 2 (one per context)", got)
	}

	h.probeMu.Lock()
	h.probes["a.example.com"].expiresAt = time.Now().Add(-time.Second)
	h.probeMu.Unlock()
	if disabled, _ := h.LocalAuthDecision(ctxA); !disabled {
		t.Fatal("context A should re-probe after the TTL and see the provider is back")
	}

	idp.status.Store(http.StatusBadGateway)
	h.EvictHost("a.example.com")
	h.entries["a.example.com"] = h.entries["b.example.com"]
	if disabled, _ := h.LocalAuthDecision(ctxA); disabled {
		t.Fatal("saving settings must drop the cached probe")
	}
}

// A caller that hangs up mid-probe must not record an outage for everyone else.
func TestOidcProviderProbeIgnoresCallerCancellation(t *testing.T) {
	t.Parallel()
	idp := newCountingIdP(t)
	h := newProbeHandler(t, idp.srv.URL, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if disabled, reason := h.LocalAuthDecision(ctx); !disabled {
		t.Fatalf("a cancelled request recorded an outage: %q", reason)
	}
}

func TestBcryptGateGivesUpWhenTheRequestGoes(t *testing.T) {
	t.Parallel()
	full := make(bcryptGate, 1)
	full <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := full.compare(ctx, dummyPasswordHash, "x"); err == nil {
		t.Fatal("a queued compare must give up when its request goes away")
	}
}

// Both the dummy and the real comparison must queue on the shared gate.
func TestLoginComparesGoThroughTheBcryptGate(t *testing.T) {
	f := newLoginFixture(t, 1000)
	for i := 0; i < cap(loginBcrypt); i++ {
		loginBcrypt <- struct{}{}
	}
	released := false
	release := func() {
		if !released {
			for i := 0; i < cap(loginBcrypt); i++ {
				<-loginBcrypt
			}
			released = true
		}
	}
	t.Cleanup(release)

	for name, username := range map[string]string{"real account": f.active, "unknown user": "nobody-" + uuid.New().String()[:8]} {
		done := make(chan int, 1)
		go func() { done <- postLogin(f.h, username, "wrong-password").Code }()
		select {
		case code := <-done:
			t.Fatalf("%s: login finished (%d) while every bcrypt slot was taken", name, code)
		case <-time.After(100 * time.Millisecond):
		}
		release()
		select {
		case code := <-done:
			if code != http.StatusUnauthorized {
				t.Fatalf("%s: got %d after the gate opened, want 401", name, code)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: login never completed after the gate opened", name)
		}
		for i := 0; i < cap(loginBcrypt); i++ {
			loginBcrypt <- struct{}{}
		}
		released = false
	}
}
