package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

const allPermsJSON = `{"can_view_dashboard":true,"can_view_hosts":true,"can_manage_hosts":true,"can_view_packages":true,"can_manage_packages":true,"can_view_users":true,"can_manage_users":true,"can_manage_superusers":true,"can_view_reports":true,"can_export_data":true,"can_manage_settings":true,"can_manage_notifications":true,"can_view_notification_logs":true,"can_manage_patching":true,"can_manage_compliance":true,"can_manage_docker":true,"can_manage_alerts":true,"can_manage_automation":true,"can_use_remote_access":true,"can_manage_billing":true}`

func TestMissingPermissionsCoversEveryFlag(t *testing.T) {
	t.Parallel()
	all := &models.RolePermission{}
	v := reflect.ValueOf(all).Elem()
	flags := 0
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.Bool {
			v.Field(i).SetBool(true)
			flags++
		}
	}
	missing := missingPermissions(&models.RolePermission{}, all)
	if len(missing) != flags || flags != 20 {
		t.Fatalf("missing=%d flags=%d, want 20 each: %v", len(missing), flags, missing)
	}
	for _, m := range missing {
		if !strings.HasPrefix(m, "can_") {
			t.Errorf("flag %q has no can_ db tag", m)
		}
	}
	if got := missingPermissions(all, all); len(got) != 0 {
		t.Errorf("full set should cover itself, missing %v", got)
	}
}

func roleRequest(method, role, callerRole, body string) *http.Request {
	req := httptest.NewRequest(method, "/permissions/roles/"+role, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("role", role)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, middleware.UserRoleKey, callerRole)
	return req.WithContext(ctx)
}

