// Package models defines data structures for database entities.
package models

import "time"

// Alert matches alerts table. JSON tags match Node/Prisma response for frontend compatibility.
type Alert struct {
	ID               string     `db:"id" json:"id"`
	Type             string     `db:"type" json:"type"`
	Severity         string     `db:"severity" json:"severity"`
	Title            string     `db:"title" json:"title"`
	Message          string     `db:"message" json:"message"`
	Metadata         JSON       `db:"metadata" json:"metadata"`
	IsActive         bool       `db:"is_active" json:"is_active"`
	AssignedToUserID *string    `db:"assigned_to_user_id" json:"assigned_to_user_id"`
	ResolvedAt       *time.Time `db:"resolved_at" json:"resolved_at"`
	ResolvedByUserID *string    `db:"resolved_by_user_id" json:"resolved_by_user_id"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updated_at"`
}

// AlertHistory matches alert_history table.
type AlertHistory struct {
	ID        string    `db:"id"`
	AlertID   string    `db:"alert_id"`
	UserID    *string   `db:"user_id"`
	Action    string    `db:"action"`
	Metadata  JSON      `db:"metadata"`
	CreatedAt time.Time `db:"created_at"`
}

// AlertAction matches alert_actions table.
type AlertAction struct {
	ID               string    `db:"id"`
	Name             string    `db:"name"`
	DisplayName      string    `db:"display_name"`
	Description      *string   `db:"description"`
	IsStateAction    bool      `db:"is_state_action"`
	SeverityOverride *string   `db:"severity_override"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

// AlertConfig matches alert_config table.
type AlertConfig struct {
	ID                   string    `db:"id"`
	AlertType            string    `db:"alert_type"`
	IsEnabled            bool      `db:"is_enabled"`
	DefaultSeverity      string    `db:"default_severity"`
	AutoAssignEnabled    bool      `db:"auto_assign_enabled"`
	AutoAssignUserID     *string   `db:"auto_assign_user_id"`
	AutoAssignRule       *string   `db:"auto_assign_rule"`
	AutoAssignConditions JSON      `db:"auto_assign_conditions"`
	RetentionDays        *int      `db:"retention_days"`
	AutoResolveAfterDays *int      `db:"auto_resolve_after_days"`
	CleanupResolvedOnly  bool      `db:"cleanup_resolved_only"`
	NotificationEnabled  bool      `db:"notification_enabled"`
	EscalationEnabled    bool      `db:"escalation_enabled"`
	EscalationAfterHours *int      `db:"escalation_after_hours"`
	AlertDelaySeconds    *int      `db:"alert_delay_seconds"`
	Metadata             JSON      `db:"metadata"`
	CreatedAt            time.Time `db:"created_at"`
	UpdatedAt            time.Time `db:"updated_at"`
}
