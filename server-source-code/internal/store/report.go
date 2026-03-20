package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ExtractIPFromInterface extracts the first inet (IPv4) address from the named interface in networkInterfaces JSON.
// Returns empty string if not found or on parse error.
func ExtractIPFromInterface(networkInterfaces []byte, interfaceName string) string {
	if len(networkInterfaces) == 0 || interfaceName == "" {
		return ""
	}
	var ifaces []map[string]interface{}
	if err := json.Unmarshal(networkInterfaces, &ifaces); err != nil {
		return ""
	}
	for _, iface := range ifaces {
		name, _ := iface["name"].(string)
		if name != interfaceName {
			continue
		}
		addrs, ok := iface["addresses"].([]interface{})
		if !ok {
			return ""
		}
		for _, a := range addrs {
			addrMap, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			family, _ := addrMap["family"].(string)
			if family != "inet" {
				continue
			}
			addr, _ := addrMap["address"].(string)
			if addr != "" {
				return addr
			}
		}
		return ""
	}
	return ""
}

// ReportStore processes agent host reports (packages, system info).
type ReportStore struct {
	db database.DBProvider
}

// NewReportStore creates a new report store.
func NewReportStore(db database.DBProvider) *ReportStore {
	return &ReportStore{db: db}
}

// ReportPackage is a single package from the agent report.
type ReportPackage struct {
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Category         string  `json:"category"`
	CurrentVersion   string  `json:"currentVersion"`
	AvailableVersion *string `json:"availableVersion"`
	NeedsUpdate      bool    `json:"needsUpdate"`
	IsSecurityUpdate bool    `json:"isSecurityUpdate"`
	// WUA fields - only set for Category="Windows Update" entries
	WUAGuid           string   `json:"wuaGuid,omitempty"`
	WUAKb             string   `json:"wuaKb,omitempty"`
	WUASeverity       string   `json:"wuaSeverity,omitempty"`
	WUACategories     []string `json:"wuaCategories,omitempty"`
	WUASupportURL     string   `json:"wuaSupportUrl,omitempty"`
	WUARevisionNumber int32    `json:"wuaRevisionNumber,omitempty"`
}

// ReportRepository is a single repository from the agent report.
type ReportRepository struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Distribution string `json:"distribution"`
	Components   string `json:"components"`
	RepoType     string `json:"repoType"`
	IsEnabled    bool   `json:"isEnabled"`
	IsSecure     bool   `json:"isSecure"`
}

// ReportPayload is the agent's report payload (subset of fields we process).
type ReportPayload struct {
	Packages               []ReportPackage    `json:"packages"`
	Repositories           []ReportRepository `json:"repositories"`
	OSType                 string             `json:"osType"`
	OSVersion              string             `json:"osVersion"`
	Hostname               string             `json:"hostname"`
	IP                     string             `json:"ip"`
	Architecture           string             `json:"architecture"`
	AgentVersion           string             `json:"agentVersion"`
	MachineID              string             `json:"machineId"`
	KernelVersion          string             `json:"kernelVersion"`
	InstalledKernelVersion string             `json:"installedKernelVersion"`
	SELinuxStatus          string             `json:"selinuxStatus"`
	SystemUptime           string             `json:"systemUptime"`
	LoadAverage            []float64          `json:"loadAverage"`
	CPUModel               string             `json:"cpuModel"`
	CPUCores               int                `json:"cpuCores"`
	RAMInstalled           float64            `json:"ramInstalled"`
	SwapSize               float64            `json:"swapSize"`
	DiskDetails            json.RawMessage    `json:"diskDetails"`
	GatewayIP              string             `json:"gatewayIp"`
	DNSServers             []string           `json:"dnsServers"`
	NetworkInterfaces      json.RawMessage    `json:"networkInterfaces"`
	ExecutionTime          float64            `json:"executionTime"`
	NeedsReboot            bool               `json:"needsReboot"`
	RebootReason           string             `json:"rebootReason"`
	PackageManager         string             `json:"packageManager"`
}

// ProcessReportResult is the result of processing a host report.
type ProcessReportResult struct {
	PackagesProcessed int
	UpdatesAvailable  int
	SecurityUpdates   int
}

