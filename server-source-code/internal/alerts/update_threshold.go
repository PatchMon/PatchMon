package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/jackc/pgx/v5"
)

const defaultThresholdIntervalMinutes = 30

type thresholdCheck struct {
	alertType        string
	resolvedType     string
	defaultThreshold int
	count            func(db.GetPendingUpdateCountsPerHostRow) int
	title            func(host string, n int) string
	message          func(host string, n, threshold int) string
	resolvedTitle    func(host string) string
	resolvedMessage  func(host string, n, threshold int) string
	resolvedReason   string
}

var thresholdChecks = []thresholdCheck{
	{
		alertType:        "host_security_updates_exceeded",
		resolvedType:     "host_security_updates_resolved",
		defaultThreshold: 1,
		count:            func(c db.GetPendingUpdateCountsPerHostRow) int { return int(c.SecurityCount) },
		title: func(h string, n int) string {
			return fmt.Sprintf("Host %s has %d security updates pending", h, n)
		},
		message: func(h string, n, t int) string {
			return fmt.Sprintf("Host \"%s\" has %d pending security updates, exceeding threshold of %d.", h, n, t)
		},
		resolvedTitle: func(h string) string { return fmt.Sprintf("Host %s security updates resolved", h) },
		resolvedMessage: func(h string, n, t int) string {
			return fmt.Sprintf("Host \"%s\" security update count (%d) is now within threshold (%d).", h, n, t)
		},
		resolvedReason: "Security update count fell below threshold",
	},
	{
		alertType:        "host_pending_updates_exceeded",
		resolvedType:     "host_pending_updates_resolved",
		defaultThreshold: 10,
		count:            func(c db.GetPendingUpdateCountsPerHostRow) int { return int(c.PendingCount) },
		title: func(h string, n int) string {
			return fmt.Sprintf("Host %s has %d pending updates", h, n)
		},
		message: func(h string, n, t int) string {
			return fmt.Sprintf("Host \"%s\" has %d pending updates, exceeding threshold of %d.", h, n, t)
		},
		resolvedTitle: func(h string) string { return fmt.Sprintf("Host %s pending updates resolved", h) },
		resolvedMessage: func(h string, n, t int) string {
			return fmt.Sprintf("Host \"%s\" pending update count (%d) is now within threshold (%d).", h, n, t)
		},
		resolvedReason: "Pending update count fell below threshold",
	},
}

type dueThresholdCheck struct {
	thresholdCheck
	cfg *store.AlertConfigWithUser
}

// ProcessUpdateThresholdMonitor checks per-host pending/security update counts against configured thresholds
// and emits alert events when thresholds are exceeded or resolved. Each type runs on its own check interval.
func ProcessUpdateThresholdMonitor(ctx context.Context, d *database.DB, tenantHost string, emit *notifications.Emitter, log *slog.Logger) (int, error) {
	enabled, err := IsAlertsEnabled(ctx, d)
	if err != nil || !enabled {
		log.Debug("update_threshold: alerts disabled")
		return 0, nil
	}

	now := time.Now()
	var due []dueThresholdCheck
	for _, chk := range thresholdChecks {
		cfg, err := GetConfigForType(ctx, d, chk.alertType)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !cfg.IsEnabled) {
			emit.ClearConditions(ctx, tenantHost, chk.alertType)
			continue
		}
		if err != nil {
			log.Debug("update_threshold: config lookup failed", "alert_type", chk.alertType, "error", err)
			continue
		}
		if !emit.ClaimCheckDue(ctx, tenantHost, chk.alertType, thresholdInterval(cfg), now) {
			continue
		}
		due = append(due, dueThresholdCheck{thresholdCheck: chk, cfg: cfg})
	}
	if len(due) == 0 {
		return 0, nil
	}

	release := func() {
		for _, chk := range due {
			emit.ReleaseCheck(ctx, tenantHost, chk.alertType)
		}
	}

	counts, err := d.Queries.GetPendingUpdateCountsPerHost(ctx)
	if err != nil {
		release()
		return 0, err
	}
	countByHost := make(map[string]db.GetPendingUpdateCountsPerHostRow, len(counts))
	for _, c := range counts {
		countByHost[c.HostID] = c
	}

	hostRows, err := d.Queries.ListHosts(ctx)
	if err != nil {
		release()
		return 0, err
	}

	alertsCreated := 0
	for _, chk := range due {
		n, err := evaluateThreshold(ctx, d, tenantHost, emit, chk, hostRows, countByHost, now)
		if err != nil {
			emit.ReleaseCheck(ctx, tenantHost, chk.alertType)
			log.Warn("update_threshold: evaluation failed", "alert_type", chk.alertType, "error", err)
			continue
		}
		alertsCreated += n
	}
	return alertsCreated, nil
}

