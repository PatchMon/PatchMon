package store

import (
	"context"
	"errors"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/jackc/pgx/v5"
)

// PendingConfigStore provides access to host pending configuration.
type PendingConfigStore struct {
	db database.DBProvider
}

// NewPendingConfigStore creates a new pending config store.
func NewPendingConfigStore(db database.DBProvider) *PendingConfigStore {
	return &PendingConfigStore{db: db}
}

// PendingConfigFields holds optional fields to merge into pending config.
type PendingConfigFields struct {
	DockerEnabled                *bool
	ComplianceEnabled            *bool
	ComplianceOnDemandOnly       *bool
	ComplianceOpenscapEnabled    *bool
	ComplianceDockerBenchEnabled *bool
}

// GetPendingConfig returns the pending config for a host, or nil if none exists.
func (s *PendingConfigStore) GetPendingConfig(ctx context.Context, hostID string) (*db.HostPendingConfig, error) {
	d := s.db.DB(ctx)
	pc, err := d.Queries.GetPendingConfig(ctx, hostID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &pc, nil
}

// SetPendingConfig merges the given fields into the pending config for the host.
// Only non-nil fields are updated.
func (s *PendingConfigStore) SetPendingConfig(ctx context.Context, hostID string, fields PendingConfigFields) error {
	d := s.db.DB(ctx)
	return d.Queries.UpsertPendingConfig(ctx, db.UpsertPendingConfigParams{
		HostID:                       hostID,
		DockerEnabled:                fields.DockerEnabled,
		ComplianceEnabled:            fields.ComplianceEnabled,
		ComplianceOnDemandOnly:       fields.ComplianceOnDemandOnly,
		ComplianceOpenscapEnabled:    fields.ComplianceOpenscapEnabled,
		ComplianceDockerBenchEnabled: fields.ComplianceDockerBenchEnabled,
	})
}

// ClearPendingConfig removes the pending config for a host.
func (s *PendingConfigStore) ClearPendingConfig(ctx context.Context, hostID string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeletePendingConfig(ctx, hostID)
}
