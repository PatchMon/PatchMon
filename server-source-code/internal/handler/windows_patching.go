package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

// MinWindowsPatchAgentVersion is the oldest agent that may be sent a Windows patch run.
const MinWindowsPatchAgentVersion = "2.1.4"

// Lowest semver pre-release of the minimum, so 2.1.4-rc.N builds pass the gate.
const minWindowsPatchAgentFloor = MinWindowsPatchAgentVersion + "-0"

const (
	maxPatchTargets          = 100
	maxWindowsPackageNameLen = 512
)

const (
	rejectNotFound          = "not_found"
	rejectNotPending        = "not_pending"
	rejectNoWingetID        = "no_winget_id"
	rejectInvalidIdentifier = "invalid_identifier"
	rejectWingetPinned      = "winget_pinned"
	rejectStoreApp          = "store_app"
)

type windowsScope struct {
	Updates *bool `json:"updates"`
	Apps    *bool `json:"apps"`
}

type windowsTargetReject struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type windowsRunPlan struct {
	Items          []store.NewPatchRunItem
	IncludeUpdates *bool
	IncludeApps    *bool
}

type runRequestError struct {
	Status int
	Body   map[string]interface{}
}

func badRunRequest(msg string) *runRequestError {
	return &runRequestError{Status: http.StatusBadRequest, Body: map[string]interface{}{"error": msg}}
}

func windowsAgentVersionError(agentVersion *string) *runRequestError {
	v := ""
	if agentVersion != nil {
		v = *agentVersion
	}
	if v != "" && util.CompareVersions(v, minWindowsPatchAgentFloor) >= 0 {
		return nil
	}
	return badRunRequest("Update the PatchMon agent on this host to v" + MinWindowsPatchAgentVersion + " or later to patch Windows")
}

