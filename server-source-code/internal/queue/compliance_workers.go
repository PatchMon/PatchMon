package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const (
	complianceInstallJobPrefix    = "compliance_install_job:"
	complianceInstallCancelPrefix = "compliance_install_cancel:"
	complianceInstallTimeout      = 5 * time.Minute
	compliancePollInterval        = 2 * time.Second
	complianceScanRetryDelay      = 1 * time.Minute
	complianceInstallRetryDelay   = 30 * time.Second
	// maxScanRequeueAttempts limits how many times a run_scan job will be
	// re-queued when the agent is offline, preventing infinite loops for
	// decommissioned or permanently unreachable agents.
	maxScanRequeueAttempts = 30 // ~30 minutes at 1-minute intervals
)

type complianceScannerStatusPayload struct {
	Components  map[string]string `json:"components"`
	ScannerInfo struct {
		OpenSCAPAvailable    *bool `json:"openscap_available"`
		DockerBenchAvailable *bool `json:"docker_bench_available"`
	} `json:"scanner_info"`
}

// ValidateComplianceScanReadiness checks scanner state the agent has already reported.
// Unknown or absent scanner status is allowed so older agents are not blocked.
func ValidateComplianceScanReadiness(scannerStatus []byte, profileType string, profileID *string, openscapEnabled, dockerBenchEnabled bool) error {
	requested := requestedComplianceScanners(profileType, profileID)
	if len(requested) == 0 {
		if openscapEnabled {
			requested = append(requested, "openscap")
		}
		if dockerBenchEnabled {
			requested = append(requested, "docker-bench")
		}
		if len(requested) == 0 {
			return fmt.Errorf("no compliance scanners are enabled for this host")
		}
	}

	for _, scanner := range requested {
		switch scanner {
		case "openscap":
			if !openscapEnabled {
				return fmt.Errorf("OpenSCAP scanner is disabled for this host")
			}
		case "docker-bench":
			if !dockerBenchEnabled {
				return fmt.Errorf("docker-bench scanner is disabled for this host")
			}
		}
	}

	status, ok := parseComplianceScannerStatus(scannerStatus)
	if !ok {
		return nil
	}
	for _, scanner := range requested {
		if componentStatus, blocked := blockedScannerStatus(status, scanner); blocked {
			return fmt.Errorf("%s scanner is %s", scannerDisplayName(scanner), componentStatus)
		}
	}
	return nil
}

func requestedComplianceScanners(profileType string, profileID *string) []string {
	if profileID != nil && strings.TrimSpace(*profileID) == "docker-bench" {
		return []string{"docker-bench"}
	}

	switch strings.TrimSpace(profileType) {
	case "openscap":
		return []string{"openscap"}
	case "docker-bench":
		return []string{"docker-bench"}
	case "", "all":
		return nil
	default:
		return []string{"openscap"}
	}
}

func parseComplianceScannerStatus(raw []byte) (complianceScannerStatusPayload, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return complianceScannerStatusPayload{}, false
	}
	var status complianceScannerStatusPayload
	if err := json.Unmarshal(raw, &status); err != nil {
		return complianceScannerStatusPayload{}, false
	}
	return status, true
}

func blockedScannerStatus(status complianceScannerStatusPayload, scanner string) (string, bool) {
	if status.Components != nil {
		if value, ok := status.Components[scanner]; ok {
			normalized := strings.ToLower(strings.TrimSpace(value))
			if normalized != "" && normalized != "ready" {
				return normalized, true
			}
			return "", false
		}
	}

	switch scanner {
	case "openscap":
		if status.ScannerInfo.OpenSCAPAvailable != nil && !*status.ScannerInfo.OpenSCAPAvailable {
			return "unavailable", true
		}
	case "docker-bench":
		if status.ScannerInfo.DockerBenchAvailable != nil && !*status.ScannerInfo.DockerBenchAvailable {
			return "unavailable", true
		}
	}
	return "", false
}

func scannerDisplayName(scanner string) string {
	switch scanner {
	case "openscap":
		return "OpenSCAP"
	case "docker-bench":
		return "Docker Bench"
	default:
		return scanner
	}
}

