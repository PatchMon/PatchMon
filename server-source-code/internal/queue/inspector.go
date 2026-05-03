package queue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const (
	hostJobsInspectPageSize = 100
	hostJobsInspectMaxPages = 5
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

// GetHostJobs returns queue stats and live job list for a host.
// It inspects queues that can contain host-targeted jobs and filters tasks by
// api_id or hostId in the task payload. Caller should merge with DB job_history
// for full history.
func GetHostJobs(ctx context.Context, inspector *asynq.Inspector, hostID, apiID string, limit int) (*HostQueueData, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	data := &HostQueueData{JobHistory: []HostJobRow{}}

	queueNames := []string{
		QueueAgentCommands,
		QueueCompliance,
		QueuePatching,
		QueueScheduledReports,
	}

	// Helper to filter tasks by api_id / host_id in payload.
	filterByApiID := func(tasks []*asynq.TaskInfo) ([]*asynq.TaskInfo, int) {
		var out []*asynq.TaskInfo
		for _, t := range tasks {
			var p struct {
				ApiID       string `json:"api_id"`
				HostID      string `json:"hostId"`
				HostIDSnake string `json:"host_id"`
			}
			if err := json.Unmarshal(t.Payload, &p); err != nil {
				continue
			}
			if (apiID != "" && p.ApiID == apiID) || (hostID != "" && (p.HostID == hostID || p.HostIDSnake == hostID)) {
				out = append(out, t)
			}
		}
		return out, len(out)
	}
	listAll := func(fetch func(int) ([]*asynq.TaskInfo, error)) []*asynq.TaskInfo {
		var out []*asynq.TaskInfo
		for page := 1; page <= hostJobsInspectMaxPages; page++ {
			if ctx.Err() != nil {
				return out
			}
			tasks, err := fetch(page)
			if err != nil {
				return out
			}
			out = append(out, tasks...)
			if len(tasks) < hostJobsInspectPageSize {
				return out
			}
		}
		return out
	}

	// Build job history: live jobs first (active, waiting, delayed, retry, completed), then we'll merge with DB
	liveJobIDs := make(map[string]bool)
	rows := []HostJobRow{}

	appendTask := func(queueName string, t *asynq.TaskInfo, state string) {
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
			Status:        state,
			AttemptNumber: attempt,
			ErrorMessage:  errMsg,
			CreatedAt:     &createdAt,
		}
		qn := queueName
		row.QueueName = &qn
		if state == "completed" && !t.CompletedAt.IsZero() {
			row.CompletedAt = &t.CompletedAt
		}
		rows = append(rows, row)
	}

	for _, queueName := range queueNames {
		pending := listAll(func(page int) ([]*asynq.TaskInfo, error) {
			return inspector.ListPendingTasks(queueName, asynq.Page(page), asynq.PageSize(hostJobsInspectPageSize))
		})
		active := listAll(func(page int) ([]*asynq.TaskInfo, error) {
			return inspector.ListActiveTasks(queueName, asynq.Page(page), asynq.PageSize(hostJobsInspectPageSize))
		})
		scheduled := listAll(func(page int) ([]*asynq.TaskInfo, error) {
			return inspector.ListScheduledTasks(queueName, asynq.Page(page), asynq.PageSize(hostJobsInspectPageSize))
		})
		retry := listAll(func(page int) ([]*asynq.TaskInfo, error) {
			return inspector.ListRetryTasks(queueName, asynq.Page(page), asynq.PageSize(hostJobsInspectPageSize))
		})
		completed, _ := inspector.ListCompletedTasks(queueName, asynq.PageSize(limit))

		hostPending, n1 := filterByApiID(pending)
		hostActive, n2 := filterByApiID(active)
		hostScheduled, n3 := filterByApiID(scheduled)
		hostRetry, n4 := filterByApiID(retry)
		hostCompleted, _ := filterByApiID(completed)

		data.Waiting += n1
		data.Active += n2
		data.Delayed += n3
		data.Failed += n4

		for _, t := range hostActive {
			appendTask(queueName, t, "active")
		}
		for _, t := range hostPending {
			appendTask(queueName, t, "waiting")
		}
		for _, t := range hostScheduled {
			appendTask(queueName, t, "delayed")
		}
		for _, t := range hostRetry {
			appendTask(queueName, t, "failed")
		}
		for _, t := range hostCompleted {
			appendTask(queueName, t, "completed")
		}
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
