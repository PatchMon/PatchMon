package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AlertsStore provides alert access.
type AlertsStore struct {
	db database.DBProvider
}

// NewAlertsStore creates a new alerts store.
func NewAlertsStore(db database.DBProvider) *AlertsStore {
	return &AlertsStore{db: db}
}

// AlertWithDetails is the API response shape for an alert (matches Node/frontend).
type AlertWithDetails struct {
	models.Alert
	UsersAssigned *UserRef      `json:"users_assigned"`
	CurrentState  *CurrentState `json:"current_state"`
}

// UserRef is a minimal user reference for alert assignment.
type UserRef struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	Email     string  `json:"email"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
}

// CurrentState is the latest history action for an alert.
type CurrentState struct {
	Action    string    `json:"action"`
	User      *UserRef  `json:"user"`
	Timestamp time.Time `json:"timestamp"`
}

func rowToUserRef(id, username, email *string, firstName, lastName *string) *UserRef {
	if id == nil || *id == "" {
		return nil
	}
	u := &UserRef{}
	if id != nil {
		u.ID = *id
	}
	if username != nil {
		u.Username = *username
	}
	if email != nil {
		u.Email = *email
	}
	u.FirstName = firstName
	u.LastName = lastName
	return u
}

func getAlertByIDRowToAlertWithDetails(r db.GetAlertByIDRow, latest *db.ListAlertHistoryByAlertIDRow) AlertWithDetails {
	a := AlertWithDetails{
		Alert: models.Alert{
			ID:               r.ID,
			Type:             r.Type,
			Severity:         r.Severity,
			Title:            r.Title,
			Message:          r.Message,
			Metadata:         models.JSON(r.Metadata),
			IsActive:         r.IsActive,
			AssignedToUserID: r.AssignedToUserID,
			ResolvedAt:       pgTimePtrToTime(r.ResolvedAt),
			ResolvedByUserID: r.ResolvedByUserID,
			CreatedAt:        pgTime(r.CreatedAt),
			UpdatedAt:        pgTime(r.UpdatedAt),
		},
		UsersAssigned: rowToUserRef(r.AssignedUserID, r.AssignedUsername, r.AssignedEmail, r.AssignedFirstName, r.AssignedLastName),
	}
	if latest != nil {
		a.CurrentState = &CurrentState{
			Action:    latest.Action,
			User:      rowToUserRef(latest.UserIDVal, latest.Username, latest.Email, latest.FirstName, latest.LastName),
			Timestamp: pgTime(latest.CreatedAt),
		}
	}
	return a
}

func pgTimePtrToTime(t pgtype.Timestamp) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}

// List returns every alert, optionally restricted to one assignee. Includes
// inactive (resolved) alerts. Callers that render a page of alerts should use
// ListFiltered instead — this unbounded form exists for the overview widgets
// and the scheduled report renderer, which aggregate over the whole set.
func (s *AlertsStore) List(ctx context.Context, assignedToUserID *string) ([]AlertWithDetails, error) {
	p := AlertListParams{}
	if assignedToUserID != nil {
		p.Assignment = *assignedToUserID
	}
	alerts, _, err := s.ListFiltered(ctx, p)
	return alerts, err
}

// ListFiltered returns a filtered, sorted page of alerts plus the total
// number of alerts matching the filters (ignoring the page window). When
// p.Limit is zero every match is returned and the total is simply the row
// count, so the COUNT query is skipped.
func (s *AlertsStore) ListFiltered(ctx context.Context, p AlertListParams) ([]AlertWithDetails, int, error) {
	d := s.db.DB(ctx)

	total := 0
	if p.Limit > 0 {
		countSQL, countArgs := countAlertsSQL(p)
		if err := d.RawQueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
			return nil, 0, err
		}
		if total == 0 {
			return []AlertWithDetails{}, 0, nil
		}
	}

	listSQL, listArgs := listAlertsSQL(p)
	rows, err := d.Raw(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []AlertWithDetails{}
	for rows.Next() {
		a, err := scanAlertWithDetails(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if p.Limit <= 0 {
		total = len(out)
	}
	return out, total, nil
}

// scanAlertWithDetails decodes one row of listAlertsSQL. Column order must
// stay in step with the SELECT list there.
func scanAlertWithDetails(rows pgx.Rows) (AlertWithDetails, error) {
	var (
		a                 AlertWithDetails
		metadata          []byte
		resolvedAt        *time.Time
		createdAt         time.Time
		updatedAt         time.Time
		assignedUserID    *string
		assignedUsername  *string
		assignedEmail     *string
		assignedFirstName *string
		assignedLastName  *string
		historyAction     *string
		historyCreatedAt  *time.Time
		historyUserID     *string
		historyUsername   *string
		historyEmail      *string
		historyFirstName  *string
		historyLastName   *string
		severityRank      int
	)

	if err := rows.Scan(
		&a.ID, &a.Type, &a.Severity, &a.Title, &a.Message, &metadata, &a.IsActive,
		&a.AssignedToUserID, &resolvedAt, &a.ResolvedByUserID, &createdAt, &updatedAt,
		&assignedUserID, &assignedUsername, &assignedEmail, &assignedFirstName, &assignedLastName,
		&historyAction, &historyCreatedAt, &historyUserID, &historyUsername, &historyEmail,
		&historyFirstName, &historyLastName,
		&severityRank,
	); err != nil {
		return AlertWithDetails{}, err
	}

	a.Metadata = models.JSON(metadata)
	a.ResolvedAt = resolvedAt
	a.CreatedAt = createdAt
	a.UpdatedAt = updatedAt
	a.UsersAssigned = rowToUserRef(assignedUserID, assignedUsername, assignedEmail, assignedFirstName, assignedLastName)

	if historyAction != nil {
		state := &CurrentState{
			Action: *historyAction,
			User:   rowToUserRef(historyUserID, historyUsername, historyEmail, historyFirstName, historyLastName),
		}
		if historyCreatedAt != nil {
			state.Timestamp = *historyCreatedAt
		}
		a.CurrentState = state
	}

	return a, nil
}

// DistinctTypes returns the alert types currently present in the table, for
// the Alerts page type filter. The paginated list can no longer derive them
// from the rows it holds.
func (s *AlertsStore) DistinctTypes(ctx context.Context) ([]string, error) {
	d := s.db.DB(ctx)
	rows, err := d.Raw(ctx, "SELECT DISTINCT type FROM alerts WHERE type IS NOT NULL AND type <> '' ORDER BY type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetByID returns an alert by ID.
func (s *AlertsStore) GetByID(ctx context.Context, id string) (*AlertWithDetails, error) {
	d := s.db.DB(ctx)
	row, err := d.Queries.GetAlertByID(ctx, id)
	if err != nil {
		return nil, err
	}
	history, _ := d.Queries.ListAlertHistoryByAlertID(ctx, id)
	var latest *db.ListAlertHistoryByAlertIDRow
	if len(history) > 0 {
		latest = &history[0]
	}
	a := getAlertByIDRowToAlertWithDetails(row, latest)
	return &a, nil
}

// Create creates a new alert. Returns nil if alerts are disabled.
func (s *AlertsStore) Create(ctx context.Context, alertType, severity, title, message string, metadata map[string]interface{}) (*models.Alert, error) {
	d := s.db.DB(ctx)
	enabled, err := s.isAlertsEnabled(ctx)
	if err != nil || !enabled {
		return nil, err
	}
	id := uuid.New().String()
	metaJSON, _ := json.Marshal(metadata)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	_, err = d.Queries.CreateAlert(ctx, db.CreateAlertParams{
		ID:       id,
		Type:     alertType,
		Severity: severity,
		Title:    title,
		Message:  message,
		Column6:  metaJSON,
		Column7:  true,
	})
	if err != nil {
		return nil, err
	}
	// Record "created" in history
	_, _ = d.Queries.InsertAlertHistory(ctx, db.InsertAlertHistoryParams{
		ID:      uuid.New().String(),
		AlertID: id,
		UserID:  nil,
		Action:  "created",
		Column5: []byte(`{"system_action":true}`),
	})
	return &models.Alert{ID: id, Type: alertType, Severity: severity, Title: title, Message: message}, nil
}

func (s *AlertsStore) isAlertsEnabled(ctx context.Context) (bool, error) {
	d := s.db.DB(ctx)
	settings, err := d.Queries.GetFirstSettings(ctx)
	if err != nil {
		return true, err
	}
	return settings.AlertsEnabled, nil
}

// UpdateResolved marks alert as resolved.
func (s *AlertsStore) UpdateResolved(ctx context.Context, id string, userID *string) error {
	d := s.db.DB(ctx)
	now := time.Now()
	return d.Queries.UpdateAlertResolved(ctx, db.UpdateAlertResolvedParams{
		ID:               id,
		IsActive:         false,
		ResolvedAt:       pgtime.From(now),
		ResolvedByUserID: userID,
	})
}

// UpdateUnresolve marks alert as active again.
func (s *AlertsStore) UpdateUnresolve(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateAlertUnresolve(ctx, id)
}

// UpdateAssignment sets assigned_to_user_id.
func (s *AlertsStore) UpdateAssignment(ctx context.Context, id, userID string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateAlertAssignment(ctx, db.UpdateAlertAssignmentParams{
		ID:               id,
		AssignedToUserID: &userID,
	})
}

// UpdateUnassign clears assignment.
func (s *AlertsStore) UpdateUnassign(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.UpdateAlertUnassign(ctx, id)
}

// RecordHistory inserts an alert history entry.
func (s *AlertsStore) RecordHistory(ctx context.Context, alertID string, userID *string, action string, metadata map[string]interface{}) error {
	d := s.db.DB(ctx)
	metaJSON, _ := json.Marshal(metadata)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	_, err := d.Queries.InsertAlertHistory(ctx, db.InsertAlertHistoryParams{
		ID:      uuid.New().String(),
		AlertID: alertID,
		UserID:  userID,
		Action:  action,
		Column5: metaJSON,
	})
	return err
}

// Delete deletes an alert.
func (s *AlertsStore) Delete(ctx context.Context, id string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteAlert(ctx, id)
}

// BulkDelete deletes multiple alerts.
func (s *AlertsStore) BulkDelete(ctx context.Context, ids []string) error {
	d := s.db.DB(ctx)
	return d.Queries.DeleteAlertsByIDs(ctx, ids)
}

// GetStats returns severity counts for active unresolved alerts.
func (s *AlertsStore) GetStats(ctx context.Context) (map[string]int, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.GetAlertStatsBySeverity(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]int{
		"informational": 0,
		"warning":       0,
		"error":         0,
		"critical":      0,
		"total":         0,
	}
	for _, r := range rows {
		sev := r.Severity
		if sev == "" {
			continue
		}
		if _, ok := out[sev]; ok {
			out[sev] = int(r.Count)
			out["total"] += int(r.Count)
		}
	}
	return out, nil
}
