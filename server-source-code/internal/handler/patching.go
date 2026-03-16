package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

const patchRunJobIDPrefix = "patch-run-"

var packageNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.+-_]*$`)

// PatchingHandler handles patching endpoints.
type PatchingHandler struct {
	patchRuns     *store.PatchRunsStore
	patchPolicies *store.PatchPoliciesStore
	assignments   *store.PatchPolicyAssignmentsStore
	exclusions    *store.PatchPolicyExclusionsStore
	hosts         *store.HostsStore
	queueClient   *asynq.Client
	log           *slog.Logger
}

// NewPatchingHandler creates a new patching handler.
func NewPatchingHandler(
	patchRuns *store.PatchRunsStore,
	patchPolicies *store.PatchPoliciesStore,
	assignments *store.PatchPolicyAssignmentsStore,
	exclusions *store.PatchPolicyExclusionsStore,
	hosts *store.HostsStore,
	queueClient *asynq.Client,
	log *slog.Logger,
) *PatchingHandler {
	if log == nil {
		log = slog.Default()
	}
	return &PatchingHandler{
		patchRuns:     patchRuns,
		patchPolicies: patchPolicies,
		assignments:   assignments,
		exclusions:    exclusions,
		hosts:         hosts,
		queueClient:   queueClient,
		log:           log,
	}
}

func isValidPatchUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func isValidPackageName(s string) bool {
	return len(s) > 0 && len(s) <= 256 && packageNameRegex.MatchString(s)
}

// ServePatchOutput handles POST /patching/runs/:id/output (agent-facing, API key auth).
func (h *PatchingHandler) ServePatchOutput(w http.ResponseWriter, r *http.Request) {
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "API ID and Key required"})
		return
	}
	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}
	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API credentials"})
		return
	}

	patchRunID := chi.URLParam(r, "id")
	if !isValidPatchUUID(patchRunID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid patch run ID"})
		return
	}

	var body struct {
		Stage        string `json:"stage"`
		Output       string `json:"output"`
		ErrorMessage string `json:"error_message"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}

	run, err := h.patchRuns.GetByID(r.Context(), patchRunID)
	if err != nil || run == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Patch run not found"})
		return
	}
	if run.HostID != host.ID {
		JSON(w, http.StatusForbidden, map[string]string{"error": "Patch run does not belong to this host"})
		return
	}

	if err := h.patchRuns.UpdateOutput(r.Context(), patchRunID, body.Stage, body.Output, body.ErrorMessage); err != nil {
		h.log.Error("patching: failed to update output", "patch_run_id", patchRunID, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save patch output"})
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Dashboard returns GET /patching/dashboard.
func (h *PatchingHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	total, byStatus, recent, active, err := h.patchRuns.GetDashboard(r.Context())
	if err != nil {
		h.log.Error("patching: dashboard error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load patching dashboard"})
		return
	}
	statusCounts := map[string]int{
		"queued": 0, "running": 0, "completed": 0, "failed": 0, "cancelled": 0,
	}
	for k, v := range byStatus {
		statusCounts[k] = v
	}
	summary := map[string]interface{}{
		"total_runs": total,
		"queued":     statusCounts["queued"],
		"running":    statusCounts["running"],
		"completed":  statusCounts["completed"],
		"failed":     statusCounts["failed"],
		"cancelled":  statusCounts["cancelled"],
	}
	recentResp := patchRunsToResponse(recent)
	activeResp := patchRunsActiveToResponse(active)
	JSON(w, http.StatusOK, map[string]interface{}{
		"summary":     summary,
		"recent_runs": recentResp,
		"active_runs": activeResp,
	})
}

// PreviewRun returns GET /patching/preview-run?host_id=.
func (h *PatchingHandler) PreviewRun(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host_id")
	if hostID == "" || !isValidPatchUUID(hostID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Valid host_id is required"})
		return
	}
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Host not found"})
		return
	}
	policy, _ := h.patchPolicies.ResolveEffectivePolicy(r.Context(), hostID)
	runAt := h.patchPolicies.ComputeRunAt(policy)
	hostName := host.FriendlyName
	if hostName == "" && host.Hostname != nil {
		hostName = *host.Hostname
	}
	if hostName == "" {
		hostName = hostID
	}
	policyName := "Default (immediate)"
	policyID := ""
	patchDelayType := "immediate"
	if policy != nil {
		policyName = policy.Name
		policyID = policy.ID
		patchDelayType = policy.PatchDelayType
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"host_id":          hostID,
		"host_name":        hostName,
		"run_at_iso":       runAt.UTC().Format(time.RFC3339),
		"policy_name":      policyName,
		"policy_id":        policyID,
		"patch_delay_type": patchDelayType,
	})
}