// RunScanHandler handles run_scan jobs.
type RunScanHandler struct {
	registry          *agentregistry.Registry
	db                *database.DB
	poolCache         *hostctx.PoolCache
	compliance        *store.ComplianceStore
	integrationStatus *store.IntegrationStatusStore
	log               *slog.Logger
}

// NewRunScanHandler creates a run_scan handler.
func NewRunScanHandler(registry *agentregistry.Registry, db *database.DB, poolCache *hostctx.PoolCache, compliance *store.ComplianceStore, integrationStatus *store.IntegrationStatusStore, log *slog.Logger) *RunScanHandler {
	return &RunScanHandler{
		registry:          registry,
		db:                db,
		poolCache:         poolCache,
		compliance:        compliance,
		integrationStatus: integrationStatus,
		log:               log,
	}
}

// ProcessTask implements asynq.Handler.
func (h *RunScanHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p RunScanPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("decode run_scan payload: %v: %w", err, asynq.SkipRetry)
	}

	// Resolve per-context DB and put it on ctx so DBProvider-backed stores
	// (h.compliance) resolve to the same database.
	ctx, d, err := resolveDBFromPayload(ctx, t.Payload(), h.db, h.poolCache)
	if err != nil {
		return err
	}
	if d != nil {
		ctx = hostctx.WithDB(ctx, d)
	}
	taskID, _ := asynq.GetTaskID(ctx)
	retryCount, _ := asynq.GetRetryCount(ctx)
	attempt := int32(retryCount + 1)

	if d != nil && taskID != "" && retryCount == 0 {
		host, err := d.Queries.GetHostByApiID(ctx, p.ApiID)
		var hostID *string
		if err == nil {
			hostID = &host.ID
		}
		apiIDPtr := &p.ApiID
		_ = d.Queries.InsertJobHistory(ctx, db.InsertJobHistoryParams{
			ID:            uuid.New().String(),
			JobID:         taskID,
			QueueName:     QueueCompliance,
			JobName:       TypeRunScan,
			HostID:        hostID,
			ApiID:         apiIDPtr,
			Status:        "active",
			AttemptNumber: attempt,
		})
	}

	if !h.registry.IsConnected(p.ApiID) {
		if h.integrationStatus != nil && h.integrationStatus.IsComplianceScanCancelled(ctx, p.HostID) {
			h.log.Info("run_scan: cancelled, not re-queuing", "api_id", p.ApiID, "host_id", p.HostID)
			if taskID != "" && d != nil {
				_ = d.Queries.UpdateJobHistoryCompleted(ctx, taskID)
			}
			return nil
		}
		if scanRetriesExhausted(ctx) {
			h.log.Warn("run_scan: agent offline, max re-queue attempts reached - giving up",
				"api_id", p.ApiID, "host_id", p.HostID)
			if taskID != "" && d != nil {
				msg := "Agent remained offline after maximum retry attempts"
				_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{
					JobID: taskID, ErrorMessage: &msg,
				})
			}
			return nil
		}
		h.log.Info("run_scan: agent offline, will retry", "api_id", p.ApiID)
		if taskID != "" && d != nil {
			_ = d.Queries.UpdateJobHistoryDelayed(ctx, taskID)
		}
		return fmt.Errorf("agent offline: %w", ErrRetryLater)
	}

	openscapEnabled := true
	dockerBenchEnabled := false
	host, err := d.Queries.GetHostByID(ctx, p.HostID)
	if err == nil {
		openscapEnabled = host.ComplianceOpenscapEnabled
		dockerBenchEnabled = host.ComplianceDockerBenchEnabled
	}

	effectiveProfileType := p.ProfileType
	if effectiveProfileType == "" || effectiveProfileType == "all" {
		if openscapEnabled && !dockerBenchEnabled {
			effectiveProfileType = "openscap"
		} else if !openscapEnabled && dockerBenchEnabled {
			effectiveProfileType = "docker-bench"
		} else if !openscapEnabled && !dockerBenchEnabled {
			h.log.Warn("run_scan: both scanners disabled, skipping", "host_id", p.HostID)
			if taskID != "" && d != nil {
				_ = d.Queries.UpdateJobHistoryCompleted(ctx, taskID)
			}
			return nil
		} else {
			effectiveProfileType = "all"
		}
	}

	if err == nil {
		if readinessErr := ValidateComplianceScanReadiness(host.ComplianceScannerStatus, effectiveProfileType, p.ProfileID, openscapEnabled, dockerBenchEnabled); readinessErr != nil {
			h.log.Warn("run_scan: scanner not ready", "host_id", p.HostID, "error", readinessErr)
			if taskID != "" && d != nil {
				msg := readinessErr.Error()
				_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{
					JobID: taskID, ErrorMessage: &msg,
				})
			}
			return nil
		}
	}

	profileID := ""
	if p.ProfileID != nil {
		profileID = *p.ProfileID
	}
	msg := map[string]interface{}{
		"type":                   "compliance_scan",
		"profile_type":           effectiveProfileType,
		"profile_id":             nil,
		"enable_remediation":     p.EnableRemediation,
		"fetch_remote_resources": p.FetchRemoteResources,
		"openscap_enabled":       openscapEnabled,
		"docker_bench_enabled":   dockerBenchEnabled,
	}
	if profileID != "" {
		msg["profile_id"] = profileID
	}
	if err := h.registry.SendJSON(p.ApiID, msg); err != nil {
		if h.integrationStatus != nil && h.integrationStatus.IsComplianceScanCancelled(ctx, p.HostID) {
			h.log.Info("run_scan: cancelled after write failed", "api_id", p.ApiID, "host_id", p.HostID)
			if taskID != "" && d != nil {
				_ = d.Queries.UpdateJobHistoryCompleted(ctx, taskID)
			}
			return nil
		}
		h.log.Warn("run_scan: write failed", "api_id", p.ApiID, "error", err)
		if scanRetriesExhausted(ctx) {
			h.log.Warn("run_scan: write failed, max re-queue attempts reached - giving up",
				"api_id", p.ApiID, "host_id", p.HostID)
			if taskID != "" && d != nil {
				msg := "Failed to communicate with agent after maximum retry attempts"
				_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{
					JobID: taskID, ErrorMessage: &msg,
				})
			}
			return nil
		}
		if taskID != "" && d != nil {
			_ = d.Queries.UpdateJobHistoryDelayed(ctx, taskID)
		}
		return fmt.Errorf("send to agent: %w", ErrRetryLater)
	}

	if h.integrationStatus != nil && h.integrationStatus.IsComplianceScanCancelled(ctx, p.HostID) {
		h.log.Info("run_scan: cancelled after dispatch, recalling scan", "api_id", p.ApiID, "host_id", p.HostID)
		_ = h.registry.SendJSON(p.ApiID, map[string]interface{}{"type": "compliance_scan_cancel"})
		if taskID != "" && d != nil {
			_ = d.Queries.UpdateJobHistoryCompleted(ctx, taskID)
		}
		return nil
	}

	profilesToUse := []string{}
	if effectiveProfileType == "all" || effectiveProfileType == "openscap" {
		prof, err := h.compliance.GetOrCreateProfile(ctx, "OpenSCAP Scan", "openscap")
		if err != nil {
			h.log.Warn("run_scan: could not resolve OpenSCAP profile", "host_id", p.HostID, "error", err)
		} else {
			profilesToUse = append(profilesToUse, prof.ID)
		}
	}
	if effectiveProfileType == "all" || effectiveProfileType == "docker-bench" {
		prof, err := h.compliance.GetOrCreateProfile(ctx, "Docker Bench Security", "docker-bench")
		if err != nil {
			h.log.Warn("run_scan: could not resolve Docker Bench profile", "host_id", p.HostID, "error", err)
		} else {
			profilesToUse = append(profilesToUse, prof.ID)
		}
	}
	for _, profileID := range profilesToUse {
		if err := h.compliance.CreateRunningScan(ctx, p.HostID, profileID); err != nil {
			h.log.Warn("run_scan: could not record running scan", "host_id", p.HostID, "profile_id", profileID, "error", err)
		}
	}
	if h.integrationStatus != nil && h.integrationStatus.IsComplianceScanCancelled(ctx, p.HostID) {
		if _, err := h.compliance.CancelActiveScans(ctx, p.HostID); err != nil {
			h.log.Warn("run_scan: could not cancel placeholders after late cancel", "host_id", p.HostID, "error", err)
		}
	}

	if taskID != "" && d != nil {
		_ = d.Queries.UpdateJobHistoryCompleted(ctx, taskID)
	}
	h.log.Info("run_scan: triggered", "host_id", p.HostID, "api_id", p.ApiID)
	return nil
}

