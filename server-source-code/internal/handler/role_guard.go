package handler

import (
	"context"
	"errors"
	"reflect"

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