// ActiveRuns returns GET /patching/runs/active.
func (h *PatchingHandler) ActiveRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.patchRuns.ListActive(r.Context())
	if err != nil {
		h.log.Error("patching: active runs error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load active runs"})
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"runs": patchRunsActiveToResponse(runs)})
}

// ListRuns returns GET /patching/runs.
func (h *PatchingHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQuery(r, "limit", 50)
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	offset := parseIntQuery(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}
	hostID := r.URL.Query().Get("host_id")
	status := r.URL.Query().Get("status")
	patchType := r.URL.Query().Get("patch_type")
	sortBy := r.URL.Query().Get("sort_by")
	sortDir := r.URL.Query().Get("sort_dir")
	if hostID != "" && !isValidPatchUUID(hostID) {
		hostID = ""
	}
	if patchType != "" && patchType != "patch_all" && patchType != "patch_package" {
		patchType = ""
	}
	// Validate sort params
	validSortBy := map[string]bool{"created_at": true, "started_at": true, "completed_at": true, "status": true}
	if !validSortBy[sortBy] {
		sortBy = "created_at"
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "desc"
	}

	runs, total, err := h.patchRuns.List(r.Context(), hostID, status, patchType, sortBy, sortDir, limit, offset)
	if err != nil {
		h.log.Error("patching: runs list error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load patch runs"})
		return
	}
	pages := int(total) / limit
	if int(total)%limit > 0 {
		pages++
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"runs": patchRunsListToResponse(runs),
		"pagination": map[string]interface{}{
			"total":  total,
			"limit":  limit,
			"offset": offset,
			"pages":  pages,
		},
	})
}

// GetRun returns GET /patching/runs/:id.
func (h *PatchingHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isValidPatchUUID(id) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid run ID"})
		return
	}
	run, err := h.patchRuns.GetByID(r.Context(), id)
	if err != nil || run == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Patch run not found"})
		return
	}
	JSON(w, http.StatusOK, patchRunToResponse(run))
}

// ApproveRun handles POST /patching/runs/:id/approve.
// Transitions a validated dry-run run back to queued so it gets executed.
func (h *PatchingHandler) ApproveRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isValidPatchUUID(id) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid run ID"})
		return
	}
	run, err := h.patchRuns.GetByID(r.Context(), id)
	if err != nil || run == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Patch run not found"})
		return
	}
	if run.Status != "validated" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Only validated runs can be approved"})
		return
	}
	var approvedBy *string
	if userID, _ := r.Context().Value(middleware.UserIDKey).(string); userID != "" {
		approvedBy = &userID
	}
	if err := h.patchRuns.ApproveRun(r.Context(), id, approvedBy); err != nil {
		h.log.Error("patching: approve run error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to approve run"})
		return
	}
	// Re-enqueue the actual patch job (non-dry-run)
	host, err := h.hosts.GetByID(r.Context(), run.HostID)
	if err != nil || host == nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Host not found for run"})
		return
	}
	var pkgName *string
	if run.PackageName != nil {
		pkgName = run.PackageName
	}
	var pkgNames []string
	if len(run.PackageNames) > 0 {
		_ = json.Unmarshal(run.PackageNames, &pkgNames)
	}
	task, err := queue.NewRunPatchTask(queue.RunPatchPayload{
		HostID:       run.HostID,
		Host:         r.Header.Get("X-Forwarded-Host"),
		ApiID:        host.ApiID,
		PatchRunID:   id,
		PatchType:    run.PatchType,
		PackageName:  pkgName,
		PackageNames: pkgNames,
		DryRun:       false,
	})
	if err != nil {
		h.log.Error("patching: create approve task error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create patch task"})
		return
	}
	if _, err := h.queueClient.Enqueue(task); err != nil {
		h.log.Error("patching: enqueue approve error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to queue approved patch"})
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"message": "Run approved and queued", "patch_run_id": id})
}

