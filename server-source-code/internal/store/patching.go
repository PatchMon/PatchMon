package store

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/PatchMon/PatchMon/server-source-code/internal/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const freeBSDBasePackageName = "freebsd-base"

var freeBSDPkgSummaryLinePattern = regexp.MustCompile(`^\s+(\S+):\s+`)
var freeBSDPkgActionLinePattern = regexp.MustCompile(`^\[\d+/\d+\]\s+(?:Installing|Upgrading|Reinstalling)\s+(\S+)`)

// isFreeBSD reports whether the host OS type represents FreeBSD. The synthetic
// "freebsd-base" package must only be emitted for FreeBSD hosts; otherwise
// generic phrases like "will be installed" from apt/dnf/yum/pkg output would
// trigger false positives on Linux.
func isFreeBSD(osType string) bool {
	return strings.EqualFold(strings.TrimSpace(osType), "freebsd")
}

func parsePackagesAffectedFromDryRunOutput(osType, output string) []string {
	var pkgs []string
	seen := make(map[string]bool)
	addPkg := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		pkgs = append(pkgs, name)
	}

	if isFreeBSD(osType) && freeBSDUpdateOutputHasPendingUpdates(output) {
		addPkg(freeBSDBasePackageName)
	}

	inFreeBSDPkgSection := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)

		// apt-get simulate: "Inst pkgname ..."
		if strings.HasPrefix(trimmed, "Inst ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				addPkg(fields[1])
			}
			continue
		}

		upper := strings.ToUpper(trimmed)
		switch {
		case strings.Contains(upper, "TO BE UPGRADED"),
			strings.Contains(upper, "TO BE INSTALLED"),
			strings.Contains(upper, "TO BE REINSTALLED"):
			inFreeBSDPkgSection = true
			continue
		case strings.HasPrefix(trimmed, "Number of "),
			strings.Contains(upper, "TO BE REMOVED"),
			strings.Contains(upper, "TO BE DOWNGRADED"):
			inFreeBSDPkgSection = false
		}

		if !inFreeBSDPkgSection || trimmed == "" {
			continue
		}

		matches := freeBSDPkgSummaryLinePattern.FindStringSubmatch(line)
		if len(matches) == 2 {
			addPkg(matches[1])
		}
	}

	return pkgs
}

// parsePackagesAffectedFromRealOutput extracts package names from real (non-dry-run) output.
// Handles multiple package manager formats:
//   - apt-get (real): "Unpacking pkgname" / "Setting up pkgname" lines
//   - apt-get (simulate): "Inst pkgname" lines (fallback, same as dry-run parser)
//   - dnf/yum: "Upgrading  : pkgname.arch" / "Installing : pkgname.arch" or transaction summary sections
//   - FreeBSD pkg: transaction summary lines and "[1/3] Upgrading pkgname ..." execution lines
//   - freebsd-update: base-system updates are recorded as the synthetic "freebsd-base" package (FreeBSD hosts only)
func parsePackagesAffectedFromRealOutput(osType, output string) []string {
	seen := make(map[string]bool)
	var pkgs []string
	addPkg := func(name string) {
		// Strip architecture suffix (e.g. "libssl3:amd64" -> "libssl3", "pkg.x86_64" -> "pkg")
		name = strings.SplitN(name, ":", 2)[0]
		name = strings.SplitN(name, ".", 2)[0]
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			pkgs = append(pkgs, name)
		}
	}

	if isFreeBSD(osType) && freeBSDUpdateOutputHasPendingUpdates(output) {
		addPkg(freeBSDBasePackageName)
	}

	inFreeBSDPkgSection := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		// apt-get simulate: "Inst pkgname ..."
		if strings.HasPrefix(trimmed, "Inst ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				addPkg(fields[1])
			}
			continue
		}
		// apt-get real: "Unpacking pkgname (version) ..."
		if strings.HasPrefix(trimmed, "Unpacking ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				addPkg(fields[1])
			}
			continue
		}
		// apt-get real: "Setting up pkgname (version) ..." - "Setting up" is two words, so pkg is at index 2
		if strings.HasPrefix(trimmed, "Setting up ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 3 {
				addPkg(fields[2])
			}
			continue
		}
		// dnf/yum real: "  Upgrading   : pkgname-version.arch" / "  Installing  : pkgname-version.arch"
		// Also handles "  Updating    : ..." (yum synonym for upgrading)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "upgrading") || strings.HasPrefix(lower, "installing") || strings.HasPrefix(lower, "updating") {
			// Format: "Upgrading   : pkgname-ver.arch  N/M"
			colonIdx := strings.Index(trimmed, ":")
			if colonIdx >= 0 {
				rest := strings.TrimSpace(trimmed[colonIdx+1:])
				// Take only the first token (package name+version+arch)
				fields := strings.Fields(rest)
				if len(fields) >= 1 {
					// Strip version: "pkgname-1.0-1.x86_64" -> "pkgname"
					pkgFull := fields[0]
					// Remove arch suffix first
					pkgNoArch := strings.SplitN(pkgFull, ".", 2)[0]
					// Remove version: last hyphen-separated segment that starts with a digit
					parts := strings.Split(pkgNoArch, "-")
					nameParts := parts[:1]
					for i := 1; i < len(parts); i++ {
						if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
							break
						}
						nameParts = append(nameParts, parts[i])
					}
					addPkg(strings.Join(nameParts, "-"))
				}
			}
		}

		upper := strings.ToUpper(trimmed)
		switch {
		case strings.Contains(upper, "TO BE UPGRADED"),
			strings.Contains(upper, "TO BE INSTALLED"),
			strings.Contains(upper, "TO BE REINSTALLED"):
			inFreeBSDPkgSection = true
			continue
		case strings.HasPrefix(trimmed, "Number of "),
			strings.Contains(upper, "TO BE REMOVED"),
			strings.Contains(upper, "TO BE DOWNGRADED"):
			inFreeBSDPkgSection = false
		}

		if inFreeBSDPkgSection && trimmed != "" {
			matches := freeBSDPkgSummaryLinePattern.FindStringSubmatch(line)
			if len(matches) == 2 {
				addPkg(matches[1])
				continue
			}
		}

		if matches := freeBSDPkgActionLinePattern.FindStringSubmatch(trimmed); len(matches) == 2 {
			addPkg(normalizeFreeBSDPkgActionTarget(matches[1]))
		}
	}
	return pkgs
}

