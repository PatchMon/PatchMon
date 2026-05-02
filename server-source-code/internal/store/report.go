package store

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/google/uuid"
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
	SourceRepository string  `json:"sourceRepository,omitempty"`
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
	// Sections, when non-nil, restricts which top-level blocks the server
	// processes. Empty / absent means "full report" for backwards compatibility
	// with old agents.
	Sections []string `json:"sections,omitempty"`
	// Hashes carries the agent-computed canonical hashes for the sections it
	// included. Server validates them against its own canonicalisation and
	// stores them on the host row so the next ping can be hash-gated.
	Hashes ReportHashes `json:"hashes,omitempty"`
	// AgentExecutionMs is the agent-side data-collection wall-clock time in
	// milliseconds. Optional (older agents may omit it); when present it is
	// stored on the matching update_history row so the Agent Activity UI can
	// show "agent took N ms" alongside the server-processing duration.
	AgentExecutionMs *int `json:"agentExecutionMs,omitempty"`
}

// ReportHashes is the four "main report" canonical hashes the agent ships
// alongside a (possibly partial) /hosts/update payload. Docker and compliance
// hashes flow through their own endpoints — see the dedicated handlers.
type ReportHashes struct {
	PackagesHash   string `json:"packagesHash,omitempty"`
	ReposHash      string `json:"reposHash,omitempty"`
	InterfacesHash string `json:"interfacesHash,omitempty"`
	HostnameHash   string `json:"hostnameHash,omitempty"`
}

// ProcessReportResult is the result of processing a host report.
type ProcessReportResult struct {
	PackagesProcessed int
	UpdatesAvailable  int
	SecurityUpdates   int
}

// AgentActivityInsert is the thin set of fields the synchronous "side-channel"
// handlers (ping, docker integration, compliance scan) write to update_history
// for the Agent Activity feed. The full /hosts/update path goes through
// ProcessReport directly and uses its own InsertUpdateHistory call inside the
// transaction.
type AgentActivityInsert struct {
	HostID            string
	ReportType        string // "ping" | "docker" | "compliance"
	SectionsSent      []string
	SectionsUnchanged []string
	PayloadSizeKb     *float64
	ServerProcessing  *float64 // store unit is seconds (matches execution_time legacy)
	AgentExecutionMs  *int
	Status            string  // "success" | "error"
	ErrorMessage      *string // optional human-readable failure reason
}

// InsertActivityRow inserts a single non-/hosts/update agent comm row into
// update_history. Used by the ping handler, docker integration handler, and
// compliance handler so the Agent Activity feed shows every cycle (not just
// full/partial reports). Failure here is intentionally non-fatal at the call
// site — the activity row is a UI nicety, not part of the core data path.
func (s *ReportStore) InsertActivityRow(ctx context.Context, in AgentActivityInsert) error {
	d := s.db.DB(ctx)
	sectionsSent := in.SectionsSent
	if sectionsSent == nil {
		sectionsSent = []string{}
	}
	sectionsUnchanged := in.SectionsUnchanged
	if sectionsUnchanged == nil {
		sectionsUnchanged = []string{}
	}
	var agentExec *int32
	if in.AgentExecutionMs != nil {
		v := int32(*in.AgentExecutionMs)
		agentExec = &v
	}
	return d.Queries.InsertUpdateHistory(ctx, db.InsertUpdateHistoryParams{
		ID:                uuid.New().String(),
		HostID:            in.HostID,
		PackagesCount:     0,
		SecurityCount:     0,
		TotalPackages:     nil,
		PayloadSizeKb:     in.PayloadSizeKb,
		ExecutionTime:     in.ServerProcessing,
		Status:            in.Status,
		ErrorMessage:      in.ErrorMessage,
		ReportType:        in.ReportType,
		SectionsSent:      sectionsSent,
		SectionsUnchanged: sectionsUnchanged,
		AgentExecutionMs:  agentExec,
	})
}

// ReportSections selects which top-level blocks ProcessReport will write.
// Empty (zero value) means "everything" — used by old agents whose payloads
// have no Sections discriminator.
type ReportSections struct {
	Packages   bool
	Repos      bool
	Interfaces bool
	Hostname   bool
}