func (h *PatchingHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	var body struct {
		HostID           string   `json:"host_id"`
		PatchType        string   `json:"patch_type"`
		PackageName      string   `json:"package_name"`
		PackageNames     []string `json:"package_names"`
		DryRun           bool     `json:"dry_run"`
		ScheduleOverride string   `json:"schedule_override"` // "immediate" to bypass policy delay
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if body.HostID == "" || !isValidPatchUUID(body.HostID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Valid host_id is required"})
		return
	}
	if body.PatchType != "patch_all" && body.PatchType != "patch_package" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "patch_type must be patch_all or patch_package"})
		return
	}
	if body.DryRun && body.PatchType != "patch_package" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "dry_run is only supported for patch_package"})
		return
	}

	var pkgName *string
	var pkgNames []string
	if body.PatchType == "patch_package" {
		if len(body.PackageNames) > 0 {
			for _, n := range body.PackageNames {
				if !isValidPackageName(n) {
					JSON(w, http.StatusBadRequest, map[string]string{"error": "Every package_names entry must be a valid package name"})
					return
				}
			}
			if len(body.PackageNames) > 100 {
				JSON(w, http.StatusBadRequest, map[string]string{"error": "package_names limited to 100 packages per run"})
				return
			}
			pkgNames = body.PackageNames
		} else if body.PackageName != "" && isValidPackageName(body.PackageName) {
			pkgName = &body.PackageName
		} else {
			JSON(w, http.StatusBadRequest, map[string]string{"error": "Valid package_name or non-empty package_names is required for patch_package"})
			return
		}
	}

	host, err := h.hosts.GetByID(r.Context(), body.HostID)
	if err != nil || host == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Host not found"})
		return
	}

	policy, _ := h.patchPolicies.ResolveEffectivePolicy(r.Context(), body.HostID)
	runAt := h.patchPolicies.ComputeRunAt(policy)
	delayMs := time.Until(runAt).Milliseconds()
	if delayMs < 0 {
		delayMs = 0
	}
	// Dry runs run immediately (no policy delay)
	if body.DryRun {
		delayMs = 0
	}
	// Manual schedule override: "immediate" bypasses policy delay
	if body.ScheduleOverride == "immediate" {
		delayMs = 0
	}

	patchRunID := uuid.New().String()
	jobID := patchRunJobIDPrefix + patchRunID
	var triggeredBy *string
	if userID, _ := r.Context().Value(middleware.UserIDKey).(string); userID != "" {
		triggeredBy = &userID
	}
	var scheduledAt *time.Time
	if delayMs > 0 {
		scheduledAt = &runAt
	}
	_, err = h.patchRuns.CreateRun(r.Context(), patchRunID, body.HostID, jobID, body.PatchType, pkgName, pkgNames, triggeredBy, body.DryRun, scheduledAt)
	if err != nil {
		h.log.Error("patching: create run error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create patch run"})
		return
	}
	jobID = patchRunJobIDPrefix + patchRunID

	task, err := queue.NewRunPatchTask(queue.RunPatchPayload{
		HostID:       body.HostID,
		Host:         r.Header.Get("X-Forwarded-Host"),
		ApiID:        host.ApiID,
		PatchRunID:   patchRunID,
		PatchType:    body.PatchType,
		PackageName:  pkgName,
		PackageNames: pkgNames,
		DryRun:       body.DryRun,
	})
	if err != nil {
		h.log.Error("patching: create task error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Patch queue is not available"})
		return
	}
	opts := []asynq.Option{}
	if delayMs > 0 {
		opts = append(opts, asynq.ProcessIn(time.Duration(delayMs)*time.Millisecond))
	}
	if _, err := h.queueClient.Enqueue(task, opts...); err != nil {
		h.log.Error("patching: enqueue error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to queue patch"})
		return
	}

	msg := "Patch queued"
	if body.DryRun {
		msg = "Dry run queued"
	} else if delayMs > 0 {
		msg = "Patch scheduled for " + runAt.UTC().Format(time.RFC3339)
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":      msg,
		"patch_run_id": patchRunID,
		"job_id":       jobID,
		"run_at":       runAt.UTC().Format(time.RFC3339),
		"queued":       true,
	})
}

