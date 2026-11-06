package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// MinExcludePackagesAgentVersion is the oldest agent that honours exclude_packages
// on a patch_all run. Older agents ignore the field and would upgrade held packages.
const MinExcludePackagesAgentVersion = "2.2.0"

const minExcludePackagesAgentFloor = MinExcludePackagesAgentVersion + "-0"

const maxGroupPackageHolds = 200

type queuePatchRequest struct {
	Host               *models.Host
	PatchType          string
	PackageName        string
	PackageNames       []string
	DryRun             bool
	PendingApproval    bool
	ScheduleOverride   string
	WindowsScope       *windowsScope
	ForwardedHost      string
	UserID             string
	SkipWhenNoPackages bool
}

type queuePatchResult struct {
	Status          string
	Message         string
	PatchRunID      string
	FriendlyName    string
	PendingApproval bool
	Body            map[string]interface{}
}

// TriggerGroup handles POST /patching/groups/{id}/trigger.
// Each current member gets its own patch run. One host's failure does not stop the others.
func (h *PatchingHandler) TriggerGroup(w http.ResponseWriter, r *http.Request) {
	groupID := chi.URLParam(r, "id")
	if !isValidPatchUUID(groupID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid host group ID"})
		return
	}
	if h.hostGroups == nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Host groups are not available"})
		return
	}
	if _, err := h.hostGroups.GetByID(r.Context(), groupID); err != nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Host group not found"})
		return
	}
	var body struct {
		Mode             string `json:"mode"`
		DryRun           bool   `json:"dry_run"`
		PendingApproval  bool   `json:"pending_approval"`
		ScheduleOverride string `json:"schedule_override"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if body.Mode != "patch_all" && body.Mode != "security" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be patch_all or security"})
		return
	}
	if body.DryRun && body.PendingApproval {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "dry_run and pending_approval are mutually exclusive"})
		return
	}
	if body.DryRun && body.Mode == "patch_all" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "dry_run is only supported for security mode"})
		return
	}

	hostIDs, err := h.hostGroups.GetHostIDs(r.Context(), groupID)
	if err != nil {
		h.log.Error("patching: list group members error", "group_id", groupID, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load hosts in group"})
		return
	}

	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	forwarded := r.Header.Get("X-Forwarded-Host")
	results := make([]map[string]interface{}, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		results = append(results, h.triggerGroupMember(r.Context(), hostID, body.Mode, body.DryRun, body.PendingApproval, body.ScheduleOverride, forwarded, userID))
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"group_id": groupID,
		"mode":     body.Mode,
		"results":  results,
	})
}

func (h *PatchingHandler) triggerGroupMember(ctx context.Context, hostID, mode string, dryRun, pending bool, scheduleOverride, forwarded, userID string) map[string]interface{} {
	row := map[string]interface{}{"host_id": hostID}
	host, err := h.hosts.GetByID(ctx, hostID)
	if err != nil || host == nil {
		row["status"] = "error"
		row["message"] = "Host not found"
		return row
	}
	row["friendly_name"] = host.FriendlyName

	req := queuePatchRequest{
		Host:               host,
		DryRun:             dryRun,
		PendingApproval:    pending,
		ScheduleOverride:   scheduleOverride,
		ForwardedHost:      forwarded,
		UserID:             userID,
		SkipWhenNoPackages: true,
	}
	switch mode {
	case "patch_all":
		req.PatchType = "patch_all"
	case "security":
		names, err := h.hostGroups.SecurityUpdatePackageNames(ctx, host.ID)
		if err != nil {
			h.log.Error("patching: list security packages error", "host_id", host.ID, "error", err)
			row["status"] = "error"
			row["message"] = "Failed to load security updates"
			return row
		}
		if len(names) == 0 {
			row["status"] = "skipped"
			row["message"] = "No security updates"
			return row
		}
		if len(names) > maxPatchTargets {
			row["status"] = "error"
			row["message"] = "package_names limited to 100 packages per run"
			return row
		}
		req.PatchType = "patch_package"
		req.PackageNames = names
	default:
		row["status"] = "error"
		row["message"] = "mode must be patch_all or security"
		return row
	}

	res, reqErr, err := h.queuePatch(ctx, req)
	if err != nil {
		h.log.Error("patching: group member trigger error", "host_id", host.ID, "error", err)
		row["status"] = "error"
		row["message"] = "Failed to queue patch"
		return row
	}
	if reqErr != nil {
		status := "error"
		if reqErr.Status == http.StatusConflict {
			status = "conflict"
		}
		row["status"] = status
		if msg, ok := reqErr.Body["error"].(string); ok {
			row["message"] = msg
		}
		return row
	}
	row["status"] = res.Status
	row["message"] = res.Message
	if res.PatchRunID != "" {
		row["patch_run_id"] = res.PatchRunID
	}
	return row
}

func (h *PatchingHandler) queuePatch(ctx context.Context, req queuePatchRequest) (*queuePatchResult, *runRequestError, error) {
	host := req.Host
	if host == nil {
		return nil, badRunRequest("Host not found"), nil
	}
	if req.PatchType != "patch_all" && req.PatchType != "patch_package" {
		return nil, badRunRequest("patch_type must be patch_all or patch_package"), nil
	}
	if req.DryRun && req.PendingApproval {
		return nil, badRunRequest("dry_run and pending_approval are mutually exclusive"), nil
	}
	if e := selectedPatchRefusal(host, req.PatchType); e != nil {
		return nil, e, nil
	}
	windows := store.IsWindowsOSType(host.OSType)
	if req.DryRun && req.PatchType != "patch_package" && !windows {
		return nil, badRunRequest("dry_run is only supported for patch_package"), nil
	}

	holds, err := h.effectiveHolds(ctx, host.ID)
	if err != nil {
		return nil, nil, err
	}
	var heldBack []string
	if req.PatchType == "patch_package" && len(holds) > 0 {
		names := req.PackageNames
		if len(names) == 0 && req.PackageName != "" {
			names = []string{req.PackageName}
		}
		keep, back := subtractPackageNames(names, holds)
		heldBack = back
		if len(names) > 0 && len(keep) == 0 {
			if req.SkipWhenNoPackages {
				return &queuePatchResult{
					Status:       "skipped",
					Message:      "Every selected package is held by a host group",
					FriendlyName: host.FriendlyName,
				}, nil, nil
			}
			return nil, badRunRequest("Every selected package is held by a host group"), nil
		}
		if len(back) > 0 {
			req.PackageNames = keep
			req.PackageName = ""
		}
	}
	var excludes []string
	if req.PatchType == "patch_all" && len(holds) > 0 {
		if windows {
			return nil, badRunRequest("Package holds are not applied on Windows, so Patch all was not queued for this host"), nil
		}
		if msg := excludeAgentRefusal(host); msg != "" {
			return nil, badRunRequest(msg), nil
		}
		excludes = holds
	}

	var pkgName *string
	var pkgNames []string
	var createOpts *store.CreateRunOpts
	if windows {
		names := req.PackageNames
		if len(names) == 0 && req.PackageName != "" {
			names = []string{req.PackageName}
		}
		plan, reqErr, err := h.validateWindowsRun(ctx, host.ID, host.AgentVersion, req.PatchType, names, req.WindowsScope)
		if err != nil {
			return nil, nil, err
		}
		if reqErr != nil {
			return nil, reqErr, nil
		}
		if req.PatchType == "patch_package" {
			pkgNames = itemTitles(plan.Items)
		}
		createOpts = &store.CreateRunOpts{
			Items:                 plan.Items,
			WindowsIncludeUpdates: plan.IncludeUpdates,
			WindowsIncludeApps:    plan.IncludeApps,
		}
	} else if req.PatchType == "patch_package" {
		if len(req.PackageNames) > 0 {
			for _, n := range req.PackageNames {
				if !isValidPackageName(n) {
					return nil, badRunRequest("Every package_names entry must be a valid package name"), nil
				}
			}
			if len(req.PackageNames) > maxPatchTargets {
				return nil, badRunRequest("package_names limited to 100 packages per run"), nil
			}
			pkgNames = req.PackageNames
		} else if req.PackageName != "" && isValidPackageName(req.PackageName) {
			pkgName = &req.PackageName
		} else {
			return nil, badRunRequest("Valid package_name or non-empty package_names is required for patch_package"), nil
		}
	}
	if len(excludes) > 0 {
		if createOpts == nil {
			createOpts = &store.CreateRunOpts{}
		}
		createOpts.ExcludePackages = excludes
	}

	orgTZ := h.resolveOrgTimezone(ctx)
	policy, _ := h.patchPolicies.ResolveEffectivePolicy(ctx, host.ID)
	runAt, schedMeta := h.patchPolicies.ComputeRunAtWithMeta(policy, orgTZ)
	if schedMeta.Err != nil {
		h.log.Warn("patching: fixed_time_utc parse failed - running immediately",
			"policy_id", policyIDForLog(policy), "fixed_time_utc", fixedTimeForLog(policy), "error", schedMeta.Err)
	}
	delayMs := time.Until(runAt).Milliseconds()
	if delayMs < 0 {
		delayMs = 0
	}
	if req.DryRun || req.PendingApproval {
		delayMs = 0
	}
	if req.ScheduleOverride == "immediate" {
		delayMs = 0
	}
	if delayMs == 0 {
		runAt = time.Now()
	}

	patchRunID := uuid.New().String()
	jobID := patchRunJobIDPrefix + patchRunID
	var triggeredBy *string
	if req.UserID != "" {
		triggeredBy = &req.UserID
	}
	var scheduledAt *time.Time
	if delayMs > 0 {
		scheduledAt = &runAt
	}
	var policyID, policyNamePtr *string
	var policySnapshot []byte
	if policy != nil {
		policyID = &policy.ID
		snap := map[string]interface{}{
			"name":             policy.Name,
			"patch_delay_type": policy.PatchDelayType,
			"delay_minutes":    policy.DelayMinutes,
			"fixed_time_utc":   policy.FixedTimeUtc,
			"timezone":         policy.Timezone,
		}
		if policy.PatchDelayType == "fixed_time" {
			snap["schedule_timezone"] = schedMeta.Timezone
		}
		policySnapshot, _ = json.Marshal(snap)
		n := policy.Name
		policyNamePtr = &n
	} else {
		policySnapshot, _ = json.Marshal(map[string]interface{}{
			"patch_delay_type": "immediate",
		})
	}
	if req.PendingApproval {
		if createOpts == nil {
			createOpts = &store.CreateRunOpts{}
		}
		createOpts.InitialStatus = "pending_approval"
	}
	if _, err := h.patchRuns.CreateRun(ctx, patchRunID, host.ID, jobID, req.PatchType, pkgName, pkgNames, triggeredBy, req.DryRun, scheduledAt, policyID, policyNamePtr, policySnapshot, createOpts); err != nil {
		if errors.Is(err, store.ErrHostHasActivePatchRun) {
			return nil, &runRequestError{Status: http.StatusConflict, Body: map[string]interface{}{"error": activePatchRunMessage}}, nil
		}
		return nil, nil, err
	}

	status := "queued"
	msg := "Patch queued"
	queued := true
	if req.PendingApproval {
		status = "pending_approval"
		msg = "Submitted for approval"
		queued = false
	} else if req.DryRun {
		msg = "Dry run queued"
	} else if delayMs > 0 {
		status = "scheduled"
		msg = "Patch scheduled for " + runAt.UTC().Format(time.RFC3339)
	}

	if !req.PendingApproval {
		task, err := queue.NewRunPatchTask(queue.RunPatchPayload{
			HostID:          host.ID,
			Host:            req.ForwardedHost,
			ApiID:           host.ApiID,
			PatchRunID:      patchRunID,
			PatchType:       req.PatchType,
			PackageName:     pkgName,
			PackageNames:    pkgNames,
			ExcludePackages: excludes,
			DryRun:          req.DryRun,
		})
		if err != nil {
			h.cancelOrphanedRun(ctx, patchRunID)
			return nil, nil, err
		}
		opts := []asynq.Option{}
		if delayMs > 0 {
			opts = append(opts, asynq.ProcessIn(time.Duration(delayMs)*time.Millisecond))
		}
		if _, err := h.queueClient.Enqueue(task, opts...); err != nil {
			h.cancelOrphanedRun(ctx, patchRunID)
			return nil, nil, err
		}
	}

	body := map[string]interface{}{
		"message":          msg,
		"patch_run_id":     patchRunID,
		"job_id":           jobID,
		"run_at":           runAt.UTC().Format(time.RFC3339),
		"queued":           queued,
		"pending_approval": req.PendingApproval,
	}
	if len(heldBack) > 0 {
		body["held_back"] = heldBack
	}
	return &queuePatchResult{
		Status:          status,
		Message:         msg,
		PatchRunID:      patchRunID,
		FriendlyName:    host.FriendlyName,
		PendingApproval: req.PendingApproval,
		Body:            body,
	}, nil, nil
}

func (h *PatchingHandler) effectiveHolds(ctx context.Context, hostID string) ([]string, error) {
	if h.hostGroups == nil {
		return nil, nil
	}
	return h.hostGroups.EffectivePackageHolds(ctx, hostID)
}

func excludeAgentRefusal(host *models.Host) string {
	v := ""
	if host != nil && host.AgentVersion != nil {
		v = strings.TrimPrefix(strings.TrimSpace(*host.AgentVersion), "v")
	}
	if v != "" && util.CompareVersions(v, minExcludePackagesAgentFloor) >= 0 {
		return ""
	}
	return "Update the PatchMon agent on this host to v" + MinExcludePackagesAgentVersion + " or later before Patch all can skip held packages"
}

func subtractPackageNames(names, holds []string) (keep, heldBack []string) {
	if len(holds) == 0 {
		return append([]string(nil), names...), nil
	}
	held := make(map[string]struct{}, len(holds))
	for _, n := range holds {
		held[n] = struct{}{}
	}
	for _, n := range names {
		if _, ok := held[n]; ok {
			heldBack = append(heldBack, n)
			continue
		}
		keep = append(keep, n)
	}
	return keep, heldBack
}
