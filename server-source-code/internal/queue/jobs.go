package queue

import (
	"context"
	"encoding/json"
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
)

const (
	TypeReportNow                = "report_now"
	TypeRefreshIntegrationStatus = "refresh_integration_status"
	TypeDockerInventoryRefresh   = "docker_inventory_refresh"
	TypeUpdateAgent              = "update_agent"
	TypeSessionCleanup           = "session-cleanup"
	TypeOrphanedRepoCleanup      = "orphaned-repo-cleanup"
	TypeOrphanedPkgCleanup       = "orphaned-package-cleanup"
	TypeDockerInvCleanup         = "docker-inventory-cleanup"
	TypeSystemStatistics         = "system-statistics"
	TypeVersionUpdateCheck       = "version-update-check"
	TypeComplianceScanCleanup    = "compliance-scan-cleanup"
	TypeRunScan                  = "run_scan"
	TypeInstallComplianceTools   = "install_compliance_tools"
	TypeRunPatch                 = "run_patch"
	QueueAgentCommands           = "agent-commands"
	QueuePatching                = "patching"
	QueueCompliance              = "compliance"
	QueueHostStatus              = "host-status-monitor"
	QueueAlertCleanup            = "alert-cleanup"
	QueueSessionCleanup          = "session-cleanup"
	QueueOrphanedRepoCleanup     = "orphaned-repo-cleanup"
	QueueOrphanedPkgCleanup      = "orphaned-package-cleanup"
	QueueDockerInvCleanup        = "docker-inventory-cleanup"
	QueueSystemStatistics        = "system-statistics"
	QueueVersionUpdateCheck      = "version-update-check"
	QueueComplianceScanCleanup   = "compliance-scan-cleanup"
)

// RunScanPayload is the payload for run_scan job.
type RunScanPayload struct {
	HostID               string  `json:"hostId"`
	ApiID                string  `json:"api_id"`
	Host                 string  `json:"host,omitempty"`
	ProfileType          string  `json:"profile_type"`
	ProfileID            *string `json:"profile_id,omitempty"`
	EnableRemediation    bool    `json:"enable_remediation"`
	FetchRemoteResources bool    `json:"fetch_remote_resources"`
}

// NewRunScanTask creates a run_scan task. Use TaskID for deduplication: compliance-scan-{hostId}.
func NewRunScanTask(p RunScanPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCompliance),
		asynq.MaxRetry(10),
		asynq.TaskID("compliance-scan-" + p.HostID),
	}
	return asynq.NewTask(TypeRunScan, payload, opts...), nil
}

// InstallComplianceToolsPayload is the payload for install_compliance_tools job.
type InstallComplianceToolsPayload struct {
	HostID string `json:"hostId"`
	ApiID  string `json:"api_id"`
	Host   string `json:"host,omitempty"`
}

