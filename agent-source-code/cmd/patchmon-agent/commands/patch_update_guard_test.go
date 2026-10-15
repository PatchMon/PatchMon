package commands

import (
	"errors"
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
		activePatchRuns, agentUpdateDeferred, agentUpdating = 0, false, false
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
	endA, _ := beginPatchRun()
	endB, _ := beginPatchRun()

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
	end, _ := beginPatchRun()
	end()
	if *calls != 1 {
		t.Fatal("a later run with no deferred update must not update")
	}
}

func TestPatchRunRefusedWhileAgentUpdates(t *testing.T) {
	stubAgentUpdater(t)
	updating := make(chan struct{})
	release := make(chan error)
	agentUpdater = func() error {
		close(updating)
		return <-release
	}
	done := make(chan error)
	go func() {
		_, err := updateAgentUnlessPatching()
		done <- err
	}()
	<-updating
	if end, ok := beginPatchRun(); ok || end != nil {
		t.Fatal("a patch run started while the agent was updating, and the update would kill it")
	}
	release <- errors.New("download failed")
	if err := <-done; err == nil {
		t.Fatal("the update error was lost")
	}
	end, ok := beginPatchRun()
	if !ok {
		t.Fatal("patch runs must be allowed again after a failed update")
	}
	end()
}

func TestDeferredUpdateBlocksNewRunsUntilItReturns(t *testing.T) {
	stubAgentUpdater(t)
	end, _ := beginPatchRun()
	if deferred, _ := updateAgentUnlessPatching(); !deferred {
		t.Fatal("update not deferred")
	}
	var refused bool
	agentUpdater = func() error {
		_, ok := beginPatchRun()
		refused = !ok
		return errors.New("failed")
	}
	end()
	if !refused {
		t.Fatal("a patch run could start while the deferred update was running")
	}
	if e, ok := beginPatchRun(); !ok {
		t.Fatal("runs must be allowed after the deferred update failed")
	} else {
		e()
	}
}
