package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/google/uuid"
)

// DashboardPreferencesStore provides dashboard preferences and layout access.
type DashboardPreferencesStore struct {
	db database.DBProvider
}

// NewDashboardPreferencesStore creates a new dashboard preferences store.
func NewDashboardPreferencesStore(db database.DBProvider) *DashboardPreferencesStore {
	return &DashboardPreferencesStore{db: db}
}

// ListByUserID returns all dashboard preferences for a user, ordered by order asc.
func (s *DashboardPreferencesStore) ListByUserID(ctx context.Context, userID string) ([]models.DashboardPreference, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListDashboardPreferencesByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.DashboardPreference, len(rows))
	for i := range rows {
		out[i] = dbDashboardPreferenceToModel(rows[i])
	}
	return out, nil
}

// ReplaceAll deletes existing preferences for the user and inserts the new ones.
func (s *DashboardPreferencesStore) ReplaceAll(ctx context.Context, userID string, prefs []models.DashboardPreference) error {
	d := s.db.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := d.Queries.WithTx(tx)
	if err := q.DeleteDashboardPreferencesByUserID(ctx, userID); err != nil {
		return err
	}

	now := time.Now()
	ts := pgtime.From(now)
	for i := range prefs {
		prefs[i].ID = uuid.New().String()
		prefs[i].UserID = userID
		prefs[i].CreatedAt = now
		prefs[i].UpdatedAt = now
		arg := db.InsertDashboardPreferenceParams{
			ID:        prefs[i].ID,
			UserID:    userID,
			CardID:    prefs[i].CardID,
			Enabled:   prefs[i].Enabled,
			Order:     int32(prefs[i].Order),
			ColSpan:   int32(prefs[i].ColSpan),
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		if err := q.InsertDashboardPreference(ctx, arg); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// GetLayout returns the dashboard layout for a user, or nil if not found.
func (s *DashboardPreferencesStore) GetLayout(ctx context.Context, userID string) (*models.DashboardLayout, error) {
	d := s.db.DB(ctx)
	layout, err := d.Queries.GetDashboardLayout(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := dbDashboardLayoutToModel(layout)
	return &out, nil
}

// UpsertLayout creates or updates the dashboard layout for a user.
func (s *DashboardPreferencesStore) UpsertLayout(ctx context.Context, layout *models.DashboardLayout) error {
	d := s.db.DB(ctx)
	layout.UpdatedAt = time.Now()
	arg := db.UpsertDashboardLayoutParams{
		UserID:        layout.UserID,
		StatsColumns:  int32(layout.StatsColumns),
		ChartsColumns: int32(layout.ChartsColumns),
		UpdatedAt:     pgtime.From(layout.UpdatedAt),
	}
	return d.Queries.UpsertDashboardLayout(ctx, arg)
}
