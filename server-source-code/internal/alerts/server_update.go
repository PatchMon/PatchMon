package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
)

const serverVersionDNS = "server.vcheck.patchmon.net"

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// ProcessServerUpdate runs the server version check: DNS lookup, settings update, and alert create/resolve.
// Called by the version-update-check queue job.
func ProcessServerUpdate(ctx context.Context, db *database.DB, serverVersion string, tenantHost string, emit *notifications.Emitter, log *slog.Logger) error {
	enabled, err := IsAlertsEnabled(ctx, db)
	if err != nil || !enabled {
		log.Debug("server_update: alerts disabled")
		return nil
	}

	// DNS lookup for latest version
	latest, dnsErr := util.LookupVersionFromDNS(serverVersionDNS)
	if dnsErr != nil {
		log.Warn("server_update: DNS lookup failed", "error", dnsErr)
	} else if latest != "" {
		latest = strings.TrimSpace(strings.Trim(latest, "\"'"))
		if !semverRe.MatchString(latest) {
			latest = ""
		}
	}

	// Persist to settings (so /version/current shows fresh data)
	settingsStore := store.NewSettingsStore(db)
	s, err := settingsStore.GetFirst(ctx)
	if err == nil && (serverVersion != "" || latest != "") {
		now := time.Now()
		s.LastUpdateCheck = &now
		if latest != "" {
			s.LatestVersion = &latest
			s.UpdateAvailable = util.CompareVersions(latest, serverVersion) > 0
		}
		_ = settingsStore.Update(ctx, s)
	}

	// Create/resolve alerts (only when server_update config is enabled)
	cfg, _ := GetConfigForType(ctx, db, "server_update")
	if cfg == nil || !cfg.IsEnabled || serverVersion == "" || latest == "" {
		return nil
	}

	alertsStore := store.NewAlertsStore(db)
	severity := DefaultSeverity(cfg.DefaultSeverity, "informational")

	if util.CompareVersions(latest, serverVersion) > 0 {
		// Update available: create alert only if none exists for this latest version
		active, _ := db.Queries.ListActiveAlertsByType(ctx, "server_update")
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
			title := "Server Update Available"
			msg := fmt.Sprintf("A new server version (%s) is available. Current version: %s", latest, serverVersion)
			meta := map[string]interface{}{"current_version": serverVersion, "latest_version": latest}
			alert, _ := alertsStore.Create(ctx, "server_update", severity, title, msg, meta)
			if alert != nil && emit != nil {
				a := &models.Alert{ID: alert.ID, Type: alert.Type, Severity: alert.Severity, Title: alert.Title, Message: alert.Message}
				emit.EmitForAlert(ctx, db, tenantHost, a, meta, cfg)
			}
			log.Info("server_update: created alert", "current", serverVersion, "latest", latest)
		}
	} else {
		// Up to date: resolve all active server_update alerts
		active, _ := db.Queries.ListActiveAlertsByType(ctx, "server_update")
		for _, a := range active {
			_ = alertsStore.UpdateResolved(ctx, a.ID, nil)
			log.Info("server_update: resolved alert (up to date)", "id", a.ID)
		}
	}

	return nil
}
