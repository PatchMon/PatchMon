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
