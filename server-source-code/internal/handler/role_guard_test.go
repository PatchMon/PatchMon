package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

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
		req := roleRequest(http.MethodPost, target, callerRole, "")
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
		{"custom role may shrink its own role", put("rg_helpdesk", "rg_helpdesk", `{"can_view_dashboard":true,"can_view_users":true,"can_manage_users":true,"can_manage_settings":true}`), http.StatusOK},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
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
		{"rg_helpdesk", "rg_does_not_exist", true},
	}
	for _, a := range assigns {
		if got := assign(a.caller, a.target); got != a.want {
			t.Errorf("canAssignRole(%s -> %s) = %v, want %v", a.caller, a.target, got, a.want)
		}
	}
}
