package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const scopedAPIPrefix = "/api/v1/api"

type scopedAPICredential struct {
	id, key, secret string
}

type scopedAPIFixture struct {
	t   *testing.T
	d   *database.DB
	h   http.Handler
	jwt string
}

func newScopedAPIFixture(t *testing.T) *scopedAPIFixture {
	t.Helper()
	t.Setenv("ENV_FILE", "/nonexistent")
	t.Setenv("DATABASE_URL", pgtest.URL(t))
	t.Setenv("JWT_SECRET", "scoped-api-router-test-secret-0123456789")
	t.Setenv("AI_ENCRYPTION_KEY", "scoped-api-encryption-key-0123456")
	t.Setenv("APP_ENV", "test")
	t.Setenv("CORS_ORIGIN", "http://example.com")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	d := pgtest.OpenEmpty(t)
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	hash, err := bcrypt.GenerateFromPassword([]byte("Scoped-Router-Test-9!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hashString := string(hash)
	admin := &models.User{
		ID: uuid.New().String(), Username: "scoped-admin", Email: "scoped@example.com",
		Role: "superadmin", IsActive: true, PasswordHash: &hashString,
	}
	if err := store.NewUsersStore(d).Create(context.Background(), admin); err != nil {
		t.Fatal(err)
	}
	h, _ := NewRouter(context.Background(), cfg, d, rdb, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	f := &scopedAPIFixture{t: t, d: d, h: h}
	r := f.request(http.MethodPost, "/api/v1/auth/login", `{"username":"scoped-admin","password":"Scoped-Router-Test-9!"}`, nil, false)
	f.wantStatus(r, http.StatusOK)
	var login struct {
		Token string `json:"token"`
	}
	f.decode(r, &login)
	if login.Token == "" {
		t.Fatal("login did not return a JWT")
	}
	f.jwt = login.Token
	return f
}

func (f *scopedAPIFixture) request(method, path, body string, credential *scopedAPICredential, jwt bool) *httptest.ResponseRecorder {
	f.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "203.0.113.7:43210"
	r.Header.Set("Content-Type", "application/json")
	if credential != nil {
		r.SetBasicAuth(credential.key, credential.secret)
	}
	if jwt {
		r.Header.Set("Authorization", "Bearer "+f.jwt)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w
}

func (f *scopedAPIFixture) wantStatus(r *httptest.ResponseRecorder, want int) {
	f.t.Helper()
	if r.Code != want {
		f.t.Fatalf("status = %d, want %d: %s", r.Code, want, r.Body.String())
	}
}

func (f *scopedAPIFixture) decode(r *httptest.ResponseRecorder, target interface{}) {
	f.t.Helper()
	if err := json.Unmarshal(r.Body.Bytes(), target); err != nil {
		f.t.Fatalf("decode response: %v: %s", err, r.Body.String())
	}
}

func (f *scopedAPIFixture) credential(integrationType string, scopes map[string][]string) scopedAPICredential {
	f.t.Helper()
	c := scopedAPICredential{id: uuid.New().String(), key: "patchmon_ae_" + uuid.New().String(), secret: "test-secret-" + uuid.New().String()}
	hash, err := bcrypt.GenerateFromPassword([]byte(c.secret), bcrypt.MinCost)
	if err != nil {
		f.t.Fatal(err)
	}
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		f.t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]string{"integration_type": integrationType})
	if err != nil {
		f.t.Fatal(err)
	}
	if err := store.NewAutoEnrollmentStore(f.d).Create(context.Background(), db.CreateAutoEnrollmentTokenParams{
		ID: c.id, TokenName: integrationType + " credential", TokenKey: c.key, TokenSecret: string(hash),
		IsActive: true, AllowedIpRanges: []string{}, MaxHostsPerDay: 100,
		Metadata: metadata, Scopes: scopesJSON, UpdatedAt: pgtime.Now(),
	}); err != nil {
		f.t.Fatal(err)
	}
	return c
}

func allScopedAPIPermissions() map[string][]string {
	return map[string][]string{
		"host":             {"get", "delete"},
		"host_group":       {"get", "create", "update", "delete"},
		"enrollment_token": {"get", "create", "update", "delete"},
	}
}

func (f *scopedAPIFixture) createHost(groupID string) *models.Host {
	f.t.Helper()
	h := &models.Host{FriendlyName: "scoped-host-" + uuid.New().String(), OSType: "Debian", OSVersion: "13", Status: "active", ApiID: uuid.New().String(), ApiKey: "hashed-host-secret"}
	hs := store.NewHostsStore(f.d)
	if err := hs.Create(context.Background(), h); err != nil {
		f.t.Fatal(err)
	}
	if groupID != "" {
		if err := hs.SetHostGroups(context.Background(), h.ID, []string{groupID}); err != nil {
			f.t.Fatal(err)
		}
	}
	return h
}

func TestRouterScopedHostGroupsAndExistingHostAPI(t *testing.T) {
	f := newScopedAPIFixture(t)
	c := f.credential("api", allScopedAPIPermissions())
	description := `{"netbox_object":"tenancy.tenant:203","netbox_slug":"customer-example","timestamp":"2026-11-19T14:00:00Z"}`
	body, _ := json.Marshal(map[string]string{"name": "Customer Example", "description": description, "color": "#3B82F6"})
	r := f.request(http.MethodPost, scopedAPIPrefix+"/host-groups", string(body), &c, false)
	f.wantStatus(r, http.StatusCreated)
	var group struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
	}
	f.decode(r, &group)
	if _, err := uuid.Parse(group.ID); err != nil || group.Description != description {
		t.Fatalf("unexpected group: %+v", group)
	}
	groupID := group.ID
	host := f.createHost(groupID)
	updatedDescription := "  {\"netbox_object\": \"tenancy.tenant:203\", \"note\": \"preserve whitespace\"}\n"
	body, _ = json.Marshal(map[string]string{"name": "Customer Updated", "description": updatedDescription, "color": "#F97316"})
	r = f.request(http.MethodPut, scopedAPIPrefix+"/host-groups/"+groupID, string(body), &c, false)
	f.wantStatus(r, http.StatusOK)
	f.decode(r, &group)
	if group.ID != groupID || group.Name != "Customer Updated" || group.Description != updatedDescription || group.Color != "#F97316" {
		t.Fatalf("update changed group incorrectly: %+v", group)
	}
	r = f.request(http.MethodGet, scopedAPIPrefix+"/host-groups/"+groupID, "", &c, false)
	f.wantStatus(r, http.StatusOK)
	f.decode(r, &group)
	if group.ID != groupID || group.Description != updatedDescription {
		t.Fatalf("description did not roundtrip: %+v", group)
	}
	ids, err := store.NewHostGroupsStore(f.d).GetHostIDs(context.Background(), groupID)
	if err != nil || len(ids) != 1 || ids[0] != host.ID {
		t.Fatalf("membership changed during update: %v %v", ids, err)
	}
	r = f.request(http.MethodGet, scopedAPIPrefix+"/host-groups", "", &c, false)
	f.wantStatus(r, http.StatusOK)
	var groups []map[string]interface{}
	f.decode(r, &groups)
	if len(groups) != 1 || groups[0]["id"] != groupID || groups[0]["description"] != updatedDescription {
		t.Fatalf("unexpected group list: %s", r.Body.String())
	}
	// Credentials with only historical host scopes continue to list and delete hosts.
	hostOnly := f.credential("api", map[string][]string{"host": {"get", "delete"}})
	r = f.request(http.MethodGet, scopedAPIPrefix+"/hosts", "", &hostOnly, false)
	f.wantStatus(r, http.StatusOK)
	if !strings.Contains(r.Body.String(), host.ID) {
		t.Fatalf("host missing from existing API: %s", r.Body.String())
	}
	f.wantStatus(f.request(http.MethodDelete, scopedAPIPrefix+"/hosts/"+host.ID, "", &hostOnly, false), http.StatusOK)
	f.wantStatus(f.request(http.MethodDelete, scopedAPIPrefix+"/host-groups/"+groupID, "", &c, false), http.StatusOK)
	f.wantStatus(f.request(http.MethodGet, scopedAPIPrefix+"/host-groups/"+groupID, "", &c, false), http.StatusNotFound)
	var audits int
	if err := f.d.RawQueryRow(context.Background(), `SELECT COUNT(*) FROM audit_logs WHERE details::jsonb->>'resource_id' = $1 AND details::jsonb->>'api_token_id' = $2 AND success = true`, groupID, c.id).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 3 {
		t.Fatalf("group mutation audit count = %d, want 3", audits)
	}
}

func TestRouterScopedEachRouteRequiresItsOwnAction(t *testing.T) {
	f := newScopedAPIFixture(t)
	group := &models.HostGroup{Name: "permission target"}
	if err := store.NewHostGroupsStore(f.d).Create(context.Background(), group); err != nil {
		t.Fatal(err)
	}
	target := f.credential("proxmox-lxc", nil)
	routes := []struct{ method, path, resource, action string }{
		{http.MethodGet, "/host-groups", "host_group", "get"},
		{http.MethodGet, "/host-groups/" + group.ID, "host_group", "get"},
		{http.MethodPost, "/host-groups", "host_group", "create"},
		{http.MethodPut, "/host-groups/" + group.ID, "host_group", "update"},
		{http.MethodDelete, "/host-groups/" + group.ID, "host_group", "delete"},
		{http.MethodGet, "/auto-enrollment/tokens", "enrollment_token", "get"},
		{http.MethodGet, "/auto-enrollment/tokens/" + target.id, "enrollment_token", "get"},
		{http.MethodPost, "/auto-enrollment/tokens", "enrollment_token", "create"},
		{http.MethodPatch, "/auto-enrollment/tokens/" + target.id, "enrollment_token", "update"},
		{http.MethodDelete, "/auto-enrollment/tokens/" + target.id, "enrollment_token", "delete"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			scopes := allScopedAPIPermissions()
			var allowed []string
			for _, action := range scopes[route.resource] {
				if action != route.action {
					allowed = append(allowed, action)
				}
			}
			scopes[route.resource] = allowed
			c := f.credential("api", scopes)
			f.wantStatus(f.request(route.method, scopedAPIPrefix+route.path, `{}`, &c, false), http.StatusForbidden)
			f.wantStatus(f.request(route.method, scopedAPIPrefix+route.path, `{}`, &target, false), http.StatusUnauthorized)
			f.wantStatus(f.request(route.method, scopedAPIPrefix+route.path, `{}`, nil, true), http.StatusUnauthorized)
		})
	}
}

