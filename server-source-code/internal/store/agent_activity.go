package store

import (
	"context"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/PatchMon/PatchMon/server-source-code/internal/safeconv"
)

// AgentActivityStore returns the per-host Agent Activity feed (merged stream
// of inbound update_history reports and outbound job_history jobs).
type AgentActivityStore struct {
	db database.DBProvider
}

// NewAgentActivityStore creates a new Agent Activity store.
func NewAgentActivityStore(db database.DBProvider) *AgentActivityStore {
	return &AgentActivityStore{db: db}
}

// AgentActivityRow is the typed shape returned to handlers — empty strings on
// nullable text fields collapse to nil pointers so JSON serialisation matches
// the rest of the API (omitempty-friendly).
type AgentActivityRow struct {
	Kind               string     `json:"kind"` // "report" or "job"
	ID                 string     `json:"id"`
	OccurredAt         time.Time  `json:"occurred_at"`
	Type               string     `json:"type"`
	JobID              *string    `json:"job_id,omitempty"`
	JobName            *string    `json:"job_name,omitempty"`
	QueueName          *string    `json:"queue_name,omitempty"`
	SectionsSent       []string   `json:"sections_sent"`
	SectionsUnchanged  []string   `json:"sections_unchanged"`
	PayloadSizeKb      *float64   `json:"payload_size_kb,omitempty"`
	ServerProcessingMs *float64   `json:"server_processing_ms,omitempty"`
	AgentExecutionMs   *int       `json:"agent_execution_ms,omitempty"`
	AttemptNumber      *int       `json:"attempt_number,omitempty"`
	Status             string     `json:"status"`
	ErrorMessage       *string    `json:"error_message,omitempty"`
	PackagesCount      *int       `json:"packages_count,omitempty"`
	SecurityCount      *int       `json:"security_count,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	Output             *string    `json:"output,omitempty"`
}

// ListAgentActivityParams configures the merged feed query.
type ListAgentActivityParams struct {
	HostID    string
	Direction string   // "" | "in" | "out"
	Types     []string // empty means all report types and job names
	Statuses  []string // empty means all statuses
	Search    string   // "" means no text search; ILIKE on error_message + job output
	Since     *time.Time
	Limit     int
	Offset    int
}

// List returns the merged report+job feed for a host, ordered by occurred_at
// DESC. The second return value is the total row count of the merged set
// before LIMIT/OFFSET — used by the UI for accurate pagination. Returns 0
// when there are no rows.
func (s *AgentActivityStore) List(ctx context.Context, p ListAgentActivityParams) ([]AgentActivityRow, int64, error) {
	d := s.db.DB(ctx)
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}
	types := p.Types
	if types == nil {
		types = []string{}
	}
	statuses := p.Statuses
	if statuses == nil {
		statuses = []string{}
	}
	countParams := db.CountAgentActivityParams{
		HostID:    p.HostID,
		Direction: p.Direction,
		Types:     types,
		Statuses:  statuses,
		SinceTs:   pgtime.FromPtr(p.Since),
		Search:    p.Search,
	}
	total, err := d.Queries.CountAgentActivity(ctx, countParams)
	if err != nil {
		return nil, 0, err
	}
	rows, err := d.Queries.ListAgentActivity(ctx, db.ListAgentActivityParams{
		HostID:    p.HostID,
		Direction: p.Direction,
		Types:     types,
		Statuses:  statuses,
		Search:    p.Search,
		SinceTs:   pgtime.FromPtr(p.Since),
		RowOffset: safeconv.ClampToInt32(offset),
		RowLimit:  safeconv.ClampToInt32(limit),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]AgentActivityRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, agentActivityRowFromDB(r))
	}
	return out, int64(total), nil
}

func agentActivityRowFromDB(r db.ListAgentActivityRow) AgentActivityRow {
	row := AgentActivityRow{
		Kind:              r.Kind,
		ID:                r.RowID,
		OccurredAt:        pgTime(r.OccurredAt),
		Type:              r.Type,
		SectionsSent:      r.SectionsSent,
		SectionsUnchanged: r.SectionsUnchanged,
		PayloadSizeKb:     r.PayloadSizeKb,
		Status:            r.Status,
		ErrorMessage:      r.ErrorMessage,
	}
	if row.SectionsSent == nil {
		row.SectionsSent = []string{}
	}
	if row.SectionsUnchanged == nil {
		row.SectionsUnchanged = []string{}
	}
	if r.JobID != "" {
		v := r.JobID
		row.JobID = &v
	}
	if r.JobName != "" {
		v := r.JobName
		row.JobName = &v
	}
	if r.QueueName != "" {
		v := r.QueueName
		row.QueueName = &v
	}
	if r.ServerProcessingMs != nil {
		v := *r.ServerProcessingMs
		// update_history.execution_time is stored in seconds (legacy unit);
		// jobs already produce ms via EXTRACT(EPOCH ...)*1000. Normalise the
		// report side here so the UI can render one unit without branching.
		if r.Kind == "report" {
			v *= 1000
		}
		row.ServerProcessingMs = &v
	}
	if r.AgentExecutionMs != nil {
		v := int(*r.AgentExecutionMs)
		row.AgentExecutionMs = &v
	}
	if r.Kind == "job" {
		v := int(r.AttemptNumber)
		row.AttemptNumber = &v
	}
	if r.Kind == "report" {
		pc := int(r.PackagesCount)
		row.PackagesCount = &pc
		sc := int(r.SecurityCount)
		row.SecurityCount = &sc
	}
	if r.CompletedAt.Valid {
		t := r.CompletedAt.Time
		row.CompletedAt = &t
	}
	if strings.TrimSpace(r.Output) != "" {
		v := r.Output
		row.Output = &v
	}
	return row
}
