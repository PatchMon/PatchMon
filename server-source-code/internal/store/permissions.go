package store

import (
	"context"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/google/uuid"
)

// PermissionsStore provides role permissions access.
type PermissionsStore struct {
	db database.DBProvider
}

// NewPermissionsStore creates a new permissions store.
func NewPermissionsStore(db database.DBProvider) *PermissionsStore {
	return &PermissionsStore{db: db}
}

// GetByRole returns permissions for a role.
func (s *PermissionsStore) GetByRole(ctx context.Context, role string) (*models.RolePermission, error) {
	d := s.db.DB(ctx)
	r, err := d.Queries.GetRolePermissions(ctx, role)
	if err != nil {
		return nil, err
	}
	out := dbRolePermissionToModel(r)
	return &out, nil
}

// ListRoles returns all roles.
func (s *PermissionsStore) ListRoles(ctx context.Context) ([]models.RolePermission, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.RolePermission, len(rows))
	for i := range rows {
		out[i] = dbRolePermissionToModel(rows[i])
	}
	return out, nil
}

// UpsertRole creates or updates role permissions.
func (s *PermissionsStore) UpsertRole(ctx context.Context, p *models.RolePermission) error {
	d := s.db.DB(ctx)
	existing, _ := s.GetByRole(ctx, p.Role)
	id := uuid.New().String()
	if existing != nil {
		id = existing.ID
	}
	_, err := d.Queries.UpsertRolePermissions(ctx, db.UpsertRolePermissionsParams{
		ID:                      id,
		Role:                    p.Role,
		CanViewDashboard:        p.CanViewDashboard,
		CanViewHosts:            p.CanViewHosts,
		CanManageHosts:          p.CanManageHosts,
		CanViewPackages:         p.CanViewPackages,
		CanManagePackages:       p.CanManagePackages,
		CanViewUsers:            p.CanViewUsers,
		CanManageUsers:          p.CanManageUsers,
		CanManageSuperusers:     p.CanManageSuperusers,
		CanViewReports:          p.CanViewReports,
		CanExportData:           p.CanExportData,
		CanManageSettings:       p.CanManageSettings,
		CanManageNotifications:  p.CanManageNotifications,
		CanViewNotificationLogs: p.CanViewNotificationLogs,
		CanManagePatching:       p.CanManagePatching,
		CanManageCompliance:     p.CanManageCompliance,
		CanManageDocker:         p.CanManageDocker,
		CanManageAlerts:         p.CanManageAlerts,
		CanManageAutomation:     p.CanManageAutomation,
		CanUseRemoteAccess:      p.CanUseRemoteAccess,
		CanManageBilling:        p.CanManageBilling,
	})
	return err
}

// DeleteRole removes a role's permissions.
func (s *PermissionsStore) DeleteRole(ctx context.Context, role string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteRolePermissions(ctx, role)
}

// CountUsersByRole returns the number of users with the given role.
func (s *PermissionsStore) CountUsersByRole(ctx context.Context, role string) (int, error) {
	d := s.db.DB(ctx)
	n, err := d.Queries.CountUsersByRole(ctx, role)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