func normalizeFreeBSDPkgActionTarget(name string) string {
	name = strings.TrimSpace(strings.TrimRight(name, ".,:;)]}"))
	if name == "" {
		return ""
	}
	if idx := strings.LastIndex(name, "-"); idx > 0 && idx+1 < len(name) {
		next := name[idx+1]
		if next >= '0' && next <= '9' {
			return name[:idx]
		}
	}
	return name
}

func freeBSDUpdateOutputHasPendingUpdates(output string) bool {
	if output == "" {
		return false
	}
	if strings.Contains(output, "No updates needed") || strings.Contains(output, "No updates are available") {
		return false
	}
	return strings.Contains(output, "will be updated") || strings.Contains(output, "will be installed")
}

// PatchRunsStore provides patch run access.
type PatchRunsStore struct {
	db database.DBProvider
}

// NewPatchRunsStore creates a new patch runs store.
// Pass DBProvider (e.g. DBResolver) for per-tenant DB in multi-host mode.
func NewPatchRunsStore(db database.DBProvider) *PatchRunsStore {
	return &PatchRunsStore{db: db}
}

// CreateRunOpts holds optional fields for CreateRun.
type CreateRunOpts struct {
	ValidationRunID  *string
	ApprovedByUserID *string
	// InitialStatus overrides the default status derivation. When empty the
	// status is "pending_validation" for dry runs, "queued" otherwise. Set to
	// "pending_approval" with dryRun=false to represent a "submit for
	// approval" run that hasn't been executed and isn't a dry-run either.
	InitialStatus string
}

// CreateRun creates a new patch run. If id is empty, a new UUID is generated.
// triggeredByUserID is the user who initiated the run (optional; nil for agent/system).
// dryRun when true creates a validation-only run with status "pending_validation".
// scheduledAt is when the patch will run (optional; nil for immediate runs).
// policyID, policyName, policySnapshot capture the effective policy at trigger time (all optional).
// opts.InitialStatus, if non-empty, overrides the dry-run-derived status.
func (s *PatchRunsStore) CreateRun(ctx context.Context, id, hostID, jobID, patchType string, packageName *string, packageNames []string, triggeredByUserID *string, dryRun bool, scheduledAt *time.Time, policyID, policyName *string, policySnapshot []byte, opts *CreateRunOpts) (string, error) {
	if id == "" {
		id = uuid.New().String()
	}
	var pkgNames []byte
	if len(packageNames) > 0 {
		var err error
		pkgNames, err = json.Marshal(packageNames)
		if err != nil {
			return "", err
		}
	}
	status := "queued"
	if dryRun {
		status = "pending_validation"
	}
	if opts != nil && opts.InitialStatus != "" {
		status = opts.InitialStatus
	}
	sched := pgtime.FromPtr(scheduledAt)
	var validationRunID *string
	var approvedByUserID *string
	if opts != nil {
		validationRunID = opts.ValidationRunID
		approvedByUserID = opts.ApprovedByUserID
	}
	d := s.db.DB(ctx)
	err := d.Queries.CreatePatchRun(ctx, db.CreatePatchRunParams{
		ID:                id,
		HostID:            hostID,
		JobID:             jobID,
		PatchType:         patchType,
		PackageName:       packageName,
		PackageNames:      pkgNames,
		Status:            status,
		ShellOutput:       "",
		TriggeredByUserID: triggeredByUserID,
		DryRun:            dryRun,
		ScheduledAt:       sched,
		PolicyID:          policyID,
		PolicyName:        policyName,
		PolicySnapshot:    policySnapshot,
		ValidationRunID:   validationRunID,
		ApprovedByUserID:  approvedByUserID,
	})
	return id, err
}

