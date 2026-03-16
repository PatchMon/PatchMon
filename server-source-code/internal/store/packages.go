package store

import (
	"context"
	"errors"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/safeconv"
	"github.com/jackc/pgx/v5"
)

// PackagesStore provides package access.
type PackagesStore struct {
	db database.DBProvider
}

// NewPackagesStore creates a new packages store.
func NewPackagesStore(db database.DBProvider) *PackagesStore {
	return &PackagesStore{db: db}
}

// ListParams holds query params for List.
type ListParams struct {
	Page             int
	Limit            int
	Search           string
	Category         string
	NeedsUpdate      string
	IsSecurityUpdate string
	Host             string
}

// PackageWithStats is a package with host counts and stats.
type PackageWithStats struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       *string          `json:"description"`
	Category          *string          `json:"category"`
	LatestVersion     *string          `json:"latest_version"`
	CreatedAt         time.Time        `json:"created_at"`
	PackageHostsCount int              `json:"packageHostsCount"`
	PackageHosts      []PackageHostRef `json:"packageHosts"`
	Stats             PackageStats     `json:"stats"`
}

// PackageHostRef is a host reference in package list.
type PackageHostRef struct {
	HostID           string  `json:"hostId"`
	FriendlyName     string  `json:"friendlyName"`
	OSType           string  `json:"osType"`
	CurrentVersion   string  `json:"currentVersion"`
	AvailableVersion *string `json:"availableVersion"`
	NeedsUpdate      bool    `json:"needsUpdate"`
	IsSecurityUpdate bool    `json:"isSecurityUpdate"`
}

// PackageStats holds package statistics.
type PackageStats struct {
	TotalInstalls   int `json:"totalInstalls"`
	UpdatesNeeded   int `json:"updatesNeeded"`
	SecurityUpdates int `json:"securityUpdates"`
}

