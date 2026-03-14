package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// HostsStore provides host access via sqlc.
type HostsStore struct {
	db database.DBProvider
}

// NewHostsStore creates a new hosts store.
func NewHostsStore(db database.DBProvider) *HostsStore {
	return &HostsStore{db: db}
}

// List returns all hosts.
func (s *HostsStore) List(ctx context.Context) ([]models.Host, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.Host, len(rows))
	for i, h := range rows {
		out[i] = *dbHostToModel(h)
	}
	return out, nil
}

// ListPaginated returns hosts with pagination.
func (s *HostsStore) ListPaginated(ctx context.Context, limit, offset int) ([]models.Host, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListHostsPaginated(ctx, db.ListHostsPaginatedParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]models.Host, len(rows))
	for i, r := range rows {
		out[i] = models.Host{
			ID:                r.ID,
			FriendlyName:      r.FriendlyName,
			Hostname:          r.Hostname,
			IP:                r.Ip,
			OSType:            r.OsType,
			OSVersion:         r.OsVersion,
			Architecture:      r.Architecture,
			LastUpdate:        pgTime(r.LastUpdate),
			Status:            r.Status,
			ApiID:             r.ApiID,
			AgentVersion:      r.AgentVersion,
			AutoUpdate:        r.AutoUpdate,
			CreatedAt:         pgTime(r.CreatedAt),
			Notes:             r.Notes,
			SystemUptime:      r.SystemUptime,
			NeedsReboot:       r.NeedsReboot,
			DockerEnabled:     r.DockerEnabled,
			ComplianceEnabled: r.ComplianceEnabled,
		}
	}
	return out, nil
}

// Count returns total host count.
func (s *HostsStore) Count(ctx context.Context) (int, error) {
	d := s.db.DB(ctx)
	n, err := d.Queries.CountHosts(ctx)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// GetByID returns a host by ID.
func (s *HostsStore) GetByID(ctx context.Context, id string) (*models.Host, error) {
	hosts, err := s.GetByIDs(ctx, []string{id})
	if err != nil || len(hosts) == 0 {
		return nil, err
	}
	return &hosts[0], nil
}

// GetByIDs returns hosts by IDs in one query. Preserves input order.
func (s *HostsStore) GetByIDs(ctx context.Context, ids []string) ([]models.Host, error) {
	d := s.db.DB(ctx)
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := d.Queries.GetHostsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]models.Host)
	for _, h := range rows {
		byID[h.ID] = *dbHostToModel(h)
	}
	ordered := make([]models.Host, 0, len(ids))
	for _, id := range ids {
		if h, ok := byID[id]; ok {
			ordered = append(ordered, h)
		}
	}
	return ordered, nil
}

// GetByApiID returns a host by api_id.
func (s *HostsStore) GetByApiID(ctx context.Context, apiID string) (*models.Host, error) {
	d := s.db.DB(ctx)
	h, err := d.Queries.GetHostByApiID(ctx, apiID)
	if err != nil {
		return nil, err
	}
	return dbHostToModel(h), nil
}

// Create creates a new host.
func (s *HostsStore) Create(ctx context.Context, h *models.Host) error {
	d := s.db.DB(ctx)
	if h.ID == "" {
		h.ID = uuid.New().String()
	}
	now := time.Now()
	h.CreatedAt = now
	h.UpdatedAt = now
	h.LastUpdate = now
	pgNow := pgtype.Timestamp{Time: now, Valid: true}
	return d.Queries.CreateHost(ctx, db.CreateHostParams{
		ID:                     h.ID,
		MachineID:              h.MachineID,
		FriendlyName:           h.FriendlyName,
		Ip:                     h.IP,
		OsType:                 h.OSType,
		OsVersion:              h.OSVersion,
		Architecture:           h.Architecture,
		LastUpdate:             pgNow,
		Status:                 h.Status,
		ApiID:                  h.ApiID,
		ApiKey:                 h.ApiKey,
		AgentVersion:           h.AgentVersion,
		AutoUpdate:             h.AutoUpdate,
		CreatedAt:              pgNow,
		UpdatedAt:              pgNow,
		DockerEnabled:          h.DockerEnabled,
		ComplianceEnabled:      h.ComplianceEnabled,
		ComplianceOnDemandOnly: h.ComplianceOnDemandOnly,
		ExpectedPlatform:       h.ExpectedPlatform,
	})
}

