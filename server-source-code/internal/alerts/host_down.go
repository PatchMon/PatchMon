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

// defaultHostDownThresholdSeconds is the fallback used when the host_down
// alert_config row has no metadata.threshold value. Mirrors the seed in
// migration 000042.
const defaultHostDownThresholdSeconds = 30

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

	// Threshold semantics: seconds. parseThreshold reads alert_config.metadata.threshold
	// (a JSONB number); migration 000042 seeds the default of 30 seconds.
	thresholdSeconds := parseThreshold(cfg, defaultHostDownThresholdSeconds)
	if thresholdSeconds <= 0 {
		thresholdSeconds = defaultHostDownThresholdSeconds
	}
	threshold := time.Now().Add(-time.Duration(thresholdSeconds) * time.Second)

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
			// User-facing rename: "Host agent down" replaces the older "Host
			// down" / "is offline" wording so the alert clearly refers to the
			// PatchMon agent's reporting channel rather than the host itself.
			title := "Host agent down: " + hostName
			// Keep `threshold_minutes` in the metadata for any external
			// integrations that already key on it; add `threshold_seconds` so
			// new consumers see the configured value at full precision.
			thresholdMinutesRounded := (thresholdSeconds + 59) / 60
			meta := map[string]interface{}{
				"host_id":           host.ID,
				"host_name":         hostName,
				"last_update":       lastUpdate.Time,
				"threshold_seconds": thresholdSeconds,
				"threshold_minutes": thresholdMinutesRounded,
			}
			msg := fmt.Sprintf("Host \"%s\" has not reported in %s. Last update: %s", hostName, formatHostDownThreshold(thresholdSeconds), lastUpdate.Time.Format(time.RFC3339))

			// Emit event — notification routing decides which destinations receive it
			// (including internal alerts if that destination is enabled).
			if emit != nil {
				emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
					Type: "host_down", Severity: severity, Title: title, Message: msg,
					ReferenceType: "host", ReferenceID: host.ID,
					Metadata: meta,
				})
				alertsCreated++
			}
		} else if !isStale {
			hadAlert := false
			// Resolve any active host_down alert for this host.
			if alertID, exists := alertsByHostID[host.ID]; exists {
				hadAlert = true
				if cfg.AutoResolveAfterDays == nil {
					_ = alertsStore.UpdateResolved(ctx, alertID, nil)
					_ = alertsStore.RecordHistory(ctx, alertID, nil, "resolved", map[string]interface{}{"resolved_reason": "Host came back online", "system_action": true})
					delete(alertsByHostID, host.ID)
				}
			}
			// Emit host_recovered only when resolving an active host_down alert.
			if emit != nil && hadAlert {
				hn := hostDisplayName(host)
				emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
					Type:     "host_recovered",
					Severity: ResolveSeverity(ctx, d, "host_recovered", "informational"),
					// User-facing rename: "Host agent recovered" replaces
					// "Host back online" — the alert tracks the agent
					// reporting channel, not host availability per se.
					Title:         "Host agent recovered: " + hn,
					Message:       fmt.Sprintf("Host %s is reporting again.", hn),
					ReferenceType: "host",
					ReferenceID:   host.ID,
					Metadata:      map[string]interface{}{"host_id": host.ID, "host_name": hn},
				})
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
	// On WS-disconnect we don't actually wait for the configured threshold —
	// the alert fires immediately because the agent is unreachable. Surface
	// the configured threshold in metadata so notifications can render it,
	// and keep `threshold_minutes: 0` for back-compat with consumers that
	// branch on "0 means immediate".
	thresholdSeconds := parseThreshold(cfg, defaultHostDownThresholdSeconds)
	if thresholdSeconds <= 0 {
		thresholdSeconds = defaultHostDownThresholdSeconds
	}
	meta := map[string]interface{}{
		"host_id":           host.ID,
		"host_name":         hostName,
		"last_update":       lastUpdate,
		"threshold_seconds": thresholdSeconds,
		"threshold_minutes": 0,
		"disconnect_reason": "websocket",
	}
	// User-facing rename: WS disconnect maps to the same "Host agent down"
	// alert wording so the four-pill UI legend stays consistent.
	title := "Host agent down: " + hostName
	msg := fmt.Sprintf("Host \"%s\" WebSocket connection lost. Last update: %s", hostName, lastUpdate.Format(time.RFC3339))

	// Emit event — notification routing decides which destinations receive it
	// (including internal alerts if that destination is enabled).
	if emit != nil {
		emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
			Type: "host_down", Severity: severity, Title: title, Message: msg,
			ReferenceType: "host", ReferenceID: host.ID,
			Metadata: meta,
		})
		log.Info("host_down: emitted alert event on disconnect", "api_id", apiID, "host_id", host.ID)
	}
}