// GetByID returns a patch run by ID with host info.
func (s *PatchRunsStore) GetByID(ctx context.Context, id string) (*db.GetPatchRunByIDRow, error) {
	d := s.db.DB(ctx)
	row, err := d.Queries.GetPatchRunByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// UpdateOutput updates patch run based on agent stage.
// Valid stages: started, progress, completed, failed, dry_run_completed, cancelled.
//
// osType is the host's OS type (e.g. "ubuntu", "freebsd"). It gates the
// synthetic "freebsd-base" package emission so that only FreeBSD hosts can
// ever produce a "freebsd-base" entry in packages_affected.
//
// Semantics:
//   - "started" clears any prior output and flips status=running
//   - "progress" APPENDS the chunk to shell_output (live streaming)
//   - Terminal stages (completed, failed, cancelled, dry_run_completed) REPLACE
//     shell_output with the authoritative full output from the agent. This
//     avoids duplication of progress chunks already appended during the run.
func (s *PatchRunsStore) UpdateOutput(ctx context.Context, id, osType, stage, output, errorMessage string) error {
	d := s.db.DB(ctx)
	switch stage {
	case "started":
		return d.Queries.UpdatePatchRunStarted(ctx, id)
	case "progress":
		if output == "" {
			return nil
		}
		return d.Queries.UpdatePatchRunProgress(ctx, db.UpdatePatchRunProgressParams{ID: id, ShellOutput: output})
	case "completed":
		if err := d.Queries.UpdatePatchRunCompleted(ctx, db.UpdatePatchRunCompletedParams{ID: id, ShellOutput: output}); err != nil {
			return err
		}
		// Parse packages_affected from real run output for visibility.
		// Uses a multi-format parser covering apt "Unpacking"/"Setting up" lines
		// and dnf/yum "Upgrading:"/"Installing:" lines.
		if pkgs := parsePackagesAffectedFromRealOutput(osType, output); len(pkgs) > 0 {
			b, _ := json.Marshal(pkgs)
			_ = d.Queries.UpdatePatchRunPackagesAffected(ctx, db.UpdatePatchRunPackagesAffectedParams{ID: id, PackagesAffected: b})
		}
		return nil
	case "dry_run_completed":
		pkgs := parsePackagesAffectedFromDryRunOutput(osType, output)
		var b []byte
		if len(pkgs) > 0 {
			b, _ = json.Marshal(pkgs)
		}
		return d.Queries.UpdatePatchRunValidated(ctx, db.UpdatePatchRunValidatedParams{ID: id, ShellOutput: output, PackagesAffected: b})
	case "failed":
		return d.Queries.UpdatePatchRunFailed(ctx, db.UpdatePatchRunFailedParams{
			ID:           id,
			ShellOutput:  output,
			ErrorMessage: &errorMessage,
		})
	case "cancelled":
		errPtr := &errorMessage
		if errorMessage == "" {
			errPtr = nil
		}
		if err := d.Queries.UpdatePatchRunCancelled(ctx, db.UpdatePatchRunCancelledParams{
			ID:           id,
			ShellOutput:  output,
			ErrorMessage: errPtr,
		}); err != nil {
			return err
		}
		// Record packages that were actually applied before the stop so the
		// UI can still show partial state on a cancelled run.
		if pkgs := parsePackagesAffectedFromRealOutput(osType, output); len(pkgs) > 0 {
			b, _ := json.Marshal(pkgs)
			_ = d.Queries.UpdatePatchRunPackagesAffected(ctx, db.UpdatePatchRunPackagesAffectedParams{ID: id, PackagesAffected: b})
		}
		return nil
	default:
		return nil
	}
}

// UpdateStatus updates patch run status (e.g. for re-queuing when agent offline).
func (s *PatchRunsStore) UpdateStatus(ctx context.Context, id, status string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdatePatchRunStatus(ctx, db.UpdatePatchRunStatusParams{ID: id, Status: status})
}

// MarkValidationApproved marks a validation run as approved (terminal state).
// The actual patch run is created separately via CreateRun with ValidationRunID set.
func (s *PatchRunsStore) MarkValidationApproved(ctx context.Context, id string, approvedByUserID *string) error {
	d := s.db.DB(ctx)
	return d.Queries.MarkValidationApproved(ctx, db.MarkValidationApprovedParams{
		ID:               id,
		ApprovedByUserID: approvedByUserID,
	})
}

// SetPolicySnapshot stores the effective policy snapshot on a run (called at trigger/approve time).
func (s *PatchRunsStore) SetPolicySnapshot(ctx context.Context, id string, policyID, policyName *string, snapshot []byte) error {
	d := s.db.DB(ctx)
	return d.Queries.SetPatchRunPolicySnapshot(ctx, db.SetPatchRunPolicySnapshotParams{
		ID:             id,
		PolicyID:       policyID,
		PolicyName:     policyName,
		PolicySnapshot: snapshot,
	})
}

// SetScheduledAt stores when a queued patch run is scheduled to execute.
func (s *PatchRunsStore) SetScheduledAt(ctx context.Context, id string, scheduledAt time.Time) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdatePatchRunScheduledAt(ctx, db.UpdatePatchRunScheduledAtParams{
		ID:          id,
		ScheduledAt: pgtime.From(scheduledAt),
	})
}