// ProcessReport processes an agent report: updates host, replaces packages, records history.
func (s *ReportStore) ProcessReport(ctx context.Context, hostID string, payload *ReportPayload) (*ProcessReportResult, error) {
	d := s.db.DB(ctx)
	securityCount := 0
	updatesCount := 0
	for _, p := range payload.Packages {
		if p.NeedsUpdate {
			updatesCount++
		}
		if p.IsSecurityUpdate {
			securityCount++
		}
	}

	// Marshal JSONB fields (DiskDetails and NetworkInterfaces are already JSON from agent)
	// Validate JSON before sending - invalid JSON causes PostgreSQL "transaction aborted" errors
	var diskDetails, dnsServers, networkInterfaces, loadAverage []byte
	if len(payload.DiskDetails) > 0 && json.Valid(payload.DiskDetails) {
		diskDetails = payload.DiskDetails
	}
	if len(payload.DNSServers) > 0 {
		if b, err := json.Marshal(payload.DNSServers); err == nil {
			dnsServers = b
		}
	}
	if len(payload.NetworkInterfaces) > 0 && json.Valid(payload.NetworkInterfaces) {
		networkInterfaces = payload.NetworkInterfaces
	}
	if len(payload.LoadAverage) > 0 {
		if b, err := json.Marshal(payload.LoadAverage); err == nil {
			loadAverage = b
		}
	}

	// Build optional params for host update
	params := db.UpdateHostFromReportParams{
		ID: hostID,
	}
	if payload.MachineID != "" {
		params.MachineID = &payload.MachineID
	}
	if payload.OSType != "" {
		params.OsType = &payload.OSType
	}
	if payload.OSVersion != "" {
		params.OsVersion = &payload.OSVersion
	}
	if payload.Hostname != "" {
		params.Hostname = &payload.Hostname
	}
	// If host has a primary interface set, derive IP from that interface in the report's network_interfaces.
	// Otherwise use payload.IP as before.
	if payload.IP != "" || len(networkInterfaces) > 0 {
		derivedIP := ""
		host, err := d.Queries.GetHostByID(ctx, hostID)
		if err == nil && host.PrimaryInterface != nil && *host.PrimaryInterface != "" && len(networkInterfaces) > 0 {
			derivedIP = ExtractIPFromInterface(networkInterfaces, *host.PrimaryInterface)
		}
		if derivedIP == "" {
			derivedIP = payload.IP
		}
		if derivedIP != "" {
			params.Ip = &derivedIP
		}
	}
	if payload.Architecture != "" {
		params.Architecture = &payload.Architecture
	}
	if payload.AgentVersion != "" {
		params.AgentVersion = &payload.AgentVersion
	}
	if payload.CPUModel != "" {
		params.CpuModel = &payload.CPUModel
	}
	if payload.CPUCores > 0 {
		c := int32(payload.CPUCores)
		params.CpuCores = &c
	}
	if payload.RAMInstalled > 0 {
		params.RamInstalled = &payload.RAMInstalled
	}
	if payload.SwapSize >= 0 {
		params.SwapSize = &payload.SwapSize
	}
	if len(diskDetails) > 0 {
		params.DiskDetails = diskDetails
	}
	if payload.GatewayIP != "" {
		params.GatewayIp = &payload.GatewayIP
	}
	if len(dnsServers) > 0 {
		params.DnsServers = dnsServers
	}
	if len(networkInterfaces) > 0 {
		params.NetworkInterfaces = networkInterfaces
	}
	if payload.KernelVersion != "" {
		params.KernelVersion = &payload.KernelVersion
	}
	if payload.InstalledKernelVersion != "" {
		params.InstalledKernelVersion = &payload.InstalledKernelVersion
	}
	if payload.SELinuxStatus != "" {
		params.SelinuxStatus = &payload.SELinuxStatus
	}
	if payload.SystemUptime != "" {
		params.SystemUptime = &payload.SystemUptime
	}
	if len(loadAverage) > 0 {
		params.LoadAverage = loadAverage
	}
	params.NeedsReboot = &payload.NeedsReboot
	if payload.RebootReason != "" {
		params.RebootReason = &payload.RebootReason
	}
	if payload.PackageManager != "" {
		params.PackageManager = &payload.PackageManager
	}

	tx, err := d.BeginLong(ctx)
	if err != nil {
		return nil, err
	}
	// Use uncancellable context for rollback so we always clean up even if request context is cancelled.
	// Otherwise the connection can be returned to the pool in an aborted state, causing 25P02 on subsequent requests.
	defer func() {
		rollbackCtx := context.WithoutCancel(ctx)
		_ = tx.Rollback(rollbackCtx)
	}()

	q := d.Queries.WithTx(tx)

	if err := q.UpdateHostFromReport(ctx, params); err != nil {
		return nil, fmt.Errorf("UpdateHostFromReport: %w", err)
	}

	if err := q.DeleteHostPackagesByHostID(ctx, hostID); err != nil {
		return nil, fmt.Errorf("DeleteHostPackagesByHostID: %w", err)
	}

	payloadSizeKb := 0.0
	if raw, err := json.Marshal(payload); err == nil {
		payloadSizeKb = float64(len(raw)) / 1024
	}

	for _, pkg := range payload.Packages {
		av := pkg.AvailableVersion
		desc := &pkg.Description
		if pkg.Description == "" {
			desc = nil
		}
		cat := (*string)(nil)
		if pkg.Category != "" {
			cat = &pkg.Category
		}

		pkgID, err := q.InsertPackage(ctx, db.InsertPackageParams{
			ID:            uuid.New().String(),
			Name:          pkg.Name,
			Description:   desc,
			Category:      cat,
			LatestVersion: av,
		})
		if err != nil {
			return nil, fmt.Errorf("InsertPackage %q: %w", pkg.Name, err)
		}

		if pkg.WUAGuid != "" {
			// Windows Update entry - persist WUA-specific metadata
			var wuaCats []byte
			if len(pkg.WUACategories) > 0 {
				wuaCats, _ = json.Marshal(pkg.WUACategories)
			}
			optStr := func(s string) *string {
				if s == "" {
					return nil
				}
				return &s
			}
			var revNum *int32
			if pkg.WUARevisionNumber != 0 {
				n := pkg.WUARevisionNumber
				revNum = &n
			}
			if err := q.InsertHostPackageWithWUA(ctx, db.InsertHostPackageWithWUAParams{
				ID:                uuid.New().String(),
				HostID:            hostID,
				PackageID:         pkgID,
				CurrentVersion:    pkg.CurrentVersion,
				AvailableVersion:  pkg.AvailableVersion,
				NeedsUpdate:       pkg.NeedsUpdate,
				IsSecurityUpdate:  pkg.IsSecurityUpdate,
				WuaGuid:           optStr(pkg.WUAGuid),
				WuaKb:             optStr(pkg.WUAKb),
				WuaSeverity:       optStr(pkg.WUASeverity),
				WuaCategories:     wuaCats,
				WuaDescription:    optStr(pkg.Description),
				WuaSupportUrl:     optStr(pkg.WUASupportURL),
				WuaRevisionNumber: revNum,
			}); err != nil {
				return nil, fmt.Errorf("InsertHostPackageWithWUA %q: %w", pkg.Name, err)
			}
		} else if err := q.InsertHostPackage(ctx, db.InsertHostPackageParams{
			ID:               uuid.New().String(),
			HostID:           hostID,
			PackageID:        pkgID,
			CurrentVersion:   pkg.CurrentVersion,
			AvailableVersion: pkg.AvailableVersion,
			NeedsUpdate:      pkg.NeedsUpdate,
			IsSecurityUpdate: pkg.IsSecurityUpdate,
		}); err != nil {
			return nil, fmt.Errorf("InsertHostPackage %q: %w", pkg.Name, err)
		}
	}

	// Process repositories if provided (same flow as Node backend)
	if len(payload.Repositories) > 0 {
		if err := q.DeleteHostRepositoriesByHostID(ctx, hostID); err != nil {
			return nil, fmt.Errorf("DeleteHostRepositoriesByHostID: %w", err)
		}

		// Deduplicate by url|distribution|components
		uniqueRepos := make(map[string]ReportRepository)
		for _, r := range payload.Repositories {
			key := r.URL + "|" + r.Distribution + "|" + r.Components
			if _, ok := uniqueRepos[key]; !ok {
				uniqueRepos[key] = r
			}
		}

		for _, repoData := range uniqueRepos {
			repoID := ""
			existing, err := q.GetRepositoryByURLDistComponents(ctx, db.GetRepositoryByURLDistComponentsParams{
				Url:          repoData.URL,
				Distribution: repoData.Distribution,
				Components:   repoData.Components,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					// Create new repository
					newID := uuid.New().String()
					desc := repoData.RepoType + " repository for " + repoData.Distribution
					_, err = q.InsertRepository(ctx, db.InsertRepositoryParams{
						ID:           newID,
						Name:         repoData.Name,
						Url:          repoData.URL,
						Distribution: repoData.Distribution,
						Components:   repoData.Components,
						RepoType:     repoData.RepoType,
						IsActive:     true,
						IsSecure:     repoData.IsSecure,
						Priority:     nil,
						Description:  &desc,
					})
					if err != nil {
						return nil, fmt.Errorf("InsertRepository %s: %w", repoData.URL, err)
					}
					repoID = newID
				} else {
					return nil, fmt.Errorf("GetRepositoryByURLDistComponents: %w", err)
				}
			} else {
				repoID = existing.ID
			}

			isEnabled := repoData.IsEnabled
			if err := q.InsertHostRepository(ctx, db.InsertHostRepositoryParams{
				ID:           uuid.New().String(),
				HostID:       hostID,
				RepositoryID: repoID,
				IsEnabled:    isEnabled,
			}); err != nil {
				return nil, fmt.Errorf("InsertHostRepository %s: %w", repoData.URL, err)
			}
		}
	}

	totalPkg := int32(len(payload.Packages))
	execTime := payload.ExecutionTime
	if err := q.InsertUpdateHistory(ctx, db.InsertUpdateHistoryParams{
		ID:            uuid.New().String(),
		HostID:        hostID,
		PackagesCount: int32(updatesCount),
		SecurityCount: int32(securityCount),
		TotalPackages: &totalPkg,
		PayloadSizeKb: &payloadSizeKb,
		ExecutionTime: &execTime,
		Status:        "success",
		ErrorMessage:  nil,
	}); err != nil {
		return nil, fmt.Errorf("InsertUpdateHistory: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &ProcessReportResult{
		PackagesProcessed: len(payload.Packages),
		UpdatesAvailable:  updatesCount,
		SecurityUpdates:   securityCount,
	}, nil
}