// OnConnect resolves any active host_down alert for the host when an agent reconnects.
// Called by the agent WebSocket connect handler.
func OnConnect(ctx context.Context, d *database.DB, apiID string, tenantHost string, emit *notifications.Emitter, log *slog.Logger) {
	enabled, err := IsAlertsEnabled(ctx, d)
	if err != nil || !enabled {
		return
	}

	host, err := d.Queries.GetHostByApiID(ctx, apiID)
	if err != nil {
		return
	}

	// Resolve any active host_down alert for this host.
	hadAlert := false
	activeAlerts, _ := d.Queries.ListActiveAlertsByType(ctx, "host_down")
	alertsStore := store.NewAlertsStore(d)
	for _, a := range activeAlerts {
		var meta map[string]interface{}
		if len(a.Metadata) > 0 {
			_ = json.Unmarshal(a.Metadata, &meta)
		}
		if meta != nil {
			if hid, ok := meta["host_id"].(string); ok && hid == host.ID {
				hadAlert = true
				if err := alertsStore.UpdateResolved(ctx, a.ID, nil); err != nil {
					log.Debug("host_down: failed to resolve alert on connect", "api_id", apiID, "alert_id", a.ID, "error", err)
				} else {
					_ = alertsStore.RecordHistory(ctx, a.ID, nil, "resolved", map[string]interface{}{
						"resolved_reason": "Host reconnected via WebSocket",
						"system_action":   true,
					})
					log.Info("host_down: resolved alert on connect", "api_id", apiID, "host_id", host.ID, "alert_id", a.ID)
				}
				break
			}
		}
	}

	// Emit host_recovered when resolving an active host_down alert (recovery from tracked down state).
	// If host_down is disabled, emit as a standalone "host up" signal on WebSocket connect.
	if emit != nil {
		hn := hostDisplayNameFromRow(host)
		if hadAlert {
			emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
				Type:          "host_recovered",
				Severity:      ResolveSeverity(ctx, d, "host_recovered", "informational"),
				Title:         "Host agent recovered: " + hn,
				Message:       fmt.Sprintf("Host %s WebSocket reconnected.", hn),
				ReferenceType: "host",
				ReferenceID:   host.ID,
				Metadata:      map[string]interface{}{"host_id": host.ID, "host_name": hn},
			})
		} else {
			// host_down is disabled or no alert existed — emit host_recovered
			// as a standalone "host up" notification if that event type is enabled.
			cfg, _ := GetConfigForType(ctx, d, "host_recovered")
			if cfg != nil && cfg.IsEnabled {
				emit.EmitEvent(ctx, d, tenantHost, notifications.Event{
					Type:          "host_recovered",
					Severity:      ResolveSeverity(ctx, d, "host_recovered", "informational"),
					Title:         "Host agent connected: " + hn,
					Message:       fmt.Sprintf("Host %s is online.", hn),
					ReferenceType: "host",
					ReferenceID:   host.ID,
					Metadata:      map[string]interface{}{"host_id": host.ID, "host_name": hn},
				})
			}
		}
	}
}

// formatHostDownThreshold renders the threshold in operator-friendly units:
// seconds for sub-minute thresholds, minutes for >=60s.
func formatHostDownThreshold(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	minutes := seconds / 60
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
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