// UpdateFriendlyName updates a host's friendly name.
func (s *HostsStore) UpdateFriendlyName(ctx context.Context, id, friendlyName string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostFriendlyName(ctx, db.UpdateHostFriendlyNameParams{
		FriendlyName: friendlyName,
		ID:           id,
	})
}

// UpdateNotes updates a host's notes.
func (s *HostsStore) UpdateNotes(ctx context.Context, id string, notes *string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostNotes(ctx, db.UpdateHostNotesParams{
		Notes: notes,
		ID:    id,
	})
}

// UpdateConnection updates a host's ip and hostname.
func (s *HostsStore) UpdateConnection(ctx context.Context, id string, ip, hostname *string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostConnection(ctx, db.UpdateHostConnectionParams{
		Ip:       ip,
		Hostname: hostname,
		ID:       id,
	})
}

// SetPrimaryInterface sets the primary (main) network interface for a host.
// Pass nil or empty string to clear. Does not update hosts.ip; caller should derive and call UpdateConnection if needed.
func (s *HostsStore) SetPrimaryInterface(ctx context.Context, id string, interfaceName *string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostPrimaryInterface(ctx, db.UpdateHostPrimaryInterfaceParams{
		PrimaryInterface: interfaceName,
		ID:               id,
	})
}

// UpdateAutoUpdate updates a host's auto_update setting.
func (s *HostsStore) UpdateAutoUpdate(ctx context.Context, id string, autoUpdate bool) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostAutoUpdate(ctx, db.UpdateHostAutoUpdateParams{
		AutoUpdate: autoUpdate,
		ID:         id,
	})
}

// UpdateHostDownAlerts updates a host's host_down_alerts_enabled setting.
func (s *HostsStore) UpdateHostDownAlerts(ctx context.Context, id string, enabled *bool) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostDownAlerts(ctx, db.UpdateHostDownAlertsParams{
		HostDownAlertsEnabled: enabled,
		ID:                    id,
	})
}

// UpdateDockerEnabled updates a host's docker_enabled setting.
func (s *HostsStore) UpdateDockerEnabled(ctx context.Context, id string, enabled bool) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostDockerEnabled(ctx, db.UpdateHostDockerEnabledParams{
		DockerEnabled: enabled,
		ID:            id,
	})
}

// UpdateComplianceEnabled updates a host's compliance_enabled setting.
func (s *HostsStore) UpdateComplianceEnabled(ctx context.Context, id string, enabled bool) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostComplianceEnabled(ctx, db.UpdateHostComplianceEnabledParams{
		ComplianceEnabled: enabled,
		ID:                id,
	})
}

// UpdateComplianceMode updates compliance_enabled and compliance_on_demand_only.
func (s *HostsStore) UpdateComplianceMode(ctx context.Context, id string, complianceEnabled, complianceOnDemandOnly bool) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostComplianceMode(ctx, db.UpdateHostComplianceModeParams{
		ComplianceEnabled:      complianceEnabled,
		ComplianceOnDemandOnly: complianceOnDemandOnly,
		ID:                     id,
	})
}

// UpdateComplianceScanners updates openscap and docker bench scanner toggles.
func (s *HostsStore) UpdateComplianceScanners(ctx context.Context, id string, openscapEnabled, dockerBenchEnabled bool) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostComplianceScanners(ctx, db.UpdateHostComplianceScannersParams{
		ComplianceOpenscapEnabled:    openscapEnabled,
		ComplianceDockerBenchEnabled: dockerBenchEnabled,
		ID:                           id,
	})
}

// UpdateComplianceScannerStatus updates the persisted compliance scanner status from agent.
func (s *HostsStore) UpdateComplianceScannerStatus(ctx context.Context, id string, statusJSON []byte, updatedAt time.Time) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostComplianceScannerStatus(ctx, db.UpdateHostComplianceScannerStatusParams{
		ComplianceScannerStatus:    statusJSON,
		ComplianceScannerUpdatedAt: pgtype.Timestamp{Time: updatedAt, Valid: true},
		ID:                         id,
	})
}

// UpdateApiCredentials updates api_id and api_key for a host.
func (s *HostsStore) UpdateApiCredentials(ctx context.Context, id, apiID, apiKeyHash string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostApiCredentials(ctx, db.UpdateHostApiCredentialsParams{
		ApiID:  apiID,
		ApiKey: apiKeyHash,
		ID:     id,
	})
}