// ListPolicies returns GET /patching/policies.
func (h *PatchingHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := h.patchPolicies.List(r.Context())
	if err != nil {
		h.log.Error("patching: policies list error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load policies"})
		return
	}
	// Include counts for assignments and exclusions
	out := make([]map[string]interface{}, len(policies))
	for i, p := range policies {
		assignments, _ := h.assignments.ListByPolicy(r.Context(), p.ID)
		exclusions, _ := h.exclusions.ListByPolicy(r.Context(), p.ID)
		out[i] = map[string]interface{}{
			"id":               p.ID,
			"name":             p.Name,
			"description":      p.Description,
			"patch_delay_type": p.PatchDelayType,
			"delay_minutes":    p.DelayMinutes,
			"fixed_time_utc":   p.FixedTimeUtc,
			"timezone":         p.Timezone,
			"created_at":       pgTimeToISO(p.CreatedAt),
			"updated_at":       pgTimeToISO(p.UpdatedAt),
			"_count":           map[string]int{"assignments": len(assignments), "exclusions": len(exclusions)},
		}
	}
	JSON(w, http.StatusOK, out)
}

// GetPolicy returns GET /patching/policies/:id.
func (h *PatchingHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isValidPatchUUID(id) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid policy ID"})
		return
	}
	policy, err := h.patchPolicies.GetByID(r.Context(), id)
	if err != nil || policy == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Policy not found"})
		return
	}
	assignments, _ := h.assignments.ListByPolicy(r.Context(), id)
	exclRows, _ := h.exclusions.ListByPolicy(r.Context(), id)
	exclusions := make([]map[string]interface{}, len(exclRows))
	for i, e := range exclRows {
		exclusions[i] = map[string]interface{}{
			"id":         e.ID,
			"host_id":    e.HostID,
			"created_at": pgTimeToISO(e.CreatedAt),
			"updated_at": pgTimeToISO(e.UpdatedAt),
			"hosts": map[string]interface{}{
				"id":            e.HostID,
				"friendly_name": e.HostFriendlyName,
				"hostname":      e.HostHostname,
			},
		}
	}
	assignmentsResp := make([]map[string]interface{}, len(assignments))
	for i, a := range assignments {
		assignmentsResp[i] = map[string]interface{}{
			"id":          a.ID,
			"target_type": a.TargetType,
			"target_id":   a.TargetID,
			"created_at":  pgTimeToISO(a.CreatedAt),
			"updated_at":  pgTimeToISO(a.UpdatedAt),
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"id":               policy.ID,
		"name":             policy.Name,
		"description":      policy.Description,
		"patch_delay_type": policy.PatchDelayType,
		"delay_minutes":    policy.DelayMinutes,
		"fixed_time_utc":   policy.FixedTimeUtc,
		"timezone":         policy.Timezone,
		"created_at":       pgTimeToISO(policy.CreatedAt),
		"updated_at":       pgTimeToISO(policy.UpdatedAt),
		"assignments":      assignmentsResp,
		"exclusions":       exclusions,
	})
}