func evaluateThreshold(ctx context.Context, d *database.DB, tenantHost string, emit *notifications.Emitter, chk dueThresholdCheck, hostRows []db.Host, countByHost map[string]db.GetPendingUpdateCountsPerHostRow, now time.Time) (int, error) {
	threshold := parseThreshold(chk.cfg, chk.defaultThreshold)
	alertsByHostID := buildAlertsByHostID(ctx, d, chk.alertType)

	exceeded := make([]string, 0)
	for _, host := range hostRows {
		if host.Status == "active" && chk.count(countByHost[host.ID]) > threshold {
			exceeded = append(exceeded, host.ID)
		}
	}

	entered, left, err := emit.SyncConditions(ctx, tenantHost, chk.alertType, exceeded, now)
	if err != nil {
		return 0, err
	}
	isNew := make(map[string]bool, len(entered))
	for _, id := range entered {
		isNew[id] = true
	}
	if len(left) > 0 {
		recovered := make(map[string]time.Time, len(left))
		for _, id := range left {
			recovered[id] = now
		}
		emit.CancelPending(ctx, tenantHost, chk.alertType, recovered)
	}

	alertsStore := store.NewAlertsStore(d)
	alertsCreated := 0
	for _, host := range hostRows {
		if host.Status != "active" {
			continue
		}
		n := chk.count(countByHost[host.ID])
		hostName := hostDisplayName(host)
		alertID, hasAlert := alertsByHostID[host.ID]

		if n > threshold {
			if hasAlert {
				_ = d.Queries.UpdateAlert(ctx, alertID)
				continue
			}
			if !isNew[host.ID] || emit == nil {
				continue
			}
			emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
				Type:          chk.alertType,
				Severity:      DefaultSeverity(chk.cfg.DefaultSeverity, "warning"),
				Title:         chk.title(hostName, n),
				Message:       chk.message(hostName, n, threshold),
				ReferenceType: "host",
				ReferenceID:   host.ID,
				Metadata: map[string]interface{}{
					"host_id":   host.ID,
					"host_name": hostName,
					"count":     n,
					"threshold": threshold,
				},
			})
			alertsCreated++
			continue
		}

		if !hasAlert {
			continue
		}
		_ = alertsStore.UpdateResolved(ctx, alertID, nil)
		_ = alertsStore.RecordHistory(ctx, alertID, nil, "resolved", map[string]interface{}{
			"resolved_reason": chk.resolvedReason,
			"system_action":   true,
		})
		if emit != nil {
			emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
				Type:          chk.resolvedType,
				Severity:      ResolveSeverity(ctx, d, chk.resolvedType, "informational"),
				Title:         chk.resolvedTitle(hostName),
				Message:       chk.resolvedMessage(hostName, n, threshold),
				ReferenceType: "host",
				ReferenceID:   host.ID,
				Metadata:      map[string]interface{}{"host_id": host.ID, "host_name": hostName, "count": n, "threshold": threshold},
			})
		}
	}
	return alertsCreated, nil
}

func thresholdInterval(cfg *store.AlertConfigWithUser) time.Duration {
	minutes := defaultThresholdIntervalMinutes
	if cfg != nil && cfg.CheckIntervalMinutes != nil && *cfg.CheckIntervalMinutes >= 1 {
		minutes = *cfg.CheckIntervalMinutes
	}
	return time.Duration(minutes) * time.Minute
}

// buildAlertsByHostID returns a map of host_id → alert_id for active alerts of the given type.
func buildAlertsByHostID(ctx context.Context, d *database.DB, alertType string) map[string]string {
	m := make(map[string]string)
	activeAlerts, _ := d.Queries.ListActiveAlertsByType(ctx, alertType)
	for _, a := range activeAlerts {
		var meta map[string]interface{}
		if len(a.Metadata) > 0 {
			_ = json.Unmarshal(a.Metadata, &meta)
		}
		if meta != nil {
			if hid, ok := meta["host_id"].(string); ok {
				m[hid] = a.ID
			}
		}
	}
	return m
}

// parseThreshold extracts the threshold value from alert config metadata JSONB.
func parseThreshold(cfg *store.AlertConfigWithUser, defaultVal int) int {
	if cfg == nil || len(cfg.Metadata) == 0 {
		return defaultVal
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(cfg.Metadata, &meta); err != nil {
		return defaultVal
	}
	if t, ok := meta["threshold"]; ok {
		switch v := t.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return defaultVal
}
