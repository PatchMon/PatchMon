package handler

import (
	"errors"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

func TestEnqueueScanReplacesArchivedTask(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close() })
	h := &ComplianceHandler{queueClient: client, queueInspector: inspector}

	hostID := "8ab66de3-b7ef-45f1-bcc9-105075f77623"
	newTask := func() *asynq.Task {
		task, err := queue.NewRunScanTask(queue.RunScanPayload{HostID: hostID, ApiID: "api", ProfileType: "all"})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}

	if _, err := h.enqueueScan(newTask(), hostID); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if _, err := h.enqueueScan(newTask(), hostID); !errors.Is(err, asynq.ErrTaskIDConflict) {
		t.Fatalf("a pending scan must still block a duplicate, got %v", err)
	}

	if err := inspector.ArchiveTask(queue.QueueCompliance, queue.RunScanTaskID(hostID)); err != nil {
		t.Fatalf("archive: %v", err)
	}
	info, err := h.enqueueScan(newTask(), hostID)
	if err != nil {
		t.Fatalf("an archived scan must not block a new one: %v", err)
	}
	if info.State != asynq.TaskStatePending {
		t.Fatalf("new scan should be pending, got %v", info.State)
	}
}
