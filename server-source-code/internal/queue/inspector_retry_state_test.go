package queue

import (
	"fmt"
	"testing"

	"github.com/hibiken/asynq"
)

func TestRetryTaskState(t *testing.T) {
	waiting := fmt.Errorf("agent offline: %w", ErrRetryLater).Error()
	installWaiting := fmt.Errorf("Agent is not connected. Cannot run install.: %w", ErrRetryLater).Error()
	tests := map[string]string{
		waiting:                  "delayed",
		installWaiting:           "delayed",
		"pq: connection refused": "failed",
		"":                       "failed",
	}
	for lastErr, want := range tests {
		if got := retryTaskState(&asynq.TaskInfo{LastErr: lastErr}); got != want {
			t.Errorf("%q: got %s, want %s", lastErr, got, want)
		}
	}
}

func TestDisplayTaskError(t *testing.T) {
	waiting := fmt.Errorf("agent offline: %w", ErrRetryLater).Error()
	if got := DisplayTaskError(waiting); got != "agent offline" {
		t.Fatalf("retry marker should be dropped, got %q", got)
	}
	if got := DisplayTaskError("pq: connection refused"); got != "pq: connection refused" {
		t.Fatalf("other errors must be shown as is, got %q", got)
	}
}