// FullReport is the all-true ReportSections used for backwards-compatible
// full-payload reports.
func FullReport() ReportSections {
	return ReportSections{
		Packages:   true,
		Repos:      true,
		Interfaces: true,
		Hostname:   true,
	}
}

// SectionsFromList parses a section-name list (closed set per
// models.AllSections) into a ReportSections struct. Unknown names are
// silently ignored — caller should pre-validate against AllSections.
func SectionsFromList(names []string) ReportSections {
	var s ReportSections
	for _, n := range names {
		switch n {
		case "packages":
			s.Packages = true
		case "repos":
			s.Repos = true
		case "interfaces":
			s.Interfaces = true
		case "hostname":
			s.Hostname = true
		}
	}
	return s
}

// mainSectionsFromSections returns the section identifiers (closed set) the
// agent shipped fresh data for. Used to populate update_history.sections_sent
// for the Agent Activity feed.
func mainSectionsFromSections(s ReportSections) []string {
	out := []string{}
	if s.Packages {
		out = append(out, "packages")
	}
	if s.Repos {
		out = append(out, "repos")
	}
	if s.Interfaces {
		out = append(out, "interfaces")
	}
	if s.Hostname {
		out = append(out, "hostname")
	}
	return out
}

// mainSectionsUnchanged returns the main-report section identifiers NOT in
// `sent`. Drives the "Skipped" chip rendering in the Agent Activity UI.
func mainSectionsUnchanged(sent []string) []string {
	all := []string{"packages", "repos", "interfaces", "hostname"}
	in := make(map[string]struct{}, len(sent))
	for _, s := range sent {
		in[s] = struct{}{}
	}
	out := make([]string, 0, len(all))
	for _, s := range all {
		if _, ok := in[s]; !ok {
			out = append(out, s)
		}
	}
	return out
}

// sortReportInputs sorts payload.Packages and payload.Repositories in-place
// using the same key order the SQL bulk-upsert paths use. Concurrent host
// reports against the shared `packages` and `repositories` tables therefore
// acquire row-locks in the same order, eliminating the 40P01 deadlocks
// observed in production at 100+ hosts.
//
// Mutation is in-place. Note that ProcessReport is wrapped in
// database.WithRetry at the handler layer, so on a 40P01 / 40001 retry this
// function may be called multiple times with the same payload pointer. That
// is safe and intentional: re-sorting an already-sorted slice is an O(n)
// no-op for slices.SortStableFunc, and dedupePackagesByName below is also
// idempotent. Callers therefore do not need to copy the payload between
// retries.
func sortReportInputs(payload *ReportPayload) {
	// Packages: sort by name (UNIQUE column on packages table).
	slices.SortStableFunc(payload.Packages, func(a, b ReportPackage) int {
		return cmp.Compare(a.Name, b.Name)
	})
	// Repositories: sort by the natural uniqueness tuple
	// (URL, Distribution, Components) — even though there is no DB-level
	// UNIQUE constraint, this is what GetRepositoryByURLDistComponents
	// looks up, so it is the relevant lock-acquisition key.
	slices.SortStableFunc(payload.Repositories, func(a, b ReportRepository) int {
		return cmp.Or(
			cmp.Compare(a.URL, b.URL),
			cmp.Compare(a.Distribution, b.Distribution),
			cmp.Compare(a.Components, b.Components),
		)
	})
}