// ClearScheduledAt removes the scheduled_at field (used when a run executes immediately).
func (s *PatchRunsStore) ClearScheduledAt(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.ClearScheduledAt(ctx, id)
}

// Delete removes a patch run by ID.
func (s *PatchRunsStore) Delete(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeletePatchRun(ctx, id)
}

// List returns paginated patch runs with optional filters and sort.
func (s *PatchRunsStore) List(ctx context.Context, hostID, status, patchType, sortBy, sortDir string, limit, offset int) ([]db.ListPatchRunsRow, int64, error) {
	d := s.db.DB(ctx)
	dbParams := db.ListPatchRunsParams{
		HostID:    hostID,
		Status:    status,
		PatchType: patchType,
		LimitArg:  safeconv.ClampToInt32(limit),
		OffsetArg: safeconv.ClampToInt32(offset),
	}
	orderParams := db.ListPatchRunsOrderByStartedAtParams{
		HostID: hostID, Status: status, PatchType: patchType,
		OffsetArg: safeconv.ClampToInt32(offset), LimitArg: safeconv.ClampToInt32(limit),
	}
	countParams := db.CountPatchRunsParams{HostID: hostID, Status: status, PatchType: patchType}

	var rows []db.ListPatchRunsRow
	var err error

	switch sortBy {
	case "started_at":
		if sortDir == "asc" {
			r, e := d.Queries.ListPatchRunsOrderByStartedAtAsc(ctx, db.ListPatchRunsOrderByStartedAtAscParams(orderParams))
			err = e
			rows = convertStartedAtAscRows(r)
		} else {
			r, e := d.Queries.ListPatchRunsOrderByStartedAt(ctx, orderParams)
			err = e
			rows = convertStartedAtRows(r)
		}
	case "completed_at":
		if sortDir == "asc" {
			r, e := d.Queries.ListPatchRunsOrderByCompletedAtAsc(ctx, db.ListPatchRunsOrderByCompletedAtAscParams(orderParams))
			err = e
			rows = convertCompletedAtAscRows(r)
		} else {
			r, e := d.Queries.ListPatchRunsOrderByCompletedAt(ctx, db.ListPatchRunsOrderByCompletedAtParams(orderParams))
			err = e
			rows = convertCompletedAtRows(r)
		}
	case "status":
		if sortDir == "desc" {
			r, e := d.Queries.ListPatchRunsOrderByStatusDesc(ctx, db.ListPatchRunsOrderByStatusDescParams(orderParams))
			err = e
			rows = convertStatusDescRows(r)
		} else {
			r, e := d.Queries.ListPatchRunsOrderByStatus(ctx, db.ListPatchRunsOrderByStatusParams(orderParams))
			err = e
			rows = convertStatusRows(r)
		}
	case "created_at":
		if sortDir == "asc" {
			r, e := d.Queries.ListPatchRunsOrderByCreatedAtAsc(ctx, db.ListPatchRunsOrderByCreatedAtAscParams(orderParams))
			err = e
			rows = convertCreatedAtAscRows(r)
		} else {
			rows, err = d.Queries.ListPatchRuns(ctx, dbParams)
		}
	default:
		// created_at (default)
		rows, err = d.Queries.ListPatchRuns(ctx, dbParams)
	}

	if err != nil {
		return nil, 0, err
	}
	count, err := d.Queries.CountPatchRuns(ctx, countParams)
	if err != nil {
		return nil, 0, err
	}
	return rows, count, nil
}

func convertStartedAtRows(r []db.ListPatchRunsOrderByStartedAtRow) []db.ListPatchRunsRow {
	out := make([]db.ListPatchRunsRow, len(r))
	for i := range r {
		out[i] = db.ListPatchRunsRow{
			ID:                  r[i].ID,
			HostID:              r[i].HostID,
			JobID:               r[i].JobID,
			PatchType:           r[i].PatchType,
			PackageName:         r[i].PackageName,
			PackageNames:        r[i].PackageNames,
			Status:              r[i].Status,
			ShellOutput:         r[i].ShellOutput,
			ErrorMessage:        r[i].ErrorMessage,
			StartedAt:           r[i].StartedAt,
			CompletedAt:         r[i].CompletedAt,
			ScheduledAt:         r[i].ScheduledAt,
			TriggeredByUserID:   r[i].TriggeredByUserID,
			CreatedAt:           r[i].CreatedAt,
			UpdatedAt:           r[i].UpdatedAt,
			HostFriendlyName:    r[i].HostFriendlyName,
			HostHostname:        r[i].HostHostname,
			TriggeredByUsername: r[i].TriggeredByUsername,
			ValidationRunID:     r[i].ValidationRunID,
		}
	}
	return out
}

