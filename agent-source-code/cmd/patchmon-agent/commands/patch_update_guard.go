package commands

import "sync"

var (
	patchUpdateMu       sync.Mutex
	activePatchRuns     int
	agentUpdateDeferred bool
	agentUpdater        = updateAgent
)

// beginPatchRun marks a patch run active. The returned func ends it and applies any agent
// update that arrived meanwhile, after the run's post-patch report.
func beginPatchRun() func() {
	patchUpdateMu.Lock()
	activePatchRuns++
	patchUpdateMu.Unlock()
	return func() {
		patchUpdateMu.Lock()
		activePatchRuns--
		apply := activePatchRuns == 0 && agentUpdateDeferred
		if apply {
			agentUpdateDeferred = false
		}
		patchUpdateMu.Unlock()
		if apply {
			logger.Info("Patch run finished, applying the deferred agent update")
			if err := agentUpdater(); err != nil {
				logger.WithError(err).Warn("Deferred agent update failed")
			}
		}
	}
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
	patchUpdateMu.Unlock()
	return false, agentUpdater()
}
