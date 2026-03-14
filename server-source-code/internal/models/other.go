package models

import "time"

// UpdateHistory matches update_history table.
type UpdateHistory struct {
	ID            string    `db:"id"`
	HostID        string    `db:"host_id"`
	PackagesCount int       `db:"packages_count"`
	SecurityCount int       `db:"security_count"`
	TotalPackages *int      `db:"total_packages"`
	PayloadSizeKb *float64  `db:"payload_size_kb"`
	ExecutionTime *float64  `db:"execution_time"`
	Timestamp     time.Time `db:"timestamp"`
	Status        string    `db:"status"`
	ErrorMessage  *string   `db:"error_message"`
}

// SystemStatistics matches system_statistics table.
type SystemStatistics struct {
	ID                  string    `db:"id"`
	UniquePackagesCount int       `db:"unique_packages_count"`
	UniqueSecurityCount int       `db:"unique_security_count"`
	TotalPackages       int       `db:"total_packages"`
	TotalHosts          int       `db:"total_hosts"`
	HostsNeedingUpdates int       `db:"hosts_needing_updates"`
	Timestamp           time.Time `db:"timestamp"`
}

// AutoEnrollmentToken matches auto_enrollment_tokens table.
type AutoEnrollmentToken struct {
	ID                 string      `db:"id"`
	TokenName          string      `db:"token_name"`
	TokenKey           string      `db:"token_key"`
	TokenSecret        string      `db:"token_secret"`
	CreatedByUserID    *string     `db:"created_by_user_id"`
	IsActive           bool        `db:"is_active"`
	AllowedIPRanges    StringArray `db:"allowed_ip_ranges"`
	MaxHostsPerDay     int         `db:"max_hosts_per_day"`
	HostsCreatedToday  int         `db:"hosts_created_today"`
	LastResetDate      time.Time   `db:"last_reset_date"`
	DefaultHostGroupID *string     `db:"default_host_group_id"`
	CreatedAt          time.Time   `db:"created_at"`
	UpdatedAt          time.Time   `db:"updated_at"`
	LastUsedAt         *time.Time  `db:"last_used_at"`
	ExpiresAt          *time.Time  `db:"expires_at"`
	Metadata           JSON        `db:"metadata"`
	Scopes             JSON        `db:"scopes"`
}

// JobHistory matches job_history table.
type JobHistory struct {
	ID            string     `db:"id"`
	JobID         string     `db:"job_id"`
	QueueName     string     `db:"queue_name"`
	JobName       string     `db:"job_name"`
	HostID        *string    `db:"host_id"`
	ApiID         *string    `db:"api_id"`
	Status        string     `db:"status"`
	AttemptNumber int        `db:"attempt_number"`
	ErrorMessage  *string    `db:"error_message"`
	Output        JSON       `db:"output"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
	CompletedAt   *time.Time `db:"completed_at"`
}

// ReleaseNotesAcceptance matches release_notes_acceptances table.
type ReleaseNotesAcceptance struct {
	ID         string    `db:"id"`
	UserID     string    `db:"user_id"`
	Version    string    `db:"version"`
	AcceptedAt time.Time `db:"accepted_at"`
}

// AuditLog matches audit_logs table.
type AuditLog struct {
	ID           string    `db:"id"`
	Event        string    `db:"event"`
	UserID       *string   `db:"user_id"`
	TargetUserID *string   `db:"target_user_id"`
	IPAddress    *string   `db:"ip_address"`
	UserAgent    *string   `db:"user_agent"`
	RequestID    *string   `db:"request_id"`
	Details      *string   `db:"details"`
	Success      bool      `db:"success"`
	CreatedAt    time.Time `db:"created_at"`
}
