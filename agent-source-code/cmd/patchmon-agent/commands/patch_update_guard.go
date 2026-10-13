package commands

import (
	"context"
	"sync"
	"time"

	"patchmon-agent/internal/client"
	"patchmon-agent/internal/logutil"
)

var (
	patchUpdateMu       sync.Mutex
	activePatchRuns     int
	agentUpdateDeferred bool
	agentUpdating       bool
	agentUpdater        = updateAgent
)

const msgAgentUpdating = "Agent update in progress, retry shortly"

// beginPatchRun marks a patch run active, or refuses it while an agent update is running,
// since the update ends the process. The returned func ends the run and applies any agent
// update that arrived meanwhile, after the run's post-patch report.
func beginPatchRun() (end func(), ok bool) {
	patchUpdateMu.Lock()
	if agentUpdating {
		patchUpdateMu.Unlock()
		return nil, false
	}
	activePatchRuns++
	patchUpdateMu.Unlock()
	return func() {
		patchUpdateMu.Lock()
		activePatchRuns--
		apply := activePatchRuns == 0 && agentUpdateDeferred
		if apply {
			agentUpdateDeferred = false
			agentUpdating = true
		}
		patchUpdateMu.Unlock()
		if apply {
			logger.Info("Patch run finished, applying the deferred agent update")
			if err := runAgentUpdate(); err != nil {
				logger.WithError(err).Warn("Deferred agent update failed")
			}
		}
	}, true
}

// updateAgentUnlessPatching updates the agent now, or defers it while a patch run is active.
func updateAgentUnlessPatching() (deferred bool, err error) {
	patchUpdateMu.Lock()
	if activePatchRuns > 0 {
		agentUpdateDeferred = true
		patchUpdateMu.Unlock()
		logger.Info("Deferring agent update until the patch run finishes")
		return true, nil
	}
	agentUpdating = true
	patchUpdateMu.Unlock()
	return false, runAgentUpdate()
}

// runAgentUpdate expects agentUpdating to be set. A successful update exits the process;
// any return means it did not happen, so patch runs are allowed again.
func runAgentUpdate() error {
	err := agentUpdater()
	patchUpdateMu.Lock()
	agentUpdating = false
	patchUpdateMu.Unlock()
	return err
}

func rejectPatchRun(patchRunID, reason string) {
	logger.WithField("patch_run_id", logutil.Sanitize(patchRunID)).Warn("Patch run refused: " + reason)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := client.New(cfgManager, logger).SendPatchOutput(ctx, patchRunID, "failed", "", reason); err != nil {
		logger.WithError(err).Warn("Failed to send patch failed output to server")
	}
}
