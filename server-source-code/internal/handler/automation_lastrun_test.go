package handler

import (
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

// #698: asynq returns completed tasks oldest first, so reading page one made
// "Last Run" freeze at the first retained completion while the total-runs
// counter kept climbing. The newest completion lives on the final page.
func TestLastCompletedPage(t *testing.T) {
	tests := []struct {
		name      string
		completed int
		pageSize  int
		want      int
	}{
		{"empty", 0, 100, 1},
		{"single task", 1, 100, 1},
		{"exactly one full page", 100, 100, 1},
		{"one past a full page", 101, 100, 2},
		{"several pages", 752, 100, 8},
		{"exact multiple", 300, 100, 3},
		{"guards zero page size", 50, 0, 1},
		{"guards negative count", -5, 100, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastCompletedPage(tt.completed, tt.pageSize); got != tt.want {
				t.Errorf("lastCompletedPage(%d, %d) = %d, want %d",
					tt.completed, tt.pageSize, got, tt.want)
			}
		})
	}
}

func TestNewestByCompletedAt(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	mk := func(offset time.Duration) *asynq.TaskInfo {
		return &asynq.TaskInfo{CompletedAt: base.Add(offset)}
	}

	t.Run("picks the latest regardless of position", func(t *testing.T) {
		// Retention differences mean expiry order need not match completion
		// order, so the newest is not reliably last.
		tasks := []*asynq.TaskInfo{
			mk(10 * time.Minute),
			mk(45 * time.Minute),
			mk(5 * time.Minute),
		}
		got := newestByCompletedAt(tasks)
		if got == nil || !got.CompletedAt.Equal(base.Add(45*time.Minute)) {
			t.Errorf("got %v, want %v", got, base.Add(45*time.Minute))
		}
	})

	t.Run("empty page yields nil", func(t *testing.T) {
		if got := newestByCompletedAt(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("skips nil entries", func(t *testing.T) {
		got := newestByCompletedAt([]*asynq.TaskInfo{nil, mk(time.Minute), nil})
		if got == nil || !got.CompletedAt.Equal(base.Add(time.Minute)) {
			t.Errorf("got %v, want %v", got, base.Add(time.Minute))
		}
	})
}
