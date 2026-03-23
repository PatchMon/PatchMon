package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// ProcessUpdateThresholdMonitor checks per-host pending/security update counts against configured thresholds
// and emits alert events when thresholds are exceeded or resolved.
func ProcessUpdateThresholdMonitor(ctx context.Context, d *database.DB, tenantHost string, emit *notifications.Emitter, log *slog.Logger) (int, error) {
	enabled, err := IsAlertsEnabled(ctx, d)
	if err != nil || !enabled {
		log.Debug("update_threshold: alerts disabled")
		return 0, nil
	}

	secCfg, err := GetConfigForType(ctx, d, "host_security_updates_exceeded")
	if err != nil {
		secCfg = nil
	}
	pendCfg, err := GetConfigForType(ctx, d, "host_pending_updates_exceeded")
	if err != nil {
		pendCfg = nil
	}

	// If both configs are nil or disabled, nothing to do.
	secEnabled := secCfg != nil && secCfg.IsEnabled
	pendEnabled := pendCfg != nil && pendCfg.IsEnabled
	if !secEnabled && !pendEnabled {
		log.Debug("update_threshold: both threshold alerts disabled")
		return 0, nil
	}

	secThreshold := parseThreshold(secCfg, 1)
	pendThreshold := parseThreshold(pendCfg, 10)

	// Get per-host update counts.
	counts, err := d.Queries.GetPendingUpdateCountsPerHost(ctx)
	if err != nil {
		return 0, err
	}
	countByHost := make(map[string]db.GetPendingUpdateCountsPerHostRow, len(counts))
	for _, c := range counts {
		countByHost[c.HostID] = c
	}

	// Load hosts for display names.
	hostRows, err := d.Queries.ListHosts(ctx)
	if err != nil {
		return 0, err
	}
	hostMap := make(map[string]db.Host, len(hostRows))
	for _, h := range hostRows {
		hostMap[h.ID] = h
	}

	// Build active alert maps for both types.
	secAlertsByHostID := buildAlertsByHostID(ctx, d, "host_security_updates_exceeded")
	pendAlertsByHostID := buildAlertsByHostID(ctx, d, "host_pending_updates_exceeded")

	alertsStore := store.NewAlertsStore(d)
	alertsCreated := 0

	for _, host := range hostRows {
		if host.Status != "active" {
			continue
		}
		c := countByHost[host.ID]
		hostName := hostDisplayName(host)

		// Security updates threshold check.
		if secEnabled {
			secCount := int(c.SecurityCount)
			if secCount > secThreshold {
				alertID, exists := secAlertsByHostID[host.ID]
				if exists {
					_ = d.Queries.UpdateAlert(ctx, alertID)
				} else if emit != nil {
					severity := DefaultSeverity(secCfg.DefaultSeverity, "warning")
					title := fmt.Sprintf("Host %s has %d security updates pending", hostName, secCount)
					msg := fmt.Sprintf("Host \"%s\" has %d pending security updates, exceeding threshold of %d.", hostName, secCount, secThreshold)
					meta := map[string]interface{}{
						"host_id":   host.ID,
						"host_name": hostName,
						"count":     secCount,
						"threshold": secThreshold,
					}
					emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
						Type: "host_security_updates_exceeded", Severity: severity, Title: title, Message: msg,
						ReferenceType: "host", ReferenceID: host.ID,
						Metadata: meta,
					})
					alertsCreated++
				}
			} else if alertID, exists := secAlertsByHostID[host.ID]; exists {
				_ = alertsStore.UpdateResolved(ctx, alertID, nil)
				_ = alertsStore.RecordHistory(ctx, alertID, nil, "resolved", map[string]interface{}{
					"resolved_reason": "Security update count fell below threshold",
					"system_action":   true,
				})
				if emit != nil {
					emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
						Type:          "host_security_updates_resolved",
						Severity:      ResolveSeverity(ctx, d, "host_security_updates_resolved", "informational"),
						Title:         fmt.Sprintf("Host %s security updates resolved", hostName),
						Message:       fmt.Sprintf("Host \"%s\" security update count (%d) is now within threshold (%d).", hostName, secCount, secThreshold),
						ReferenceType: "host",
						ReferenceID:   host.ID,
						Metadata:      map[string]interface{}{"host_id": host.ID, "host_name": hostName, "count": secCount, "threshold": secThreshold},
					})
				}
			}
		}

		// Pending updates threshold check.
		if pendEnabled {
			pendCount := int(c.PendingCount)
			if pendCount > pendThreshold {
				alertID, exists := pendAlertsByHostID[host.ID]
				if exists {
					_ = d.Queries.UpdateAlert(ctx, alertID)
				} else if emit != nil {
					severity := DefaultSeverity(pendCfg.DefaultSeverity, "warning")
					title := fmt.Sprintf("Host %s has %d pending updates", hostName, pendCount)
					msg := fmt.Sprintf("Host \"%s\" has %d pending updates, exceeding threshold of %d.", hostName, pendCount, pendThreshold)
					meta := map[string]interface{}{
						"host_id":   host.ID,
						"host_name": hostName,
						"count":     pendCount,
						"threshold": pendThreshold,
					}
					emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
						Type: "host_pending_updates_exceeded", Severity: severity, Title: title, Message: msg,
						ReferenceType: "host", ReferenceID: host.ID,
						Metadata: meta,
					})
					alertsCreated++
				}
			} else if alertID, exists := pendAlertsByHostID[host.ID]; exists {
				_ = alertsStore.UpdateResolved(ctx, alertID, nil)
				_ = alertsStore.RecordHistory(ctx, alertID, nil, "resolved", map[string]interface{}{
					"resolved_reason": "Pending update count fell below threshold",
					"system_action":   true,
				})
				if emit != nil {
					emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
						Type:          "host_pending_updates_resolved",
						Severity:      ResolveSeverity(ctx, d, "host_pending_updates_resolved", "informational"),
						Title:         fmt.Sprintf("Host %s pending updates resolved", hostName),
						Message:       fmt.Sprintf("Host \"%s\" pending update count (%d) is now within threshold (%d).", hostName, pendCount, pendThreshold),
						ReferenceType: "host",
						ReferenceID:   host.ID,
						Metadata:      map[string]interface{}{"host_id": host.ID, "host_name": hostName, "count": pendCount, "threshold": pendThreshold},
					})
				}
			}
		}
	}

	return alertsCreated, nil
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
