package queue

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/hibiken/asynq"
)

// With DISABLE_TELEMETRY the job must return before touching any database; a
// nil DB would panic if it got that far.
func TestMetricsSendHandler_Disabled(t *testing.T) {
	h := NewMetricsSendHandler(nil, nil, "1.0.0", true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := h.ProcessTask(context.Background(), asynq.NewTask(TypeMetricsSend, nil)); err != nil {
		t.Fatalf("ProcessTask = %v, want nil", err)
	}
}
