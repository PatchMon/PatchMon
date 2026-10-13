package commands

import (
	"testing"
)

func stubAgentUpdater(t *testing.T) *int {
	t.Helper()
	calls := 0
	prev := agentUpdater
	agentUpdater = func() error { calls++; return nil }
	t.Cleanup(func() {
		agentUpdater = prev
		patchUpdateMu.Lock()
		activePatchRuns, agentUpdateDeferred = 0, false
		patchUpdateMu.Unlock()
	})
	return &calls
}

func TestAgentUpdateRunsWhenNoPatchRun(t *testing.T) {
	calls := stubAgentUpdater(t)
	deferred, err := updateAgentUnlessPatching()
	if err != nil || deferred || *calls != 1 {
		t.Fatalf("deferred=%v err=%v calls=%d", deferred, err, *calls)
	}
}

func TestAgentUpdateDeferredUntilLastPatchRunEnds(t *testing.T) {
	calls := stubAgentUpdater(t)
	endA := beginPatchRun()
	endB := beginPatchRun()

	for range 2 {
		if deferred, err := updateAgentUnlessPatching(); err != nil || !deferred {
			t.Fatalf("update during a run: deferred=%v err=%v", deferred, err)
		}
	}
	if *calls != 0 {
		t.Fatalf("updated during a patch run (%d calls)", *calls)
	}
	endA()
	if *calls != 0 {
		t.Fatal("updated while another patch run was still active")
	}
	endB()
	if *calls != 1 {
		t.Fatalf("deferred update ran %d times after the last run, want 1", *calls)
	}
	end := beginPatchRun()
	end()
	if *calls != 1 {
		t.Fatal("a later run with no deferred update must not update")
	}
}