// List returns packages with stats, supporting filters.
func (s *PackagesStore) List(ctx context.Context, p ListParams) ([]PackageWithStats, int, error) {
	d := s.db.DB(ctx)
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 10000 {
		p.Limit = 10000
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	offset := (p.Page - 1) * p.Limit

	listArg := db.ListPackagesParams{
		Limit:  safeconv.ClampToInt32(p.Limit),
		Offset: safeconv.ClampToInt32(offset),
	}
	if p.Search != "" {
		listArg.Search = &p.Search
	}
	if p.Category != "" {
		listArg.Category = &p.Category
	}
	if p.Host != "" {
		listArg.HostID = &p.Host
	}
	if p.NeedsUpdate == "true" {
		listArg.NeedsUpdate = &p.NeedsUpdate
	}
	if p.IsSecurityUpdate == "true" {
		listArg.IsSecurityUpdate = &p.IsSecurityUpdate
	}

	countArg := db.CountPackagesParams{}
	if p.Search != "" {
		countArg.Search = &p.Search
	}
	if p.Category != "" {
		countArg.Category = &p.Category
	}
	if p.Host != "" {
		countArg.HostID = &p.Host
	}
	if p.NeedsUpdate == "true" {
		countArg.NeedsUpdate = &p.NeedsUpdate
	}
	if p.IsSecurityUpdate == "true" {
		countArg.IsSecurityUpdate = &p.IsSecurityUpdate
	}

	total, err := d.Queries.CountPackages(ctx, countArg)
	if err != nil {
		return nil, 0, err
	}

	pkgs, err := d.Queries.ListPackages(ctx, listArg)
	if err != nil {
		return nil, 0, err
	}

	if len(pkgs) == 0 {
		return []PackageWithStats{}, int(total), nil
	}

	ids := make([]string, len(pkgs))
	for i, p := range pkgs {
		ids[i] = p.ID
	}

	statsArg := db.GetHostPackageStatsByPackageIDsParams{Column1: ids}
	if p.Host != "" {
		statsArg.HostID = &p.Host
	}
	updatesArg := db.GetUpdatesCountByPackageIDsParams{Column1: ids}
	if p.Host != "" {
		updatesArg.HostID = &p.Host
	}
	securityArg := db.GetSecurityCountByPackageIDsParams{Column1: ids}
	if p.Host != "" {
		securityArg.HostID = &p.Host
	}
	hostRefsArg := db.GetHostRefsForPackageIDsParams{Column1: ids}
	if p.Host != "" {
		hostRefsArg.HostID = &p.Host
	}

	totalRows, _ := d.Queries.GetHostPackageStatsByPackageIDs(ctx, statsArg)
	updatesRows, _ := d.Queries.GetUpdatesCountByPackageIDs(ctx, updatesArg)
	securityRows, _ := d.Queries.GetSecurityCountByPackageIDs(ctx, securityArg)
	hostRefs, _ := d.Queries.GetHostRefsForPackageIDs(ctx, hostRefsArg)

	totalMap := make(map[string]int)
	for _, r := range totalRows {
		totalMap[r.PackageID] = int(r.Cnt)
	}
	updatesMap := make(map[string]int)
	for _, r := range updatesRows {
		updatesMap[r.PackageID] = int(r.Cnt)
	}
	securityMap := make(map[string]int)
	for _, r := range securityRows {
		securityMap[r.PackageID] = int(r.Cnt)
	}

	hostsByPkg := make(map[string][]PackageHostRef)
	for _, r := range hostRefs {
		ref := PackageHostRef{
			HostID:           r.HostID,
			FriendlyName:     r.FriendlyName,
			OSType:           r.OsType,
			CurrentVersion:   r.CurrentVersion,
			AvailableVersion: r.AvailableVersion,
			NeedsUpdate:      r.NeedsUpdate,
			IsSecurityUpdate: r.IsSecurityUpdate,
		}
		hostsByPkg[r.PackageID] = append(hostsByPkg[r.PackageID], ref)
		if len(hostsByPkg[r.PackageID]) >= 10 {
			hostsByPkg[r.PackageID] = hostsByPkg[r.PackageID][:10]
		}
	}

	out := make([]PackageWithStats, len(pkgs))
	for i, p := range pkgs {
		totalInstalls := totalMap[p.ID]
		updatesNeeded := updatesMap[p.ID]
		securityUpdates := securityMap[p.ID]
		out[i] = PackageWithStats{
			ID:                p.ID,
			Name:              p.Name,
			Description:       p.Description,
			Category:          p.Category,
			LatestVersion:     p.LatestVersion,
			CreatedAt:         pgTime(p.CreatedAt),
			PackageHostsCount: totalInstalls,
			PackageHosts:      hostsByPkg[p.ID],
			Stats: PackageStats{
				TotalInstalls:   totalInstalls,
				UpdatesNeeded:   updatesNeeded,
				SecurityUpdates: securityUpdates,
			},
		}
	}
	return out, int(total), nil
}

// GetByID returns a package by ID with host_packages, stats, and distributions.
// Supports lookup by package ID (UUID) or by package name for links from patch runs.
func (s *PackagesStore) GetByID(ctx context.Context, id string) (*PackageDetail, error) {
	d := s.db.DB(ctx)
	pkg, err := d.Queries.GetPackageByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Fallback: try lookup by name (e.g. /packages/bindutils from patch run links)
			pkgByName, errByName := d.Queries.GetPackageByName(ctx, id)
			if errByName != nil {
				return nil, nil
			}
			pkg = pkgByName
			id = pkg.ID
		} else {
			return nil, err
		}
	} else {
		id = pkg.ID
	}

	rows, err := d.Queries.GetHostPackagesWithHostsByPackageID(ctx, id)
	if err != nil {
		return nil, err
	}

	hostPackages := make([]HostPackageWithHost, len(rows))
	for i, r := range rows {
		hostPackages[i] = HostPackageWithHost{
			ID:               r.ID,
			HostID:           r.HostID,
			PackageID:        id,
			CurrentVersion:   r.CurrentVersion,
			AvailableVersion: r.AvailableVersion,
			NeedsUpdate:      r.NeedsUpdate,
			IsSecurityUpdate: r.IsSecurityUpdate,
			LastChecked:      pgTime(r.LastChecked),
			Hosts: HostPackageHostRef{
				ID:           r.HostID,
				FriendlyName: r.HostFriendlyName,
				Hostname:     r.HostHostname,
				IP:           r.HostIp,
				OSType:       r.HostOsType,
				OSVersion:    r.HostOsVersion,
				LastUpdate:   pgTime(r.HostLastUpdate),
				NeedsReboot:  r.HostNeedsReboot,
			},
		}
	}

	totalInstalls := len(rows)
	updatesNeeded, securityUpdates, upToDate := 0, 0, 0
	versionDist := make(map[string]int)
	osDist := make(map[string]int)
	for _, r := range rows {
		if r.NeedsUpdate {
			updatesNeeded++
			if r.IsSecurityUpdate {
				securityUpdates++
			}
		} else {
			upToDate++
		}
		versionDist[r.CurrentVersion]++
		osDist[r.HostOsType]++
	}

	versions := make([]VersionCount, 0, len(versionDist))
	for v, c := range versionDist {
		versions = append(versions, VersionCount{Version: v, Count: c})
	}
	osTypes := make([]OSTypeCount, 0, len(osDist))
	for os, c := range osDist {
		osTypes = append(osTypes, OSTypeCount{OSType: os, Count: c})
	}

	return &PackageDetail{
		Package:      dbPackageToModel(pkg),
		HostPackages: hostPackages,
		Stats: PackageDetailStats{
			TotalInstalls:   totalInstalls,
			UpdatesNeeded:   updatesNeeded,
			SecurityUpdates: securityUpdates,
			UpToDate:        upToDate,
		},
		Distributions: PackageDistributions{
			Versions: versions,
			OSTypes:  osTypes,
		},
	}, nil
}