// UpdatePing updates last_update and status to active for a host (agent ping).
func (s *HostsStore) UpdatePing(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateHostPing(ctx, id)
}

// Delete deletes a host.
func (s *HostsStore) Delete(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteHost(ctx, id)
}

// DeleteMany deletes multiple hosts.
func (s *HostsStore) DeleteMany(ctx context.Context, ids []string) (int64, error) {
	d := s.db.DB(ctx)
	if len(ids) == 0 {
		return 0, nil
	}
	if err := d.Queries.DeleteHostsByIDs(ctx, ids); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

// GetHostGroups returns host groups for a host.
func (s *HostsStore) GetHostGroups(ctx context.Context, hostID string) ([]models.HostGroup, error) {
	groups, err := s.GetHostGroupsForHosts(ctx, []string{hostID})
	if err != nil {
		return nil, err
	}
	return groups[hostID], nil
}

// GetHostGroupsForHosts returns host groups for multiple hosts in one query.
func (s *HostsStore) GetHostGroupsForHosts(ctx context.Context, hostIDs []string) (map[string][]models.HostGroup, error) {
	d := s.db.DB(ctx)
	out := make(map[string][]models.HostGroup)
	if len(hostIDs) == 0 {
		return out, nil
	}
	for _, id := range hostIDs {
		out[id] = nil
	}
	rows, err := d.Queries.GetHostGroupsForHosts(ctx, hostIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.HostID] = append(out[r.HostID], dbHostGroupToModel(r))
	}
	return out, nil
}

// SetHostGroups replaces host group memberships for a host.
func (s *HostsStore) SetHostGroups(ctx context.Context, hostID string, groupIDs []string) error {
	d := s.db.DB(ctx)
	if err := d.Queries.DeleteHostGroupMemberships(ctx, hostID); err != nil {
		return err
	}
	for _, gid := range groupIDs {
		if err := d.Queries.InsertHostGroupMembership(ctx, db.InsertHostGroupMembershipParams{
			ID:          uuid.New().String(),
			HostID:      hostID,
			HostGroupID: gid,
		}); err != nil {
			return err
		}
	}
	return nil
}

// SetHostGroupsBulk updates group memberships for multiple hosts.
func (s *HostsStore) SetHostGroupsBulk(ctx context.Context, hostIDs, groupIDs []string) error {
	for _, hid := range hostIDs {
		if err := s.SetHostGroups(ctx, hid, groupIDs); err != nil {
			return err
		}
	}
	return nil
}

// HostPackageStats holds package counts for a host (for scoped API include=stats).
type HostPackageStats struct {
	Total    int
	Outdated int
	Security int
}

// ListForScopedApi returns hosts optionally filtered by group IDs, with host groups and optional package stats.
// If groupIDs is empty, all hosts are returned. If includeStats is true, statsMap is populated per host ID.
func (s *HostsStore) ListForScopedApi(ctx context.Context, groupIDs []string, includeStats bool) (hosts []models.Host, groupsMap map[string][]models.HostGroup, statsMap map[string]HostPackageStats, err error) {
	d := s.db.DB(ctx)
	var hostIDs []string
	if len(groupIDs) == 0 {
		all, err := s.List(ctx)
		if err != nil {
			return nil, nil, nil, err
		}
		hosts = all
		for _, h := range all {
			hostIDs = append(hostIDs, h.ID)
		}
	} else {
		hostIDs, err = d.Queries.GetHostIDsByGroupIDs(ctx, groupIDs)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(hostIDs) == 0 {
			return []models.Host{}, make(map[string][]models.HostGroup), nil, nil
		}
		hosts, err = s.GetByIDs(ctx, hostIDs)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	groupsMap, err = s.GetHostGroupsForHosts(ctx, hostIDs)
	if err != nil {
		return nil, nil, nil, err
	}

	if includeStats && len(hostIDs) > 0 {
		rows, err := d.Queries.GetHostPackageStatsByHostIDs(ctx, hostIDs)
		if err != nil {
			return nil, nil, nil, err
		}
		statsMap = make(map[string]HostPackageStats)
		for _, r := range rows {
			statsMap[r.HostID] = HostPackageStats{
				Total:    int(r.Total),
				Outdated: int(r.Outdated),
				Security: int(r.Security),
			}
		}
	}

	return hosts, groupsMap, statsMap, nil
}
