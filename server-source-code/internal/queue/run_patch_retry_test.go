package queue

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

func TestNextRunPatchRetryTaskIDAlternates(t *testing.T) {
	ids := RunPatchRetryTaskIDs("r1")
	cases := map[string]string{
		"patch-run-r1": ids[0],
		ids[0]:         ids[1],
		ids[1]:         ids[0],
		"":             ids[0],
	}
	for current, want := range cases {
		if got := NextRunPatchRetryTaskID("r1", current); got != want {
			t.Errorf("NextRunPatchRetryTaskID(%q) = %q, want %q", current, got, want)
		}
	}
}

// A retry that finds the agent still offline must be able to schedule the next one
// from inside its own run; reusing its own task ID conflicts and strands the run.
func TestRunPatchRetryCanRescheduleFromItself(t *testing.T) {
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	defer func() { _ = client.Close() }()
	inspector := asynq.NewInspector(opt)
	defer func() { _ = inspector.Close() }()

	p := RunPatchPayload{ApiID: "a", PatchRunID: "r1", PatchType: "patch_package"}
	results := make(chan error, 1)
	srv := asynq.NewServer(opt, asynq.Config{
		Queues:      map[string]int{QueuePatching: 1},
		Concurrency: 1,
		LogLevel:    asynq.FatalLevel,
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeRunPatch, func(ctx context.Context, _ *asynq.Task) error {
		current, _ := asynq.GetTaskID(ctx)
		task, err := NewRunPatchRetryTask(p, NextRunPatchRetryTaskID(p.PatchRunID, current))
		if err == nil {
			_, err = client.Enqueue(task, asynq.ProcessIn(5*time.Minute))
		}
		results <- err
		return nil
	})
	if err := srv.Start(mux); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Shutdown()

	first := RunPatchRetryTaskIDs(p.PatchRunID)[0]
	task, err := NewRunPatchRetryTask(p, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Enqueue(task); err != nil {
		t.Fatalf("enqueue first retry: %v", err)
	}

	select {
	case err := <-results:
		if err != nil {
			t.Fatalf("retry could not reschedule itself: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("retry task never ran")
	}
	next := RunPatchRetryTaskIDs(p.PatchRunID)[1]
	info, err := inspector.GetTaskInfo(QueuePatching, next)
	if err != nil {
		t.Fatalf("next retry %q not queued: %v", next, err)
	}
	if info.State != asynq.TaskStateScheduled {
		t.Fatalf("next retry state = %v, want scheduled", info.State)
	}
}