// InstallComplianceToolsHandler handles install_compliance_tools jobs.
type InstallComplianceToolsHandler struct {
	registry   *agentregistry.Registry
	db         *database.DB
	poolCache  *hostctx.PoolCache
	rdb        *redis.Client
	redisCache *hostctx.RedisCache
	log        *slog.Logger
}

// NewInstallComplianceToolsHandler creates an install_compliance_tools handler.
func NewInstallComplianceToolsHandler(registry *agentregistry.Registry, db *database.DB, poolCache *hostctx.PoolCache, rdb *redis.Client, redisCache *hostctx.RedisCache, log *slog.Logger) *InstallComplianceToolsHandler {
	return &InstallComplianceToolsHandler{
		registry:   registry,
		db:         db,
		poolCache:  poolCache,
		rdb:        rdb,
		redisCache: redisCache,
		log:        log,
	}
}

// ProcessTask implements asynq.Handler.
func (h *InstallComplianceToolsHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p InstallComplianceToolsPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	ctx, d, err := resolveDBForHost(ctx, p.Host, h.db, h.poolCache)
	if err != nil {
		return err
	}

	// Resolve Redis from payload.Host when set; fall back to system rdb.
	rdb := h.rdb
	if h.redisCache != nil && p.Host != "" {
		if resolved, err := h.redisCache.GetOrCreate(ctx, p.Host); err == nil && resolved != nil {
			rdb = resolved
		}
	}
	taskID, _ := asynq.GetTaskID(ctx)
	retryCount, _ := asynq.GetRetryCount(ctx)
	attempt := int32(retryCount + 1)

	if d != nil && taskID != "" && retryCount == 0 {
		host, err := d.Queries.GetHostByApiID(ctx, p.ApiID)
		var hostID *string
		if err == nil {
			hostID = &host.ID
		}
		apiIDPtr := &p.ApiID
		_ = d.Queries.InsertJobHistory(ctx, db.InsertJobHistoryParams{
			ID:            uuid.New().String(),
			JobID:         taskID,
			QueueName:     QueueCompliance,
			JobName:       TypeInstallComplianceTools,
			HostID:        hostID,
			ApiID:         apiIDPtr,
			Status:        "active",
			AttemptNumber: attempt,
		})
	}

	cancelKey := workerTenantKey(p.Host, complianceInstallCancelPrefix+taskID)
	if rdb != nil {
		if cancelled, _ := rdb.Get(ctx, cancelKey).Result(); cancelled != "" {
			_ = rdb.Del(ctx, cancelKey).Err()
			errMsg := "Cancelled by user"
			if taskID != "" && d != nil {
				_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &errMsg})
			}
			return nil
		}
	}

	if !h.registry.IsConnected(p.ApiID) {
		msg := "Agent is not connected. Cannot run install."
		maxRetry, _ := asynq.GetMaxRetry(ctx)
		if taskID != "" && d != nil {
			if retryCount >= maxRetry {
				_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &msg})
			} else {
				_ = d.Queries.UpdateJobHistoryDelayed(ctx, taskID)
			}
		}
		h.log.Info("install_compliance_tools: agent not connected", "api_id", p.ApiID, "attempt", attempt)
		return fmt.Errorf("%s: %w", msg, ErrRetryLater)
	}

	msg := map[string]interface{}{"type": "install_scanner", "job_id": taskID}
	if err := h.registry.SendJSON(p.ApiID, msg); err != nil {
		errMsg := "Failed to send install_scanner command to agent"
		if taskID != "" && d != nil {
			_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &errMsg})
		}
		return err
	}

	if rdb == nil {
		if taskID != "" && d != nil {
			_ = d.Queries.UpdateJobHistoryCompleted(ctx, taskID)
		}
		h.log.Info("install_compliance_tools: sent (no Redis for polling)", "host_id", p.HostID)
		return nil
	}

	statusKey := workerTenantKey(p.Host, "integration_status:"+p.ApiID+":compliance")
	deadline := time.Now().Add(complianceInstallTimeout)
	pollTimer := time.NewTimer(compliancePollInterval)
	defer pollTimer.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			errMsg := "Job cancelled (context cancelled)"
			if taskID != "" && d != nil {
				_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &errMsg})
			}
			return ctx.Err()
		default:
		}

		cancelled, _ := rdb.Get(ctx, cancelKey).Result()
		if cancelled != "" {
			_ = rdb.Del(ctx, cancelKey).Err()
			errMsg := "Cancelled by user"
			if taskID != "" && d != nil {
				_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &errMsg})
			}
			return nil
		}

		raw, err := rdb.Get(ctx, statusKey).Result()
		if err == nil {
			if finished, failure := InstallOutcome(raw, taskID); finished {
				if failure == "" {
					if taskID != "" && d != nil {
						_ = d.Queries.UpdateJobHistoryCompleted(ctx, taskID)
					}
					h.log.Info("install_compliance_tools: completed", "host_id", p.HostID)
					return nil
				}
				if taskID != "" && d != nil {
					_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &failure})
				}
				return nil
			}
		}

		pollTimer.Reset(compliancePollInterval)
		select {
		case <-ctx.Done():
			errMsg := "Job cancelled (context cancelled)"
			if taskID != "" && d != nil {
				_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &errMsg})
			}
			return ctx.Err()
		case <-pollTimer.C:
		}
	}

	errMsg := "Install timed out after 5 minutes"
	if taskID != "" && d != nil {
		_ = d.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &errMsg})
	}
	h.log.Warn("install_compliance_tools: timeout", "host_id", p.HostID)
	return nil
}

