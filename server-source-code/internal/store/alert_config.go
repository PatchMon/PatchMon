package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// AlertConfigStore provides alert config access.
type AlertConfigStore struct {
	db database.DBProvider
}

// NewAlertConfigStore creates a new alert config store.
func NewAlertConfigStore(db database.DBProvider) *AlertConfigStore {
	return &AlertConfigStore{db: db}
}

// AlertConfigWithUser extends AlertConfig with auto-assign user info.
type AlertConfigWithUser struct {
	models.AlertConfig
	AutoAssignUser *UserRef `json:"users_auto_assign,omitempty"`
}

func alertConfigRowToModel(r db.GetAlertConfigByTypeRow) AlertConfigWithUser {
	return AlertConfigWithUser{
		AlertConfig: models.AlertConfig{
			ID:                   r.ID,
			AlertType:            r.AlertType,
			IsEnabled:            r.IsEnabled,
			DefaultSeverity:      r.DefaultSeverity,
			AutoAssignEnabled:    r.AutoAssignEnabled,
			AutoAssignUserID:     r.AutoAssignUserID,
			AutoAssignRule:       r.AutoAssignRule,
			AutoAssignConditions: models.JSON(r.AutoAssignConditions),
			RetentionDays:        ptrInt32ToInt(r.RetentionDays),
			AutoResolveAfterDays: ptrInt32ToInt(r.AutoResolveAfterDays),
			CleanupResolvedOnly:  r.CleanupResolvedOnly,
			NotificationEnabled:  r.NotificationEnabled,
			EscalationEnabled:    r.EscalationEnabled,
			EscalationAfterHours: ptrInt32ToInt(r.EscalationAfterHours),
			AlertDelaySeconds:    ptrInt32ToInt(r.AlertDelaySeconds),
			Metadata:             models.JSON(r.Metadata),
			Category:             r.Category,
			CheckIntervalMinutes: ptrInt32ToInt(r.CheckIntervalMinutes),
			CreatedAt:            pgTime(r.CreatedAt),
			UpdatedAt:            pgTime(r.UpdatedAt),
		},
		AutoAssignUser: rowToUserRef(r.AutoAssignUserIDVal, r.AutoAssignUsername, r.AutoAssignEmail, r.AutoAssignFirstName, r.AutoAssignLastName),
	}
}