func convertStartedAtAscRows(r []db.ListPatchRunsOrderByStartedAtAscRow) []db.ListPatchRunsRow {
	out := make([]db.ListPatchRunsRow, len(r))
	for i := range r {
		out[i] = db.ListPatchRunsRow{
			ID:                  r[i].ID,
			HostID:              r[i].HostID,
			JobID:               r[i].JobID,
			PatchType:           r[i].PatchType,
			PackageName:         r[i].PackageName,
			PackageNames:        r[i].PackageNames,
			Status:              r[i].Status,
			ShellOutput:         r[i].ShellOutput,
			ErrorMessage:        r[i].ErrorMessage,
			StartedAt:           r[i].StartedAt,
			CompletedAt:         r[i].CompletedAt,
			ScheduledAt:         r[i].ScheduledAt,
			TriggeredByUserID:   r[i].TriggeredByUserID,
			CreatedAt:           r[i].CreatedAt,
			UpdatedAt:           r[i].UpdatedAt,
			HostFriendlyName:    r[i].HostFriendlyName,
			HostHostname:        r[i].HostHostname,
			TriggeredByUsername: r[i].TriggeredByUsername,
			ValidationRunID:     r[i].ValidationRunID,
		}
	}
	return out
}

func convertCompletedAtRows(r []db.ListPatchRunsOrderByCompletedAtRow) []db.ListPatchRunsRow {
	out := make([]db.ListPatchRunsRow, len(r))
	for i := range r {
		out[i] = db.ListPatchRunsRow{
			ID:                  r[i].ID,
			HostID:              r[i].HostID,
			JobID:               r[i].JobID,
			PatchType:           r[i].PatchType,
			PackageName:         r[i].PackageName,
			PackageNames:        r[i].PackageNames,
			Status:              r[i].Status,
			ShellOutput:         r[i].ShellOutput,
			ErrorMessage:        r[i].ErrorMessage,
			StartedAt:           r[i].StartedAt,
			CompletedAt:         r[i].CompletedAt,
			ScheduledAt:         r[i].ScheduledAt,
			TriggeredByUserID:   r[i].TriggeredByUserID,
			CreatedAt:           r[i].CreatedAt,
			UpdatedAt:           r[i].UpdatedAt,
			HostFriendlyName:    r[i].HostFriendlyName,
			HostHostname:        r[i].HostHostname,
			TriggeredByUsername: r[i].TriggeredByUsername,
			ValidationRunID:     r[i].ValidationRunID,
		}
	}
	return out
}

func convertCompletedAtAscRows(r []db.ListPatchRunsOrderByCompletedAtAscRow) []db.ListPatchRunsRow {
	out := make([]db.ListPatchRunsRow, len(r))
	for i := range r {
		out[i] = db.ListPatchRunsRow{
			ID:                  r[i].ID,
			HostID:              r[i].HostID,
			JobID:               r[i].JobID,
			PatchType:           r[i].PatchType,
			PackageName:         r[i].PackageName,
			PackageNames:        r[i].PackageNames,
			Status:              r[i].Status,
			ShellOutput:         r[i].ShellOutput,
			ErrorMessage:        r[i].ErrorMessage,
			StartedAt:           r[i].StartedAt,
			CompletedAt:         r[i].CompletedAt,
			ScheduledAt:         r[i].ScheduledAt,
			TriggeredByUserID:   r[i].TriggeredByUserID,
			CreatedAt:           r[i].CreatedAt,
			UpdatedAt:           r[i].UpdatedAt,
			HostFriendlyName:    r[i].HostFriendlyName,
			HostHostname:        r[i].HostHostname,
			TriggeredByUsername: r[i].TriggeredByUsername,
			ValidationRunID:     r[i].ValidationRunID,
		}
	}
	return out
}

func convertStatusRows(r []db.ListPatchRunsOrderByStatusRow) []db.ListPatchRunsRow {
	out := make([]db.ListPatchRunsRow, len(r))
	for i := range r {
		out[i] = db.ListPatchRunsRow{
			ID:                  r[i].ID,
			HostID:              r[i].HostID,
			JobID:               r[i].JobID,
			PatchType:           r[i].PatchType,
			PackageName:         r[i].PackageName,
			PackageNames:        r[i].PackageNames,
			Status:              r[i].Status,
			ShellOutput:         r[i].ShellOutput,
			ErrorMessage:        r[i].ErrorMessage,
			StartedAt:           r[i].StartedAt,
			CompletedAt:         r[i].CompletedAt,
			ScheduledAt:         r[i].ScheduledAt,
			TriggeredByUserID:   r[i].TriggeredByUserID,
			CreatedAt:           r[i].CreatedAt,
			UpdatedAt:           r[i].UpdatedAt,
			HostFriendlyName:    r[i].HostFriendlyName,
			HostHostname:        r[i].HostHostname,
			TriggeredByUsername: r[i].TriggeredByUsername,
			ValidationRunID:     r[i].ValidationRunID,
		}
	}
	return out
}

