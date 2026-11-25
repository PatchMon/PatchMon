package middleware

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

type fakeAPITokenStore struct {
	token       db.AutoEnrollmentToken
	err         error
	lastUsedIDs []string
	keys        []string
}

func (s *fakeAPITokenStore) GetByKey(_ context.Context, key string) (db.AutoEnrollmentToken, error) {
	s.keys = append(s.keys, key)
	return s.token, s.err
}

func (s *fakeAPITokenStore) UpdateLastUsedAt(_ context.Context, id string) error {
	s.lastUsedIDs = append(s.lastUsedIDs, id)
	return nil
}

func TestApiAuthCredentialRestrictions(t *testing.T) {
	secret := "private-enrollment-secret"
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		modify     func(*fakeAPITokenStore, *http.Request)
		wantStatus int
	}{
		{"valid scoped API credential", nil, http.StatusNoContent},
		{"missing authorization", func(_ *fakeAPITokenStore, r *http.Request) { r.Header.Del("Authorization") }, http.StatusUnauthorized},
		{"JWT cannot access API", func(_ *fakeAPITokenStore, r *http.Request) { r.Header.Set("Authorization", "Bearer user-jwt") }, http.StatusUnauthorized},
		{"invalid base64", func(_ *fakeAPITokenStore, r *http.Request) { r.Header.Set("Authorization", "Basic !!!") }, http.StatusUnauthorized},
		{"empty key", func(_ *fakeAPITokenStore, r *http.Request) { r.SetBasicAuth("", secret) }, http.StatusUnauthorized},
		{"empty secret", func(_ *fakeAPITokenStore, r *http.Request) { r.SetBasicAuth("patchmon_ae_key", "") }, http.StatusUnauthorized},
		{"unknown or deleted key", func(s *fakeAPITokenStore, _ *http.Request) { s.err = errors.New("not found") }, http.StatusUnauthorized},
		{"invalid secret", func(_ *fakeAPITokenStore, r *http.Request) { r.SetBasicAuth("patchmon_ae_key", "incorrect-secret") }, http.StatusUnauthorized},
		{"revoked credential", func(s *fakeAPITokenStore, _ *http.Request) { s.token.IsActive = false }, http.StatusUnauthorized},
		{"expired credential", func(s *fakeAPITokenStore, _ *http.Request) {
			s.token.ExpiresAt = pgtype.Timestamp{Time: time.Now().Add(-time.Minute), Valid: true}
		}, http.StatusUnauthorized},
		{"future expiration", func(s *fakeAPITokenStore, _ *http.Request) {
			s.token.ExpiresAt = pgtype.Timestamp{Time: time.Now().Add(time.Hour), Valid: true}
		}, http.StatusNoContent},
		{"enrollment-only credential", func(s *fakeAPITokenStore, _ *http.Request) {
			s.token.Metadata = []byte(`{"integration_type":"proxmox-lxc"}`)
		}, http.StatusUnauthorized},
		{"direct enrollment credential", func(s *fakeAPITokenStore, _ *http.Request) {
			s.token.Metadata = []byte(`{"integration_type":"direct-host"}`)
		}, http.StatusUnauthorized},
		{"gethomepage credential", func(s *fakeAPITokenStore, _ *http.Request) {
			s.token.Metadata = []byte(`{"integration_type":"gethomepage"}`)
		}, http.StatusUnauthorized},
		{"missing integration type", func(s *fakeAPITokenStore, _ *http.Request) { s.token.Metadata = []byte(`{}`) }, http.StatusUnauthorized},
		{"invalid integration metadata", func(s *fakeAPITokenStore, _ *http.Request) { s.token.Metadata = []byte(`[]`) }, http.StatusUnauthorized},
		{"allowed CIDR", func(s *fakeAPITokenStore, _ *http.Request) { s.token.AllowedIpRanges = []string{"203.0.113.0/24"} }, http.StatusNoContent},
		{"allowed exact IP", func(s *fakeAPITokenStore, _ *http.Request) { s.token.AllowedIpRanges = []string{"203.0.113.7"} }, http.StatusNoContent},
		{"allowed IPv6", func(s *fakeAPITokenStore, r *http.Request) {
			s.token.AllowedIpRanges = []string{"2001:db8::/32"}
			r.RemoteAddr = "[2001:db8::7]:43342"
		}, http.StatusNoContent},
		{"denied IP", func(s *fakeAPITokenStore, _ *http.Request) { s.token.AllowedIpRanges = []string{"198.51.100.0/24"} }, http.StatusForbidden},
		{"invalid IP range fails closed", func(s *fakeAPITokenStore, _ *http.Request) { s.token.AllowedIpRanges = []string{"invalid"} }, http.StatusForbidden},
		{"forwarded header cannot bypass allowlist", func(s *fakeAPITokenStore, r *http.Request) {
			s.token.AllowedIpRanges = []string{"198.51.100.1"}
			r.Header.Set("X-Forwarded-For", "198.51.100.1")
		}, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &fakeAPITokenStore{token: db.AutoEnrollmentToken{
				ID: "api-credential-1", IsActive: true, TokenSecret: string(hash),
				Metadata: []byte(`{"integration_type":"api"}`),
				Scopes:   []byte(`{"host":["get"]}`),
			}}
			r := httptest.NewRequest(http.MethodGet, "/api/v1/api/hosts", nil)
			r.RemoteAddr = "203.0.113.7:43342"
			r.SetBasicAuth("patchmon_ae_key", secret)
			if tt.modify != nil {
				tt.modify(s, r)
			}
			var logs bytes.Buffer
			log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if token := GetApiToken(r.Context()); token == nil || token.ID != s.token.ID {
					t.Error("authenticated token missing from request context")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			w := httptest.NewRecorder()
			ApiAuth(s, log)(next).ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if called != (tt.wantStatus == http.StatusNoContent) {
				t.Fatalf("next handler called = %v", called)
			}
			if len(s.lastUsedIDs) != 0 && !called {
				t.Fatal("failed authentication updated last_used_at")
			}
			if called && (len(s.lastUsedIDs) != 1 || s.lastUsedIDs[0] != s.token.ID) {
				t.Fatal("successful authentication did not update last_used_at")
			}
			for _, sensitive := range []string{"patchmon_ae_key", secret, string(hash), r.Header.Get("Authorization")} {
				if sensitive != "" && (strings.Contains(logs.String(), sensitive) || strings.Contains(w.Body.String(), sensitive)) {
					t.Fatal("authentication exposed credentials")
				}
			}
		})
	}
}

func TestApiAuthForIntegrationPreservesGetHomepage(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeAPITokenStore{token: db.AutoEnrollmentToken{
		ID: "widget-credential", IsActive: true, TokenSecret: string(hash),
		Metadata: []byte(`{"integration_type":"gethomepage"}`),
	}}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/gethomepage/health", nil)
	r.SetBasicAuth("widget-key", "secret")
	w := httptest.NewRecorder()
	ApiAuthForIntegration(s, "gethomepage", nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
}
