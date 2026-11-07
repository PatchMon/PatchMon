package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// PrometheusExporterHandler serves Prometheus metrics at GET /metrics.
// The endpoint is public (no session required) but returns 404 when disabled.
type PrometheusExporterHandler struct {
	settings *store.SettingsStore
	hosts    *store.HostsStore
	dbp      database.DBProvider
}

// NewPrometheusExporterHandler creates a new Prometheus exporter handler.
func NewPrometheusExporterHandler(settings *store.SettingsStore, hosts *store.HostsStore, dbp database.DBProvider) *PrometheusExporterHandler {
	return &PrometheusExporterHandler{settings: settings, hosts: hosts, dbp: dbp}
}

// hostUpdateRow holds per-host update counts fetched by a raw query.
type hostUpdateRow struct {
	HostID         string
	FriendlyName   string
	Hostname       string
	OSType         string
	OSVersion      string
	LastUpdate     time.Time
	Regular        int
	Security       int
	TotalInstalled int
	NeedsReboot    bool
}

// prometheusSettings holds Prometheus-related settings returned to the UI.
type prometheusSettings struct {
	Enabled bool `json:"prometheus_enabled"`
}

// GetSettings handles GET /api/v1/prometheus/settings.
func (h *PrometheusExporterHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}
	JSON(w, http.StatusOK, prometheusSettings{Enabled: s.PrometheusEnabled})
}

// UpdateSettings handles PUT /api/v1/prometheus/settings.
func (h *PrometheusExporterHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"prometheus_enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Enabled == nil {
		Error(w, http.StatusBadRequest, "prometheus_enabled is required")
		return
	}

	s, err := h.settings.GetFirst(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load settings")
		return
	}

	s.PrometheusEnabled = *req.Enabled
	if err := h.settings.Update(r.Context(), s); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update settings")
		return
	}

	JSON(w, http.StatusOK, prometheusSettings{Enabled: s.PrometheusEnabled})
}

