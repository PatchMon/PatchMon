package handler

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/jackc/pgx/v5"
)

// missingPermissions lists the permission flags target grants that holder lacks.
func missingPermissions(holder, target *models.RolePermission) []string {
	hv := reflect.ValueOf(holder).Elem()
	tv := reflect.ValueOf(target).Elem()
	var missing []string
	for i := 0; i < tv.NumField(); i++ {
		if tv.Field(i).Kind() == reflect.Bool && tv.Field(i).Bool() && !hv.Field(i).Bool() {
			missing = append(missing, tv.Type().Field(i).Tag.Get("db"))
		}
	}
	return missing
}

// roleCovers reports whether callerRole may assign targetRole, or edit or act on its users:
// superadmin always may, anyone else must hold every permission targetRole grants.
func roleCovers(ctx context.Context, perms *store.PermissionsStore, callerRole, targetRole string) bool {
	if callerRole == "superadmin" {
		return true
	}
	caller, err := perms.GetByRole(ctx, callerRole)
	if err != nil || caller == nil {
		return false
	}
	target, err := perms.GetByRole(ctx, targetRole)
	if errors.Is(err, pgx.ErrNoRows) {
		// UserPermissions grants full access to admin and superadmin when their row is missing.
		return targetRole != "admin" && targetRole != "superadmin"
	}
	if err != nil || target == nil {
		return false
	}
	return len(missingPermissions(caller, target)) == 0
}

// canGrantRole reports whether callerRole may give targetRole, which must exist, to someone.
// Only superadmin may grant admin.
func canGrantRole(ctx context.Context, perms *store.PermissionsStore, callerRole, targetRole string) bool {
	targetRole = strings.TrimSpace(targetRole)
	if perms == nil || targetRole == "" {
		return false
	}
	if _, err := perms.GetByRole(ctx, targetRole); err != nil {
		return false
	}
	if callerRole == "superadmin" {
		return true
	}
	if targetRole == "admin" {
		return false
	}
	return roleCovers(ctx, perms, callerRole, targetRole)
}

// ungrantableRoleChange returns the first role whose default-role or OIDC group mapping differs
// between before and after and that callerRole may not grant, or "" if there is none.
func ungrantableRoleChange(ctx context.Context, perms *store.PermissionsStore, callerRole string, before, after *models.Settings) string {
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	changes := []struct {
		role    string
		changed bool
	}{
		{after.DefaultUserRole, after.DefaultUserRole != before.DefaultUserRole},
		{deref(after.OidcDefaultRole), deref(after.OidcDefaultRole) != deref(before.OidcDefaultRole)},
		{"superadmin", deref(after.OidcSuperadminGroup) != deref(before.OidcSuperadminGroup)},
		{"admin", deref(after.OidcAdminGroup) != deref(before.OidcAdminGroup)},
		{"host_manager", deref(after.OidcHostManagerGroup) != deref(before.OidcHostManagerGroup)},
		{"readonly", deref(after.OidcReadonlyGroup) != deref(before.OidcReadonlyGroup)},
		{"user", deref(after.OidcUserGroup) != deref(before.OidcUserGroup)},
	}
	for _, c := range changes {
		if c.changed && !canGrantRole(ctx, perms, callerRole, c.role) {
			return c.role
		}
	}
	return ""
}

// roleOutranks reports whether callerRole is strictly above targetRole: superadmin, or covering
// every permission of targetRole without targetRole covering every permission of callerRole.
func roleOutranks(ctx context.Context, perms *store.PermissionsStore, callerRole, targetRole string) bool {
	if callerRole == "superadmin" {
		return true
	}
	return roleCovers(ctx, perms, callerRole, targetRole) && !roleCovers(ctx, perms, targetRole, callerRole)
}