func ptrInt32ToInt(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

// GetAll returns all alert configs.
func (s *AlertConfigStore) GetAll(ctx context.Context) ([]AlertConfigWithUser, error) {
	d := s.db.DB(ctx)
	rows, err := d.Queries.ListAlertConfig(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AlertConfigWithUser, len(rows))
	for i, r := range rows {
		out[i] = AlertConfigWithUser{
			AlertConfig: models.AlertConfig{
				ID:                   r.ID,
				AlertType:            r.AlertType,
				IsEnabled:            r.IsEnabled,
				DefaultSeverity:      r.DefaultSeverity,
				AutoAssignEnabled:    r.AutoAssignEnabled,
				AutoAssignUserID:     r.AutoAssignUserID,
				AutoAssignRule:       r.AutoAssignRule,
				AutoAssignConditions: models.JSON(r.AutoAssignConditions),
				RetentionDays:        ptrInt32ToInt(r.RetentionDays),
				AutoResolveAfterDays: ptrInt32ToInt(r.AutoResolveAfterDays),
				CleanupResolvedOnly:  r.CleanupResolvedOnly,
				NotificationEnabled:  r.NotificationEnabled,
				EscalationEnabled:    r.EscalationEnabled,
				EscalationAfterHours: ptrInt32ToInt(r.EscalationAfterHours),
				AlertDelaySeconds:    ptrInt32ToInt(r.AlertDelaySeconds),
				Metadata:             models.JSON(r.Metadata),
				Category:             r.Category,
				CheckIntervalMinutes: ptrInt32ToInt(r.CheckIntervalMinutes),
				CreatedAt:            pgTime(r.CreatedAt),
				UpdatedAt:            pgTime(r.UpdatedAt),
			},
			AutoAssignUser: rowToUserRef(r.AutoAssignUserIDVal, r.AutoAssignUsername, r.AutoAssignEmail, r.AutoAssignFirstName, r.AutoAssignLastName),
		}
	}
	return out, nil
}

// GetByType returns config for an alert type.
func (s *AlertConfigStore) GetByType(ctx context.Context, alertType string) (*AlertConfigWithUser, error) {
	d := s.db.DB(ctx)
	row, err := d.Queries.GetAlertConfigByType(ctx, alertType)
	if err != nil {
		return nil, err
	}
	c := alertConfigRowToModel(row)
	return &c, nil
}

// Upsert creates or updates alert config.
func (s *AlertConfigStore) Upsert(ctx context.Context, cfg *models.AlertConfig) error {
	d := s.db.DB(ctx)
	meta := cfg.Metadata
	if meta == nil {
		meta = []byte("{}")
	}
	ac := cfg.AutoAssignConditions
	if ac == nil {
		ac = []byte("{}")
	}
	var retention, autoResolve *int32
	if cfg.RetentionDays != nil {
		v := int32(*cfg.RetentionDays)
		retention = &v
	}
	if cfg.AutoResolveAfterDays != nil {
		v := int32(*cfg.AutoResolveAfterDays)
		autoResolve = &v
	}
	var escHours *int32
	if cfg.EscalationAfterHours != nil {
		v := int32(*cfg.EscalationAfterHours)
		escHours = &v
	}
	var alertDelay *int32
	if cfg.AlertDelaySeconds != nil {
		v := int32(*cfg.AlertDelaySeconds)
		alertDelay = &v
	}
	var checkInterval *int32
	if cfg.CheckIntervalMinutes != nil {
		v := int32(*cfg.CheckIntervalMinutes)
		checkInterval = &v
	}
	category := cfg.Category
	if category == "" {
		category = "general"
	}
	_, err := d.Queries.UpsertAlertConfig(ctx, db.UpsertAlertConfigParams{
		ID:                   uuid.New().String(),
		AlertType:            cfg.AlertType,
		IsEnabled:            cfg.IsEnabled,
		DefaultSeverity:      cfg.DefaultSeverity,
		AutoAssignEnabled:    cfg.AutoAssignEnabled,
		AutoAssignUserID:     cfg.AutoAssignUserID,
		AutoAssignRule:       cfg.AutoAssignRule,
		AutoAssignConditions: ac,
		RetentionDays:        retention,
		AutoResolveAfterDays: autoResolve,
		CleanupResolvedOnly:  cfg.CleanupResolvedOnly,
		NotificationEnabled:  cfg.NotificationEnabled,
		EscalationEnabled:    cfg.EscalationEnabled,
		EscalationAfterHours: escHours,
		AlertDelaySeconds:    alertDelay,
		Column16:             meta,
		Category:             category,
		CheckIntervalMinutes: checkInterval,
	})
	return err
}

// GetAlertsToCleanup returns alerts that should be cleaned up per retention config.
func (s *AlertConfigStore) GetAlertsToCleanup(ctx context.Context) ([]struct {
	ID, Type  string
	CreatedAt time.Time
}, error) {
	d := s.db.DB(ctx)
	configs, err := d.Queries.ListAlertConfig(ctx)
	if err != nil {
		return nil, err
	}
	var out []struct {
		ID, Type  string
		CreatedAt time.Time
	}
	for _, c := range configs {
		if c.RetentionDays == nil || *c.RetentionDays <= 0 {
			continue
		}
		cutoff := time.Now().AddDate(0, 0, -int(*c.RetentionDays))
		rows, err := d.Queries.GetAlertsForCleanup(ctx, db.GetAlertsForCleanupParams{
			Type:      c.AlertType,
			CreatedAt: pgtype.Timestamp{Time: cutoff, Valid: true},
			Column3:   c.CleanupResolvedOnly,
		})
		if err != nil {
			continue
		}
		for _, r := range rows {
			out = append(out, struct {
				ID, Type  string
				CreatedAt time.Time
			}{
				ID: r.ID, Type: r.Type, CreatedAt: pgTime(r.CreatedAt),
			})
		}
	}
	return out, nil
}

// CleanupOldAlerts deletes alerts per retention config.
func (s *AlertConfigStore) CleanupOldAlerts(ctx context.Context) (int, error) {
	toClean, err := s.GetAlertsToCleanup(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	d := s.db.DB(ctx)
	for _, a := range toClean {
		if err := d.Queries.DeleteAlert(ctx, a.ID); err != nil {
			continue
		}
		deleted++
	}
	return deleted, nil
}

// AutoResolveOldAlerts auto-resolves active alerts that exceed auto_resolve_after_days.
// Returns the number of alerts auto-resolved.
func (s *AlertConfigStore) AutoResolveOldAlerts(ctx context.Context) (int, error) {
	d := s.db.DB(ctx)
	configs, err := d.Queries.ListAlertConfig(ctx)
	if err != nil {
		return 0, err
	}
	alertsStore := NewAlertsStore(s.db)
	resolved := 0
	for _, c := range configs {
		if c.AutoResolveAfterDays == nil || *c.AutoResolveAfterDays <= 0 {
			continue
		}
		cutoff := time.Now().AddDate(0, 0, -int(*c.AutoResolveAfterDays))
		rows, err := d.Queries.GetAlertsToAutoResolve(ctx, db.GetAlertsToAutoResolveParams{
			Type:      c.AlertType,
			CreatedAt: pgtype.Timestamp{Time: cutoff, Valid: true},
		})
		if err != nil {
			continue
		}
		for _, r := range rows {
			if err := alertsStore.UpdateResolved(ctx, r.ID, nil); err != nil {
				continue
			}
			_ = alertsStore.RecordHistory(ctx, r.ID, nil, "resolved", map[string]interface{}{
				"reason":                  "auto_resolved",
				"auto_resolve_after_days": *c.AutoResolveAfterDays,
			})
			resolved++
		}
	}
	return resolved, nil
}