// CreatePolicy handles POST /patching/policies.
func (h *PatchingHandler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name           string  `json:"name"`
		Description    string  `json:"description"`
		PatchDelayType string  `json:"patch_delay_type"`
		DelayMinutes   *int32  `json:"delay_minutes"`
		FixedTimeUtc   *string `json:"fixed_time_utc"`
		Timezone       *string `json:"timezone"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if body.Name == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if body.PatchDelayType != "immediate" && body.PatchDelayType != "delayed" && body.PatchDelayType != "fixed_time" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "patch_delay_type must be immediate, delayed, or fixed_time"})
		return
	}
	if body.PatchDelayType == "delayed" && (body.DelayMinutes == nil || *body.DelayMinutes < 0) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "delay_minutes is required for delayed policy"})
		return
	}
	if body.PatchDelayType == "fixed_time" && (body.FixedTimeUtc == nil || *body.FixedTimeUtc == "") {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "fixed_time_utc is required for fixed_time policy"})
		return
	}

	id, err := h.patchPolicies.Create(r.Context(), body.Name, body.Description, body.PatchDelayType, body.DelayMinutes, body.FixedTimeUtc, body.Timezone)
	if err != nil {
		h.log.Error("patching: create policy error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create policy"})
		return
	}
	policy, _ := h.patchPolicies.GetByID(r.Context(), id)
	resp := map[string]interface{}{
		"id":               id,
		"name":             body.Name,
		"description":      body.Description,
		"patch_delay_type": body.PatchDelayType,
		"delay_minutes":    body.DelayMinutes,
		"fixed_time_utc":   body.FixedTimeUtc,
		"timezone":         body.Timezone,
		"created_at":       time.Now().UTC().Format(time.RFC3339),
		"updated_at":       time.Now().UTC().Format(time.RFC3339),
	}
	if policy != nil {
		resp["created_at"] = pgTimeToISO(policy.CreatedAt)
		resp["updated_at"] = pgTimeToISO(policy.UpdatedAt)
	}
	JSON(w, http.StatusCreated, resp)
}

// UpdatePolicy handles PUT /patching/policies/:id.
func (h *PatchingHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isValidPatchUUID(id) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid policy ID"})
		return
	}
	var body struct {
		Name           string  `json:"name"`
		Description    string  `json:"description"`
		PatchDelayType string  `json:"patch_delay_type"`
		DelayMinutes   *int32  `json:"delay_minutes"`
		FixedTimeUtc   *string `json:"fixed_time_utc"`
		Timezone       *string `json:"timezone"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if body.Name == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if body.PatchDelayType != "immediate" && body.PatchDelayType != "delayed" && body.PatchDelayType != "fixed_time" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "patch_delay_type must be immediate, delayed, or fixed_time"})
		return
	}
	if body.PatchDelayType == "delayed" && (body.DelayMinutes == nil || *body.DelayMinutes < 0) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "delay_minutes is required for delayed policy"})
		return
	}
	if body.PatchDelayType == "fixed_time" && (body.FixedTimeUtc == nil || *body.FixedTimeUtc == "") {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "fixed_time_utc is required for fixed_time policy"})
		return
	}

	if err := h.patchPolicies.Update(r.Context(), id, body.Name, body.Description, body.PatchDelayType, body.DelayMinutes, body.FixedTimeUtc, body.Timezone); err != nil {
		h.log.Error("patching: update policy error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update policy"})
		return
	}
	policy, _ := h.patchPolicies.GetByID(r.Context(), id)
	resp := map[string]interface{}{
		"id":               id,
		"name":             body.Name,
		"description":      body.Description,
		"patch_delay_type": body.PatchDelayType,
		"delay_minutes":    body.DelayMinutes,
		"fixed_time_utc":   body.FixedTimeUtc,
		"timezone":         body.Timezone,
	}
	if policy != nil {
		resp["created_at"] = pgTimeToISO(policy.CreatedAt)
		resp["updated_at"] = pgTimeToISO(policy.UpdatedAt)
	}
	JSON(w, http.StatusOK, resp)
}