func convertStatusDescRows(r []db.ListPatchRunsOrderByStatusDescRow) []db.ListPatchRunsRow {
	out := make([]db.ListPatchRunsRow, len(r))
	for i := range r {
		out[i] = db.ListPatchRunsRow{
			ID:                  r[i].ID,
			HostID:              r[i].HostID,
			JobID:               r[i].JobID,
			PatchType:           r[i].PatchType,
			PackageName:         r[i].PackageName,
			PackageNames:        r[i].PackageNames,
			Status:              r[i].Status,
			ShellOutput:         r[i].ShellOutput,
			ErrorMessage:        r[i].ErrorMessage,
			StartedAt:           r[i].StartedAt,
			CompletedAt:         r[i].CompletedAt,
			ScheduledAt:         r[i].ScheduledAt,
			TriggeredByUserID:   r[i].TriggeredByUserID,
			CreatedAt:           r[i].CreatedAt,
			UpdatedAt:           r[i].UpdatedAt,
			HostFriendlyName:    r[i].HostFriendlyName,
			HostHostname:        r[i].HostHostname,
			TriggeredByUsername: r[i].TriggeredByUsername,
			ValidationRunID:     r[i].ValidationRunID,
		}
	}
	return out
}

func convertCreatedAtAscRows(r []db.ListPatchRunsOrderByCreatedAtAscRow) []db.ListPatchRunsRow {
	out := make([]db.ListPatchRunsRow, len(r))
	for i := range r {
		out[i] = db.ListPatchRunsRow{
			ID:                  r[i].ID,
			HostID:              r[i].HostID,
			JobID:               r[i].JobID,
			PatchType:           r[i].PatchType,
			PackageName:         r[i].PackageName,
			PackageNames:        r[i].PackageNames,
			Status:              r[i].Status,
			ShellOutput:         r[i].ShellOutput,
			ErrorMessage:        r[i].ErrorMessage,
			StartedAt:           r[i].StartedAt,
			CompletedAt:         r[i].CompletedAt,
			ScheduledAt:         r[i].ScheduledAt,
			TriggeredByUserID:   r[i].TriggeredByUserID,
			CreatedAt:           r[i].CreatedAt,
			UpdatedAt:           r[i].UpdatedAt,
			HostFriendlyName:    r[i].HostFriendlyName,
			HostHostname:        r[i].HostHostname,
			TriggeredByUsername: r[i].TriggeredByUsername,
			ValidationRunID:     r[i].ValidationRunID,
		}
	}
	return out
}

// ListActive returns runs with status queued or running.
func (s *PatchRunsStore) ListActive(ctx context.Context) ([]db.ListActivePatchRunsRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListActivePatchRuns(ctx)
}

// GetDashboard returns dashboard summary: total count, by status, recent runs, active runs.
func (s *PatchRunsStore) GetDashboard(ctx context.Context) (total int64, byStatus map[string]int, recent []db.ListRecentPatchRunsRow, active []db.ListActivePatchRunsRow, err error) {
	d := s.db.DB(ctx)
	total, err = d.Queries.CountPatchRunsTotal(ctx)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	statusRows, err := d.Queries.ListPatchRunsByStatus(ctx)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	byStatus = make(map[string]int)
	for _, r := range statusRows {
		byStatus[r.Status] = int(r.Count)
	}
	recent, err = d.Queries.ListRecentPatchRuns(ctx, 10)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	active, err = d.Queries.ListActivePatchRuns(ctx)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	return total, byStatus, recent, active, nil
}

// PatchPoliciesStore provides patch policy access.
type PatchPoliciesStore struct {
	db database.DBProvider
}

// NewPatchPoliciesStore creates a new patch policies store.
func NewPatchPoliciesStore(db database.DBProvider) *PatchPoliciesStore {
	return &PatchPoliciesStore{db: db}
}

// ResolveEffectivePolicy returns the effective patch policy for a host.
// Precedence: direct host assignment > first group assignment (by created_at) that contains host.
// Exclusions only apply to group-based assignments; a direct assignment cannot be excluded.
func (s *PatchPoliciesStore) ResolveEffectivePolicy(ctx context.Context, hostID string) (*db.PatchPolicy, error) {
	d := s.db.DB(ctx)
	// 1. Direct host assignment - exclusions do not apply here.
	policy, err := d.Queries.GetDirectPatchPolicyAssignment(ctx, hostID)
	if err == nil {
		return &policy, nil
	}

	// 2. Group assignments - exclusions apply.
	groupIDs, err := d.Queries.GetHostGroupMemberships(ctx, hostID)
	if err != nil || len(groupIDs) == 0 {
		return nil, nil
	}
	for _, gid := range groupIDs {
		policy, err := d.Queries.GetPatchPolicyByGroupAssignment(ctx, gid)
		if err != nil {
			continue
		}
		excluded, _ := d.Queries.ExistsPatchPolicyExclusion(ctx, db.ExistsPatchPolicyExclusionParams{
			PatchPolicyID: policy.ID,
			HostID:        hostID,
		})
		if !excluded {
			return &policy, nil
		}
	}
	return nil, nil
}