func TestRouterScopedEnrollmentTokenLifecycle(t *testing.T) {
	f := newScopedAPIFixture(t)
	c := f.credential("api", allScopedAPIPermissions())
	group := &models.HostGroup{Name: "NetBox customer"}
	if err := store.NewHostGroupsStore(f.d).Create(context.Background(), group); err != nil {
		t.Fatal(err)
	}
	expiration := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second).Format(time.RFC3339)
	body := fmt.Sprintf(`{"token_name":"NetBox customer-example","max_hosts_per_day":100,"default_host_group_id":%q,"allowed_ip_ranges":["203.0.113.0/24"],"expires_at":%q,"metadata":{"source":"netbox","tenant_id":203}}`, group.ID, expiration)
	r := f.request(http.MethodPost, scopedAPIPrefix+"/auto-enrollment/tokens", body, &c, false)
	f.wantStatus(r, http.StatusCreated)
	var created struct {
		Token struct {
			ID     string `json:"id"`
			Key    string `json:"token_key"`
			Secret string `json:"token_secret"`
		} `json:"token"`
	}
	f.decode(r, &created)
	if created.Token.ID == "" || created.Token.Key == "" || created.Token.Secret == "" {
		t.Fatalf("creation credentials absent: %s", r.Body.String())
	}
	tokenStore := store.NewAutoEnrollmentStore(f.d)
	raw, err := tokenStore.GetRaw(context.Background(), created.Token.ID)
	if err != nil {
		t.Fatal(err)
	}
	if raw.TokenSecret == created.Token.Secret || bcrypt.CompareHashAndPassword([]byte(raw.TokenSecret), []byte(created.Token.Secret)) != nil {
		t.Fatal("secret is not securely hashed")
	}
	if raw.DefaultHostGroupID == nil || *raw.DefaultHostGroupID != group.ID {
		t.Fatal("default host group was not saved")
	}
	tokenPath := scopedAPIPrefix + "/auto-enrollment/tokens/" + created.Token.ID
	assertNoSecret := func(rec *httptest.ResponseRecorder) {
		t.Helper()
		for _, secret := range []string{`"token_secret"`, `"token_key"`, created.Token.Key, created.Token.Secret, raw.TokenSecret} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Fatalf("secret exposed: %s", rec.Body.String())
			}
		}
	}
	r = f.request(http.MethodGet, tokenPath, "", &c, false)
	f.wantStatus(r, http.StatusOK)
	assertNoSecret(r)
	var item store.TokenListItem
	f.decode(r, &item)
	if item.DefaultHostGroupID == nil || *item.DefaultHostGroupID != group.ID || item.ExpiresAt == nil {
		t.Fatalf("configuration missing: %+v", item)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(item.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["integration_type"] != "proxmox-lxc" || metadata["source"] != "netbox" {
		t.Fatalf("metadata = %v", metadata)
	}
	// API and dashboard credentials share storage, but are excluded from enrollment administration.
	widget := f.credential("gethomepage", nil)
	r = f.request(http.MethodGet, scopedAPIPrefix+"/auto-enrollment/tokens", "", &c, false)
	f.wantStatus(r, http.StatusOK)
	assertNoSecret(r)
	var items []store.TokenListItem
	f.decode(r, &items)
	if len(items) != 1 || items[0].ID != created.Token.ID {
		t.Fatalf("non-enrollment credentials visible: %s", r.Body.String())
	}
	for _, restricted := range []scopedAPICredential{c, widget} {
		for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
			f.wantStatus(f.request(method, scopedAPIPrefix+"/auto-enrollment/tokens/"+restricted.id, `{}`, &c, false), http.StatusForbidden)
		}
	}
	r = f.request(http.MethodPatch, tokenPath, `{"token_name":"Updated token","is_active":false,"max_hosts_per_day":25,"allowed_ip_ranges":["2001:db8::/32"],"metadata":{"tenant_slug":"customer-example"},"expires_at":null,"default_host_group_id":null}`, &c, false)
	f.wantStatus(r, http.StatusOK)
	assertNoSecret(r)
	raw, err = tokenStore.GetRaw(context.Background(), created.Token.ID)
	if err != nil {
		t.Fatal(err)
	}
	if raw.IsActive || raw.MaxHostsPerDay != 25 || raw.ExpiresAt.Valid || raw.DefaultHostGroupID != nil {
		t.Fatalf("patch did not update/clear configuration: %+v", raw)
	}
	if len(raw.AllowedIpRanges) != 1 || raw.AllowedIpRanges[0] != "2001:db8::/32" {
		t.Fatalf("allowlist = %v", raw.AllowedIpRanges)
	}
	if err := json.Unmarshal(raw.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["source"] != "netbox" || metadata["tenant_slug"] != "customer-example" || metadata["integration_type"] != "proxmox-lxc" {
		t.Fatalf("metadata was not merged safely: %v", metadata)
	}
	r = f.request(http.MethodPatch, tokenPath, `{"is_active":true}`, &c, false)
	f.wantStatus(r, http.StatusOK)
	assertNoSecret(r)
	for _, body := range []string{
		`{"scopes":{"host":["delete"]}}`,
		`{"scopes":{}}`,
		`{"scopes":null}`,
	} {
		f.wantStatus(f.request(http.MethodPatch, tokenPath, body, &c, false), http.StatusForbidden)
	}
	for _, body := range []string{
		`{"metadata":{"integration_type":"api"}}`,
		`{"metadata":{"integration_type":"gethomepage"}}`,
		`{"metadata":{"INTEGRATION_TYPE":"api"}}`,
		`{"allowed_ip_ranges":["not-an-ip"]}`,
		`{"expires_at":"not-a-date"}`,
		`{"max_hosts_per_day":0}`,
		`{"max_hosts_per_day":1001}`,
		`{"default_host_group_id":"missing-group"}`,
		`{"metadata":[]}`,
	} {
		f.wantStatus(f.request(http.MethodPatch, tokenPath, body, &c, false), http.StatusBadRequest)
	}
	for _, body := range []string{
		`{"token_name":"escalation","metadata":{"integration_type":"api"},"scopes":{"host":["delete"]}}`,
		`{"token_name":"escalation","metadata":{"integration_type":"gethomepage"}}`,
		`{"token_name":"escalation","scopes":{"host":["delete"]}}`,
		`{"token_name":"escalation","scopes":null}`,
		`{"token_name":"escalation","metadata":{"INTEGRATION_TYPE":"api"}}`,
	} {
		f.wantStatus(f.request(http.MethodPost, scopedAPIPrefix+"/auto-enrollment/tokens", body, &c, false), http.StatusForbidden)
	}
	for _, body := range []string{
		`{"token_name":"invalid","max_hosts_per_day":0}`,
		`{"token_name":"invalid","allowed_ip_ranges":["invalid"]}`,
		`{"token_name":"invalid","expires_at":"invalid"}`,
		`{"token_name":"invalid","metadata":[]}`,
	} {
		f.wantStatus(f.request(http.MethodPost, scopedAPIPrefix+"/auto-enrollment/tokens", body, &c, false), http.StatusBadRequest)
	}
	f.wantStatus(f.request(http.MethodDelete, tokenPath, "", &c, false), http.StatusOK)
	f.wantStatus(f.request(http.MethodGet, tokenPath, "", &c, false), http.StatusNotFound)
	var audits int
	if err := f.d.RawQueryRow(context.Background(), `SELECT COUNT(*) FROM audit_logs WHERE details::jsonb->>'resource_id' = $1 AND details::jsonb->>'api_token_id' = $2 AND success = true`, created.Token.ID, c.id).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 4 {
		t.Fatalf("token mutation audit count = %d, want 4", audits)
	}
}

func TestRouterJWTAdministrationCanGrantNewScopes(t *testing.T) {
	f := newScopedAPIFixture(t)
	// API credential management stays behind the existing JWT administration routes.
	body := `{"token_name":"Deployment API","metadata":{"integration_type":"api"},"scopes":{"host":["get"],"host_group":["get","create","update"],"enrollment_token":["get","create","update"]}}`
	r := f.request(http.MethodPost, "/api/v1/auto-enrollment/tokens", body, nil, true)
	f.wantStatus(r, http.StatusCreated)
	var result struct {
		Token struct {
			ID     string `json:"id"`
			Key    string `json:"token_key"`
			Secret string `json:"token_secret"`
		} `json:"token"`
	}
	f.decode(r, &result)
	c := scopedAPICredential{id: result.Token.ID, key: result.Token.Key, secret: result.Token.Secret}
	f.wantStatus(f.request(http.MethodGet, scopedAPIPrefix+"/host-groups", "", &c, false), http.StatusOK)
	f.wantStatus(f.request(http.MethodGet, scopedAPIPrefix+"/auto-enrollment/tokens", "", &c, false), http.StatusOK)
	f.wantStatus(f.request(http.MethodDelete, scopedAPIPrefix+"/host-groups/"+uuid.New().String(), "", &c, false), http.StatusForbidden)
	patch := `{"scopes":{"host":["get","delete"],"host_group":["get","create","update","delete"],"enrollment_token":["get","create","update","delete"]}}`
	r = f.request(http.MethodPatch, "/api/v1/auto-enrollment/tokens/"+c.id, patch, nil, true)
	f.wantStatus(r, http.StatusOK)
	if strings.Contains(r.Body.String(), `"token_secret"`) || strings.Contains(r.Body.String(), c.secret) {
		t.Fatal("editing API credential exposed a secret")
	}
	f.wantStatus(f.request(http.MethodDelete, scopedAPIPrefix+"/host-groups/"+uuid.New().String(), "", &c, false), http.StatusNotFound)
	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/auto-enrollment/tokens", ""},
		{http.MethodPost, "/api/v1/auto-enrollment/tokens", body},
		{http.MethodPatch, "/api/v1/auto-enrollment/tokens/" + c.id, patch},
		{http.MethodPost, "/api/v1/host-groups", `{"name":"Unauthorized"}`},
	} {
		f.wantStatus(f.request(route.method, route.path, route.body, &c, false), http.StatusUnauthorized)
	}
	// Existing JWT group administration retains its response and permissions.
	r = f.request(http.MethodPost, "/api/v1/host-groups", `{"name":"JWT managed group","description":"{}"}`, nil, true)
	f.wantStatus(r, http.StatusCreated)
	var group models.HostGroup
	f.decode(r, &group)
	if group.ID == "" || group.Name != "JWT managed group" {
		t.Fatalf("unexpected JWT group response: %s", r.Body.String())
	}
	f.wantStatus(f.request(http.MethodPut, "/api/v1/host-groups/"+group.ID, `{"description":"updated"}`, nil, true), http.StatusOK)
	f.wantStatus(f.request(http.MethodDelete, "/api/v1/host-groups/"+group.ID, "", nil, true), http.StatusOK)
}