// PackageDetail is package with host_packages, stats, distributions.
type PackageDetail struct {
	models.Package
	HostPackages  []HostPackageWithHost `json:"host_packages"`
	Stats         PackageDetailStats    `json:"stats"`
	Distributions PackageDistributions  `json:"distributions"`
}

// HostPackageWithHost is host_package with nested host.
type HostPackageWithHost struct {
	ID               string             `json:"id"`
	HostID           string             `json:"host_id"`
	PackageID        string             `json:"package_id"`
	CurrentVersion   string             `json:"current_version"`
	AvailableVersion *string            `json:"available_version"`
	NeedsUpdate      bool               `json:"needs_update"`
	IsSecurityUpdate bool               `json:"is_security_update"`
	LastChecked      time.Time          `json:"last_checked"`
	Hosts            HostPackageHostRef `json:"hosts"`
}

// HostPackageHostRef has host fields for package detail.
type HostPackageHostRef struct {
	ID           string    `json:"id"`
	FriendlyName string    `json:"friendly_name"`
	Hostname     *string   `json:"hostname"`
	IP           *string   `json:"ip"`
	OSType       string    `json:"os_type"`
	OSVersion    string    `json:"os_version"`
	LastUpdate   time.Time `json:"last_update"`
	NeedsReboot  *bool     `json:"needs_reboot"`
}

// PackageDetailStats for package detail.
type PackageDetailStats struct {
	TotalInstalls   int `json:"totalInstalls"`
	UpdatesNeeded   int `json:"updatesNeeded"`
	SecurityUpdates int `json:"securityUpdates"`
	UpToDate        int `json:"upToDate"`
}

// PackageDistributions for package detail.
type PackageDistributions struct {
	Versions []VersionCount `json:"versions"`
	OSTypes  []OSTypeCount  `json:"osTypes"`
}

// VersionCount is version with count.
type VersionCount struct {
	Version string `json:"version"`
	Count   int    `json:"count"`
}

// OSTypeCount is os type with count.
type OSTypeCount struct {
	OSType string `json:"osType"`
	Count  int    `json:"count"`
}

// GetHostsParams for GetHosts.
type GetHostsParams struct {
	Page      int
	Limit     int
	Search    string
	SortBy    string
	SortOrder string
}

// PackageHostEntry is a host where package is installed.
type PackageHostEntry struct {
	HostID           string    `json:"hostId"`
	FriendlyName     string    `json:"friendlyName"`
	Hostname         *string   `json:"hostname"`
	OSType           string    `json:"osType"`
	OSVersion        string    `json:"osVersion"`
	LastUpdate       time.Time `json:"lastUpdate"`
	CurrentVersion   string    `json:"currentVersion"`
	AvailableVersion *string   `json:"availableVersion"`
	NeedsUpdate      bool      `json:"needsUpdate"`
	IsSecurityUpdate bool      `json:"isSecurityUpdate"`
	LastChecked      time.Time `json:"lastChecked"`
	NeedsReboot      *bool     `json:"needsReboot"`
}

// GetHosts returns hosts where a package is installed.
// packageIDOrName can be either the package UUID (id) or the package name (e.g. "cpp-13").
func (s *PackagesStore) GetHosts(ctx context.Context, packageIDOrName string, p GetHostsParams) ([]PackageHostEntry, int, error) {
	d := s.db.DB(ctx)
	// Resolve package ID: backend stores UUID in host_packages.package_id, but callers may pass package name
	resolvedID := packageIDOrName
	if pkg, err := d.Queries.GetPackageByID(ctx, packageIDOrName); err == nil {
		resolvedID = pkg.ID
	} else if pkg, err := d.Queries.GetPackageByName(ctx, packageIDOrName); err == nil {
		resolvedID = pkg.ID
	}

	if p.Limit <= 0 {
		p.Limit = 25
	}
	if p.Page <= 0 {
		p.Page = 1
	}
	offset := (p.Page - 1) * p.Limit

	countArg := db.CountHostsForPackageParams{PackageID: resolvedID}
	if p.Search != "" {
		countArg.Search = &p.Search
	}
	total, err := d.Queries.CountHostsForPackage(ctx, countArg)
	if err != nil {
		return nil, 0, err
	}

	listArg := db.ListHostsForPackageParams{
		PackageID: resolvedID,
		Limit:     safeconv.ClampToInt32(p.Limit),
		Offset:    safeconv.ClampToInt32(offset),
	}
	if p.Search != "" {
		listArg.Search = &p.Search
	}

	rows, err := d.Queries.ListHostsForPackage(ctx, listArg)
	if err != nil {
		return nil, 0, err
	}

	out := make([]PackageHostEntry, len(rows))
	for i, r := range rows {
		out[i] = PackageHostEntry{
			HostID:           r.ID,
			FriendlyName:     r.FriendlyName,
			Hostname:         r.Hostname,
			OSType:           r.OsType,
			OSVersion:        r.OsVersion,
			LastUpdate:       pgTime(r.LastUpdate),
			CurrentVersion:   r.CurrentVersion,
			AvailableVersion: r.AvailableVersion,
			NeedsUpdate:      r.NeedsUpdate,
			IsSecurityUpdate: r.IsSecurityUpdate,
			LastChecked:      pgTime(r.LastChecked),
			NeedsReboot:      r.NeedsReboot,
		}
	}
	return out, int(total), nil
}

