package models

import "time"

// Package matches packages table.
type Package struct {
	ID            string    `db:"id" json:"id"`
	Name          string    `db:"name" json:"name"`
	Description   *string   `db:"description" json:"description"`
	Category      *string   `db:"category" json:"category"`
	LatestVersion *string   `db:"latest_version" json:"latest_version"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// Repository matches repositories table.
type Repository struct {
	ID           string    `db:"id" json:"id"`
	Name         string    `db:"name" json:"name"`
	URL          string    `db:"url" json:"url"`
	Distribution string    `db:"distribution" json:"distribution"`
	Components   string    `db:"components" json:"components"`
	RepoType     string    `db:"repo_type" json:"repo_type"`
	IsActive     bool      `db:"is_active" json:"is_active"`
	IsSecure     bool      `db:"is_secure" json:"isSecure"`
	Priority     *int      `db:"priority" json:"priority,omitempty"`
	Description  *string   `db:"description" json:"description,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}
