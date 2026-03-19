package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

const agentVersionDNS = "agent.vcheck.patchmon.net"

var agentSemverRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// ProcessAgentUpdate runs the agent version check: binary version + DNS latest, then create/resolve alerts.
// Called by the version-update-check queue job.
// The Go server reads the agent binary version by executing patchmon-agent (same as Agent Version tab in Settings).
func ProcessAgentUpdate(ctx context.Context, d *database.DB, agentsDir string, tenantHost string, emit *notifications.Emitter, log *slog.Logger) error {
	enabled, err := IsAlertsEnabled(ctx, d)
	if err != nil || !enabled {
		log.Debug("agent_update: alerts disabled")
		return nil
	}

	// Current version from binary (same logic as /agent/version handler)
	currentVersion := util.GetCurrentAgentVersionFromBinary(ctx, agentsDir)

	// Latest version from DNS
	latest, dnsErr := util.LookupVersionFromDNS(agentVersionDNS)
	if dnsErr != nil {
		log.Warn("agent_update: DNS lookup failed", "error", dnsErr)
	} else if latest != "" {
		latest = strings.TrimSpace(strings.Trim(latest, "\"'"))
		if !agentSemverRe.MatchString(latest) {
			latest = ""
		}
	}

	// Create/resolve alerts (only when agent_update config is enabled)
	cfg, _ := GetConfigForType(ctx, d, "agent_update")
	if cfg == nil || !cfg.IsEnabled {
		return nil
	}

	alertsStore := store.NewAlertsStore(d)
	severity := DefaultSeverity(cfg.DefaultSeverity, "informational")

	if currentVersion != "" && latest != "" && util.CompareVersions(latest, currentVersion) > 0 {
		// Create alert only if none exists for this latest version
		active, _ := d.Queries.ListActiveAlertsByType(ctx, "agent_update")
		hasMatching := false
		for _, a := range active {
			var m map[string]interface{}
			if len(a.Metadata) > 0 && json.Unmarshal(a.Metadata, &m) == nil {
				if lv, _ := m["latest_version"].(string); lv == latest {
					hasMatching = true
					break
				}
			}
		}
		if !hasMatching {
			title := "Agent Files Update Available"
			msg := fmt.Sprintf("A new agent version (%s) is available. Current version: %s", latest, currentVersion)
			meta := map[string]interface{}{"current_version": currentVersion, "latest_version": latest}
			alert, _ := alertsStore.Create(ctx, "agent_update", severity, title, msg, meta)
			if alert != nil && emit != nil {
				a := &models.Alert{ID: alert.ID, Type: alert.Type, Severity: alert.Severity, Title: alert.Title, Message: alert.Message}
				emit.EmitForAlert(ctx, d, tenantHost, a, meta, cfg)
			}
			log.Info("agent_update: created alert", "current", currentVersion, "latest", latest)
		}
	} else if currentVersion != "" && latest != "" && util.CompareVersions(latest, currentVersion) <= 0 {
		// Up to date: resolve all active agent_update alerts
		active, _ := d.Queries.ListActiveAlertsByType(ctx, "agent_update")
		for _, a := range active {
			_ = alertsStore.UpdateResolved(ctx, a.ID, nil)
			log.Info("agent_update: resolved alert (up to date)", "id", a.ID)
		}
	}

	return nil
}