// DeletePolicy handles DELETE /patching/policies/:id.
func (h *PatchingHandler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isValidPatchUUID(id) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid policy ID"})
		return
	}
	if err := h.patchPolicies.Delete(r.Context(), id); err != nil {
		h.log.Error("patching: delete policy error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete policy"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListPolicyAssignments returns GET /patching/policies/:id/assignments.
func (h *PatchingHandler) ListPolicyAssignments(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	if !isValidPatchUUID(policyID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid policy ID"})
		return
	}
	assignments, err := h.assignments.ListByPolicy(r.Context(), policyID)
	if err != nil {
		h.log.Error("patching: list assignments error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load assignments"})
		return
	}
	out := make([]map[string]interface{}, len(assignments))
	for i, a := range assignments {
		out[i] = map[string]interface{}{
			"id":          a.ID,
			"target_type": a.TargetType,
			"target_id":   a.TargetID,
			"created_at":  pgTimeToISO(a.CreatedAt),
			"updated_at":  pgTimeToISO(a.UpdatedAt),
		}
	}
	JSON(w, http.StatusOK, out)
}

// AddPolicyAssignment handles POST /patching/policies/:id/assignments.
func (h *PatchingHandler) AddPolicyAssignment(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	if !isValidPatchUUID(policyID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid policy ID"})
		return
	}
	var body struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if body.TargetType != "host" && body.TargetType != "host_group" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "target_type must be host or host_group"})
		return
	}
	if body.TargetID == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "target_id is required"})
		return
	}
	id, err := h.assignments.Create(r.Context(), policyID, body.TargetType, body.TargetID)
	if err != nil {
		h.log.Error("patching: add assignment error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to add assignment"})
		return
	}
	a, _ := h.assignments.GetByID(r.Context(), id)
	resp := map[string]interface{}{
		"id":          id,
		"target_type": body.TargetType,
		"target_id":   body.TargetID,
	}
	if a != nil {
		resp["created_at"] = pgTimeToISO(a.CreatedAt)
		resp["updated_at"] = pgTimeToISO(a.UpdatedAt)
	}
	JSON(w, http.StatusCreated, resp)
}