// NewInstallComplianceToolsTask creates an install_compliance_tools task.
func NewInstallComplianceToolsTask(hostID, apiID, host string) (*asynq.Task, error) {
	payload, err := json.Marshal(InstallComplianceToolsPayload{HostID: hostID, ApiID: apiID, Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeInstallComplianceTools, payload, asynq.Queue(QueueCompliance), asynq.MaxRetry(2)), nil
}

// RunPatchPayload is the payload for run_patch job.
type RunPatchPayload struct {
	HostID       string   `json:"hostId"`
	Host         string   `json:"host,omitempty"` // tenant host (e.g. iby1.dev.local) for per-tenant DB resolution
	ApiID        string   `json:"api_id"`
	PatchRunID   string   `json:"patch_run_id"`
	PatchType    string   `json:"patch_type"`
	PackageName  *string  `json:"package_name,omitempty"`
	PackageNames []string `json:"package_names,omitempty"`
	DryRun       bool     `json:"dry_run,omitempty"`
}

// NewRunPatchTask creates a run_patch task.
func NewRunPatchTask(p RunPatchPayload) (*asynq.Task, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	opts := []asynq.Option{
		asynq.Queue(QueuePatching),
		asynq.MaxRetry(10),
		asynq.TaskID("patch-run-" + p.PatchRunID),
	}
	return asynq.NewTask(TypeRunPatch, payload, opts...), nil
}

// ReportNowPayload is the payload for report_now job.
type ReportNowPayload struct {
	ApiID string `json:"api_id"`
	Host  string `json:"host,omitempty"`
}

// NewReportNowTask creates a report_now task.
func NewReportNowTask(apiID, host string) (*asynq.Task, error) {
	payload, err := json.Marshal(ReportNowPayload{ApiID: apiID, Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeReportNow, payload, asynq.Queue(QueueAgentCommands), asynq.MaxRetry(3)), nil
}

// NewRefreshIntegrationStatusTask creates a refresh_integration_status task.
func NewRefreshIntegrationStatusTask(apiID, host string) (*asynq.Task, error) {
	payload, err := json.Marshal(ReportNowPayload{ApiID: apiID, Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeRefreshIntegrationStatus, payload, asynq.Queue(QueueAgentCommands), asynq.MaxRetry(2)), nil
}

// NewDockerInventoryRefreshTask creates a docker_inventory_refresh task.
func NewDockerInventoryRefreshTask(apiID, host string) (*asynq.Task, error) {
	payload, err := json.Marshal(ReportNowPayload{ApiID: apiID, Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeDockerInventoryRefresh, payload, asynq.Queue(QueueAgentCommands), asynq.MaxRetry(2)), nil
}

// UpdateAgentPayload is the payload for update_agent job.
type UpdateAgentPayload struct {
	ApiID          string `json:"api_id"`
	Host           string `json:"host,omitempty"`
	BypassSettings bool   `json:"bypass_settings"`
}

// NewUpdateAgentTask creates an update_agent task.
func NewUpdateAgentTask(apiID, host string, bypassSettings bool) (*asynq.Task, error) {
	payload, err := json.Marshal(UpdateAgentPayload{ApiID: apiID, Host: host, BypassSettings: bypassSettings})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeUpdateAgent, payload, asynq.Queue(QueueAgentCommands), asynq.MaxRetry(3)), nil
}

// AutomationRetention keeps completed automation tasks in Redis for 7 days for dashboard visibility.
const AutomationRetention = 168 * time.Hour // 7 days

// AutomationPayload is the payload for automation jobs that need host resolution.
type AutomationPayload struct {
	Host string `json:"host,omitempty"`
}

// OrphanedPkgCleanupPayload is the payload for orphaned-package-cleanup job.
type OrphanedPkgCleanupPayload struct {
	Host string `json:"host,omitempty"`
}

// NewOrphanedRepoCleanupTask creates an orphaned-repo-cleanup task.
func NewOrphanedRepoCleanupTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(AutomationPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeOrphanedRepoCleanup, payload, asynq.Queue(QueueOrphanedRepoCleanup), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// NewOrphanedPkgCleanupTask creates an orphaned-package-cleanup task.
func NewOrphanedPkgCleanupTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(OrphanedPkgCleanupPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeOrphanedPkgCleanup, payload, asynq.Queue(QueueOrphanedPkgCleanup), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// NewDockerInvCleanupTask creates a docker-inventory-cleanup task.
func NewDockerInvCleanupTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(AutomationPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeDockerInvCleanup, payload, asynq.Queue(QueueDockerInvCleanup), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// NewSystemStatisticsTask creates a system-statistics task.
func NewSystemStatisticsTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(AutomationPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeSystemStatistics, payload, asynq.Queue(QueueSystemStatistics), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// NewVersionUpdateCheckTask creates a version-update-check task.
func NewVersionUpdateCheckTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(AutomationPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeVersionUpdateCheck, payload, asynq.Queue(QueueVersionUpdateCheck), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// NewComplianceScanCleanupTask creates a compliance-scan-cleanup task.
func NewComplianceScanCleanupTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(AutomationPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeComplianceScanCleanup, payload, asynq.Queue(QueueComplianceScanCleanup), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// NewSessionCleanupTask creates a session-cleanup task.
func NewSessionCleanupTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(AutomationPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeSessionCleanup, payload, asynq.Queue(QueueSessionCleanup), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// NewAlertCleanupTask creates an alert-cleanup task.
func NewAlertCleanupTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(AutomationPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeAlertCleanup, payload, asynq.Queue(QueueAlertCleanup), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// NewHostStatusMonitorTask creates a host-status-monitor task.
func NewHostStatusMonitorTask(host string) (*asynq.Task, error) {
	payload, err := json.Marshal(AutomationPayload{Host: host})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeHostStatusMonitor, payload, asynq.Queue(QueueHostStatus), asynq.MaxRetry(2), asynq.Retention(AutomationRetention)), nil
}

// ReportNowHandler handles report_now jobs.
type ReportNowHandler struct {
	registry *agentregistry.Registry
	db       *database.DB
	log      *slog.Logger
}

// NewReportNowHandler creates a report_now handler.
func NewReportNowHandler(registry *agentregistry.Registry, db *database.DB, log *slog.Logger) *ReportNowHandler {
	return &ReportNowHandler{registry: registry, db: db, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *ReportNowHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p ReportNowPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}

	taskID, _ := asynq.GetTaskID(ctx)
	retryCount, _ := asynq.GetRetryCount(ctx)
	attempt := int32(retryCount + 1)

	// Log to job_history on first attempt so it persists in Agent Queue tab (like BullMQ)
	if h.db != nil && taskID != "" && retryCount == 0 {
		host, err := h.db.Queries.GetHostByApiID(ctx, p.ApiID)
		var hostID *string
		if err == nil {
			hostID = &host.ID
		}
		apiIDPtr := &p.ApiID
		_ = h.db.Queries.InsertJobHistory(ctx, db.InsertJobHistoryParams{
			ID:            uuid.New().String(),
			JobID:         taskID,
			QueueName:     QueueAgentCommands,
			JobName:       TypeReportNow,
			HostID:        hostID,
			ApiID:         apiIDPtr,
			Status:        "active",
			AttemptNumber: attempt,
		})
	}

	conn := h.registry.GetConnection(p.ApiID)
	if conn == nil {
		h.log.Warn("report_now: agent not connected", "api_id", p.ApiID)
		if taskID != "" && h.db != nil {
			msg := "Agent not connected"
			_ = h.db.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &msg})
		}
		return nil // Don't retry - agent may connect later, user can retry
	}
	msg := []byte(`{"type":"report_now"}`)
	if err := conn.WriteMessage(1, msg); err != nil {
		h.log.Warn("report_now: write failed", "api_id", p.ApiID, "error", err)
		return err // Retry on write failure - don't update job_history yet
	}

	if taskID != "" && h.db != nil {
		_ = h.db.Queries.UpdateJobHistoryCompleted(ctx, taskID)
	}
	h.log.Info("report_now sent", "api_id", p.ApiID)
	return nil
}

// sendAgentCommand is a helper that sends a JSON command to the agent and updates job_history.
func sendAgentCommand(ctx context.Context, h *ReportNowHandler, p ReportNowPayload, msgType, taskID string, retryCount int) error {
	taskIDVal, _ := asynq.GetTaskID(ctx)
	if taskID == "" {
		taskID = taskIDVal
	}
	attempt := int32(retryCount + 1)

	if h.db != nil && taskID != "" && retryCount == 0 {
		host, err := h.db.Queries.GetHostByApiID(ctx, p.ApiID)
		var hostID *string
		if err == nil {
			hostID = &host.ID
		}
		apiIDPtr := &p.ApiID
		_ = h.db.Queries.InsertJobHistory(ctx, db.InsertJobHistoryParams{
			ID:            uuid.New().String(),
			JobID:         taskID,
			QueueName:     QueueAgentCommands,
			JobName:       msgType,
			HostID:        hostID,
			ApiID:         apiIDPtr,
			Status:        "active",
			AttemptNumber: attempt,
		})
	}

	conn := h.registry.GetConnection(p.ApiID)
	if conn == nil {
		h.log.Warn(msgType+": agent not connected", "api_id", p.ApiID)
		if taskID != "" && h.db != nil {
			msg := "Agent not connected"
			_ = h.db.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &msg})
		}
		return nil
	}
	msg := []byte(`{"type":"` + msgType + `"}`)
	if err := conn.WriteMessage(1, msg); err != nil {
		h.log.Warn(msgType+": write failed", "api_id", p.ApiID, "error", err)
		return err
	}

	if taskID != "" && h.db != nil {
		_ = h.db.Queries.UpdateJobHistoryCompleted(ctx, taskID)
	}
	h.log.Info(msgType+" sent", "api_id", p.ApiID)
	return nil
}

// RefreshIntegrationStatusHandler handles refresh_integration_status jobs.
type RefreshIntegrationStatusHandler struct {
	*ReportNowHandler
}

// NewRefreshIntegrationStatusHandler creates a refresh_integration_status handler.
func NewRefreshIntegrationStatusHandler(registry *agentregistry.Registry, db *database.DB, log *slog.Logger) *RefreshIntegrationStatusHandler {
	return &RefreshIntegrationStatusHandler{ReportNowHandler: NewReportNowHandler(registry, db, log)}
}

// ProcessTask implements asynq.Handler.
func (h *RefreshIntegrationStatusHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p ReportNowPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	retryCount, _ := asynq.GetRetryCount(ctx)
	return sendAgentCommand(ctx, h.ReportNowHandler, p, TypeRefreshIntegrationStatus, "", retryCount)
}

// DockerInventoryRefreshHandler handles docker_inventory_refresh jobs.
type DockerInventoryRefreshHandler struct {
	*ReportNowHandler
}

// NewDockerInventoryRefreshHandler creates a docker_inventory_refresh handler.
func NewDockerInventoryRefreshHandler(registry *agentregistry.Registry, db *database.DB, log *slog.Logger) *DockerInventoryRefreshHandler {
	return &DockerInventoryRefreshHandler{ReportNowHandler: NewReportNowHandler(registry, db, log)}
}

// ProcessTask implements asynq.Handler.
func (h *DockerInventoryRefreshHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p ReportNowPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	retryCount, _ := asynq.GetRetryCount(ctx)
	return sendAgentCommand(ctx, h.ReportNowHandler, p, TypeDockerInventoryRefresh, "", retryCount)
}

// UpdateAgentHandler handles update_agent jobs.
type UpdateAgentHandler struct {
	registry *agentregistry.Registry
	db       *database.DB
	log      *slog.Logger
}

// NewUpdateAgentHandler creates an update_agent handler.
func NewUpdateAgentHandler(registry *agentregistry.Registry, db *database.DB, log *slog.Logger) *UpdateAgentHandler {
	return &UpdateAgentHandler{registry: registry, db: db, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *UpdateAgentHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p UpdateAgentPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}

	taskID, _ := asynq.GetTaskID(ctx)
	retryCount, _ := asynq.GetRetryCount(ctx)
	attempt := int32(retryCount + 1)

	if h.db != nil && taskID != "" && retryCount == 0 {
		host, err := h.db.Queries.GetHostByApiID(ctx, p.ApiID)
		var hostID *string
		if err == nil {
			hostID = &host.ID
		}
		apiIDPtr := &p.ApiID
		_ = h.db.Queries.InsertJobHistory(ctx, db.InsertJobHistoryParams{
			ID:            uuid.New().String(),
			JobID:         taskID,
			QueueName:     QueueAgentCommands,
			JobName:       TypeUpdateAgent,
			HostID:        hostID,
			ApiID:         apiIDPtr,
			Status:        "active",
			AttemptNumber: attempt,
		})
	}

	if !p.BypassSettings {
		settings, err := h.db.Queries.GetFirstSettings(ctx)
		if err != nil || !settings.AutoUpdate {
			msg := "Auto-update is disabled in server settings"
			if taskID != "" && h.db != nil {
				_ = h.db.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &msg})
			}
			h.log.Info("update_agent: skipped", "api_id", p.ApiID, "reason", msg)
			return nil
		}
		host, err := h.db.Queries.GetHostByApiID(ctx, p.ApiID)
		if err != nil {
			msg := "Host not found"
			if taskID != "" && h.db != nil {
				_ = h.db.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &msg})
			}
			return nil
		}
		if !host.AutoUpdate {
			msg := "Auto-update is disabled for this host"
			if taskID != "" && h.db != nil {
				_ = h.db.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &msg})
			}
			h.log.Info("update_agent: skipped", "api_id", p.ApiID, "reason", msg)
			return nil
		}
	}

	conn := h.registry.GetConnection(p.ApiID)
	if conn == nil {
		h.log.Warn("update_agent: agent not connected", "api_id", p.ApiID)
		if taskID != "" && h.db != nil {
			msg := "Agent not connected"
			_ = h.db.Queries.UpdateJobHistoryFailed(ctx, db.UpdateJobHistoryFailedParams{JobID: taskID, ErrorMessage: &msg})
		}
		return nil
	}
	msg := []byte(`{"type":"update_agent"}`)
	if err := conn.WriteMessage(1, msg); err != nil {
		h.log.Warn("update_agent: write failed", "api_id", p.ApiID, "error", err)
		return err
	}

	if taskID != "" && h.db != nil {
		_ = h.db.Queries.UpdateJobHistoryCompleted(ctx, taskID)
	}
	h.log.Info("update_agent sent", "api_id", p.ApiID)
	return nil
}

