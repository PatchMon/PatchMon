package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
)

// AutomationHandler handles automation routes.
type AutomationHandler struct {
	inspector   *asynq.Inspector
	queueClient *asynq.Client
	registry    *agentregistry.Registry
	settings    *store.SettingsStore
	alertConfig *store.AlertConfigStore
}

// NewAutomationHandler creates a new automation handler.
func NewAutomationHandler(
	inspector *asynq.Inspector,
	queueClient *asynq.Client,
	registry *agentregistry.Registry,
	settings *store.SettingsStore,
	alertConfig *store.AlertConfigStore,
) *AutomationHandler {
	return &AutomationHandler{
		inspector:   inspector,
		queueClient: queueClient,
		registry:    registry,
		settings:    settings,
		alertConfig: alertConfig,
	}
}

// QueueStats matches frontend expected shape.
type QueueStats struct {
	Waiting   int `json:"waiting"`
	Active    int `json:"active"`
	Delayed   int `json:"delayed"`
	Failed    int `json:"failed"`
	Completed int `json:"completed"`
}

// getQueueStats returns stats for a queue; returns zero stats if queue doesn't exist.
func (h *AutomationHandler) getQueueStats(queueName string) QueueStats {
	if h.inspector == nil {
		return QueueStats{}
	}
	info, err := h.inspector.GetQueueInfo(queueName)
	if err != nil {
		return QueueStats{}
	}
	return QueueStats{
		Waiting:   info.Pending,
		Active:    info.Active,
		Delayed:   info.Scheduled,
		Failed:    info.Retry,
		Completed: info.Completed,
	}
}

// getQueueLastRunInfo returns lastRun, lastRunTs, and status from Asynq/Redis for a queue.
// Checks completed, active, retry, and archived tasks to reflect exact queue state.
func (h *AutomationHandler) getQueueLastRunInfo(queueName string) (lastRun string, lastRunTs int64, status string) {
	if h.inspector == nil {
		return "Never", 0, "Never run"
	}

	// 1. Completed tasks (most recent first) - only present when Retention is set
	if tasks, err := h.inspector.ListCompletedTasks(queueName, asynq.PageSize(1)); err == nil && len(tasks) > 0 {
		t := tasks[0]
		lastRunTs = t.CompletedAt.UnixMilli()
		lastRun = t.CompletedAt.Format("1/2/2006, 3:04:05 PM")
		if t.LastErr != "" {
			status = "Failed"
		} else {
			status = "Success"
		}
		return lastRun, lastRunTs, status
	}

	// 2. Active tasks (currently running)
	if tasks, err := h.inspector.ListActiveTasks(queueName); err == nil && len(tasks) > 0 {
		status = "Running"
		lastRun = "Running now"
		lastRunTs = time.Now().UnixMilli()
		return lastRun, lastRunTs, status
	}

	// 3. Retry tasks (failed, awaiting retry)
	if tasks, err := h.inspector.ListRetryTasks(queueName); err == nil && len(tasks) > 0 {
		t := tasks[0] // most recent
		status = "Retrying"
		if t.LastFailedAt.After(time.Time{}) {
			lastRunTs = t.LastFailedAt.UnixMilli()
			lastRun = t.LastFailedAt.Format("1/2/2006, 3:04:05 PM")
		} else {
			lastRun = "Last run failed (retrying)"
		}
		return lastRun, lastRunTs, status
	}

	// 4. Archived tasks (exhausted retries)
	if tasks, err := h.inspector.ListArchivedTasks(queueName, asynq.PageSize(1)); err == nil && len(tasks) > 0 {
		t := tasks[0]
		status = "Archived"
		if t.LastFailedAt.After(time.Time{}) {
			lastRunTs = t.LastFailedAt.UnixMilli()
			lastRun = t.LastFailedAt.Format("1/2/2006, 3:04:05 PM")
		} else {
			lastRun = "Last run failed"
		}
		return lastRun, lastRunTs, status
	}

	return "Never", 0, "Never run"
}

