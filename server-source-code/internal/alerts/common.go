package alerts

import (
	"context"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// IsAlertsEnabled returns true if the global alerts system is enabled.
func IsAlertsEnabled(ctx context.Context, db *database.DB) (bool, error) {
	settings, err := db.Queries.GetFirstSettings(ctx)
	if err != nil {
		return false, err
	}
	return settings.AlertsEnabled, nil
}

// GetConfigForType returns the alert config for the given type, or nil if not found/disabled.
func GetConfigForType(ctx context.Context, db *database.DB, alertType string) (*store.AlertConfigWithUser, error) {
	cfg, err := store.NewAlertConfigStore(db).GetByType(ctx, alertType)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// DefaultSeverity returns the severity to use, falling back to fallback if empty.
func DefaultSeverity(severity, fallback string) string {
	if severity != "" {
		return severity
	}
	return fallback
}

// InternalAlertsDestinationID is the fixed ID for the built-in internal alerts destination.
const InternalAlertsDestinationID = "internal-alerts"

// IsInternalAlertsEnabled checks if the built-in "Internal Alerts" destination exists and is enabled.
func IsInternalAlertsEnabled(ctx context.Context, db *database.DB) bool {
	dest, err := db.Queries.GetNotificationDestinationByID(ctx, InternalAlertsDestinationID)
	if err != nil {
		return false
	}
	return dest.Enabled
}

// ResolveSeverity looks up the default_severity for an event type from alert_config,
// falling back to the provided default if not found or not enabled.
func ResolveSeverity(ctx context.Context, db *database.DB, eventType, fallback string) string {
	cfg, err := GetConfigForType(ctx, db, eventType)
	if err != nil || cfg == nil || !cfg.IsEnabled {
		return fallback
	}
	return DefaultSeverity(cfg.DefaultSeverity, fallback)
}