// ServeMetrics handles GET /metrics — the Prometheus scrape endpoint.
func (h *PrometheusExporterHandler) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	s, err := h.settings.GetFirst(ctx)
	if err != nil || !s.PrometheusEnabled {
		http.Error(w, `Exporter disabled`, http.StatusServiceUnavailable)
		return
	}

	data, err := h.collectMetrics(ctx, s)
	if err != nil {
		http.Error(w, "Error collecting metrics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(data))
}

// collectMetrics queries the database and formats the Prometheus exposition text.
func (h *PrometheusExporterHandler) collectMetrics(ctx context.Context, s *models.Settings) (string, error) {
	d := h.dbp.DB(ctx)

	// ── Aggregate dashboard stats ──────────────────────────────────────────
	updateIntervalMinutes := s.UpdateInterval
	if updateIntervalMinutes <= 0 {
		updateIntervalMinutes = 60
	}
	now := time.Now()
	thresholdTime := now.Add(-time.Duration(updateIntervalMinutes*2) * time.Minute)
	offlineThreshold := now.Add(-time.Duration(updateIntervalMinutes*3) * time.Minute)

	agg, err := d.Queries.GetDashboardStats(ctx, db.GetDashboardStatsParams{
		LastUpdate:   pgtime.From(thresholdTime),
		LastUpdate_2: pgtime.From(offlineThreshold),
	})
	if err != nil {
		return "", fmt.Errorf("get dashboard stats: %w", err)
	}

	// ── Per-host update counts ─────────────────────────────────────────────
	perHost, err := h.queryPerHostMetrics(ctx)
	if err != nil {
		return "", fmt.Errorf("per-host metrics: %w", err)
	}

	// ── Build output ───────────────────────────────────────────────────────
	var b strings.Builder

	// patchmon_hosts_total
	writeHelp(&b, "patchmon_hosts_total", "gauge", "Total number of monitored hosts.")
	fmt.Fprintf(&b, "patchmon_hosts_total %d\n\n", agg.TotalHosts)

	// patchmon_hosts_needing_updates_total
	writeHelp(&b, "patchmon_hosts_needing_updates_total", "gauge",
		"Number of hosts with at least one pending package update.")
	fmt.Fprintf(&b, "patchmon_hosts_needing_updates_total %d\n\n", agg.HostsNeedingUpdates)

	// patchmon_hosts_needing_security_updates_total (derived from per-host data)
	var hostsWithSecurity int32
	for _, row := range perHost {
		if row.Security > 0 {
			hostsWithSecurity++
		}
	}
	writeHelp(&b, "patchmon_hosts_needing_security_updates_total", "gauge",
		"Number of hosts with at least one pending security update.")
	fmt.Fprintf(&b, "patchmon_hosts_needing_security_updates_total %d\n\n", hostsWithSecurity)

	// patchmon_hosts_errored_total
	writeHelp(&b, "patchmon_hosts_errored_total", "gauge",
		"Number of hosts that have not reported within 2x the configured update interval.")
	fmt.Fprintf(&b, "patchmon_hosts_errored_total %d\n\n", agg.ErroredHosts)

	// patchmon_hosts_offline_total
	writeHelp(&b, "patchmon_hosts_offline_total", "gauge",
		"Number of hosts that have not reported within 3x the configured update interval.")
	fmt.Fprintf(&b, "patchmon_hosts_offline_total %d\n\n", agg.OfflineHosts)

	// patchmon_hosts_needing_reboot_total
	writeHelp(&b, "patchmon_hosts_needing_reboot_total", "gauge",
		"Number of hosts with a pending reboot.")
	fmt.Fprintf(&b, "patchmon_hosts_needing_reboot_total %d\n\n", agg.HostsNeedingReboot)

	// patchmon_pending_updates_total{type}
	regularUpdates := agg.TotalOutdatedPackages - agg.SecurityUpdates
	if regularUpdates < 0 {
		regularUpdates = 0
	}
	writeHelp(&b, "patchmon_pending_updates_total", "gauge",
		`Total unique packages pending an update, by type ("regular" or "security").`)
	fmt.Fprintf(&b, "patchmon_pending_updates_total{type=\"regular\"} %d\n", regularUpdates)
	fmt.Fprintf(&b, "patchmon_pending_updates_total{type=\"security\"} %d\n\n", agg.SecurityUpdates)

	// patchmon_host_pending_updates{host_id, name, hostname, os_type}
	writeHelp(&b, "patchmon_host_pending_updates", "gauge",
		"Number of packages pending a regular (non-security) update on a specific host.")
	for _, row := range perHost {
		lbl := hostLabels(row)
		fmt.Fprintf(&b, "patchmon_host_pending_updates{%s} %d\n", lbl, row.Regular)
	}
	b.WriteByte('\n')

	// patchmon_host_security_updates{host_id, name, hostname, os_type}
	writeHelp(&b, "patchmon_host_security_updates", "gauge",
		"Number of packages pending a security update on a specific host.")
	for _, row := range perHost {
		lbl := hostLabels(row)
		fmt.Fprintf(&b, "patchmon_host_security_updates{%s} %d\n", lbl, row.Security)
	}
	b.WriteByte('\n')

	// patchmon_host_installed_packages_total{host_id, name, hostname, os_type, os_version}
	var totalInstalled int
	for _, row := range perHost {
		totalInstalled += row.TotalInstalled
	}
	writeHelp(&b, "patchmon_installed_packages_total", "gauge",
		"Total number of installed packages across all monitored hosts.")
	fmt.Fprintf(&b, "patchmon_installed_packages_total %d\n\n", totalInstalled)

	writeHelp(&b, "patchmon_host_installed_packages_total", "gauge",
		"Total number of installed packages on a specific host.")
	for _, row := range perHost {
		lbl := hostLabels(row)
		fmt.Fprintf(&b, "patchmon_host_installed_packages_total{%s} %d\n", lbl, row.TotalInstalled)
	}
	b.WriteByte('\n')

	// patchmon_host_pending_reboot{host_id, name, ...} — 1 if reboot needed, 0 otherwise
	writeHelp(&b, "patchmon_host_pending_reboot", "gauge",
		"Whether a host requires a reboot: 1 = reboot needed, 0 = no reboot needed.")
	for _, row := range perHost {
		lbl := hostLabels(row)
		rebootVal := 0
		if row.NeedsReboot {
			rebootVal = 1
		}
		fmt.Fprintf(&b, "patchmon_host_pending_reboot{%s} %d\n", lbl, rebootVal)
	}
	b.WriteByte('\n')

	// patchmon_host_last_seen_seconds{host_id, name}
	writeHelp(&b, "patchmon_host_last_seen_seconds", "gauge",
		"Unix timestamp of the last successful agent check-in for a host.")
	for _, row := range perHost {
		lbl := hostLabels(row)
		fmt.Fprintf(&b, "patchmon_host_last_seen_seconds{%s} %d\n", lbl, row.LastUpdate.Unix())
	}
	b.WriteByte('\n')

	// patchmon_scrape_timestamp_seconds — useful for stale-data detection
	writeHelp(&b, "patchmon_scrape_timestamp_seconds", "gauge",
		"Unix timestamp when these metrics were collected by PatchMon.")
	fmt.Fprintf(&b, "patchmon_scrape_timestamp_seconds %d\n", now.Unix())

	return b.String(), nil
}

// queryPerHostMetrics runs a raw query to get per-host pending update counts.
func (h *PrometheusExporterHandler) queryPerHostMetrics(ctx context.Context) ([]hostUpdateRow, error) {
	const q = `
SELECT
    ho.id,
    ho.friendly_name,
    COALESCE(ho.hostname, '') AS hostname,
    ho.os_type,
    COALESCE(ho.os_version, '') AS os_version,
    ho.last_update,
    COUNT(*) FILTER (WHERE hp.needs_update = true  AND hp.is_security_update = false)::int AS regular,
    COUNT(*) FILTER (WHERE hp.needs_update = true  AND hp.is_security_update = true)::int  AS security,
    COUNT(hp.id)::int AS total_installed,
    ho.needs_reboot
FROM hosts ho
LEFT JOIN host_packages hp ON hp.host_id = ho.id
GROUP BY ho.id, ho.friendly_name, ho.hostname, ho.os_type, ho.os_version, ho.last_update, ho.needs_reboot
ORDER BY ho.friendly_name
`
	d := h.dbp.DB(ctx)
	rows, err := d.Raw(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []hostUpdateRow
	for rows.Next() {
		var row hostUpdateRow
		var pgLastUpdate pgtype.Timestamp
		if err := rows.Scan(
			&row.HostID, &row.FriendlyName, &row.Hostname, &row.OSType, &row.OSVersion,
			&pgLastUpdate, &row.Regular, &row.Security, &row.TotalInstalled, &row.NeedsReboot,
		); err != nil {
			return nil, err
		}
		if pgLastUpdate.Valid {
			row.LastUpdate = pgLastUpdate.Time.UTC()
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// hostLabels formats the Prometheus label set for a host row.
// Label values are quoted Go-style by fmt.Sprintf %q, which produces valid
// Prometheus label strings (backslash-escaped double-quotes).
func hostLabels(row hostUpdateRow) string {
	return fmt.Sprintf(
		`host_id=%q,name=%q,hostname=%q,os_type=%q,os_version=%q`,
		row.HostID,
		row.FriendlyName,
		row.Hostname,
		row.OSType,
		row.OSVersion,
	)
}

// writeHelp writes a # HELP and # TYPE comment block.
func writeHelp(b *strings.Builder, name, metricType, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s %s\n", name, metricType)
}