// RunPatchHandler handles run_patch jobs.
type RunPatchHandler struct {
	registry    *agentregistry.Registry
	patchRuns   *store.PatchRunsStore
	poolCache   *hostctx.PoolCache
	queueClient *asynq.Client
	log         *slog.Logger
}

// NewRunPatchHandler creates a run_patch handler.
func NewRunPatchHandler(registry *agentregistry.Registry, patchRuns *store.PatchRunsStore, poolCache *hostctx.PoolCache, queueClient *asynq.Client, log *slog.Logger) *RunPatchHandler {
	return &RunPatchHandler{registry: registry, patchRuns: patchRuns, poolCache: poolCache, queueClient: queueClient, log: log}
}

// ProcessTask implements asynq.Handler.
func (h *RunPatchHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p RunPatchPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}

	// Resolve per-tenant DB when Host is in payload (multi-host mode).
	if h.poolCache != nil && strings.TrimSpace(p.Host) != "" {
		if db, err := h.poolCache.GetOrCreate(ctx, p.Host); err == nil && db != nil {
			ctx = hostctx.WithDB(ctx, db)
		}
	}

	conn := h.registry.GetConnection(p.ApiID)
	if conn == nil {
		h.log.Info("run_patch: agent offline, re-queuing", "api_id", p.ApiID, "patch_run_id", p.PatchRunID)
		_ = h.patchRuns.UpdateStatus(ctx, p.PatchRunID, "queued")
		// Re-enqueue with 1 min delay
		task, err := NewRunPatchTask(p)
		if err != nil {
			return err
		}
		_, err = h.queueClient.Enqueue(task, asynq.ProcessIn(1*time.Minute))
		if err != nil {
			h.log.Warn("run_patch: re-enqueue failed", "api_id", p.ApiID, "error", err)
			return err
		}
		return nil
	}

	// Build run_patch payload
	payload := map[string]interface{}{
		"type":         "run_patch",
		"patch_run_id": p.PatchRunID,
		"patch_type":   p.PatchType,
		"dry_run":      p.DryRun,
	}
	if p.PackageName != nil {
		payload["package_name"] = *p.PackageName
	}
	if len(p.PackageNames) > 0 {
		payload["package_names"] = p.PackageNames
	}
	msg, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := conn.WriteMessage(1, msg); err != nil {
		h.log.Warn("run_patch: write failed", "api_id", p.ApiID, "error", err)
		return err
	}

	_ = h.patchRuns.UpdateStatus(ctx, p.PatchRunID, "running")
	h.log.Info("run_patch sent", "api_id", p.ApiID, "patch_run_id", p.PatchRunID)
	return nil
}
