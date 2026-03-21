package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// ProcessHostStatusMonitor runs the periodic host-down check: finds stale hosts and creates/resolves alerts.
// Called by the host-status-monitor queue job.
func ProcessHostStatusMonitor(ctx context.Context, d *database.DB, tenantHost string, emit *notifications.Emitter, log *slog.Logger) (int, error) {
	enabled, err := IsAlertsEnabled(ctx, d)
	if err != nil || !enabled {
		log.Debug("host_down: alerts disabled")
		return 0, nil
	}

	cfg, err := GetConfigForType(ctx, d, "host_down")
	if err != nil || cfg == nil || !cfg.IsEnabled {
		log.Debug("host_down: host_down alerts disabled")
		return 0, nil
	}

	settings, err := d.Queries.GetFirstSettings(ctx)
	if err != nil {
		return 0, err
	}
	updateInterval := int(settings.UpdateInterval)
	if updateInterval <= 0 {
		updateInterval = 60
	}
	thresholdMinutes := updateInterval * 3
	threshold := time.Now().Add(-time.Duration(thresholdMinutes) * time.Minute)

	hostRows, err := d.Queries.ListHosts(ctx)
	if err != nil {
		return 0, err
	}

	hostDownAlerts, _ := d.Queries.ListActiveAlertsByType(ctx, "host_down")
	alertsByHostID := make(map[string]string)
	for _, a := range hostDownAlerts {
		var meta map[string]interface{}
		if len(a.Metadata) > 0 {
			_ = json.Unmarshal(a.Metadata, &meta)
		}
		if meta != nil {
			if hid, ok := meta["host_id"].(string); ok {
				alertsByHostID[hid] = a.ID
			}
		}
	}

	alertsStore := store.NewAlertsStore(d)
	alertsCreated := 0

	for _, host := range hostRows {
		lastUpdate := host.LastUpdate
		if !lastUpdate.Valid {
			continue
		}
		isStale := lastUpdate.Time.Before(threshold)
		hostDownEnabled := host.HostDownAlertsEnabled

		shouldCreate := false
		if hostDownEnabled != nil {
			if *hostDownEnabled {
				shouldCreate = true
			}
		} else {
			shouldCreate = cfg.IsEnabled
		}

		if isStale && host.Status == "active" && shouldCreate {
			alertID, exists := alertsByHostID[host.ID]
			if exists {
				_ = d.Queries.UpdateAlert(ctx, alertID)
				continue
			}
			hostName := hostDisplayName(host)
			severity := DefaultSeverity(cfg.DefaultSeverity, "warning")
			title := "Host " + hostName + " is offline"
			meta := map[string]interface{}{
				"host_id":           host.ID,
				"host_name":         hostName,
				"last_update":       lastUpdate.Time,
				"threshold_minutes": thresholdMinutes,
			}
			msg := fmt.Sprintf("Host \"%s\" has not reported in %d minutes. Last update: %s", hostName, thresholdMinutes, lastUpdate.Time.Format(time.RFC3339))

			// Create alert record only if Internal Alerts destination is enabled.
			if IsInternalAlertsEnabled(ctx, d) {
				alert, err := alertsStore.Create(ctx, "host_down", severity, title, msg, meta)
				if err == nil && alert != nil {
					alertsCreated++
					alertsByHostID[host.ID] = alert.ID
					if cfg.AutoAssignEnabled && cfg.AutoAssignUserID != nil {
						_ = alertsStore.UpdateAssignment(ctx, alert.ID, *cfg.AutoAssignUserID)
						_ = alertsStore.RecordHistory(ctx, alert.ID, nil, "assigned", map[string]interface{}{"assigned_to": *cfg.AutoAssignUserID})
					}
				}
			}

			// Emit event for notification routing regardless.
			if emit != nil {
				emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
					Type: "host_down", Severity: severity, Title: title, Message: msg,
					ReferenceType: "host", ReferenceID: host.ID,
					Metadata: meta,
				})
			}
		} else if !isStale {
			if alertID, exists := alertsByHostID[host.ID]; exists {
				if cfg.AutoResolveAfterDays == nil {
					_ = alertsStore.UpdateResolved(ctx, alertID, nil)
					_ = alertsStore.RecordHistory(ctx, alertID, nil, "resolved", map[string]interface{}{"resolved_reason": "Host came back online", "system_action": true})
					delete(alertsByHostID, host.ID)
					if emit != nil {
						hn := hostDisplayName(host)
						emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
							Type:          "host_recovered",
							Severity:      ResolveSeverity(ctx, d, "host_recovered", "informational"),
							Title:         "Host back online",
							Message:       fmt.Sprintf("Host %s is reporting again.", hn),
							ReferenceType: "host",
							ReferenceID:   host.ID,
							Metadata:      map[string]interface{}{"host_id": host.ID, "host_name": hn},
						})
					}
				}
			}
		}
	}

	return alertsCreated, nil
}

