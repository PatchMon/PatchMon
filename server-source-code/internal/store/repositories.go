package store

import (
	"context"
	"errors"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/safeconv"
	"github.com/jackc/pgx/v5"
)

// RepositoriesStore provides repository access.
type RepositoriesStore struct {
	db database.DBProvider
}

// NewRepositoriesStore creates a new repositories store.
func NewRepositoriesStore(db database.DBProvider) *RepositoriesStore {
	return &RepositoriesStore{db: db}
}

// RepoListParams holds optional filters for List.
type RepoListParams struct {
	HostID string
	Search string
	Status string
	Type   string
	Limit  int
	Offset int
	Sort   string
	Order  string
	Legacy bool
}

// RepositoryListResult is a paginated repository list response.
type RepositoryListResult struct {
	Items  []RepositoryWithHosts `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

// List returns repositories with aggregate host counts.
func (s *RepositoriesStore) List(ctx context.Context, params RepoListParams) (*RepositoryListResult, error) {
	d := s.db.DB(ctx)
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Legacy && params.Limit > 5000 {
		params.Limit = 5000
	}
	if !params.Legacy && params.Limit > 500 {
		params.Limit = 500
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	sortKey := repoListSortKey(params.Sort)
	sortDir := "asc"
	if params.Order == "desc" {
		sortDir = "desc"
	}
	arg := db.ListRepositoriesParams{
		SortKey:   sortKey,
		SortDir:   sortDir,
		RowLimit:  safeconv.ClampToInt32(params.Limit),
		RowOffset: safeconv.ClampToInt32(params.Offset),
	}
	if params.HostID != "" {
		arg.HostID = &params.HostID
	}
	if params.Search != "" {
		arg.Search = &params.Search
	}
	if params.Status != "" {
		arg.Status = &params.Status
	}
	if params.Type != "" {
		arg.Type = &params.Type
	}

	countArg := db.CountRepositoriesForListParams{}
	if params.HostID != "" {
		countArg.HostID = &params.HostID
	}
	if params.Search != "" {
		countArg.Search = &params.Search
	}
	if params.Status != "" {
		countArg.Status = &params.Status
	}
	if params.Type != "" {
		countArg.Type = &params.Type
	}
	total, err := d.Queries.CountRepositoriesForList(ctx, countArg)
	if err != nil {
		return nil, err
	}

	repos, err := d.Queries.ListRepositories(ctx, arg)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return &RepositoryListResult{Items: []RepositoryWithHosts{}, Total: int(total), Limit: params.Limit, Offset: params.Offset}, nil
	}

	ids := make([]string, len(repos))
	for i := range repos {
		ids[i] = repos[i].ID
	}

	countRows, err := d.Queries.GetRepoCountsForRepos(ctx, ids)
	if err != nil {
		return nil, err
	}

	type repoCounts struct {
		host    int
		enabled int
		active  int
	}
	countsByRepo := make(map[string]repoCounts, len(countRows))
	for _, r := range countRows {
		countsByRepo[r.RepositoryID] = repoCounts{
			host:    int(r.HostCount),
			enabled: int(r.EnabledHostCount),
			active:  int(r.ActiveHostCount),
		}
	}

	out := make([]RepositoryWithHosts, len(repos))
	for i := range repos {
		rid := repos[i].ID
		counts := countsByRepo[rid]
		out[i] = RepositoryWithHosts{
			Repository:       dbListRepositoryToModel(repos[i]),
			HostCount:        counts.host,
			EnabledHostCount: counts.enabled,
			ActiveHostCount:  counts.active,
			Hosts:            []HostRepoHost{},
		}
	}
	return &RepositoryListResult{Items: out, Total: int(total), Limit: params.Limit, Offset: params.Offset}, nil
}

func repoListSortKey(sort string) string {
	switch sort {
	case "name", "url", "distribution", "security", "status", "hostCount":
		return sort
	default:
		return "name"
	}
}

func dbListRepositoryToModel(r db.ListRepositoriesRow) models.Repository {
	prio := (*int)(nil)
	if r.Priority != nil {
		p := int(*r.Priority)
		prio = &p
	}
	return models.Repository{
		ID:           r.ID,
		Name:         r.Name,
		URL:          r.Url,
		Distribution: r.Distribution,
		Components:   r.Components,
		RepoType:     r.RepoType,
		IsActive:     r.IsActive,
		IsSecure:     r.IsSecure,
		Priority:     prio,
		Description:  r.Description,
		CreatedAt:    pgTime(r.CreatedAt),
		UpdatedAt:    pgTime(r.UpdatedAt),
	}
}

// RepositoryWithHosts extends Repository with host counts and host list.
type RepositoryWithHosts struct {
	models.Repository
	HostCount        int            `json:"hostCount"`
	EnabledHostCount int            `json:"enabledHostCount"`
	ActiveHostCount  int            `json:"activeHostCount"`
	Hosts            []HostRepoHost `json:"hosts"`
}

// HostRepoHost is a host entry in the repository list.
type HostRepoHost struct {
	ID           string  `json:"id"`
	FriendlyName string  `json:"friendlyName"`
	Status       string  `json:"status"`
	IsEnabled    bool    `json:"isEnabled"`
	LastChecked  *string `json:"lastChecked,omitempty"`
}

// GetByHost returns host_repositories for a host with repository details.
func (s *RepositoriesStore) GetByHost(ctx context.Context, hostID string) ([]HostRepositoryWithRepo, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetHostRepositoriesByHostID(ctx, hostID)
	if err != nil {
		return nil, err
	}

	out := make([]HostRepositoryWithRepo, len(rows))
	for i, r := range rows {
		var lastChecked *string
		if r.LastChecked.Valid {
			t := r.LastChecked.Time.Format("2006-01-02T15:04:05Z07:00")
			lastChecked = &t
		}
		prio := (*int)(nil)
		if r.RepoPriority != nil {
			p := int(*r.RepoPriority)
			prio = &p
		}
		out[i] = HostRepositoryWithRepo{
			ID:           r.ID,
			HostID:       r.HostID,
			RepositoryID: r.RepositoryID,
			IsEnabled:    r.IsEnabled,
			LastChecked:  lastChecked,
			Repositories: models.Repository{
				ID:           r.RepoID,
				Name:         r.RepoName,
				URL:          r.RepoUrl,
				Distribution: r.RepoDistribution,
				Components:   r.RepoComponents,
				RepoType:     r.RepoRepoType,
				IsActive:     r.RepoIsActive,
				IsSecure:     r.RepoIsSecure,
				Priority:     prio,
				Description:  r.RepoDescription,
				CreatedAt:    pgTime(r.RepoCreatedAt),
				UpdatedAt:    pgTime(r.RepoUpdatedAt),
			},
			Hosts: HostRef{
				ID:           r.HostId2,
				FriendlyName: r.HostFriendlyName,
			},
		}
	}
	return out, nil
}

// HostRepositoryWithRepo is host_repository with nested repository and host.
type HostRepositoryWithRepo struct {
	ID           string            `json:"id"`
	HostID       string            `json:"host_id"`
	RepositoryID string            `json:"repository_id"`
	IsEnabled    bool              `json:"is_enabled"`
	LastChecked  *string           `json:"last_checked,omitempty"`
	Repositories models.Repository `json:"repositories"`
	Hosts        HostRef           `json:"hosts"`
}

// HostRef is a minimal host reference.
type HostRef struct {
	ID           string `json:"id"`
	FriendlyName string `json:"friendly_name"`
}

// GetByID returns a repository by ID with host_repositories and hosts.
func (s *RepositoriesStore) GetByID(ctx context.Context, id string) (*RepositoryDetail, error) {
	d := s.db.DB(ctx)
	repo, err := d.Queries.GetRepositoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	hrs, err := s.getHostRepositoriesForRepo(ctx, id)
	if err != nil {
		return nil, err
	}
	return &RepositoryDetail{
		Repository:       dbRepositoryToModel(repo),
		HostRepositories: hrs,
	}, nil
}

// RepositoryDetail is repository with host_repositories.
type RepositoryDetail struct {
	models.Repository
	HostRepositories []HostRepositoryWithHost `json:"host_repositories"`
}

// HostRepositoryWithHost is host_repository with nested host.
type HostRepositoryWithHost struct {
	ID           string        `json:"id"`
	HostID       string        `json:"host_id"`
	RepositoryID string        `json:"repository_id"`
	IsEnabled    bool          `json:"is_enabled"`
	LastChecked  *string       `json:"last_checked,omitempty"`
	Hosts        HostDetailRef `json:"hosts"`
}

// HostDetailRef has host fields for repository detail.
type HostDetailRef struct {
	ID           string  `json:"id"`
	FriendlyName string  `json:"friendly_name"`
	Hostname     *string `json:"hostname"`
	IP           *string `json:"ip"`
	OSType       string  `json:"os_type"`
	OSVersion    string  `json:"os_version"`
	Status       string  `json:"status"`
	LastUpdate   string  `json:"last_update"`
	NeedsReboot  *bool   `json:"needs_reboot"`
}

func (s *RepositoriesStore) getHostRepositoriesForRepo(ctx context.Context, repoID string) ([]HostRepositoryWithHost, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetHostRepositoriesForRepo(ctx, repoID)
	if err != nil {
		return nil, err
	}

	out := make([]HostRepositoryWithHost, len(rows))
	for i, r := range rows {
		hr := HostRepositoryWithHost{
			ID:           r.ID,
			HostID:       r.HostID,
			RepositoryID: r.RepositoryID,
			IsEnabled:    r.IsEnabled,
			Hosts: HostDetailRef{
				ID:           r.HostId2,
				FriendlyName: r.HostFriendlyName,
				OSType:       r.HostOsType,
				OSVersion:    r.HostOsVersion,
				Status:       r.HostStatus,
				LastUpdate:   pgTime(r.HostLastUpdate).Format("2006-01-02T15:04:05Z07:00"),
			},
		}
		if r.LastChecked.Valid {
			t := r.LastChecked.Time.Format("2006-01-02T15:04:05Z07:00")
			hr.LastChecked = &t
		}
		hr.Hosts.Hostname = r.HostHostname
		hr.Hosts.IP = r.HostIp
		hr.Hosts.NeedsReboot = r.HostNeedsReboot
		out[i] = hr
	}
	return out, nil
}

// Update updates a repository.
func (s *RepositoriesStore) Update(ctx context.Context, id string, name *string, description *string, isActive *bool, priority *int) (*models.Repository, error) {
	d := s.db.DB(ctx)
	repo, err := d.Queries.GetRepositoryByID(ctx, id)
	if err != nil {
		return nil, err
	}

	n, desc, a, p := repo.Name, repo.Description, repo.IsActive, repo.Priority
	if name != nil {
		n = *name
	}
	if description != nil {
		desc = description
	}
	if isActive != nil {
		a = *isActive
	}
	if priority != nil {
		pi := int32(*priority)
		p = &pi
	}

	err = d.Queries.UpdateRepository(ctx, db.UpdateRepositoryParams{
		ID:          id,
		Name:        n,
		Description: desc,
		IsActive:    a,
		Priority:    p,
	})
	if err != nil {
		return nil, err
	}

	updated, err := d.Queries.GetRepositoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	out := dbRepositoryToModel(updated)
	return &out, nil
}

// ToggleHostRepository updates is_enabled for a host-repository pair.
func (s *RepositoriesStore) ToggleHostRepository(ctx context.Context, hostID, repositoryID string, isEnabled bool) (*HostRepositoryWithRepo, error) {
	d := s.db.DB(ctx)
	err := d.Queries.ToggleHostRepository(ctx, db.ToggleHostRepositoryParams{
		IsEnabled:    isEnabled,
		HostID:       hostID,
		RepositoryID: repositoryID,
	})
	if err != nil {
		return nil, err
	}
	hrs, err := s.GetByHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	for i := range hrs {
		if hrs[i].RepositoryID == repositoryID {
			return &hrs[i], nil
		}
	}
	return nil, nil
}

// GetStats returns repository statistics.
func (s *RepositoriesStore) GetStats(ctx context.Context) (*RepositoryStats, error) {
	d := s.db.DB(ctx)
	total, err := d.Queries.CountRepositories(ctx)
	if err != nil {
		return nil, err
	}
	active, err := d.Queries.CountActiveRepositories(ctx)
	if err != nil {
		return nil, err
	}
	secure, err := d.Queries.CountSecureRepositories(ctx)
	if err != nil {
		return nil, err
	}
	enabledHR, err := d.Queries.CountEnabledHostRepositories(ctx)
	if err != nil {
		return nil, err
	}
	securityPct := 0
	if total > 0 {
		securityPct = (int(secure) * 100) / int(total)
	}
	return &RepositoryStats{
		TotalRepositories:       int(total),
		ActiveRepositories:      int(active),
		SecureRepositories:      int(secure),
		EnabledHostRepositories: int(enabledHR),
		SecurityPercentage:      securityPct,
	}, nil
}

// RepositoryStats is the stats summary response.
type RepositoryStats struct {
	TotalRepositories       int `json:"totalRepositories"`
	ActiveRepositories      int `json:"activeRepositories"`
	SecureRepositories      int `json:"secureRepositories"`
	EnabledHostRepositories int `json:"enabledHostRepositories"`
	SecurityPercentage      int `json:"securityPercentage"`
}

// Delete deletes a repository.
func (s *RepositoriesStore) Delete(ctx context.Context, id string) (*DeletedRepository, error) {
	d := s.db.DB(ctx)
	repo, err := d.Queries.GetRepositoryForDelete(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	err = d.Queries.DeleteRepository(ctx, id)
	if err != nil {
		return nil, err
	}
	return &DeletedRepository{
		ID:        repo.ID,
		Name:      repo.Name,
		URL:       repo.Url,
		HostCount: int(repo.Count),
	}, nil
}

// DeletedRepository is returned after delete.
type DeletedRepository struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	HostCount int    `json:"hostCount"`
}

// CleanupOrphaned deletes repositories with no host_repositories.
func (s *RepositoriesStore) CleanupOrphaned(ctx context.Context) ([]OrphanedRepo, int, error) {
	d := s.db.DB(ctx)
	orphaned, err := d.Queries.ListOrphanedRepositories(ctx)
	if err != nil {
		return nil, 0, err
	}
	if len(orphaned) == 0 {
		return []OrphanedRepo{}, 0, nil
	}
	ids := make([]string, len(orphaned))
	for i, o := range orphaned {
		ids[i] = o.ID
	}
	err = d.Queries.DeleteRepositoriesByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]OrphanedRepo, len(orphaned))
	for i, o := range orphaned {
		out[i] = OrphanedRepo{ID: o.ID, Name: o.Name, URL: o.Url}
	}
	return out, len(out), nil
}

// OrphanedRepo is a deleted orphaned repository.
type OrphanedRepo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}
