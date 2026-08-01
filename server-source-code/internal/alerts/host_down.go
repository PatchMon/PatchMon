package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// defaultHostDownThresholdSeconds is the fallback used when the host_down
// alert_config row has no metadata.threshold value. Mirrors the seed in
// migration 000042.
const defaultHostDownThresholdSeconds = 30

// defaultUpdateIntervalMinutes is the fallback report cadence used when the
// settings row is unreadable or unset.
const defaultUpdateIntervalMinutes = 60

// reportCadenceStaleMultiplier is how many missed report cycles a host with no
// WebSocket history has to accumulate before the periodic sweep calls it down.
// Matches the pre-2.0.3 behaviour (update_interval * 3).
const reportCadenceStaleMultiplier = 3

// ProcessHostStatusMonitor runs the periodic host-down check: finds stale hosts and creates/resolves alerts.
// Called by the host-status-monitor queue job.
//
// Staleness is decided from live WebSocket state, not from hosts.last_update.
// last_update only moves on the agent's HTTP check-in, which runs on
// update_interval (default 60 MINUTES) — the 30-second host_down threshold is
// a WebSocket-disconnect duration and is meaningless against report cadence.
// Judging every host against it flagged essentially the whole fleet on every
// sweep and produced endless down/recovered flapping.
//
// reg may be nil (test fixtures, or a deployment with no registry wired); in
// that case every host falls back to the report-cadence rule below.
func ProcessHostStatusMonitor(ctx context.Context, d *database.DB, tenantHost string, reg *agentregistry.Registry, emit *notifications.Emitter, log *slog.Logger) (int, error) {
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

	// Threshold semantics: seconds, and it applies ONLY to the WebSocket
	// disconnect duration. parseThreshold reads alert_config.metadata.threshold
	// (a JSONB number); migration 000042 seeds the default of 30 seconds.
	thresholdSeconds := parseThreshold(cfg, defaultHostDownThresholdSeconds)
	if thresholdSeconds <= 0 {
		thresholdSeconds = defaultHostDownThresholdSeconds
	}
	wsThreshold := time.Duration(thresholdSeconds) * time.Second

	// Fallback threshold for hosts the registry has never seen a WebSocket for
	// (agent too old to hold a WS, or one that has never connected since this
	// process started). Those can only be judged on report cadence.
	updateIntervalMin := defaultUpdateIntervalMinutes
	if settings, sErr := d.Queries.GetFirstSettings(ctx); sErr != nil {
		log.Debug("host_down: settings read failed, using default update interval", "error", sErr)
	} else if settings.UpdateInterval > 0 {
		updateIntervalMin = int(settings.UpdateInterval)
	}
	reportCadenceThreshold := time.Now().Add(-time.Duration(updateIntervalMin*reportCadenceStaleMultiplier) * time.Minute)

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

	now := time.Now()
	for _, host := range hostRows {
		lastUpdate := host.LastUpdate
		if !lastUpdate.Valid {
			continue
		}
		decision := hostDownState(reg, host.ApiID, now, wsThreshold, lastUpdate.Time, reportCadenceThreshold, updateIntervalMin)
		isStale := decision.down
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
			// new consumers see the configured value at full precision. Both
			// report the threshold that ACTUALLY fired for this host, which is
			// the WebSocket disconnect window for tracked agents and the
			// report-cadence window for agents with no WebSocket history.
			thresholdMinutesRounded := (decision.thresholdSeconds + 59) / 60
			meta := map[string]interface{}{
				"host_id":           host.ID,
				"host_name":         hostName,
				"last_update":       lastUpdate.Time,
				"threshold_seconds": decision.thresholdSeconds,
				"threshold_minutes": thresholdMinutesRounded,
				"detection":         decision.detection,
			}
			var msg string
			if decision.detection == detectionWebsocket {
				msg = fmt.Sprintf("Host \"%s\" has had no WebSocket connection for %s. Last update: %s", hostName, formatHostDownThreshold(decision.thresholdSeconds), lastUpdate.Time.Format(time.RFC3339))
			} else {
				msg = fmt.Sprintf("Host \"%s\" has not reported in %s. Last update: %s", hostName, formatHostDownThreshold(decision.thresholdSeconds), lastUpdate.Time.Format(time.RFC3339))
			}

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

// Detection rules recorded on host_down alert metadata so operators (and
// downstream integrations) can tell which staleness rule fired.
const (
	detectionWebsocket     = "websocket"
	detectionReportCadence = "report_cadence"
)

// hostDownDecision is the outcome of the per-host staleness evaluation.
// thresholdSeconds is the window that was actually applied, so alert metadata
// and the operator-facing message quote a number that matches the rule used.
type hostDownDecision struct {
	down             bool
	detection        string
	thresholdSeconds int
}

// hostDownState decides whether a host counts as "agent down" for the periodic
// sweep.
//
// A live WebSocket is the strongest possible evidence the agent is alive, so it
// always wins. For an agent the registry has seen and then lost, the configured
// host_down threshold applies to the disconnect duration — that is the only
// signal the sub-minute default was ever meaningful for. For an agent the
// registry has no record of at all (never connected since this process
// started, or an agent build with no WebSocket), the only available signal is
// report cadence, so we fall back to the pre-2.0.3 rule of
// update_interval * 3 rather than the WebSocket threshold.
func hostDownState(
	reg *agentregistry.Registry,
	apiID string,
	now time.Time,
	wsThreshold time.Duration,
	lastUpdate time.Time,
	reportCadenceThreshold time.Time,
	updateIntervalMin int,
) hostDownDecision {
	cadence := hostDownDecision{
		down:             lastUpdate.Before(reportCadenceThreshold),
		detection:        detectionReportCadence,
		thresholdSeconds: updateIntervalMin * reportCadenceStaleMultiplier * 60,
	}
	if reg == nil {
		return cadence
	}

	info := reg.Get(apiID)
	switch {
	case info.Connected:
		// Live WebSocket: the agent is demonstrably reachable right now.
		return hostDownDecision{down: false, detection: detectionWebsocket, thresholdSeconds: int(wsThreshold.Seconds())}
	case info.DisconnectedAt != nil:
		return hostDownDecision{
			down:             now.Sub(*info.DisconnectedAt) > wsThreshold,
			detection:        detectionWebsocket,
			thresholdSeconds: int(wsThreshold.Seconds()),
		}
	default:
		// No WebSocket information at all for this agent.
		return cadence
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