// OnDisconnect creates a host_down alert when an agent's WebSocket disconnects.
// Called by the agent WebSocket disconnect handler.
func OnDisconnect(ctx context.Context, d *database.DB, apiID string, tenantHost string, emit *notifications.Emitter, log *slog.Logger) {
	enabled, err := IsAlertsEnabled(ctx, d)
	if err != nil || !enabled {
		return
	}
	cfg, err := GetConfigForType(ctx, d, "host_down")
	if err != nil || cfg == nil || !cfg.IsEnabled {
		return
	}

	host, err := d.Queries.GetHostByApiID(ctx, apiID)
	if err != nil {
		return
	}
	shouldCreate := cfg.IsEnabled
	if host.HostDownAlertsEnabled != nil {
		shouldCreate = *host.HostDownAlertsEnabled
	}
	if !shouldCreate {
		return
	}

	activeAlerts, _ := d.Queries.ListActiveAlertsByType(ctx, "host_down")
	for _, a := range activeAlerts {
		var meta map[string]interface{}
		if len(a.Metadata) > 0 {
			_ = json.Unmarshal(a.Metadata, &meta)
		}
		if meta != nil {
			if hid, ok := meta["host_id"].(string); ok && hid == host.ID {
				_ = d.Queries.UpdateAlert(ctx, a.ID)
				log.Debug("host_down: updated existing alert on disconnect", "api_id", apiID, "host_id", host.ID)
				return
			}
		}
	}

	hostName := hostDisplayNameFromRow(host)
	severity := DefaultSeverity(cfg.DefaultSeverity, "warning")
	lastUpdate := time.Now()
	if host.LastUpdate.Valid {
		lastUpdate = host.LastUpdate.Time
	}
	meta := map[string]interface{}{
		"host_id":           host.ID,
		"host_name":         hostName,
		"last_update":       lastUpdate,
		"threshold_minutes": 0,
		"disconnect_reason": "websocket",
	}
	title := "Host " + hostName + " disconnected"
	msg := fmt.Sprintf("Host \"%s\" WebSocket connection lost. Last update: %s", hostName, lastUpdate.Format(time.RFC3339))

	// Create alert record only if Internal Alerts destination is enabled.
	if IsInternalAlertsEnabled(ctx, d) {
		alertsStore := store.NewAlertsStore(d)
		alert, err := alertsStore.Create(ctx, "host_down", severity, title, msg, meta)
		if err != nil || alert == nil {
			log.Debug("host_down: failed to create alert on disconnect", "api_id", apiID, "error", err)
		} else {
			if cfg.AutoAssignEnabled && cfg.AutoAssignUserID != nil {
				_ = alertsStore.UpdateAssignment(ctx, alert.ID, *cfg.AutoAssignUserID)
				_ = alertsStore.RecordHistory(ctx, alert.ID, nil, "assigned", map[string]interface{}{"assigned_to": *cfg.AutoAssignUserID})
			}
			log.Info("host_down: created alert on disconnect", "api_id", apiID, "host_id", host.ID, "alert_id", alert.ID)
		}
	}

	// Emit event for notification routing regardless.
	if emit != nil {
		emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
			Type: "host_down", Severity: severity, Title: title, Message: msg,
			ReferenceType: "host", ReferenceID: host.ID,
			Metadata: meta,
		})
	}
}

// OnConnect resolves any active host_down alert for the host when an agent reconnects.
// Called by the agent WebSocket connect handler.
func OnConnect(ctx context.Context, d *database.DB, apiID string, tenantHost string, emit *notifications.Emitter, log *slog.Logger) {
	enabled, err := IsAlertsEnabled(ctx, d)
	if err != nil || !enabled {
		return
	}
	cfg, err := GetConfigForType(ctx, d, "host_down")
	if err != nil || cfg == nil || !cfg.IsEnabled {
		return
	}

	host, err := d.Queries.GetHostByApiID(ctx, apiID)
	if err != nil {
		return
	}

	activeAlerts, err := d.Queries.ListActiveAlertsByType(ctx, "host_down")
	if err != nil {
		return
	}
	alertsStore := store.NewAlertsStore(d)
	for _, a := range activeAlerts {
		var meta map[string]interface{}
		if len(a.Metadata) > 0 {
			_ = json.Unmarshal(a.Metadata, &meta)
		}
		if meta != nil {
			if hid, ok := meta["host_id"].(string); ok && hid == host.ID {
				if err := alertsStore.UpdateResolved(ctx, a.ID, nil); err != nil {
					log.Debug("host_down: failed to resolve alert on connect", "api_id", apiID, "alert_id", a.ID, "error", err)
					return
				}
				_ = alertsStore.RecordHistory(ctx, a.ID, nil, "resolved", map[string]interface{}{
					"resolved_reason": "Host reconnected via WebSocket",
					"system_action":   true,
				})
				log.Info("host_down: resolved alert on connect", "api_id", apiID, "host_id", host.ID, "alert_id", a.ID)
				if emit != nil {
					hn := hostDisplayNameFromRow(host)
					emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
						Type:          "host_recovered",
						Severity:      ResolveSeverity(ctx, d, "host_recovered", "informational"),
						Title:         "Host reconnected",
						Message:       fmt.Sprintf("Host %s WebSocket reconnected.", hn),
						ReferenceType: "host",
						ReferenceID:   host.ID,
						Metadata:      map[string]interface{}{"host_id": host.ID, "host_name": hn},
					})
				}
				return
			}
		}
	}
}

func hostDisplayName(host db.Host) string {
	if host.FriendlyName != "" {
		return host.FriendlyName
	}
	if host.Hostname != nil && *host.Hostname != "" {
		return *host.Hostname
	}
	return host.ApiID
}

func hostDisplayNameFromRow(host db.Host) string {
	return hostDisplayName(host)
}
