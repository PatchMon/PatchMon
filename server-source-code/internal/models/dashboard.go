package models

import "time"

// DashboardPreference matches dashboard_preferences table.
type DashboardPreference struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	CardID    string    `db:"card_id"`
	Enabled   bool      `db:"enabled"`
	Order     int       `db:"order"`
	ColSpan   int       `db:"col_span"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// DashboardLayout matches dashboard_layout table.
type DashboardLayout struct {
	UserID        string    `db:"user_id"`
	StatsColumns  int       `db:"stats_columns"`
	ChartsColumns int       `db:"charts_columns"`
	UpdatedAt     time.Time `db:"updated_at"`
}