// Overview handles GET /automation/overview.
func (h *AutomationHandler) Overview(w http.ResponseWriter, r *http.Request) {
	queues := []string{
		queue.QueueVersionUpdateCheck,
		queue.QueueSessionCleanup,
		queue.QueueOrphanedRepoCleanup,
		queue.QueueOrphanedPkgCleanup,
		queue.QueueDockerInvCleanup,
		queue.QueueAgentCommands,
		queue.QueueSystemStatistics,
		queue.QueueAlertCleanup,
		queue.QueueHostStatus,
		queue.QueueComplianceScanCleanup,
		queue.QueueSSGUpdateCheck,
	}

	stats := make(map[string]QueueStats)
	var scheduledTasks, runningTasks, failedTasks, totalAutomations int

	for _, q := range queues {
		s := h.getQueueStats(q)
		stats[q] = s
		scheduledTasks += s.Delayed
		runningTasks += s.Active
		failedTasks += s.Failed
		totalAutomations += s.Waiting + s.Active + s.Delayed + s.Failed + s.Completed
	}

	alertsEnabled := true
	if settings, _ := h.settings.GetFirst(r.Context()); settings != nil {
		alertsEnabled = settings.AlertsEnabled
	}
	updateInterval := 60
	if settings, _ := h.settings.GetFirst(r.Context()); settings != nil && settings.UpdateInterval > 0 {
		updateInterval = settings.UpdateInterval
	}

	automationDefs := []struct {
		name        string
		queue       string
		description string
		schedule    string
	}{
		{"Version Update Check", queue.QueueVersionUpdateCheck, "Checks for new PatchMon server and agent releases via DNS", "Daily at midnight"},
		{"Session Cleanup", queue.QueueSessionCleanup, "Cleans up expired user sessions", "Every hour"},
		{"Orphaned Repo Cleanup", queue.QueueOrphanedRepoCleanup, "Removes repositories with no associated hosts", "Daily at 2 AM"},
		{"Orphaned Package Cleanup", queue.QueueOrphanedPkgCleanup, "Removes packages with no associated hosts", "Daily at 3 AM"},
		{"Docker Inventory Cleanup", queue.QueueDockerInvCleanup, "Removes Docker containers and images for non-existent hosts", "Daily at 4 AM"},
		{"Collect Host Statistics", queue.QueueAgentCommands, "Collects package statistics from connected agents only", fmt.Sprintf("Every %d minutes (Agent-driven)", updateInterval)},
		{"System Statistics Collection", queue.QueueSystemStatistics, "Collects aggregated system-wide package statistics", "Every 30 minutes"},
		{"Alert Cleanup", queue.QueueAlertCleanup, "Cleans up old alerts based on retention policies and auto-resolves expired alerts", "Daily at 3 AM"},
		{"Host Status Monitor", queue.QueueHostStatus, "Monitors host status and creates alerts when hosts go offline", "Every 5 minutes"},
		{"Compliance Scan Cleanup", queue.QueueComplianceScanCleanup, "Automatically terminates compliance scans running over 3 hours", "Daily at 1 AM"},
		{"Patch Run Cleanup", queue.QueuePatchRunCleanup, "Marks patch runs stuck in running state for over PATCH_RUN_STALL_TIMEOUT_MIN minutes as timed_out", "Every 10 minutes"},
		{"Agent Reports Cleanup", queue.QueueAgentReportsCleanup, "Deletes Agent Activity rows older than AGENT_REPORTS_RETENTION_DAYS", "Daily at 2 AM"},
		{"SSG Content Update Check", queue.QueueSSGUpdateCheck, "Checks for outdated SSG compliance content on hosts and queues upgrades", "Daily at 5 AM"},
	}

	automations := make([]map[string]interface{}, 0, len(automationDefs))
	for _, def := range automationDefs {
		lastRun, lastRunTs, status := h.getQueueLastRunInfo(def.queue)

		if (def.queue == queue.QueueAlertCleanup || def.queue == queue.QueueHostStatus) && !alertsEnabled {
			status = "Skipped (Disabled)"
		}

		automations = append(automations, map[string]interface{}{
			"name":             def.name,
			"queue":            def.queue,
			"description":      def.description,
			"schedule":         def.schedule,
			"lastRun":          lastRun,
			"lastRunTimestamp": lastRunTs,
			"status":           status,
			"stats":            stats[def.queue],
		})
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"scheduledTasks":   scheduledTasks,
			"runningTasks":     runningTasks,
			"failedTasks":      failedTasks,
			"totalAutomations": totalAutomations,
			"automations":      automations,
		},
	})
}

// Stats handles GET /automation/stats.
func (h *AutomationHandler) Stats(w http.ResponseWriter, r *http.Request) {
	queues := []string{
		queue.QueueVersionUpdateCheck,
		queue.QueueSessionCleanup,
		queue.QueueOrphanedRepoCleanup,
		queue.QueueOrphanedPkgCleanup,
		queue.QueueDockerInvCleanup,
		queue.QueueAgentCommands,
		queue.QueueSystemStatistics,
		queue.QueueAlertCleanup,
		queue.QueueHostStatus,
		queue.QueueComplianceScanCleanup,
		queue.QueuePatchRunCleanup,
		queue.QueueAgentReportsCleanup,
		queue.QueueSSGUpdateCheck,
	}

	stats := make(map[string]QueueStats)
	for _, q := range queues {
		stats[q] = h.getQueueStats(q)
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    stats,
	})
}

