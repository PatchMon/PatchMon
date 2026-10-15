package store

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
)

// Patch run item kinds.
const (
	PatchItemKindWUA    = "wua"
	PatchItemKindWinget = "winget"
)

// NewPatchRunItem is a target written to patch_run_items when a run is created.
type NewPatchRunItem struct {
	Kind           string `json:"kind"`
	Identifier     string `json:"identifier"`
	RevisionNumber int32  `json:"revision_number"`
	Title          string `json:"title"`
	KB             string `json:"kb"`
}

func insertPatchRunItems(ctx context.Context, q *db.Queries, runID, hostID string, items []NewPatchRunItem) error {
	if len(items) == 0 {
		return nil
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return q.InsertPatchRunItems(ctx, db.InsertPatchRunItemsParams{PatchRunID: runID, HostID: hostID, Payload: payload})
}

// ResolveWindowsPatchTargets looks up the host's own host_packages rows for the given display names.
func (s *PatchRunsStore) ResolveWindowsPatchTargets(ctx context.Context, hostID string, names []string) ([]db.ResolveWindowsPatchTargetsRow, error) {
	d := s.db.DB(ctx)
	return d.Queries.ResolveWindowsPatchTargets(ctx, db.ResolveWindowsPatchTargetsParams{HostID: hostID, Names: names})
}

// ListItems returns every item recorded against a run.
func (s *PatchRunsStore) ListItems(ctx context.Context, runID string) ([]db.PatchRunItem, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListPatchRunItems(ctx, runID)
}

// ListPendingItems returns the run's items that the agent has not reported on yet.
func (s *PatchRunsStore) ListPendingItems(ctx context.Context, runID string) ([]db.PatchRunItem, error) {
	d := s.db.DB(ctx)
	return d.Queries.ListPendingPatchRunItems(ctx, runID)
}

// ReplaceItems swaps a run's items for a freshly resolved set in one transaction.
func (s *PatchRunsStore) ReplaceItems(ctx context.Context, runID, hostID string, items []NewPatchRunItem) error {
	d := s.db.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()
	q := d.Queries.WithTx(tx)
	if err := q.DeletePatchRunItems(ctx, runID); err != nil {
		return err
	}
	if err := insertPatchRunItems(ctx, q, runID, hostID, items); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AbortPendingItems marks the given still-pending items aborted with a reason.
func (s *PatchRunsStore) AbortPendingItems(ctx context.Context, ids []string, message string) error {
	if len(ids) == 0 {
		return nil
	}
	d := s.db.DB(ctx)
	return d.Queries.AbortPendingPatchRunItems(ctx, db.AbortPendingPatchRunItemsParams{Ids: ids, Message: &message})
}

// FailRun marks a run that never reached the agent as failed.
func (s *PatchRunsStore) FailRun(ctx context.Context, id, errorMessage string) error {
	d := s.db.DB(ctx)
	_, err := d.Queries.UpdatePatchRunFailed(ctx, db.UpdatePatchRunFailedParams{ID: id, ShellOutput: "", ErrorMessage: &errorMessage})
	return err
}

// Limits on agent item reports.
const (
	MaxItemsPerReport   = 200
	MaxItemsPerRun      = 500
	maxItemTitleRunes   = 512
	maxItemMessageRunes = 4000
)

var patchItemStatuses = map[string]bool{
	"pending": true, "would_install": true, "not_applicable": true, "installed": true,
	"installed_with_errors": true, "installed_pending_reboot": true, "already_installed": true,
	"failed": true, "aborted": true, "skipped": true,
}

var hresultPattern = regexp.MustCompile(`^0x[0-9A-Fa-f]{1,8}$`)

// AgentItemReport is one item as the agent posts it to /patching/runs/{id}/items.
type AgentItemReport struct {
	Kind           string  `json:"kind"`
	ID             string  `json:"id"`
	Revision       *int32  `json:"revision"`
	Title          string  `json:"title"`
	KB             string  `json:"kb"`
	Status         string  `json:"status"`
	ResultCode     *int32  `json:"result_code"`
	HResult        string  `json:"hresult"`
	ExitCode       *int64  `json:"exit_code"`
	RebootRequired *bool   `json:"reboot_required"`
	DownloadBytes  *int64  `json:"download_bytes"`
	Message        string  `json:"message"`
	StartedAt      *string `json:"started_at"`
	CompletedAt    *string `json:"completed_at"`
}

type agentItemRow struct {
	Kind           string  `json:"kind"`
	Identifier     string  `json:"identifier"`
	RevisionNumber int32   `json:"revision_number"`
	Title          string  `json:"title"`
	KB             string  `json:"kb"`
	Status         string  `json:"status"`
	ResultCode     *int32  `json:"result_code"`
	HResult        string  `json:"hresult"`
	ExitCode       *int64  `json:"exit_code"`
	RebootRequired *bool   `json:"reboot_required"`
	DownloadBytes  *int64  `json:"download_bytes"`
	Message        string  `json:"message"`
	StartedAt      *string `json:"started_at"`
	CompletedAt    *string `json:"completed_at"`
}

// ItemReportError describes the first invalid item in an agent report.
type ItemReportError struct {
	Index  int
	Reason string
}

func (e *ItemReportError) Error() string {
	return "item " + strconv.Itoa(e.Index) + ": " + e.Reason
}

func normaliseAgentItems(items []AgentItemReport) ([]agentItemRow, error) {
	if len(items) > MaxItemsPerReport {
		return nil, &ItemReportError{Index: MaxItemsPerReport, Reason: "too many items in one report"}
	}
	rows := make([]agentItemRow, 0, len(items))
	seen := make(map[string]int, len(items))
	for i, it := range items {
		id := strings.TrimSpace(it.ID)
		switch it.Kind {
		case PatchItemKindWUA:
			id = strings.ToLower(id)
			if !IsValidWUAGuid(id) {
				return nil, &ItemReportError{Index: i, Reason: "invalid wua id"}
			}
		case PatchItemKindWinget:
			if !IsValidWingetID(id) {
				return nil, &ItemReportError{Index: i, Reason: "invalid winget id"}
			}
		default:
			return nil, &ItemReportError{Index: i, Reason: "invalid kind"}
		}
		if !patchItemStatuses[it.Status] {
			return nil, &ItemReportError{Index: i, Reason: "invalid status"}
		}
		row := agentItemRow{
			Kind:           it.Kind,
			Identifier:     id,
			Title:          truncateRunes(sanitizeText(it.Title), maxItemTitleRunes),
			KB:             truncateRunes(sanitizeText(it.KB), maxItemTitleRunes),
			Status:         it.Status,
			ResultCode:     it.ResultCode,
			ExitCode:       it.ExitCode,
			RebootRequired: it.RebootRequired,
			Message:        truncateRunes(sanitizeText(it.Message), maxItemMessageRunes),
			StartedAt:      normaliseItemTime(it.StartedAt),
			CompletedAt:    normaliseItemTime(it.CompletedAt),
		}
		if it.Revision != nil && *it.Revision > 0 {
			row.RevisionNumber = *it.Revision
		}
		if hresultPattern.MatchString(it.HResult) {
			row.HResult = it.HResult
		}
		if it.DownloadBytes != nil && *it.DownloadBytes >= 0 {
			row.DownloadBytes = it.DownloadBytes
		}
		key := row.Kind + "\x00" + row.Identifier
		if j, dup := seen[key]; dup {
			rows[j] = row
			continue
		}
		seen[key] = len(rows)
		rows = append(rows, row)
	}
	return rows, nil
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func normaliseItemTime(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		return nil
	}
	out := t.UTC().Format(time.RFC3339Nano)
	return &out
}

// ErrRunNotAcceptingItems is returned when a run's status no longer takes agent item reports.
var ErrRunNotAcceptingItems = errors.New("patch run is not accepting item reports")

// ItemReportGrace is how long after a run stops that a late agent may still post its items.
const ItemReportGrace = 15 * time.Minute

// RunAcceptsItemReports reports whether an agent may still post items for a run in this status.
// Stopped runs accept them only within ItemReportGrace of their last change.
func RunAcceptsItemReports(status string, withinGrace bool) bool {
	switch status {
	case "queued", "running", "pending_validation":
		return true
	case "cancelled", "timed_out", "agent_disconnected":
		return withinGrace
	}
	return false
}

// ItemReportResult counts what happened to an agent item report.
type ItemReportResult struct {
	Updated  int `json:"updated"`
	Inserted int `json:"inserted"`
	Ignored  int `json:"ignored"`
}

// RecordAgentItems applies an agent item report to a run. patch_package runs only update the
// items created at trigger time; patch_all runs may add items up to MaxItemsPerRun.
func (s *PatchRunsStore) RecordAgentItems(ctx context.Context, runID, hostID string, allowInsert bool, items []AgentItemReport) (ItemReportResult, error) {
	var res ItemReportResult
	rows, err := normaliseAgentItems(items)
	if err != nil {
		return res, err
	}
	if len(rows) == 0 {
		return res, nil
	}
	d := s.db.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()
	q := d.Queries.WithTx(tx)

	lock, err := q.LockPatchRunStatus(ctx, db.LockPatchRunStatusParams{ID: runID, GraceSeconds: int32(ItemReportGrace / time.Second)})
	if err != nil {
		return res, err
	}
	if !RunAcceptsItemReports(lock.Status, lock.WithinGrace) {
		return res, ErrRunNotAcceptingItems
	}
	keys, err := q.ListPatchRunItemKeys(ctx, runID)
	if err != nil {
		return res, err
	}
	existing := make(map[string]bool, len(keys))
	for _, k := range keys {
		existing[k.Kind+"\x00"+k.Identifier] = true
	}
	known, fresh := splitItemRows(rows, existing, allowInsert, MaxItemsPerRun-len(keys))
	res.Ignored = len(rows) - len(known) - len(fresh)

	if len(known) > 0 {
		payload, err := json.Marshal(known)
		if err != nil {
			return res, err
		}
		n, err := q.UpdatePatchRunItemsFromAgent(ctx, db.UpdatePatchRunItemsFromAgentParams{PatchRunID: runID, Payload: payload})
		if err != nil {
			return res, err
		}
		res.Updated = int(n)
	}
	if len(fresh) > 0 {
		payload, err := json.Marshal(fresh)
		if err != nil {
			return res, err
		}
		if err := q.UpsertPatchRunItemsFromAgent(ctx, db.UpsertPatchRunItemsFromAgentParams{PatchRunID: runID, HostID: hostID, Payload: payload}); err != nil {
			return res, err
		}
		res.Inserted = len(fresh)
	}
	return res, tx.Commit(ctx)
}

func splitItemRows(rows []agentItemRow, existing map[string]bool, allowInsert bool, room int) (known, fresh []agentItemRow) {
	for _, r := range rows {
		if existing[r.Kind+"\x00"+r.Identifier] {
			known = append(known, r)
			continue
		}
		if allowInsert && len(fresh) < room {
			fresh = append(fresh, r)
		}
	}
	return known, fresh
}

// SetRebootRequired records the agent's restart verdict on a run.
func (s *PatchRunsStore) SetRebootRequired(ctx context.Context, runID string, required bool) error {
	d := s.db.DB(ctx)
	return d.Queries.SetPatchRunRebootRequired(ctx, db.SetPatchRunRebootRequiredParams{ID: runID, RebootRequired: &required})
}

func windowsPackagesAffected(stage string, items []db.PatchRunItem) []string {
	var out []string
	for _, it := range items {
		switch stage {
		case "dry_run_completed":
			if it.Status == "would_install" {
				out = append(out, it.Title)
			}
		case "completed", "failed", "cancelled":
			if it.Status == "installed" || it.Status == "installed_with_errors" {
				out = append(out, it.Title)
			}
		}
	}
	return out
}

func itemsNeedReboot(items []db.PatchRunItem) bool {
	for _, it := range items {
		if it.RebootRequired || it.Status == "installed_pending_reboot" {
			return true
		}
	}
	return false
}

// ItemsNeedReboot reports whether any of the run's items left the host needing a restart.
func (s *PatchRunsStore) ItemsNeedReboot(ctx context.Context, runID string) (bool, error) {
	items, err := s.ListItems(ctx, runID)
	if err != nil {
		return false, err
	}
	return itemsNeedReboot(items), nil
}

// PackageIDsByName maps each name that exists in the package inventory to its package ID.
func (s *PatchRunsStore) PackageIDsByName(ctx context.Context, names []string) (map[string]string, error) {
	out := make(map[string]string)
	if len(names) == 0 {
		return out, nil
	}
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListPackageIDsByNames(ctx, names)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Name] = r.ID
	}
	return out, nil
}
