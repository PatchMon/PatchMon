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