// Jobs handles GET /automation/jobs/:queueName.
func (h *AutomationHandler) Jobs(w http.ResponseWriter, r *http.Request) {
	queueName := chi.URLParam(r, "queueName")
	limit := parseIntQuery(r, "limit", 10)
	if limit > 50 {
		limit = 50
	}

	validQueues := map[string]bool{
		queue.QueueVersionUpdateCheck:    true,
		queue.QueueSessionCleanup:        true,
		queue.QueueOrphanedRepoCleanup:   true,
		queue.QueueOrphanedPkgCleanup:    true,
		queue.QueueDockerInvCleanup:      true,
		queue.QueueAgentCommands:         true,
		queue.QueueSystemStatistics:      true,
		queue.QueueAlertCleanup:          true,
		queue.QueueHostStatus:            true,
		queue.QueueComplianceScanCleanup: true,
		queue.QueuePatchRunCleanup:       true,
		queue.QueueAgentReportsCleanup:   true,
		queue.QueueSSGUpdateCheck:        true,
	}
	if !validQueues[queueName] {
		Error(w, http.StatusBadRequest, "Invalid queue name")
		return
	}

	if h.inspector == nil {
		JSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": []interface{}{}})
		return
	}

	tasks, err := h.inspector.ListCompletedTasks(queueName, asynq.PageSize(limit))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch jobs")
		return
	}

	formatted := make([]map[string]interface{}, 0, len(tasks))
	for _, t := range tasks {
		status := "completed"
		if t.LastErr != "" {
			status = "failed"
		}
		formatted = append(formatted, map[string]interface{}{
			"id":           t.ID,
			"name":         t.Type,
			"status":       status,
			"data":         nil,
			"failedReason": t.LastErr,
			"finishedOn":   t.CompletedAt.UnixMilli(),
			"createdAt":    t.CompletedAt.UnixMilli(),
			"attemptsMade": t.Retried,
		})
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    formatted,
	})
}

// Trigger handles POST /automation/trigger/:jobType.
func (h *AutomationHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	jobType := chi.URLParam(r, "jobType")

	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}

	var info *asynq.TaskInfo
	var err error

	host := hostFromRequest(r)
	switch jobType {
	case "version-update":
		var t *asynq.Task
		t, err = queue.NewVersionUpdateCheckTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "session-cleanup":
		var t *asynq.Task
		t, err = queue.NewSessionCleanupTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "orphaned-repo-cleanup":
		var t *asynq.Task
		t, err = queue.NewOrphanedRepoCleanupTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "orphaned-package-cleanup":
		var t *asynq.Task
		t, err = queue.NewOrphanedPkgCleanupTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "docker-inventory-cleanup":
		var t *asynq.Task
		t, err = queue.NewDockerInvCleanupTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "agent-collection":
		apiIds := h.registry.GetConnectedApiIDs()
		if len(apiIds) == 0 {
			JSON(w, http.StatusOK, map[string]interface{}{
				"success": true,
				"data":    map[string]interface{}{"enqueued": 0},
			})
			return
		}
		enqueued := 0
		for _, apiID := range apiIds {
			t, e := queue.NewReportNowTask(apiID, host)
			if e != nil {
				continue
			}
			if _, e := h.queueClient.Enqueue(t); e == nil {
				enqueued++
			}
		}
		JSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"data":    map[string]interface{}{"enqueued": enqueued},
		})
		return
	case "system-statistics":
		var t *asynq.Task
		t, err = queue.NewSystemStatisticsTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "alert-cleanup":
		var t *asynq.Task
		t, err = queue.NewAlertCleanupTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "host-status-monitor":
		var t *asynq.Task
		t, err = queue.NewHostStatusMonitorTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "compliance-scan-cleanup":
		var t *asynq.Task
		t, err = queue.NewComplianceScanCleanupTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "patch-run-cleanup":
		var t *asynq.Task
		t, err = queue.NewPatchRunCleanupTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "agent-reports-cleanup":
		var t *asynq.Task
		t, err = queue.NewAgentReportsCleanupTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	case "ssg-update-check":
		var t *asynq.Task
		t, err = queue.NewSSGUpdateCheckTask(host)
		if err == nil {
			info, err = h.queueClient.Enqueue(t)
		}
	default:
		Error(w, http.StatusBadRequest, "Invalid job type")
		return
	}

	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to trigger job: "+err.Error())
		return
	}

	jobID := ""
	if info != nil {
		jobID = info.ID
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"jobId":   jobID,
			"message": "Job triggered successfully",
		},
	})
}

// ComplianceScanCleanup handles POST /compliance/scans/cleanup (manual trigger).
func (h *AutomationHandler) ComplianceScanCleanup(w http.ResponseWriter, r *http.Request) {
	if h.queueClient == nil {
		Error(w, http.StatusServiceUnavailable, "Queue service unavailable")
		return
	}
	t, err := queue.NewComplianceScanCleanupTask(hostFromRequest(r))
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to create task")
		return
	}
	info, err := h.queueClient.Enqueue(t)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to trigger compliance scan cleanup")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"jobId":   info.ID,
			"message": "Compliance scan cleanup triggered successfully",
		},
	})
}
