package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/hibiken/asynq"
)

func (h *IntegrationsHandler) WithScannerAutoInstall(client *asynq.Client, inspector *asynq.Inspector) *IntegrationsHandler {
	h.queueClient = client
	h.queueInspector = inspector
	return h
}

func autoInstallTaskID(hostID string) string {
	return "compliance-install-auto:" + hostID
}

func osFamilyInstallable(statusJSON []byte) bool {
	var s struct {
		ScannerInfo *struct {
			OSFamily string `json:"os_family"`
		} `json:"scanner_info"`
	}
	if json.Unmarshal(statusJSON, &s) != nil || s.ScannerInfo == nil {
		return false
	}
	switch s.ScannerInfo.OSFamily {
	case "debian", "rhel", "suse":
		return true
	}
	return false
}

func (h *IntegrationsHandler) maybeAutoInstallScanner(ctx context.Context, host *models.Host, statusJSON []byte) bool {
	if h.queueClient == nil || h.queueInspector == nil || h.integrationStatus == nil {
		return false
	}
	if !host.ComplianceEnabled || host.ComplianceOnDemandOnly || !host.ComplianceOpenscapEnabled {
		return false
	}
	if openscapReported(statusJSON) || !osFamilyInstallable(statusJSON) {
		return false
	}
	if h.jobHistory == nil {
		return false
	}
	hasRun, err := h.jobHistory.HostHasJob(ctx, host.ID, queue.TypeInstallComplianceTools)
	if err != nil || hasRun {
		return false
	}
	if complianceInstallInProgress(ctx, h.queueInspector, h.integrationStatus, host.ID, host.ApiID) {
		return false
	}
	task, err := queue.NewInstallComplianceToolsTask(host.ID, host.ApiID, hostctx.TenantHostKey(ctx))
	if err != nil {
		return false
	}
	info, err := h.queueClient.Enqueue(task, asynq.TaskID(autoInstallTaskID(host.ID)))
	if err != nil {
		if !errors.Is(err, asynq.ErrTaskIDConflict) && !errors.Is(err, asynq.ErrDuplicateTask) {
			slog.Error("compliance auto-install: failed to queue", "host_id", host.ID, "error", err)
		}
		return false
	}
	_ = h.integrationStatus.ClearInstallEvents(ctx, host.ApiID, "compliance")
	_ = h.integrationStatus.SetComplianceInstallJob(ctx, host.ID, info.ID)
	slog.Info("compliance auto-install: queued scanner install", "host_id", host.ID, "job_id", info.ID)
	return true
}
