package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
)

// DefaultUpdateIntervalMinutes is the fallback agent update interval used when
// settings are unreadable or unset.
const DefaultUpdateIntervalMinutes = 60

// StatusActive is the stored provisioning status of a host that has checked in
// at least once. It is a lifecycle value: nothing ever writes "inactive" to the
// hosts.status column, which is why staleness has to be derived at read time.
const StatusActive = "active"

// StatusInactive is the derived status of an active host that has stopped
// reporting. It exists only in API responses, never in the database.
const StatusInactive = "inactive"

// UpdateIntervalMinutesFromDB reads the configured agent update interval for the
// calling context, falling back to DefaultUpdateIntervalMinutes. Callers that
// already hold a DashboardStore should use UpdateIntervalMinutes instead.
func UpdateIntervalMinutesFromDB(ctx context.Context, dbp database.DBProvider) int {
	return ResolveUpdateIntervalMinutes(ctx, dbp.DB(ctx))
}

// ResolveUpdateIntervalMinutes is the same lookup for callers that already hold
// a resolved connection, such as queue workers.
func ResolveUpdateIntervalMinutes(ctx context.Context, d *database.DB) int {
	setting, err := d.Queries.GetFirstSettings(ctx)
	if err != nil {
		return DefaultUpdateIntervalMinutes
	}
	if s := dbSettingToModel(setting); s.UpdateInterval > 0 {
		return s.UpdateInterval
	}
	return DefaultUpdateIntervalMinutes
}

// StaleCutoff returns the instant before which a host counts as stale: two
// agent update intervals back from now. Kept in one place so every surface that
// reports host status uses the same boundary as the GetHostsWithCounts SQL.
func StaleCutoff(now time.Time, updateIntervalMinutes int) time.Time {
	if updateIntervalMinutes <= 0 {
		updateIntervalMinutes = DefaultUpdateIntervalMinutes
	}
	return now.Add(-time.Duration(updateIntervalMinutes*2) * time.Minute)
}

// OverdueCutoff returns the instant before which a host has missed a report
// cycle: one agent update interval back from now. Shared by the host list SQL's
// overdue_threshold and the reconnect catch-up so both use the same boundary.
func OverdueCutoff(now time.Time, updateIntervalMinutes int) time.Time {
	if updateIntervalMinutes <= 0 {
		updateIntervalMinutes = DefaultUpdateIntervalMinutes
	}
	return now.Add(-time.Duration(updateIntervalMinutes) * time.Minute)
}

// EffectiveStatus derives the status the UI renders from the stored
// provisioning status and the agent's last check-in.
//
// Mirrors the effective_status CASE in GetHostsWithCounts exactly, including
// the deliberate asymmetry that only an "active" host degrades: a host that
// enrolled but never reported stays "pending" rather than becoming "inactive".
func EffectiveStatus(status string, lastUpdate, staleCutoff time.Time) string {
	if status == StatusActive && lastUpdate.Before(staleCutoff) {
		return StatusInactive
	}
	return status
}

// StaleCutoff resolves the stale boundary for the calling context.
func (s *HostsStore) StaleCutoff(ctx context.Context) time.Time {
	return StaleCutoff(time.Now(), UpdateIntervalMinutesFromDB(ctx, s.db))
}