// PackageActivityEntry is a completed patch run where the package was upgraded.
type PackageActivityEntry struct {
	RunID            string    `json:"run_id"`
	HostID           string    `json:"host_id"`
	HostFriendlyName string    `json:"host_friendly_name"`
	CompletedAt      time.Time `json:"completed_at"`
}

// GetActivity returns completed patch runs where the package was upgraded.
// packageIDOrName can be package ID (UUID) or package name.
func (s *PackagesStore) GetActivity(ctx context.Context, packageIDOrName string, limit, offset int) ([]PackageActivityEntry, error) {
	d := s.db.DB(ctx)
	// Resolve package name
	var pkgName string
	if pkg, err := d.Queries.GetPackageByID(ctx, packageIDOrName); err == nil {
		pkgName = pkg.Name
	} else if pkg, err := d.Queries.GetPackageByName(ctx, packageIDOrName); err == nil {
		pkgName = pkg.Name
	} else {
		return nil, nil
	}

	rows, err := d.Queries.ListPatchRunsByPackage(ctx, db.ListPatchRunsByPackageParams{
		PackageName: &pkgName,
		LimitArg:    safeconv.ClampToInt32(limit),
		OffsetArg:   safeconv.ClampToInt32(offset),
	})
	if err != nil {
		return nil, err
	}

	out := make([]PackageActivityEntry, len(rows))
	for i, r := range rows {
		hostName := ""
		if r.HostFriendlyName != nil {
			hostName = *r.HostFriendlyName
		}
		if hostName == "" && r.HostHostname != nil {
			hostName = *r.HostHostname
		}
		out[i] = PackageActivityEntry{
			RunID:            r.ID,
			HostID:           r.HostID,
			HostFriendlyName: hostName,
			CompletedAt:      pgTime(r.CompletedAt),
		}
	}
	return out, nil
}

// GetCategories returns distinct package categories.
func (s *PackagesStore) GetCategories(ctx context.Context) ([]string, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r != nil && *r != "" {
			out = append(out, *r)
		}
	}
	return out, nil
}

// ListNeedingUpdates returns packages that need updates with affected hosts (for dashboard).
func (s *PackagesStore) ListNeedingUpdates(ctx context.Context) ([]map[string]interface{}, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListNeedingUpdates(ctx)
	if err != nil {
		return nil, err
	}

	pkgMap := make(map[string]map[string]interface{})
	for _, r := range rows {
		if pkgMap[r.PkgID] == nil {
			pkgMap[r.PkgID] = map[string]interface{}{
				"id": r.PkgID, "name": r.PkgName, "description": r.Description, "category": r.Category,
				"latestVersion": r.LatestVersion, "affectedHostsCount": 0, "isSecurityUpdate": r.IsSecurityUpdate,
				"affectedHosts": []map[string]interface{}{},
			}
		}
		p := pkgMap[r.PkgID]
		p["affectedHostsCount"] = p["affectedHostsCount"].(int) + 1
		if r.IsSecurityUpdate {
			p["isSecurityUpdate"] = true
		}
		hosts := p["affectedHosts"].([]map[string]interface{})
		avail := ""
		if r.AvailableVersion != nil {
			avail = *r.AvailableVersion
		}
		hosts = append(hosts, map[string]interface{}{
			"hostId": r.HostID, "friendlyName": r.FriendlyName, "osType": r.OsType,
			"currentVersion": r.CurrentVersion, "availableVersion": avail, "isSecurityUpdate": r.IsSecurityUpdate,
		})
		p["affectedHosts"] = hosts
	}
	result := make([]map[string]interface{}, 0, len(pkgMap))
	for _, p := range pkgMap {
		result = append(result, p)
	}
	return result, nil
}