// ScheduleMeta describes the timezone resolution applied to a fixed-time policy
// computation. The caller (handler) uses it to populate the policy snapshot
// stored on the run and to log scheduling decisions for audit.
type ScheduleMeta struct {
	// Timezone is the IANA name actually used to compute the run instant.
	// "UTC" when the policy is not a fixed_time schedule or when the resolved
	// zone could not be loaded.
	Timezone string
	// Source is one of "org", "utc-fallback", or "n/a" (for non-fixed-time
	// schedules where timezone is irrelevant).
	Source string
	// Err is non-nil when fixed_time_utc could not be parsed; the caller
	// should log it and treat the run as immediate (now).
	Err error
}

// ParseHHMM parses an HH:MM or HH:MM:SS string and returns the components.
// Range-validates each component. Exported so handler-level input validation
// rejects the same malformed values that the scheduler would later reject.
func ParseHHMM(s string) (h, m, sec int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, 0, errors.New("empty time value")
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, errors.New("expected HH:MM or HH:MM:SS")
	}
	hh, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, err
	}
	mm, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, err
	}
	ss := 0
	if len(parts) == 3 {
		ss, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 || ss < 0 || ss > 59 {
		return 0, 0, 0, errors.New("time component out of range")
	}
	return hh, mm, ss, nil
}

// nextFixedWallClockUTC returns the next absolute time at which the wall-clock
// time hhmm (HH:MM or HH:MM:SS) elapses in loc, on or after now. The result is
// returned as a UTC instant. Errors on malformed input.
//
// DST behavior (Go's time.Date semantics):
//   - Spring-forward (the local hour does not exist, e.g. 02:30 on the
//     transition day): time.Date normalizes forward, so 02:30 local becomes
//     03:30 local on that day.
//   - Fall-back (the local hour occurs twice): time.Date selects the
//     standard-time (post-shift) occurrence — i.e. the later of the two
//     wall-clock instants.
//
// Both behaviors are documented in the admin guide.
func nextFixedWallClockUTC(now time.Time, hhmm string, loc *time.Location) (time.Time, error) {
	h, m, s, err := ParseHHMM(hhmm)
	if err != nil {
		return time.Time{}, err
	}
	if loc == nil {
		loc = time.UTC
	}
	// Anchor "today" in the target location so the calendar date matches the
	// zone in which the operator set the policy.
	nowLocal := now.In(loc)
	candidate := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), h, m, s, 0, loc)
	if !candidate.After(now) {
		// Build the next-day instant via time.Date (not AddDate on the
		// absolute instant) so the wall-clock hour is preserved across DST
		// transitions.
		candidate = time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day()+1, h, m, s, 0, loc)
	}
	return candidate.UTC(), nil
}

// ComputeRunAtWithMeta returns when a patch should run based on policy and the
// timezone resolution that was applied. The orgTimezone parameter should be
// the resolved organization-wide IANA name (via config.ResolveTimezone in the
// caller). For fixed_time schedules, the policy's own Timezone column is
// intentionally ignored: scheduling is governed by the org-resolved zone so
// there is a single source of truth across the system.
//
// Returns UTC; storage via pgtype.Timestamp expects UTC (see pgtime package).
func (s *PatchPoliciesStore) ComputeRunAtWithMeta(policy *db.PatchPolicy, orgTimezone string) (time.Time, ScheduleMeta) {
	now := time.Now().UTC()
	if policy == nil || policy.PatchDelayType == "immediate" {
		return now, ScheduleMeta{Timezone: "UTC", Source: "n/a"}
	}
	if policy.PatchDelayType == "delayed" && policy.DelayMinutes != nil {
		return now.Add(time.Duration(*policy.DelayMinutes) * time.Minute), ScheduleMeta{Timezone: "UTC", Source: "n/a"}
	}
	if policy.PatchDelayType == "fixed_time" && policy.FixedTimeUtc != nil && *policy.FixedTimeUtc != "" {
		loc, source, tzName := resolveScheduleLocation(orgTimezone)
		runAt, err := nextFixedWallClockUTC(now, *policy.FixedTimeUtc, loc)
		if err != nil {
			return now, ScheduleMeta{Timezone: tzName, Source: source, Err: err}
		}
		return runAt, ScheduleMeta{Timezone: tzName, Source: source}
	}
	return now, ScheduleMeta{Timezone: "UTC", Source: "n/a"}
}

// resolveScheduleLocation loads the time.Location for orgTimezone, falling
// back to UTC when the value is empty or unparseable. Returns the location,
// the source label ("org" / "utc-fallback"), and the IANA name actually used.
func resolveScheduleLocation(orgTimezone string) (*time.Location, string, string) {
	if orgTimezone != "" {
		if loc, err := time.LoadLocation(orgTimezone); err == nil {
			return loc, "org", orgTimezone
		}
	}
	return time.UTC, "utc-fallback", "UTC"
}