func isValidWindowsPackageName(s string) bool {
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxWindowsPackageNameLen {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func resolveWindowsScope(scope *windowsScope) (updates, apps bool, err *runRequestError) {
	updates, apps = true, true
	if scope != nil {
		if scope.Updates != nil {
			updates = *scope.Updates
		}
		if scope.Apps != nil {
			apps = *scope.Apps
		}
	}
	if !updates && !apps {
		return false, false, badRunRequest("Select Windows Updates, Applications (winget), or both")
	}
	return updates, apps, nil
}

type windowsTargetResolver func(names []string) ([]db.ResolveWindowsPatchTargetsRow, error)

// planWindowsRun validates a Windows run request and resolves display names to patch identifiers.
func planWindowsRun(agentVersion *string, patchType string, names []string, scope *windowsScope, resolve windowsTargetResolver) (*windowsRunPlan, *runRequestError, error) {
	if verr := windowsAgentVersionError(agentVersion); verr != nil {
		return nil, verr, nil
	}
	if patchType == "patch_all" {
		updates, apps, serr := resolveWindowsScope(scope)
		if serr != nil {
			return nil, serr, nil
		}
		return &windowsRunPlan{IncludeUpdates: &updates, IncludeApps: &apps}, nil, nil
	}

	if len(names) == 0 {
		return nil, badRunRequest("Valid package_name or non-empty package_names is required for patch_package"), nil
	}
	if len(names) > maxPatchTargets {
		return nil, badRunRequest("package_names limited to 100 packages per run"), nil
	}
	unique := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if !isValidWindowsPackageName(n) {
			return nil, badRunRequest("Every package_names entry must be a valid package name"), nil
		}
		if !seen[n] {
			seen[n] = true
			unique = append(unique, n)
		}
	}

	rows, err := resolve(unique)
	if err != nil {
		return nil, nil, err
	}
	items, rejects := classifyWindowsTargets(unique, rows)
	if len(rejects) > 0 {
		return nil, &runRequestError{Status: http.StatusBadRequest, Body: map[string]interface{}{
			"error":    "Some selected packages cannot be patched on this host. Refresh the host's inventory and select again.",
			"rejected": rejects,
		}}, nil
	}
	return &windowsRunPlan{Items: items}, nil, nil
}

func classifyWindowsTargets(names []string, rows []db.ResolveWindowsPatchTargetsRow) ([]store.NewPatchRunItem, []windowsTargetReject) {
	byName := make(map[string]db.ResolveWindowsPatchTargetsRow, len(rows))
	for _, r := range rows {
		byName[r.Name] = r
	}
	var items []store.NewPatchRunItem
	var rejects []windowsTargetReject
	for _, name := range names {
		r, ok := byName[name]
		if !ok {
			rejects = append(rejects, windowsTargetReject{Name: name, Reason: rejectNotFound})
			continue
		}
		if !r.Patchable {
			reason := r.NotPatchableReason
			if reason == "" {
				reason = rejectNoWingetID
			}
			rejects = append(rejects, windowsTargetReject{Name: name, Reason: reason})
			continue
		}
		item := store.NewPatchRunItem{Title: name}
		switch {
		case r.WuaGuid != nil && *r.WuaGuid != "":
			if !store.IsValidWUAGuid(*r.WuaGuid) {
				rejects = append(rejects, windowsTargetReject{Name: name, Reason: rejectInvalidIdentifier})
				continue
			}
			item.Kind = store.PatchItemKindWUA
			item.Identifier = strings.ToLower(*r.WuaGuid)
			if r.WuaRevisionNumber != nil {
				item.RevisionNumber = *r.WuaRevisionNumber
			}
			if r.WuaKb != nil {
				item.KB = *r.WuaKb
			}
		case r.WingetID != nil && *r.WingetID != "":
			if !store.IsValidWingetID(*r.WingetID) {
				rejects = append(rejects, windowsTargetReject{Name: name, Reason: rejectInvalidIdentifier})
				continue
			}
			item.Kind = store.PatchItemKindWinget
			item.Identifier = *r.WingetID
		default:
			rejects = append(rejects, windowsTargetReject{Name: name, Reason: rejectNoWingetID})
			continue
		}
		items = append(items, item)
	}
	return dedupeItems(items), rejects
}

func dedupeItems(items []store.NewPatchRunItem) []store.NewPatchRunItem {
	seen := make(map[string]bool, len(items))
	out := items[:0]
	for _, it := range items {
		key := it.Kind + "\x00" + it.Identifier
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
	}
	return out
}

func (h *PatchingHandler) validateWindowsRun(ctx context.Context, hostID string, agentVersion *string, patchType string, names []string, scope *windowsScope) (*windowsRunPlan, *runRequestError, error) {
	return planWindowsRun(agentVersion, patchType, names, scope, func(n []string) ([]db.ResolveWindowsPatchTargetsRow, error) {
		return h.patchRuns.ResolveWindowsPatchTargets(ctx, hostID, n)
	})
}

// itemsForApproval picks the validation run's items that the approved run should install.
func itemsForApproval(validationStatus string, items []db.PatchRunItem) []store.NewPatchRunItem {
	var out []store.NewPatchRunItem
	for _, it := range items {
		keep := it.Status == "would_install"
		if validationStatus != "validated" {
			keep = keep || it.Status == "pending"
		}
		if !keep {
			continue
		}
		n := store.NewPatchRunItem{Kind: it.Kind, Identifier: it.Identifier, Title: it.Title}
		if it.RevisionNumber != nil {
			n.RevisionNumber = *it.RevisionNumber
		}
		if it.Kb != nil {
			n.KB = *it.Kb
		}
		out = append(out, n)
	}
	return out
}

// revalidationItems is what a retried Windows validation starts from: the freshly resolved targets
// for patch_package, and nothing for patch_all, whose items the agent re-reports.
func revalidationItems(patchType string, plan *windowsRunPlan) []store.NewPatchRunItem {
	if patchType != "patch_package" || plan == nil {
		return nil
	}
	return plan.Items
}

func itemTitles(items []store.NewPatchRunItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

// loadHost writes 404 only for a host that does not exist; a failed lookup is logged and answered with 500.
func loadHost(w http.ResponseWriter, r *http.Request, hosts *store.HostsStore, hostID string) (*models.Host, bool) {
	host, err := hosts.GetByID(r.Context(), hostID)
	if err != nil {
		slog.Error("host lookup failed", "host_id", hostID, "path", r.URL.Path, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load host"})
		return nil, false
	}
	if host == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Host not found"})
		return nil, false
	}
	return host, true
}

func writeRunRequestError(w http.ResponseWriter, e *runRequestError) {
	JSON(w, e.Status, e.Body)
}