// InstallOutcome reads install_events and their job tag, never the status field,
// which the agent's availability report also writes and can predate this install.
func InstallOutcome(raw, jobID string) (finished bool, failure string) {
	var data struct {
		InstallJobID  string `json:"install_job_id"`
		InstallEvents []struct {
			Step    string `json:"step"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"install_events"`
	}
	if json.Unmarshal([]byte(raw), &data) != nil {
		return false, ""
	}
	if !InstallEventsMatchJob(data.InstallJobID, jobID) {
		return false, ""
	}
	firstFailure := ""
	for _, e := range data.InstallEvents {
		if e.Step == "complete" {
			switch e.Status {
			case "done":
				return true, ""
			case "failed":
				if firstFailure == "" {
					firstFailure = e.Message
				}
				if firstFailure == "" {
					firstFailure = "Agent reported error"
				}
				return true, firstFailure
			}
			continue
		}
		if e.Status == "failed" && e.Message != "" && firstFailure == "" {
			firstFailure = e.Message
		}
	}
	return false, ""
}

// InstallEventsMatchJob reports whether events tagged eventsJobID belong to jobID.
// Agents that predate job tagging send no ID, and their events are accepted.
func InstallEventsMatchJob(eventsJobID, jobID string) bool {
	return eventsJobID == "" || jobID == "" || eventsJobID == jobID
}

// ErrRetryLater marks an expected, retryable condition such as an offline agent.
var ErrRetryLater = errors.New("retry later")

func scanRetriesExhausted(ctx context.Context) bool {
	retried, _ := asynq.GetRetryCount(ctx)
	maxRetry, ok := asynq.GetMaxRetry(ctx)
	return !ok || retried >= maxRetry
}