// List returns all patch policies.
func (s *PatchPoliciesStore) List(ctx context.Context) ([]db.PatchPolicy, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListPatchPolicies(ctx)
}

// GetByID returns a patch policy by ID.
func (s *PatchPoliciesStore) GetByID(ctx context.Context, id string) (*db.PatchPolicy, error) {
	d := s.db.DB(ctx)
	policy, err := d.Queries.GetPatchPolicyByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &policy, nil
}

// Create creates a new patch policy.
func (s *PatchPoliciesStore) Create(ctx context.Context, name, description, patchDelayType string, delayMinutes *int32, fixedTimeUtc, timezone *string) (string, error) {
	d := s.db.DB(ctx)
	id := uuid.New().String()
	var desc *string
	if description != "" {
		desc = &description
	}
	err := d.Queries.CreatePatchPolicy(ctx, db.CreatePatchPolicyParams{
		ID:             id,
		Name:           name,
		Description:    desc,
		PatchDelayType: patchDelayType,
		DelayMinutes:   delayMinutes,
		FixedTimeUtc:   fixedTimeUtc,
		Timezone:       timezone,
	})
	return id, err
}

// Update updates a patch policy.
func (s *PatchPoliciesStore) Update(ctx context.Context, id, name, description, patchDelayType string, delayMinutes *int32, fixedTimeUtc, timezone *string) error {
	d := s.db.DB(ctx)
	var desc *string
	if description != "" {
		desc = &description
	}
	return d.Queries.UpdatePatchPolicy(ctx, db.UpdatePatchPolicyParams{
		ID:             id,
		Name:           name,
		Description:    desc,
		PatchDelayType: patchDelayType,
		DelayMinutes:   delayMinutes,
		FixedTimeUtc:   fixedTimeUtc,
		Timezone:       timezone,
	})
}

// Delete deletes a patch policy.
func (s *PatchPoliciesStore) Delete(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeletePatchPolicy(ctx, id)
}

// PatchPolicyAssignmentsStore provides patch policy assignment access.
type PatchPolicyAssignmentsStore struct {
	db database.DBProvider
}

// NewPatchPolicyAssignmentsStore creates a new patch policy assignments store.
func NewPatchPolicyAssignmentsStore(db database.DBProvider) *PatchPolicyAssignmentsStore {
	return &PatchPolicyAssignmentsStore{db: db}
}

// ListByPolicy returns assignments for a policy.
func (s *PatchPolicyAssignmentsStore) ListByPolicy(ctx context.Context, policyID string) ([]db.PatchPolicyAssignment, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListPatchPolicyAssignmentsByPolicy(ctx, policyID)
}

// Create creates an assignment.
func (s *PatchPolicyAssignmentsStore) Create(ctx context.Context, policyID, targetType, targetID string) (string, error) {
	d := s.db.DB(ctx)
	id := uuid.New().String()
	err := d.Queries.CreatePatchPolicyAssignment(ctx, db.CreatePatchPolicyAssignmentParams{
		ID:            id,
		PatchPolicyID: policyID,
		TargetType:    targetType,
		TargetID:      targetID,
	})
	return id, err
}

// Delete deletes an assignment by ID.
func (s *PatchPolicyAssignmentsStore) Delete(ctx context.Context, assignmentID string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeletePatchPolicyAssignment(ctx, assignmentID)
}

// GetByID returns an assignment by ID.
func (s *PatchPolicyAssignmentsStore) GetByID(ctx context.Context, id string) (*db.PatchPolicyAssignment, error) {
	d := s.db.DB(ctx)
	a, err := d.Queries.GetPatchPolicyAssignmentByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// PatchPolicyExclusionsStore provides patch policy exclusion access.
type PatchPolicyExclusionsStore struct {
	db database.DBProvider
}

// NewPatchPolicyExclusionsStore creates a new patch policy exclusions store.
func NewPatchPolicyExclusionsStore(db database.DBProvider) *PatchPolicyExclusionsStore {
	return &PatchPolicyExclusionsStore{db: db}
}

// ListByPolicy returns exclusions for a policy with host info.
func (s *PatchPolicyExclusionsStore) ListByPolicy(ctx context.Context, policyID string) ([]db.ListPatchPolicyExclusionsRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListPatchPolicyExclusions(ctx, policyID)
}

// Create creates an exclusion.
func (s *PatchPolicyExclusionsStore) Create(ctx context.Context, policyID, hostID string) (string, error) {
	d := s.db.DB(ctx)
	id := uuid.New().String()
	err := d.Queries.CreatePatchPolicyExclusion(ctx, db.CreatePatchPolicyExclusionParams{
		ID:            id,
		PatchPolicyID: policyID,
		HostID:        hostID,
	})
	return id, err
}

// Delete deletes an exclusion.
func (s *PatchPolicyExclusionsStore) Delete(ctx context.Context, policyID, hostID string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeletePatchPolicyExclusion(ctx, db.DeletePatchPolicyExclusionParams{
		PatchPolicyID: policyID,
		HostID:        hostID,
	})
}