// RemovePolicyAssignment handles DELETE /patching/policies/:id/assignments/:assignmentId.
func (h *PatchingHandler) RemovePolicyAssignment(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	assignmentID := chi.URLParam(r, "assignmentId")
	if !isValidPatchUUID(policyID) || !isValidPatchUUID(assignmentID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}
	a, _ := h.assignments.GetByID(r.Context(), assignmentID)
	if a == nil || a.PatchPolicyID != policyID {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Assignment not found"})
		return
	}
	if err := h.assignments.Delete(r.Context(), assignmentID); err != nil {
		h.log.Error("patching: remove assignment error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove assignment"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddPolicyExclusion handles POST /patching/policies/:id/exclusions.
func (h *PatchingHandler) AddPolicyExclusion(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	if !isValidPatchUUID(policyID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid policy ID"})
		return
	}
	var body struct {
		HostID string `json:"host_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if body.HostID == "" || !isValidPatchUUID(body.HostID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Valid host_id is required"})
		return
	}
	id, err := h.exclusions.Create(r.Context(), policyID, body.HostID)
	if err != nil {
		h.log.Error("patching: add exclusion error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to add exclusion"})
		return
	}
	JSON(w, http.StatusCreated, map[string]interface{}{
		"id":         id,
		"host_id":    body.HostID,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// RemovePolicyExclusion handles DELETE /patching/policies/:id/exclusions/:hostId.
func (h *PatchingHandler) RemovePolicyExclusion(w http.ResponseWriter, r *http.Request) {
	policyID := chi.URLParam(r, "id")
	hostID := chi.URLParam(r, "hostId")
	if !isValidPatchUUID(policyID) || !isValidPatchUUID(hostID) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}
	if err := h.exclusions.Delete(r.Context(), policyID, hostID); err != nil {
		h.log.Error("patching: remove exclusion error", "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove exclusion"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pgTimeToISO(t pgtype.Timestamp) string {
	if t.Valid {
		return t.Time.UTC().Format(time.RFC3339)
	}
	return ""
}

func patchRunToResponse(r *db.GetPatchRunByIDRow) map[string]interface{} {
	m := map[string]interface{}{
		"id":                    r.ID,
		"host_id":               r.HostID,
		"job_id":                r.JobID,
		"patch_type":            r.PatchType,
		"package_name":          r.PackageName,
		"status":                r.Status,
		"shell_output":          r.ShellOutput,
		"error_message":         r.ErrorMessage,
		"started_at":            pgTimeToISO(r.StartedAt),
		"completed_at":          pgTimeToISO(r.CompletedAt),
		"scheduled_at":          pgTimeToISO(r.ScheduledAt),
		"created_at":            pgTimeToISO(r.CreatedAt),
		"updated_at":            pgTimeToISO(r.UpdatedAt),
		"triggered_by_username": r.TriggeredByUsername,
		"approved_by_username":  r.ApprovedByUsername,
		"dry_run":               r.DryRun,
	}
	if len(r.PackageNames) > 0 {
		var names []string
		_ = json.Unmarshal(r.PackageNames, &names)
		m["package_names"] = names
	}
	if len(r.PackagesAffected) > 0 {
		var pkgs []string
		_ = json.Unmarshal(r.PackagesAffected, &pkgs)
		m["packages_affected"] = pkgs
	}
	m["hosts"] = map[string]interface{}{
		"id":            r.HostID,
		"friendly_name": r.HostFriendlyName,
		"hostname":      r.HostHostname,
	}
	return m
}

func patchRunsToResponse(rows []db.ListRecentPatchRunsRow) []map[string]interface{} {
	out := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		out[i] = patchRunRowToMap(r.ID, r.HostID, r.JobID, r.PatchType, r.PackageName, r.PackageNames, r.Status, r.ShellOutput, r.ErrorMessage, r.StartedAt, r.CompletedAt, r.ScheduledAt, r.CreatedAt, r.UpdatedAt, r.HostFriendlyName, r.HostHostname, r.TriggeredByUsername)
	}
	return out
}

func patchRunsActiveToResponse(rows []db.ListActivePatchRunsRow) []map[string]interface{} {
	out := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		out[i] = patchRunRowToMap(r.ID, r.HostID, r.JobID, r.PatchType, r.PackageName, r.PackageNames, r.Status, r.ShellOutput, r.ErrorMessage, r.StartedAt, r.CompletedAt, r.ScheduledAt, r.CreatedAt, r.UpdatedAt, r.HostFriendlyName, r.HostHostname, r.TriggeredByUsername)
	}
	return out
}

func patchRunsListToResponse(rows []db.ListPatchRunsRow) []map[string]interface{} {
	out := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		m := patchRunRowToMap(r.ID, r.HostID, r.JobID, r.PatchType, r.PackageName, r.PackageNames, r.Status, r.ShellOutput, r.ErrorMessage, r.StartedAt, r.CompletedAt, r.ScheduledAt, r.CreatedAt, r.UpdatedAt, r.HostFriendlyName, r.HostHostname, r.TriggeredByUsername)
		m["dry_run"] = r.DryRun
		if len(r.PackagesAffected) > 0 {
			var pkgs []string
			_ = json.Unmarshal(r.PackagesAffected, &pkgs)
			m["packages_affected"] = pkgs
		}
		out[i] = m
	}
	return out
}

func patchRunRowToMap(id, hostID, jobID, patchType string, pkgName *string, pkgNames []byte, status, shellOutput string, errMsg *string, startedAt, completedAt, scheduledAt pgtype.Timestamp, createdAt, updatedAt pgtype.Timestamp, hostFriendly, hostHostname, triggeredByUsername *string) map[string]interface{} {
	m := map[string]interface{}{
		"id":                    id,
		"host_id":               hostID,
		"job_id":                jobID,
		"patch_type":            patchType,
		"package_name":          pkgName,
		"status":                status,
		"shell_output":          shellOutput,
		"error_message":         errMsg,
		"started_at":            pgTimeToISO(startedAt),
		"completed_at":          pgTimeToISO(completedAt),
		"scheduled_at":          pgTimeToISO(scheduledAt),
		"created_at":            pgTimeToISO(createdAt),
		"updated_at":            pgTimeToISO(updatedAt),
		"triggered_by_username": triggeredByUsername,
		"hosts": map[string]interface{}{
			"id":            hostID,
			"friendly_name": hostFriendly,
			"hostname":      hostHostname,
		},
	}
	if len(pkgNames) > 0 {
		var names []string
		_ = json.Unmarshal(pkgNames, &names)
		m["package_names"] = names
	}
	return m
}
