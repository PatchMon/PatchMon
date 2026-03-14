package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/google/uuid"
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

func listAlertsRowToAlertWithDetails(r db.ListAlertsRow, latestMap map[string]db.GetLatestAlertHistoryForAlertsRow) AlertWithDetails {
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
	if l, ok := latestMap[r.ID]; ok {
		a.CurrentState = &CurrentState{
			Action:    l.Action,
			User:      rowToUserRef(l.UserIDVal, l.Username, l.Email, l.FirstName, l.LastName),
			Timestamp: pgTime(l.CreatedAt),
		}
	}
	return a
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

func listAlertsAssignedToRowToAlertWithDetails(r db.ListAlertsAssignedToRow, latestMap map[string]db.GetLatestAlertHistoryForAlertsRow) AlertWithDetails {
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
	if l, ok := latestMap[r.ID]; ok {
		a.CurrentState = &CurrentState{
			Action:    l.Action,
			User:      rowToUserRef(l.UserIDVal, l.Username, l.Email, l.FirstName, l.LastName),
			Timestamp: pgTime(l.CreatedAt),
		}
	}
	return a
}

// List returns alerts with optional assigned-to filter. Include inactive (resolved) alerts.
func (s *AlertsStore) List(ctx context.Context, assignedToUserID *string) ([]AlertWithDetails, error) {
	d := s.db.DB(ctx)
	ids := []string{}
	var latestMap map[string]db.GetLatestAlertHistoryForAlertsRow

	if assignedToUserID != nil && *assignedToUserID != "" {
		rows, err := d.Queries.ListAlertsAssignedTo(ctx, assignedToUserID)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, nil
		}
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		latestRows, err := d.Queries.GetLatestAlertHistoryForAlerts(ctx, ids)
		if err != nil {
			return nil, err
		}
		latestMap = make(map[string]db.GetLatestAlertHistoryForAlertsRow)
		for _, l := range latestRows {
			latestMap[l.AlertID] = l
		}
		out := make([]AlertWithDetails, len(rows))
		for i, r := range rows {
			out[i] = listAlertsAssignedToRowToAlertWithDetails(r, latestMap)
		}
		return out, nil
	}

	rows, err := d.Queries.ListAlerts(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	latestRows, err := d.Queries.GetLatestAlertHistoryForAlerts(ctx, ids)
	if err != nil {
		return nil, err
	}
	latestMap = make(map[string]db.GetLatestAlertHistoryForAlertsRow)
	for _, l := range latestRows {
		latestMap[l.AlertID] = l
	}
	out := make([]AlertWithDetails, len(rows))
	for i, r := range rows {
		out[i] = listAlertsRowToAlertWithDetails(r, latestMap)
	}
	return out, nil
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
		ResolvedAt:       pgtype.Timestamp{Time: now, Valid: true},
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
