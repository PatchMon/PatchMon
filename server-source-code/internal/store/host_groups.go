package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/google/uuid"
)

// HostGroupsStore provides host group access.
type HostGroupsStore struct {
	db database.DBProvider
}

// NewHostGroupsStore creates a new host groups store.
func NewHostGroupsStore(db database.DBProvider) *HostGroupsStore {
	return &HostGroupsStore{db: db}
}

// List returns all host groups.
func (s *HostGroupsStore) List(ctx context.Context) ([]models.HostGroup, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListHostGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.HostGroup, len(rows))
	for i := range rows {
		out[i] = dbHostGroupToModelFull(rows[i])
	}
	return out, nil
}

// ListWithHostCount returns all host groups with host count (for settings UI).
func (s *HostGroupsStore) ListWithHostCount(ctx context.Context) ([]db.ListHostGroupsWithHostCountRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListHostGroupsWithHostCount(ctx)
}

// GetByID returns a host group by ID.
func (s *HostGroupsStore) GetByID(ctx context.Context, id string) (*models.HostGroup, error) {
	d := s.db.DB(ctx)
	h, err := d.Queries.GetHostGroupByID(ctx, id)
	if err != nil {
		return nil, err
	}
	out := dbHostGroupToModelFull(h)
	return &out, nil
}

// Create creates a new host group.
func (s *HostGroupsStore) Create(ctx context.Context, g *models.HostGroup) error {
	d := s.db.DB(ctx)
	if g.ID == "" {
		g.ID = uuid.New().String()
	}
	now := time.Now()
	g.CreatedAt = now
	g.UpdatedAt = now
	arg := db.CreateHostGroupParams{
		ID:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		Color:       g.Color,
		CreatedAt:   pgtime.From(now),
		UpdatedAt:   pgtime.From(now),
	}
	return d.Queries.CreateHostGroup(ctx, arg)
}

// Update updates a host group.
func (s *HostGroupsStore) Update(ctx context.Context, g *models.HostGroup) error {
	d := s.db.DB(ctx)
	g.UpdatedAt = time.Now()
	arg := db.UpdateHostGroupParams{
		Name:        g.Name,
		Description: g.Description,
		Color:       g.Color,
		UpdatedAt:   pgtime.From(g.UpdatedAt),
		ID:          g.ID,
	}
	return d.Queries.UpdateHostGroup(ctx, arg)
}

// Delete deletes a host group.
func (s *HostGroupsStore) Delete(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteHostGroup(ctx, id)
}

// GetHostIDs returns host IDs in a group.
func (s *HostGroupsStore) GetHostIDs(ctx context.Context, groupID string) ([]string, error) {
	d := s.db.DB(ctx)
	return d.Queries.GetHostIDsByGroup(ctx, groupID)
}

// PackageHolds returns the package names held on one group, in name order.
func (s *HostGroupsStore) PackageHolds(ctx context.Context, groupID string) ([]string, error) {
	d := s.db.DB(ctx)
	names, err := d.Queries.ListHostGroupPackageHolds(ctx, groupID)
	if err != nil || names == nil {
		return []string{}, err
	}
	return names, nil
}

// AllPackageHolds returns every group's held package names.
func (s *HostGroupsStore) AllPackageHolds(ctx context.Context) (map[string][]string, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListAllHostGroupPackageHolds(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, row := range rows {
		out[row.HostGroupID] = append(out[row.HostGroupID], row.PackageName)
	}
	return out, nil
}

// EffectivePackageHolds is the union of holds from every group the host belongs to.
func (s *HostGroupsStore) EffectivePackageHolds(ctx context.Context, hostID string) ([]string, error) {
	d := s.db.DB(ctx)
	names, err := d.Queries.ListEffectivePackageHoldsForHost(ctx, hostID)
	if err != nil || names == nil {
		return []string{}, err
	}
	return names, nil
}

// ReplacePackageHolds replaces a group's held package names.
func (s *HostGroupsStore) ReplacePackageHolds(ctx context.Context, groupID string, names []string) error {
	d := s.db.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()
	q := d.Queries.WithTx(tx)
	if err := q.DeleteHostGroupPackageHolds(ctx, groupID); err != nil {
		return err
	}
	for _, name := range names {
		if err := q.InsertHostGroupPackageHold(ctx, db.InsertHostGroupPackageHoldParams{
			ID:          uuid.NewString(),
			HostGroupID: groupID,
			PackageName: name,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// SecurityUpdatePackageNames returns packages flagged as security updates on a host.
func (s *HostGroupsStore) SecurityUpdatePackageNames(ctx context.Context, hostID string) ([]string, error) {
	d := s.db.DB(ctx)
	names, err := d.Queries.ListSecurityUpdatePackageNamesByHost(ctx, hostID)
	if err != nil || names == nil {
		return []string{}, err
	}
	return names, nil
}
