package queue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

// HostQueueData is the result of GetHostJobs for a host.
type HostQueueData struct {
	Waiting    int
	Active     int
	Delayed    int
	Failed     int
	JobHistory []HostJobRow
}

// HostJobRow matches the frontend job history shape.
type HostJobRow struct {
	ID            string      `json:"id"`
	JobID         string      `json:"job_id"`
	JobName       string      `json:"job_name"`
	QueueName     *string     `json:"queue_name,omitempty"`
	Status        string      `json:"status"`
	AttemptNumber int         `json:"attempt_number"`
	ErrorMessage  *string     `json:"error_message,omitempty"`
	Output        interface{} `json:"output,omitempty"`
	CreatedAt     *time.Time  `json:"created_at,omitempty"`
	UpdatedAt     *time.Time  `json:"updated_at,omitempty"`
	CompletedAt   *time.Time  `json:"completed_at,omitempty"`
}

// GetHostJobs returns queue stats and live job list for a host (by api_id).
// It inspects the agent-commands queue and filters tasks by api_id in payload.
// Caller should merge with DB job_history for full history.
func GetHostJobs(ctx context.Context, inspector *asynq.Inspector, apiID string, limit int) (*HostQueueData, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	data := &HostQueueData{JobHistory: []HostJobRow{}}

	// Only agent-commands queue has host-specific jobs (report_now with api_id)
	queueName := QueueAgentCommands

	// Helper to filter tasks by api_id in payload
	filterByApiID := func(tasks []*asynq.TaskInfo) ([]*asynq.TaskInfo, int) {
		var out []*asynq.TaskInfo
		for _, t := range tasks {
			var p ReportNowPayload
			if err := json.Unmarshal(t.Payload, &p); err != nil {
				continue
			}
			if p.ApiID == apiID {
				out = append(out, t)
			}
		}
		return out, len(out)
	}

	// List all task types from the queue
	pending, _ := inspector.ListPendingTasks(queueName)
	active, _ := inspector.ListActiveTasks(queueName)
	scheduled, _ := inspector.ListScheduledTasks(queueName)
	retry, _ := inspector.ListRetryTasks(queueName)
	completed, _ := inspector.ListCompletedTasks(queueName, asynq.PageSize(limit))

	hostPending, n1 := filterByApiID(pending)
	hostActive, n2 := filterByApiID(active)
	hostScheduled, n3 := filterByApiID(scheduled)
	hostRetry, n4 := filterByApiID(retry)
	hostCompleted, _ := filterByApiID(completed)

	data.Waiting = n1
	data.Active = n2
	data.Delayed = n3
	data.Failed = n4

	// Build job history: live jobs first (active, waiting, delayed, retry, completed), then we'll merge with DB
	liveJobIDs := make(map[string]bool)
	rows := []HostJobRow{}

	appendTask := func(t *asynq.TaskInfo, state string) {
		if liveJobIDs[t.ID] {
			return
		}
		liveJobIDs[t.ID] = true
		var createdAt time.Time
		switch state {
		case "completed":
			createdAt = t.CompletedAt
		case "failed":
			if !t.LastFailedAt.IsZero() {
				createdAt = t.LastFailedAt
			}
		case "delayed":
			if !t.NextProcessAt.IsZero() {
				createdAt = t.NextProcessAt
			}
		}
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		attempt := t.Retried + 1
		if attempt < 1 {
			attempt = 1
		}
		var errMsg *string
		if t.LastErr != "" {
			errMsg = &t.LastErr
		}
		row := HostJobRow{
			ID:            t.ID,
			JobID:         t.ID,
			JobName:       t.Type,
			QueueName:     &queueName,
			Status:        state,
			AttemptNumber: attempt,
			ErrorMessage:  errMsg,
			CreatedAt:     &createdAt,
		}
		if state == "completed" && !t.CompletedAt.IsZero() {
			row.CompletedAt = &t.CompletedAt
		}
		rows = append(rows, row)
	}

	for _, t := range hostActive {
		appendTask(t, "active")
	}
	for _, t := range hostPending {
		appendTask(t, "waiting")
	}
	for _, t := range hostScheduled {
		appendTask(t, "delayed")
	}
	for _, t := range hostRetry {
		appendTask(t, "failed")
	}
	for _, t := range hostCompleted {
		appendTask(t, "completed")
	}

	// Trim to limit
	if len(rows) > limit {
		rows = rows[:limit]
	}
	data.JobHistory = rows
	return data, nil
}

// RunScanTaskInfo is a run_scan task with parsed payload, for active scans display.
type RunScanTaskInfo struct {
	ID          string
	HostID      string
	ApiID       string
	ProfileType string
	ProfileName string
	State       string // "pending", "active"
	StartedAt   time.Time
}

// ListRunScanTasks returns run_scan tasks from the compliance queue (pending + active).
func ListRunScanTasks(ctx context.Context, inspector *asynq.Inspector) ([]RunScanTaskInfo, error) {
	if inspector == nil {
		return nil, nil
	}
	var out []RunScanTaskInfo
	appendTask := func(t *asynq.TaskInfo, state string) {
		if t.Type != TypeRunScan {
			return
		}
		var p RunScanPayload
		if err := json.Unmarshal(t.Payload, &p); err != nil {
			return
		}
		startedAt := t.NextProcessAt
		if startedAt.IsZero() {
			startedAt = time.Now()
		}
		var profileName string
		switch p.ProfileType {
		case "openscap":
			profileName = "OpenSCAP"
		case "docker-bench":
			profileName = "Docker Bench"
		default:
			profileName = p.ProfileType
		}
		out = append(out, RunScanTaskInfo{
			ID:          t.ID,
			HostID:      p.HostID,
			ApiID:       p.ApiID,
			ProfileType: p.ProfileType,
			ProfileName: profileName,
			State:       state,
			StartedAt:   startedAt,
		})
	}
	active, _ := inspector.ListActiveTasks(QueueCompliance)
	for _, t := range active {
		appendTask(t, "active")
	}
	pending, _ := inspector.ListPendingTasks(QueueCompliance)
	for _, t := range pending {
		appendTask(t, "pending")
	}
	return out, nil
}