// ProcessReport processes an agent report: updates host, replaces packages,
// records history. The sections argument restricts which top-level blocks
// the function actually writes — useful for hash-gated partial reports.
// Pass FullReport() for the legacy full-report behaviour.
//
// hashes carries the agent-supplied canonical hashes for sections it
// included; the handler is responsible for validating these against its own
// canonicalisation BEFORE calling ProcessReport. Hashes for sections not in
// `sections` are written as nil (preserving the existing column value via
// COALESCE in the UpdateHostFromReport SQL).
func (s *ReportStore) ProcessReport(ctx context.Context, hostID string, payload *ReportPayload, sections ReportSections, hashes ReportHashes) (*ProcessReportResult, error) {
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

	// Deterministic ordering eliminates cross-transaction lock-order
	// inversion that caused 40P01 deadlocks. See sortReportInputs.
	sortReportInputs(payload)

	// Postgres `ON CONFLICT ... DO UPDATE` cannot affect the same row twice
	// in one statement (cardinality_violation 21000). Agents in the wild
	// occasionally send duplicate package names — for example a malformed
	// dpkg/rpm cache or a race between two manager invocations on the host.
	// Deduplicate defensively before BulkUpsertPackages so a single noisy
	// agent cannot 21000 the entire report.
	//
	// Last-writer-wins matches the downstream COALESCE(EXCLUDED.x, packages.x)
	// semantics already applied per-column in BulkUpsertPackages: if a
	// duplicate carries a non-empty value the latter occurrence sets it.
	// The slice is already sorted by name, so we walk in that order and keep
	// the LAST occurrence of each name.
	if len(payload.Packages) > 1 {
		seen := make(map[string]int, len(payload.Packages))
		for i, p := range payload.Packages {
			seen[p.Name] = i // last wins
		}
		if len(seen) != len(payload.Packages) {
			deduped := make([]ReportPackage, 0, len(seen))
			for i, p := range payload.Packages {
				if seen[p.Name] == i {
					deduped = append(deduped, p)
				}
			}
			payload.Packages = deduped
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
	if sections.Hostname && payload.Hostname != "" {
		params.Hostname = &payload.Hostname
	}
	// If host has a primary interface set, derive IP from that interface in the report's network_interfaces.
	// Otherwise use payload.IP as before. Only runs when interfaces section is
	// being written; partial reports without interfaces leave hosts.ip alone.
	if sections.Interfaces && (payload.IP != "" || len(networkInterfaces) > 0) {
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
	if sections.Interfaces && len(dnsServers) > 0 {
		params.DnsServers = dnsServers
	}
	if sections.Interfaces && len(networkInterfaces) > 0 {
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
	if sections.Packages && payload.PackageManager != "" {
		params.PackageManager = &payload.PackageManager
	}

	// Hash columns: only set the ones for sections we're actually writing.
	// COALESCE on the SQL side preserves the existing value for sections not
	// in this report, so a partial report never clobbers a sibling section's
	// hash.
	if sections.Packages && hashes.PackagesHash != "" {
		h := hashes.PackagesHash
		params.PackagesHash = &h
	}
	if sections.Repos && hashes.ReposHash != "" {
		h := hashes.ReposHash
		params.ReposHash = &h
	}
	if sections.Interfaces && hashes.InterfacesHash != "" {
		h := hashes.InterfacesHash
		params.InterfacesHash = &h
	}
	if sections.Hostname && hashes.HostnameHash != "" {
		h := hashes.HostnameHash
		params.HostnameHash = &h
	}
	// Stamp last_full_report_at whenever any section is being written. Used
	// by the operator UI / staleness alerts to surface hosts whose last
	// content delivery is too old.
	if sections.Packages || sections.Repos || sections.Interfaces || sections.Hostname {
		params.LastFullReportAt = pgtime.Now()
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

	// Packages section: replace host_packages only when the agent claimed the
	// packages section. Without this guard a partial report (e.g. interfaces
	// only) would wipe the package list and reinsert nothing.
	if sections.Packages {
		if err := q.DeleteHostPackagesByHostID(ctx, hostID); err != nil {
			return nil, fmt.Errorf("DeleteHostPackagesByHostID: %w", err)
		}
	}

	payloadSizeKb := 0.0
	if raw, err := json.Marshal(payload); err == nil {
		payloadSizeKb = float64(len(raw)) / 1024
	}

	// Process repositories BEFORE packages so we can build lookup maps for source repo attribution.
	reposByName := make(map[string]string)        // repo.Name -> repo ID
	reposByURLDistComp := make(map[string]string) // "url|dist|comp" -> repo ID
	reposByComponent := make(map[string]string)   // components -> repo ID

	if sections.Repos && len(payload.Repositories) > 0 {
		if err := q.DeleteHostRepositoriesByHostID(ctx, hostID); err != nil {
			return nil, fmt.Errorf("DeleteHostRepositoriesByHostID: %w", err)
		}

		// Deduplicate by url|distribution|components.
		uniqueRepos := make(map[string]ReportRepository)
		for _, r := range payload.Repositories {
			key := r.URL + "|" + r.Distribution + "|" + r.Components
			if _, ok := uniqueRepos[key]; !ok {
				uniqueRepos[key] = r
			}
		}
		// Iterate in sorted-key order so concurrent host reports take the
		// SELECT-then-INSERT path against `repositories` in the same order.
		// Plain map iteration is randomised in Go and would re-introduce
		// the lock-order inversion we just fixed for `packages`.
		repoKeys := make([]string, 0, len(uniqueRepos))
		for k := range uniqueRepos {
			repoKeys = append(repoKeys, k)
		}
		slices.Sort(repoKeys)

		for _, repoKey := range repoKeys {
			repoData := uniqueRepos[repoKey]
			// UpsertRepository (migration 000040 added the
			// (url, distribution, components) UNIQUE constraint that makes
			// this a true upsert) replaces the previous
			// GetRepositoryByURLDistComponents + InsertRepository
			// SELECT-then-INSERT. That two-step pattern had a TOCTOU race —
			// two concurrent reports could both see "no row" and both INSERT,
			// producing duplicate (url, distribution, components) rows — and
			// also cost an extra network round-trip per repository.
			desc := repoData.RepoType + " repository for " + repoData.Distribution
			repoID, err := q.UpsertRepository(ctx, db.UpsertRepositoryParams{
				ID:           uuid.New().String(),
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
				return nil, fmt.Errorf("UpsertRepository %s: %w", repoData.URL, err)
			}

			// Build lookup maps for package -> repo resolution
			reposByName[repoData.Name] = repoID
			key := repoData.URL + "|" + repoData.Distribution + "|" + repoData.Components
			reposByURLDistComp[key] = repoID
			// Also index by url|distribution with each individual component for APT.
			// APT sources.list stores multi-component entries like "main restricted universe multiverse"
			// but apt-cache policy returns a single component per source line.
			for _, comp := range strings.Fields(repoData.Components) {
				singleKey := repoData.URL + "|" + repoData.Distribution + "|" + comp
				if _, exists := reposByURLDistComp[singleKey]; !exists {
					reposByURLDistComp[singleKey] = repoID
				}
			}
			if repoData.Components != "" {
				reposByComponent[repoData.Components] = repoID
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

	// Process packages with source repo attribution.
	//
	// Two single round-trips replace what used to be 2*N per-row INSERTs:
	//   1. BulkUpsertPackages: one INSERT...SELECT...ON CONFLICT against the
	//      shared `packages` table, returning (id, name) for each input row.
	//      Names are pre-sorted (above) so concurrent host reports acquire
	//      row locks in the same order, eliminating 40P01 deadlocks.
	//   2. BulkInsertHostPackages: one INSERT...SELECT into `host_packages`.
	//      DeleteHostPackagesByHostID has already cleared this host's rows,
	//      so no ON CONFLICT path is needed.
	if sections.Packages && len(payload.Packages) > 0 {
		pkgPayload, err := buildPackageUpsertPayload(payload.Packages)
		if err != nil {
			return nil, fmt.Errorf("build package upsert payload: %w", err)
		}
		upserted, err := q.BulkUpsertPackages(ctx, pkgPayload)
		if err != nil {
			return nil, fmt.Errorf("BulkUpsertPackages: %w", err)
		}
		// Map name -> package ID for host_packages assembly. With the
		// no-op-skip WHERE on BulkUpsertPackages, RETURNING only emits rows
		// that actually fired DO UPDATE; the UNION ALL fallback returns
		// (id, name) for rows whose values were unchanged. Either way we
		// get exactly one (id, name) pair per distinct input name.
		// payload.Packages is deduped above so len(upserted) == len(packages).
		nameToID := make(map[string]string, len(upserted))
		for _, row := range upserted {
			nameToID[row.Name] = row.ID
		}
		// buildHostPackagesPayload below is the real correctness check: it
		// errors if any payload.Packages name is missing from nameToID.

		hpPayload, err := buildHostPackagesPayload(hostID, payload.Packages, nameToID,
			reposByName, reposByURLDistComp, reposByComponent)
		if err != nil {
			return nil, fmt.Errorf("build host_packages payload: %w", err)
		}
		if err := q.BulkInsertHostPackages(ctx, hpPayload); err != nil {
			return nil, fmt.Errorf("BulkInsertHostPackages: %w", err)
		}
	}

	totalPkg := int32(len(payload.Packages))
	execTime := payload.ExecutionTime
	// Discriminate full vs partial for the Agent Activity feed: a payload
	// without a Sections list is a legacy/full report, anything that arrived
	// with a Sections list is treated as a partial even if every section is
	// claimed (the agent voluntarily told us it's a /hosts/update follow-up
	// to a hash-gated ping).
	reportType := "full"
	if len(payload.Sections) > 0 {
		reportType = "partial"
	}
	sectionsSent := mainSectionsFromSections(sections)
	sectionsUnchanged := mainSectionsUnchanged(sectionsSent)
	var agentExecMs *int32
	if payload.AgentExecutionMs != nil {
		v := int32(*payload.AgentExecutionMs)
		agentExecMs = &v
	}
	// Retry safety: if WithRetry re-runs ProcessReport after a 40P01/40001,
	// the BeginLong transaction is rolled back so this update_history row
	// never commits. Each attempt allocates a fresh id and Postgres NOW()
	// gives a fresh timestamp — exactly one history row will land per
	// successful commit. That re-allocation per attempt is intentional.
	if err := q.InsertUpdateHistory(ctx, db.InsertUpdateHistoryParams{
		ID:                uuid.New().String(),
		HostID:            hostID,
		PackagesCount:     int32(updatesCount),
		SecurityCount:     int32(securityCount),
		TotalPackages:     &totalPkg,
		PayloadSizeKb:     &payloadSizeKb,
		ExecutionTime:     &execTime,
		Status:            "success",
		ErrorMessage:      nil,
		ReportType:        reportType,
		SectionsSent:      sectionsSent,
		SectionsUnchanged: sectionsUnchanged,
		AgentExecutionMs:  agentExecMs,
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

// resolveSourceRepoID matches an agent's sourceRepository string to a repository ID
// using the lookup maps built during repo processing.
func resolveSourceRepoID(sourceRepo string, reposByName, reposByURLDistComp, reposByComponent map[string]string) *string {
	if sourceRepo == "" || sourceRepo == "local" || sourceRepo == "unknown" || sourceRepo == "foreign" || sourceRepo == "@System" {
		return nil
	}

	// Try exact name match first (DNF: "baseos", Pacman: "core", FreeBSD: "FreeBSD")
	if id, ok := reposByName[sourceRepo]; ok {
		return &id
	}

	// Try APT format: "http://deb.debian.org/debian bookworm/main"
	// Parse into URL + suite/component, reconstruct key
	if strings.Contains(sourceRepo, " ") {
		parts := strings.SplitN(sourceRepo, " ", 2)
		if len(parts) == 2 {
			url := parts[0]
			suiteComp := parts[1]
			// suiteComp format: "bookworm/main" or "bookworm-security/main"
			scParts := strings.SplitN(suiteComp, "/", 2)
			if len(scParts) == 2 {
				key := url + "|" + scParts[0] + "|" + scParts[1]
				if id, ok := reposByURLDistComp[key]; ok {
					return &id
				}
			}
		}
	}

	// Try component match (APK: "main", "community")
	if id, ok := reposByComponent[sourceRepo]; ok {
		return &id
	}

	return nil
}

// packageUpsertRow is the shape consumed by BulkUpsertPackages.
// Fields use omitempty so the JSON encoder emits `null` (not "") when the
// agent omits a value, which jsonb_to_recordset maps to SQL NULL — letting
// the COALESCE in the ON CONFLICT clause preserve the existing column value.
type packageUpsertRow struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   *string `json:"description,omitempty"`
	Category      *string `json:"category,omitempty"`
	LatestVersion *string `json:"latest_version,omitempty"`
}

// buildPackageUpsertPayload encodes the package list as a jsonb array suitable
// for the BulkUpsertPackages query. Caller must have already sorted Packages
// by Name; this function only assembles the JSON.
func buildPackageUpsertPayload(packages []ReportPackage) ([]byte, error) {
	rows := make([]packageUpsertRow, 0, len(packages))
	for i := range packages {
		p := &packages[i]
		row := packageUpsertRow{
			ID:   uuid.New().String(),
			Name: p.Name,
		}
		if p.Description != "" {
			d := p.Description
			row.Description = &d
		}
		if p.Category != "" {
			c := p.Category
			row.Category = &c
		}
		if p.AvailableVersion != nil && *p.AvailableVersion != "" {
			row.LatestVersion = p.AvailableVersion
		}
		rows = append(rows, row)
	}
	return json.Marshal(rows)
}

// hostPackageRow is the shape consumed by BulkInsertHostPackages.
//
// All nullable text columns use empty-string sentinels rather than omitempty
// because the SQL side reads via `elem->>'field'` (text accessor), which
// returns SQL NULL for missing keys but "" for present-but-empty strings.
// We always emit the key so the SELECT projection is uniform across rows;
// NULLIF in the SQL collapses "" to NULL.
//
// wua_categories is a JSON array (never a string) and uses omitempty so
// non-Windows rows omit it entirely; the SQL side uses jsonb_typeof to
// distinguish.
type hostPackageRow struct {
	ID                 string   `json:"id"`
	HostID             string   `json:"host_id"`
	PackageID          string   `json:"package_id"`
	CurrentVersion     string   `json:"current_version"`
	AvailableVersion   string   `json:"available_version"`
	NeedsUpdate        bool     `json:"needs_update"`
	IsSecurityUpdate   bool     `json:"is_security_update"`
	SourceRepositoryID string   `json:"source_repository_id"`
	WUAGuid            string   `json:"wua_guid"`
	WUAKb              string   `json:"wua_kb"`
	WUASeverity        string   `json:"wua_severity"`
	WUACategories      []string `json:"wua_categories,omitempty"`
	WUADescription     string   `json:"wua_description"`
	WUASupportURL      string   `json:"wua_support_url"`
	WUARevisionNumber  int32    `json:"wua_revision_number"`
}

// buildHostPackagesPayload encodes host_packages rows as a jsonb array suitable
// for BulkInsertHostPackages. Rows are emitted in package_id order (defensive:
// since payload.Packages is already sorted by name and nameToID provides
// a stable mapping, the resulting order will be name-stable; the SQL ORDER BY
// package_id then enforces the on-disk insert order).
func buildHostPackagesPayload(
	hostID string,
	packages []ReportPackage,
	nameToID map[string]string,
	reposByName, reposByURLDistComp, reposByComponent map[string]string,
) ([]byte, error) {
	rows := make([]hostPackageRow, 0, len(packages))
	for i := range packages {
		p := &packages[i]
		pkgID, ok := nameToID[p.Name]
		if !ok {
			return nil, fmt.Errorf("no upserted ID for package %q", p.Name)
		}

		availableVersion := ""
		if p.AvailableVersion != nil {
			availableVersion = *p.AvailableVersion
		}
		sourceRepoID := ""
		if id := resolveSourceRepoID(p.SourceRepository, reposByName, reposByURLDistComp, reposByComponent); id != nil {
			sourceRepoID = *id
		}

		row := hostPackageRow{
			ID:                 uuid.New().String(),
			HostID:             hostID,
			PackageID:          pkgID,
			CurrentVersion:     p.CurrentVersion,
			AvailableVersion:   availableVersion,
			NeedsUpdate:        p.NeedsUpdate,
			IsSecurityUpdate:   p.IsSecurityUpdate,
			SourceRepositoryID: sourceRepoID,
		}

		// Windows Update entries carry WUA metadata. The agent flags them by
		// setting WUAGuid; we keep the same Go-side detection rule the
		// legacy InsertHostPackageWithWUA path used.
		if p.WUAGuid != "" {
			row.WUAGuid = p.WUAGuid
			row.WUAKb = p.WUAKb
			row.WUASeverity = p.WUASeverity
			row.WUADescription = p.Description // matches legacy: WuaDescription = pkg.Description
			row.WUASupportURL = p.WUASupportURL
			row.WUARevisionNumber = p.WUARevisionNumber
			if len(p.WUACategories) > 0 {
				row.WUACategories = p.WUACategories
			}
		}
		rows = append(rows, row)
	}
	return json.Marshal(rows)
}