func TestRoleEscalationGuards(t *testing.T) {
	t.Parallel()
	d := testDatabase(t)
	perms := store.NewPermissionsStore(d)
	ph := NewPermissionsHandler(perms)
	uh := &UsersHandler{permissions: perms}

	put := func(callerRole, role, body string) int {
		rec := httptest.NewRecorder()
		ph.UpdateRole(rec, roleRequest(http.MethodPut, role, callerRole, body))
		return rec.Code
	}
	del := func(callerRole, role string) int {
		rec := httptest.NewRecorder()
		ph.DeleteRole(rec, roleRequest(http.MethodDelete, role, callerRole, ""))
		return rec.Code
	}
	assign := func(callerRole, target string) bool {
		req := roleRequest(http.MethodPost, "assign", callerRole, "")
		return uh.canAssignRole(req, callerRole, target)
	}

	if c := put("superadmin", "rg_ops", allPermsJSON); c != http.StatusOK {
		t.Fatalf("superadmin create rg_ops: %d", c)
	}
	if c := put("superadmin", "rg_helpdesk", `{"can_view_dashboard":true,"can_view_users":true,"can_manage_users":true,"can_manage_settings":true}`); c != http.StatusOK {
		t.Fatalf("superadmin create rg_helpdesk: %d", c)
	}

	cases := []struct {
		name string
		got  int
		want int
	}{
		{"admin cannot create a role holding can_manage_superusers", put("admin", "rg_pwn", allPermsJSON), http.StatusForbidden},
		{"admin creates a role within its own permissions", put("admin", "rg_viewer", `{"can_view_dashboard":true,"can_view_hosts":true}`), http.StatusOK},
		{"custom role cannot grant its own role more", put("rg_helpdesk", "rg_helpdesk", allPermsJSON), http.StatusForbidden},
		{"custom role cannot edit a role above it", put("rg_helpdesk", "rg_ops", `{"can_view_dashboard":true}`), http.StatusForbidden},
		{"custom role cannot delete a role above it", del("rg_helpdesk", "rg_ops"), http.StatusForbidden},
		{"custom role may shrink its own role", put("rg_helpdesk", "rg_helpdesk", `{"can_view_dashboard":true,"can_view_users":true,"can_manage_users":true}`), http.StatusOK},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
	if p, err := perms.GetByRole(context.Background(), "rg_helpdesk"); err != nil || p.CanManageSettings || !p.CanManageUsers {
		t.Errorf("rg_helpdesk was not shrunk to drop can_manage_settings: %+v, %v", p, err)
	}

	assigns := []struct {
		caller, target string
		want           bool
	}{
		{"rg_helpdesk", "rg_ops", false},
		{"rg_helpdesk", "superadmin", false},
		{"rg_helpdesk", "admin", false},
		{"rg_helpdesk", "rg_helpdesk", true},
		{"admin", "rg_viewer", true},
		{"admin", "host_manager", true},
		{"admin", "admin", false},
		{"admin", "superadmin", false},
		{"rg_ops", "superadmin", true},
		{"superadmin", "admin", true},
		{"rg_helpdesk", "rg_does_not_exist", false},
		{"superadmin", "rg_does_not_exist", false},
		{"admin", " superadmin ", false},
		{"admin", " admin", false},
		{"admin", " host_manager ", true},
	}
	for _, a := range assigns {
		if got := assign(a.caller, a.target); got != a.want {
			t.Errorf("canAssignRole(%s -> %s) = %v, want %v", a.caller, a.target, got, a.want)
		}
	}
}

func TestDefaultRoleAndOidcMappingGuards(t *testing.T) {
	d := testDatabase(t)
	perms := store.NewPermissionsStore(d)
	settings := store.NewSettingsStore(d)
	sh := (&SettingsHandler{settings: settings}).WithPermissions(perms)
	oh := (&OidcHandler{cfg: &config.Config{}, settings: settings}).WithPermissions(perms)
	ph := NewPermissionsHandler(perms)

	rec := httptest.NewRecorder()
	ph.UpdateRole(rec, roleRequest(http.MethodPut, "drg_settings", "superadmin", `{"can_view_dashboard":true,"can_manage_settings":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("create drg_settings: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	ph.UpdateRole(rec, roleRequest(http.MethodPut, "drg_full", "superadmin", allPermsJSON))
	if rec.Code != http.StatusOK {
		t.Fatalf("create drg_full: %d", rec.Code)
	}

	send := func(h http.HandlerFunc, method, path, callerRole, body string, params ...string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		rctx := chi.NewRouteContext()
		for i := 0; i+1 < len(params); i += 2 {
			rctx.URLParams.Add(params[i], params[i+1])
		}
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, middleware.UserRoleKey, callerRole)
		rec := httptest.NewRecorder()
		h(rec, req.WithContext(ctx))
		return rec.Code
	}
	putSettings := func(caller, body string) int { return send(sh.Update, http.MethodPut, "/settings", caller, body) }
	patchDefault := func(caller, role string) int {
		return send(sh.UpdateEnvironmentConfig, http.MethodPatch, "/settings/environment/DEFAULT_USER_ROLE", caller, `{"value":"`+role+`"}`, "key", "DEFAULT_USER_ROLE")
	}
	putOidc := func(caller, body string) int {
		return send(oh.UpdateSettings, http.MethodPut, "/auth/oidc/settings", caller, body)
	}

	cases := []struct {
		name string
		got  int
		want int
	}{
		{"superadmin resets default role", putSettings("superadmin", `{"default_user_role":"user"}`), http.StatusOK},
		{"admin cannot default new users to superadmin", putSettings("admin", `{"default_user_role":"superadmin"}`), http.StatusForbidden},
		{"admin cannot default new users to admin", putSettings("admin", `{"default_user_role":"admin"}`), http.StatusForbidden},
		{"admin cannot default new users to admin via environment", patchDefault("admin", "admin"), http.StatusForbidden},
		{"settings-only role cannot default to host_manager", putSettings("drg_settings", `{"default_user_role":"host_manager"}`), http.StatusForbidden},
		{"settings-only role may default to its own role", putSettings("drg_settings", `{"default_user_role":"drg_settings"}`), http.StatusOK},
		{"admin may default to host_manager", patchDefault("admin", "host_manager"), http.StatusOK},
		{"admin cannot sneak admin past the check with padding", patchDefault("admin", " admin "), http.StatusForbidden},
		{"superadmin cannot default to a role that does not exist", patchDefault("superadmin", "no_such_role"), http.StatusForbidden},
		{"superadmin cannot default to a missing role via settings", putSettings("superadmin", `{"default_user_role":"no_such_role"}`), http.StatusForbidden},
		{"unchanged default role does not block other edits", putSettings("drg_settings", `{"default_user_role":"host_manager","signup_enabled":false}`), http.StatusOK},
		{"admin may save settings without touching the default role", putSettings("admin", `{"signup_enabled":false}`), http.StatusOK},
		{"admin cannot map a group to superadmin", putOidc("admin", `{"oidc_superadmin_group":"g"}`), http.StatusForbidden},
		{"admin cannot map a group to admin", putOidc("admin", `{"oidc_admin_group":"g"}`), http.StatusForbidden},
		{"admin cannot set the OIDC default role to superadmin", putOidc("admin", `{"oidc_default_role":"superadmin"}`), http.StatusForbidden},
		{"admin cannot map host_manager without can_manage_superusers", putOidc("admin", `{"oidc_host_manager_group":"ops","oidc_default_role":"readonly"}`), http.StatusForbidden},
		{"settings-only role cannot map host_manager", putOidc("drg_settings", `{"oidc_host_manager_group":"x"}`), http.StatusForbidden},
		{"superadmin maps superadmin", putOidc("superadmin", `{"oidc_superadmin_group":"supers"}`), http.StatusOK},
		{"superadmin sets the provider", putOidc("superadmin", `{"oidc_issuer_url":"https://idp.example","oidc_client_id":"pm","oidc_client_secret":"s"}`), http.StatusOK},
		{"admin cannot repoint the issuer", putOidc("admin", `{"oidc_issuer_url":"https://attacker.example"}`), http.StatusForbidden},
		{"admin cannot change the client id", putOidc("admin", `{"oidc_client_id":"evil"}`), http.StatusForbidden},
		{"admin cannot change the client secret", putOidc("admin", `{"oidc_client_secret":"evil"}`), http.StatusForbidden},
		{"admin cannot change the redirect uri", putOidc("admin", `{"oidc_redirect_uri":"https://attacker.example/cb"}`), http.StatusForbidden},
		{"admin cannot trust unverified email", putOidc("admin", `{"oidc_trust_unverified_email":true}`), http.StatusForbidden},
		{"admin cannot toggle role sync", putOidc("admin", `{"oidc_sync_roles":true}`), http.StatusForbidden},
		{"admin cannot enable or disable SSO", putOidc("admin", `{"oidc_enabled":true}`), http.StatusForbidden},
		{"admin cannot toggle the SSO-outage password fallback", putOidc("admin", `{"oidc_local_auth_fallback":true}`), http.StatusForbidden},
		{"superadmin may toggle the SSO-outage password fallback", putOidc("superadmin", `{"oidc_local_auth_fallback":true}`), http.StatusOK},
		{"admin may resend the fallback unchanged", putOidc("admin", `{"oidc_local_auth_fallback":true,"oidc_button_text":"SSO"}`), http.StatusOK},
		{"admin cannot turn the fallback back off", putOidc("admin", `{"oidc_local_auth_fallback":false}`), http.StatusForbidden},
		{"admin cannot disable local login", putOidc("admin", `{"oidc_disable_local_auth":true}`), http.StatusForbidden},
		{"admin cannot toggle user auto-creation", putOidc("admin", `{"oidc_auto_create_users":true}`), http.StatusForbidden},
		{"admin may toggle enforce https", putOidc("admin", `{"oidc_enforce_https":true}`), http.StatusOK},
		{"admin may change scopes", putOidc("admin", `{"oidc_scopes":"openid email profile groups offline_access"}`), http.StatusOK},
		{"admin may change presentation settings", putOidc("admin", `{"oidc_button_text":"Company SSO","oidc_issuer_url":"https://idp.example","oidc_client_id":"pm"}`), http.StatusOK},
		{"full custom role with can_manage_superusers may repoint the issuer", putOidc("drg_full", `{"oidc_issuer_url":"https://idp2.example","oidc_host_manager_group":"ops"}`), http.StatusOK},
		{"admin may resend the existing superadmin mapping", putOidc("admin", `{"oidc_superadmin_group":"supers","oidc_button_text":"SSO"}`), http.StatusOK},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestRoleOutranks(t *testing.T) {
	t.Parallel()
	d := testDatabase(t)
	perms := store.NewPermissionsStore(d)
	ph := NewPermissionsHandler(perms)
	for role, body := range map[string]string{
		"ro_full":   allPermsJSON,
		"ro_users":  `{"can_view_dashboard":true,"can_view_users":true,"can_manage_users":true}`,
		"ro_users2": `{"can_view_dashboard":true,"can_view_users":true,"can_manage_users":true}`,
		"ro_hosts":  `{"can_view_dashboard":true,"can_view_hosts":true,"can_manage_hosts":true}`,
		"ro_view":   `{"can_view_dashboard":true}`,
	} {
		rec := httptest.NewRecorder()
		ph.UpdateRole(rec, roleRequest(http.MethodPut, role, "superadmin", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("create %s: %d", role, rec.Code)
		}
	}
	ctx := context.Background()
	cases := []struct {
		caller, target string
		want           bool
	}{
		{"superadmin", "superadmin", true},
		{"superadmin", "admin", true},
		{"admin", "superadmin", false},
		{"admin", "admin", false},
		{"admin", "host_manager", true},
		{"admin", "ro_users", true},
		{"ro_full", "superadmin", false},
		{"ro_full", "admin", true},
		{"ro_users", "ro_view", true},
		{"ro_users", "ro_users2", false},
		{"ro_users", "ro_hosts", false},
		{"ro_hosts", "ro_users", false},
		{"ro_view", "ro_users", false},
		{"ro_users", "admin", false},
	}
	for _, c := range cases {
		if got := roleOutranks(ctx, perms, c.caller, c.target); got != c.want {
			t.Errorf("roleOutranks(%s, %s) = %v, want %v", c.caller, c.target, got, c.want)
		}
	}
}

func TestUserManagementRespectsRoleCoverage(t *testing.T) {
	t.Parallel()
	d := testDatabase(t)
	perms := store.NewPermissionsStore(d)
	users := store.NewUsersStore(d)
	ph := NewPermissionsHandler(perms)
	uh := &UsersHandler{users: users, sessions: store.NewSessionsStore(d), permissions: perms}

	for role, body := range map[string]string{
		"um_helpdesk": `{"can_view_dashboard":true,"can_view_users":true,"can_manage_users":true}`,
		"um_ops":      allPermsJSON,
		"um_viewer":   `{"can_view_dashboard":true}`,
	} {
		rec := httptest.NewRecorder()
		ph.UpdateRole(rec, roleRequest(http.MethodPut, role, "superadmin", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("create %s: %d", role, rec.Code)
		}
	}
	mk := func(name, role string) string {
		u := &models.User{Username: name, Email: name + "@ex.test", Role: role, IsActive: true}
		if err := users.Create(context.Background(), u); err != nil {
			t.Fatalf("create user %s: %v", name, err)
		}
		return u.ID
	}
	ops := mk("um_ops_user", "um_ops")
	opsForDelete := mk("um_ops_user2", "um_ops")
	viewer := mk("um_viewer_user", "um_viewer")
	viewerForDelete := mk("um_viewer_user2", "um_viewer")

	call := func(h http.HandlerFunc, method, userID, callerRole, body string) int {
		req := httptest.NewRequest(method, "/auth/admin/users/"+userID, strings.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("userId", userID)
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, middleware.UserRoleKey, callerRole)
		ctx = context.WithValue(ctx, middleware.UserIDKey, "someone-else")
		rec := httptest.NewRecorder()
		h(rec, req.WithContext(ctx))
		return rec.Code
	}
	const pw = `{"newPassword":"Takeov3r!Passw0rd"}`
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"helpdesk cannot edit a user in a role above it", call(uh.Update, http.MethodPut, ops, "um_helpdesk", `{"first_name":"x"}`), http.StatusForbidden},
		{"helpdesk cannot reset the password of a user above it", call(uh.ResetPassword, http.MethodPost, ops, "um_helpdesk", pw), http.StatusForbidden},
		{"helpdesk cannot delete a user above it", call(uh.Delete, http.MethodDelete, opsForDelete, "um_helpdesk", ""), http.StatusForbidden},
		{"helpdesk cannot promote a user into a role above it", call(uh.Update, http.MethodPut, viewer, "um_helpdesk", `{"role":"um_ops"}`), http.StatusForbidden},
		{"helpdesk edits a user below it", call(uh.Update, http.MethodPut, viewer, "um_helpdesk", `{"first_name":"x"}`), http.StatusOK},
		{"helpdesk resets the password of a user below it", call(uh.ResetPassword, http.MethodPost, viewer, "um_helpdesk", pw), http.StatusOK},
		{"helpdesk deletes a user below it", call(uh.Delete, http.MethodDelete, viewerForDelete, "um_helpdesk", ""), http.StatusOK},
		{"admin cannot reset the password of a full custom role", call(uh.ResetPassword, http.MethodPost, ops, "admin", pw), http.StatusForbidden},
		{"superadmin resets anyone", call(uh.ResetPassword, http.MethodPost, ops, "superadmin", pw), http.StatusOK},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestOidcTrustChangedTreatsMissingDefaultRoleAsUser(t *testing.T) {
	t.Parallel()
	user := "user"
	admin := "admin"
	if oidcTrustChanged(&models.Settings{}, &models.Settings{OidcDefaultRole: &user}) {
		t.Error("unset -> user counted as a change")
	}
	if !oidcTrustChanged(&models.Settings{}, &models.Settings{OidcDefaultRole: &admin}) {
		t.Error("unset -> admin not counted as a change")
	}
}

func TestOidcImportFromEnvRequiresManageSuperusers(t *testing.T) {
	t.Setenv("OIDC_CLIENT_ID", "pm-env")
	d := testDatabase(t)
	perms := store.NewPermissionsStore(d)
	oh := (&OidcHandler{
		cfg:      &config.Config{OidcClientID: "pm-env", OidcIssuerURL: "https://env-idp.example", OidcSuperadminGroup: "env-supers"},
		settings: store.NewSettingsStore(d),
	}).WithPermissions(perms)
	rec := httptest.NewRecorder()
	NewPermissionsHandler(perms).UpdateRole(rec, roleRequest(http.MethodPut, "imp_full", "superadmin", allPermsJSON))
	if rec.Code != http.StatusOK {
		t.Fatalf("create imp_full: %d", rec.Code)
	}
	importAs := func(callerRole string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/oidc/settings/import-from-env", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserRoleKey, callerRole))
		rec := httptest.NewRecorder()
		oh.ImportFromEnv(rec, req)
		return rec.Code
	}
	if got := importAs("admin"); got != http.StatusForbidden {
		t.Errorf("admin import: got %d, want 403", got)
	}
	if got := importAs("imp_full"); got != http.StatusOK {
		t.Errorf("full custom role with can_manage_superusers import: got %d, want 200", got)
	}
	if got := importAs("superadmin"); got != http.StatusOK {
		t.Errorf("superadmin import: got %d, want 200", got)
	}
}

func TestRoleChangeRequiresOutranking(t *testing.T) {
	t.Parallel()
	d := testDatabase(t)
	perms := store.NewPermissionsStore(d)
	users := store.NewUsersStore(d)
	ph := NewPermissionsHandler(perms)
	uh := &UsersHandler{users: users, sessions: store.NewSessionsStore(d), permissions: perms}
	rec := httptest.NewRecorder()
	ph.UpdateRole(rec, roleRequest(http.MethodPut, "rc_helpdesk", "superadmin", `{"can_view_dashboard":true,"can_view_users":true,"can_manage_users":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("create role: %d", rec.Code)
	}
	mk := func(name, role string) string {
		u := &models.User{Username: name, Email: name + "@ex.test", Role: role, IsActive: true}
		if err := users.Create(context.Background(), u); err != nil {
			t.Fatalf("create user %s: %v", name, err)
		}
		return u.ID
	}
	admin1, admin2, admin3 := mk("rc_admin1", "admin"), mk("rc_admin2", "admin"), mk("rc_admin3", "admin")
	plain, ro := mk("rc_user", "user"), mk("rc_ro", "readonly")
	help1, help2 := mk("rc_help1", "rc_helpdesk"), mk("rc_help2", "rc_helpdesk")
	super1, demoted, plain2 := mk("rc_super", "superadmin"), mk("rc_demoted", "user"), mk("rc_user2", "user")

	update := func(userID, callerID, callerRole, body string) int {
		req := httptest.NewRequest(http.MethodPut, "/auth/admin/users/"+userID, strings.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("userId", userID)
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, middleware.UserRoleKey, callerRole)
		ctx = context.WithValue(ctx, middleware.UserIDKey, callerID)
		rec := httptest.NewRecorder()
		uh.Update(rec, req.WithContext(ctx))
		return rec.Code
	}
	roleOf := func(id string) string {
		u, err := users.GetByID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		return u.Role
	}

	cases := []struct {
		name string
		got  int
		want int
	}{
		{"admin cannot demote a peer admin", update(admin2, admin1, "admin", `{"role":"user"}`), http.StatusForbidden},
		{"admin may still edit a peer admin's name", update(admin2, admin1, "admin", `{"first_name":"x"}`), http.StatusOK},
		{"admin saves a peer admin's name with the role unchanged", update(admin2, admin1, "admin", `{"role":"admin","first_name":"y"}`), http.StatusOK},
		{"a demoted admin's stale token cannot change roles", update(plain2, demoted, "admin", `{"role":"readonly"}`), http.StatusForbidden},
		{"admin changes a user's role", update(plain, admin1, "admin", `{"role":"readonly"}`), http.StatusOK},
		{"admin promotes readonly to host_manager", update(ro, admin1, "admin", `{"role":"host_manager"}`), http.StatusOK},
		{"admin still cannot assign admin", update(ro, admin1, "admin", `{"role":"admin"}`), http.StatusForbidden},
		{"custom role cannot change a peer with the same permissions", update(help2, help1, "rc_helpdesk", `{"role":"readonly"}`), http.StatusForbidden},
		{"nobody changes their own role", update(admin1, admin1, "admin", `{"role":"user"}`), http.StatusBadRequest},
		{"superadmin changes an admin's role", update(admin3, super1, "superadmin", `{"role":"user"}`), http.StatusOK},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
	for id, want := range map[string]string{admin2: "admin", plain: "readonly", ro: "host_manager", help2: "rc_helpdesk", admin1: "admin", admin3: "user", plain2: "user"} {
		if got := roleOf(id); got != want {
			t.Errorf("user %s role = %q, want %q", id, got, want)
		}
	}
}
