package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
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

func TestExpediteComplianceScanKeepsPayload(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close() })

	hostID := "8ab66de3-b7ef-45f1-bcc9-105075f77623"
	profile := "xccdf_org.ssgproject.content_profile_cis_level2_server"
	task, err := queue.NewRunScanTask(queue.RunScanPayload{HostID: hostID, ApiID: "api", ProfileType: "openscap", ProfileID: &profile})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Enqueue(task, asynq.ProcessIn(time.Hour)); err != nil {
		t.Fatal(err)
	}

	expediteComplianceScan(inspector, "api", hostID, slog.New(slog.NewTextHandler(io.Discard, nil)))

	info, err := inspector.GetTaskInfo(queue.QueueCompliance, queue.RunScanTaskID(hostID))
	if err != nil {
		t.Fatal(err)
	}
	if info.State != asynq.TaskStatePending {
		t.Fatalf("scan should run now, got %v", info.State)
	}
	var p queue.RunScanPayload
	if err := json.Unmarshal(info.Payload, &p); err != nil || p.ProfileID == nil || *p.ProfileID != profile || p.ProfileType != "openscap" {
		t.Fatalf("expedited scan lost its profile: %+v", p)
	}
}

func TestExpediteScannerInstall(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close(); _ = inspector.Close(); _ = rdb.Close() })
	statusStore := store.NewIntegrationStatusStore(&hostctx.RedisResolver{Default: rdb})
	ctx := context.Background()
	hostID := "8ab66de3-b7ef-45f1-bcc9-105075f77623"

	task, err := queue.NewInstallComplianceToolsTask(hostID, "api", "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := client.Enqueue(task, asynq.ProcessIn(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := statusStore.SetComplianceInstallJob(ctx, hostID, info.ID); err != nil {
		t.Fatal(err)
	}

	expediteScannerInstall(ctx, inspector, statusStore, "api", hostID, slog.New(slog.NewTextHandler(io.Discard, nil)))

	got, err := inspector.GetTaskInfo(queue.QueueCompliance, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != asynq.TaskStatePending {
		t.Fatalf("waiting install should run on reconnect, got %v", got.State)
	}
}
