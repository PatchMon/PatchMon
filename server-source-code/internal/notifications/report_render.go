package notifications

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// ReportDefinition is the JSON stored on scheduled_reports.definition.
type ReportDefinition struct {
	Version      int      `json:"version"`
	Sections     []string `json:"sections"`
	HostGroupIDs []string `json:"host_group_ids"`
	Limits       struct {
		TopHosts int `json:"top_hosts"`
	} `json:"limits"`
}

// ReportBranding carries server URL and logo paths for email templates.
type ReportBranding struct {
	ServerURL    string // e.g. "https://patchmon.example.com"
	LogoDarkURL  string // full URL to dark logo, empty if unset
	LogoLightURL string // full URL to light logo, empty if unset
}

// BuildScheduledReport produces HTML and CSV for a scheduled report.
func BuildScheduledReport(ctx context.Context, d *database.DB, reportName string, defJSON []byte, branding ReportBranding) (subject string, html string, csvOut string, err error) {
	var def ReportDefinition
	if len(defJSON) > 0 {
		_ = json.Unmarshal(defJSON, &def)
	}
	if len(def.Sections) == 0 {
		def.Sections = []string{"executive_summary", "compliance_summary", "recent_patch_runs"}
	}
	top := def.Limits.TopHosts
	if top <= 0 {
		top = 20
	}

	esc := TemplateEscape
	baseURL := strings.TrimRight(branding.ServerURL, "/")

	var body strings.Builder
	var csvB strings.Builder
	cw := csv.NewWriter(&csvB)
	_ = cw.Write([]string{"section", "metric", "value"})

	complianceStore := store.NewComplianceStore(d)
	patchStore := store.NewPatchRunsStore(d)

	for _, sec := range def.Sections {
		switch sec {
		case "executive_summary":
			body.WriteString(sectionHeader("Executive Summary"))
			dash, e := complianceStore.GetDashboard(ctx)
			if e == nil {
				body.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:24px;">`)
				body.WriteString(`<tr>`)
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.TotalHosts), "Total Hosts", "#2563eb")
				writeKPICard(&body, fmt.Sprintf("%.1f%%", dash.Summary.AverageScore), "Avg Compliance", complianceColor(dash.Summary.AverageScore))
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.HostsCritical), "Critical Hosts", criticalColor(dash.Summary.HostsCritical))
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.HostsCompliant), "Compliant Hosts", "#16a34a")
				body.WriteString(`</tr></table>`)

				_ = cw.Write([]string{"executive_summary", "total_hosts", fmt.Sprintf("%d", dash.Summary.TotalHosts)})
				_ = cw.Write([]string{"executive_summary", "average_score", fmt.Sprintf("%.1f", dash.Summary.AverageScore)})
			}
			total, byStatus, _, _, e := patchStore.GetDashboard(ctx)
			if e == nil {
				body.WriteString(`<h3 style="color:#0f172a;font-size:15px;font-weight:600;margin:16px 0 8px;">Patching Overview</h3>`)
				body.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:16px;"><tr>`)
				writeKPICard(&body, fmt.Sprintf("%d", total), "Total Runs", "#6366f1")
				completed := byStatus["completed"]
				failed := byStatus["failed"]
				running := byStatus["running"]
				writeKPICard(&body, fmt.Sprintf("%d", completed), "Completed", "#16a34a")
				writeKPICard(&body, fmt.Sprintf("%d", failed), "Failed", criticalColor(failed))
				writeKPICard(&body, fmt.Sprintf("%d", running), "Running", "#d97706")
				body.WriteString(`</tr></table>`)

				for k, v := range byStatus {
					_ = cw.Write([]string{"patching_status", k, fmt.Sprintf("%d", v)})
				}
			}

		case "compliance_summary":
			body.WriteString(sectionHeader("Compliance Summary"))
			dash, e := complianceStore.GetDashboard(ctx)
			if e == nil {
				body.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:16px;"><tr>`)
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.TotalPassedRules), "Passed Rules", "#16a34a")
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.TotalFailedRules), "Failed Rules", criticalColor(dash.Summary.TotalFailedRules))
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.HostsCritical), "Critical Hosts", criticalColor(dash.Summary.HostsCritical))
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.Unscanned), "Unscanned", "#94a3b8")
				body.WriteString(`</tr></table>`)

				// Worst hosts table
				if len(dash.WorstHosts) > 0 {
					body.WriteString(`<h3 style="color:#0f172a;font-size:14px;font-weight:600;margin:16px 0 8px;">Lowest Scoring Hosts</h3>`)
					body.WriteString(tableOpen([]string{"Host", "Score", "Profile"}))
					limit := top
					if limit > len(dash.WorstHosts) {
						limit = len(dash.WorstHosts)
					}
					for i, wh := range dash.WorstHosts[:limit] {
						hostName := ""
						if wh.Host != nil {
							if fn, ok := wh.Host["friendly_name"].(string); ok && fn != "" {
								hostName = fn
							} else if hn, ok := wh.Host["hostname"].(string); ok && hn != "" {
								hostName = hn
							}
						}
						score := "N/A"
						if wh.Score != nil {
							score = fmt.Sprintf("%.1f%%", *wh.Score)
						}
						profile := ""
						if wh.Profile != nil {
							if pn, ok := wh.Profile["name"].(string); ok {
								profile = pn
							}
						}
						hostLink := esc(hostName)
						if baseURL != "" && wh.HostID != "" {
							hostLink = fmt.Sprintf(`<a href="%s/hosts/%s" style="color:#2563eb;text-decoration:none;">%s</a>`, esc(baseURL), esc(wh.HostID), esc(hostName))
						}
						body.WriteString(tableRow([]string{hostLink, esc(score), esc(profile)}, i%2 == 1))
					}
					body.WriteString(`</table>`)
				}

				_ = cw.Write([]string{"compliance", "total_failed_rules", fmt.Sprintf("%d", dash.Summary.TotalFailedRules)})
				_ = cw.Write([]string{"compliance", "hosts_critical", fmt.Sprintf("%d", dash.Summary.HostsCritical)})
			}

		case "recent_patch_runs":
			body.WriteString(sectionHeader("Recent Patch Runs"))
			_, _, recent, _, e := patchStore.GetDashboard(ctx)
			if e == nil && len(recent) > 0 {
				body.WriteString(tableOpen([]string{"Host", "Status", "Type", "Package", "Started", "Completed"}))
				n := 0
				for _, r := range recent {
					if n >= top {
						break
					}
					host := ""
					if r.HostFriendlyName != nil && *r.HostFriendlyName != "" {
						host = *r.HostFriendlyName
					} else if r.HostHostname != nil {
						host = *r.HostHostname
					}
					pkg := ""
					if r.PackageName != nil && *r.PackageName != "" {
						pkg = *r.PackageName
					} else {
						var pkgs []string
						if len(r.PackageNames) > 0 {
							_ = json.Unmarshal(r.PackageNames, &pkgs)
						}
						if len(pkgs) > 0 {
							if len(pkgs) <= 3 {
								pkg = strings.Join(pkgs, ", ")
							} else {
								pkg = fmt.Sprintf("%s +%d more", strings.Join(pkgs[:3], ", "), len(pkgs)-3)
							}
						} else {
							pkg = "all packages"
						}
					}

					started := "-"
					if r.StartedAt.Valid {
						started = r.StartedAt.Time.Format("Jan 2, 15:04 UTC")
					} else if r.CreatedAt.Valid {
						started = r.CreatedAt.Time.Format("Jan 2, 15:04 UTC")
					}
					completed := "-"
					if r.CompletedAt.Valid {
						completed = r.CompletedAt.Time.Format("Jan 2, 15:04 UTC")
					}

					statusBadge := statusBadgeHTML(r.Status)
					hostLink := esc(host)
					if baseURL != "" {
						hostLink = fmt.Sprintf(`<a href="%s/patching/runs/%s" style="color:#2563eb;text-decoration:none;">%s</a>`, esc(baseURL), esc(r.ID), esc(host))
					}
					body.WriteString(tableRow([]string{hostLink, statusBadge, esc(r.PatchType), esc(pkg), esc(started), esc(completed)}, n%2 == 1))
					_ = cw.Write([]string{"recent_patch_run", r.ID, r.Status + "|" + r.PatchType + "|" + host})
					n++
				}
				body.WriteString(`</table>`)
			} else {
				body.WriteString(`<p style="color:#64748b;font-size:14px;">No recent patch runs.</p>`)
			}

		case "hosts_offline":
			body.WriteString(sectionHeader("Host Status"))
			hosts, e := d.Queries.ListHosts(ctx)
			if e == nil && len(hosts) > 0 {
				body.WriteString(tableOpen([]string{"Host", "Status", "Last Seen"}))
				n := 0
				for _, h := range hosts {
					if n >= top {
						break
					}
					name := h.ApiID
					if h.FriendlyName != "" {
						name = h.FriendlyName
					}
					lu := "Never"
					if h.LastUpdate.Valid {
						lu = h.LastUpdate.Time.Format("Jan 2, 2006 15:04 UTC")
					}
					statusBadge := statusBadgeHTML(h.Status)
					hostLink := esc(name)
					if baseURL != "" {
						hostLink = fmt.Sprintf(`<a href="%s/hosts/%s" style="color:#2563eb;text-decoration:none;">%s</a>`, esc(baseURL), esc(h.ID), esc(name))
					}
					body.WriteString(tableRow([]string{hostLink, statusBadge, esc(lu)}, n%2 == 1))
					_ = cw.Write([]string{"host", h.ID, h.Status + "|" + lu})
					n++
				}
				body.WriteString(`</table>`)
			}

		case "open_alerts":
			body.WriteString(sectionHeader("Open Alerts"))
			alertsStore := store.NewAlertsStore(d)

			// Stats summary
			stats, e := alertsStore.GetStats(ctx)
			if e == nil {
				body.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:16px;"><tr>`)
				writeKPICard(&body, fmt.Sprintf("%d", stats["total"]), "Total Open", "#6366f1")
				writeKPICard(&body, fmt.Sprintf("%d", stats["critical"]), "Critical", criticalColor(stats["critical"]))
				writeKPICard(&body, fmt.Sprintf("%d", stats["error"]), "Error", criticalColor(stats["error"]))
				writeKPICard(&body, fmt.Sprintf("%d", stats["warning"]), "Warning", "#d97706")
				body.WriteString(`</tr></table>`)

				for k, v := range stats {
					_ = cw.Write([]string{"open_alerts", k, fmt.Sprintf("%d", v)})
				}
			}

			// Active alerts list
			alerts, e := alertsStore.List(ctx, nil)
			if e == nil && len(alerts) > 0 {
				body.WriteString(tableOpen([]string{"Severity", "Title", "Created", "Assigned To"}))
				n := 0
				for _, a := range alerts {
					if !a.IsActive {
						continue
					}
					if n >= top {
						break
					}
					sevBadge := severityBadgeHTML(a.Severity)
					created := a.CreatedAt.Format("Jan 2, 15:04 UTC")
					assigned := "-"
					if a.UsersAssigned != nil {
						assigned = esc(a.UsersAssigned.Username)
					}
					titleLink := esc(a.Title)
					if baseURL != "" {
						titleLink = fmt.Sprintf(`<a href="%s/alerts" style="color:#2563eb;text-decoration:none;">%s</a>`, esc(baseURL), esc(a.Title))
					}
					body.WriteString(tableRow([]string{sevBadge, titleLink, esc(created), assigned}, n%2 == 1))
					_ = cw.Write([]string{"open_alert", a.ID, a.Severity + "|" + a.Title})
					n++
				}
				body.WriteString(`</table>`)
			} else if e == nil {
				body.WriteString(`<p style="color:#64748b;font-size:14px;">No open alerts.</p>`)
			}

		case "hosts_by_updates":
			body.WriteString(sectionHeader("Hosts by Outstanding Updates"))
			dashStore := store.NewDashboardStore(d)
			hostsData, e := dashStore.GetHostsWithCounts(ctx, store.HostsListParams{})
			if e == nil && len(hostsData) > 0 {
				// Sort by updates count descending
				sort.Slice(hostsData, func(i, j int) bool {
					ci, _ := hostsData[i]["updatesCount"].(int64)
					cj, _ := hostsData[j]["updatesCount"].(int64)
					if ci == 0 {
						if v, ok := hostsData[i]["updatesCount"].(int32); ok {
							ci = int64(v)
						}
					}
					if cj == 0 {
						if v, ok := hostsData[j]["updatesCount"].(int32); ok {
							cj = int64(v)
						}
					}
					return ci > cj
				})

				body.WriteString(tableOpen([]string{"Host", "Outstanding Updates", "Security Updates", "Status", "Last Seen"}))
				n := 0
				for _, h := range hostsData {
					if n >= top {
						break
					}
					name := ""
					if fn, ok := h["friendly_name"].(string); ok && fn != "" {
						name = fn
					} else if hn, ok := h["hostname"].(*string); ok && hn != nil {
						name = *hn
					}
					if name == "" {
						if apiID, ok := h["api_id"].(string); ok {
							name = apiID
						}
					}
					updatesCount := "0"
					if v, ok := h["updatesCount"].(int64); ok {
						updatesCount = fmt.Sprintf("%d", v)
					} else if v, ok := h["updatesCount"].(int32); ok {
						updatesCount = fmt.Sprintf("%d", v)
					}
					secCount := "0"
					if v, ok := h["securityUpdatesCount"].(int64); ok {
						secCount = fmt.Sprintf("%d", v)
					} else if v, ok := h["securityUpdatesCount"].(int32); ok {
						secCount = fmt.Sprintf("%d", v)
					}
					status := ""
					if s, ok := h["effectiveStatus"].(string); ok {
						status = s
					} else if s, ok := h["status"].(string); ok {
						status = s
					}
					lastSeen := "-"
					if lu, ok := h["last_update"].(string); ok && lu != "" {
						if t, err := time.Parse(time.RFC3339, lu); err == nil {
							lastSeen = t.Format("Jan 2, 15:04 UTC")
						}
					}
					hostLink := esc(name)
					if baseURL != "" {
						if hid, ok := h["id"].(string); ok {
							hostLink = fmt.Sprintf(`<a href="%s/hosts/%s" style="color:#2563eb;text-decoration:none;">%s</a>`, esc(baseURL), esc(hid), esc(name))
						}
					}
					body.WriteString(tableRow([]string{hostLink, esc(updatesCount), esc(secCount), statusBadgeHTML(status), esc(lastSeen)}, n%2 == 1))
					_ = cw.Write([]string{"hosts_by_updates", name, updatesCount + "|" + secCount})
					n++
				}
				body.WriteString(`</table>`)
			} else {
				body.WriteString(`<p style="color:#64748b;font-size:14px;">No host data available.</p>`)
			}

		case "top_security_packages":
			body.WriteString(sectionHeader("Top Outdated Security Packages"))
			pkgStore := store.NewPackagesStore(d)
			pkgs, e := pkgStore.ListNeedingUpdates(ctx)
			if e == nil && len(pkgs) > 0 {
				// Filter to security updates and sort by affected hosts count
				var secPkgs []map[string]interface{}
				for _, p := range pkgs {
					if isSec, ok := p["isSecurityUpdate"].(bool); ok && isSec {
						secPkgs = append(secPkgs, p)
					}
				}
				sort.Slice(secPkgs, func(i, j int) bool {
					ci, _ := secPkgs[i]["affectedHostsCount"].(int)
					cj, _ := secPkgs[j]["affectedHostsCount"].(int)
					return ci > cj
				})

				if len(secPkgs) > 0 {
					body.WriteString(tableOpen([]string{"Package", "Category", "Affected Hosts", "Latest Version"}))
					n := 0
					for _, p := range secPkgs {
						if n >= top {
							break
						}
						name := ""
						if v, ok := p["name"].(string); ok {
							name = v
						}
						category := "-"
						if v, ok := p["category"].(*string); ok && v != nil {
							category = *v
						} else if v, ok := p["category"].(string); ok && v != "" {
							category = v
						}
						affected := "0"
						if v, ok := p["affectedHostsCount"].(int); ok {
							affected = fmt.Sprintf("%d", v)
						}
						latest := "-"
						if v, ok := p["latestVersion"].(*string); ok && v != nil {
							latest = *v
						} else if v, ok := p["latestVersion"].(string); ok && v != "" {
							latest = v
						}
						pkgLink := esc(name)
						if baseURL != "" {
							if pid, ok := p["id"].(string); ok {
								pkgLink = fmt.Sprintf(`<a href="%s/packages/%s" style="color:#2563eb;text-decoration:none;">%s</a>`, esc(baseURL), esc(pid), esc(name))
							}
						}
						body.WriteString(tableRow([]string{pkgLink, esc(category), esc(affected), esc(latest)}, n%2 == 1))
						_ = cw.Write([]string{"top_security_package", name, affected})
						n++
					}
					body.WriteString(`</table>`)
				} else {
					body.WriteString(`<p style="color:#64748b;font-size:14px;">No outstanding security packages.</p>`)
				}
			} else {
				body.WriteString(`<p style="color:#64748b;font-size:14px;">No package data available.</p>`)
			}

		default:
			fmt.Fprintf(&body, `<p style="color:#94a3b8;font-size:14px;"><em>Unknown section: %s</em></p>`, esc(sec))
		}
	}

	cw.Flush()
	subject = fmt.Sprintf("PatchMon Report: %s", reportName)

	// Wrap body in branded email template shell
	fullHTML := wrapEmailTemplate(reportName, body.String(), branding)
	return subject, fullHTML, csvB.String(), nil
}

// wrapEmailTemplate wraps report body content in a branded HTML email shell.
func wrapEmailTemplate(reportName string, bodyContent string, branding ReportBranding) string {
	esc := TemplateEscape
	baseURL := strings.TrimRight(branding.ServerURL, "/")

	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"></head>`)
	sb.WriteString(`<body style="margin:0;padding:0;background-color:#f1f5f9;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">`)

	// Outer container
	sb.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="background-color:#f1f5f9;padding:32px 0;">`)
	sb.WriteString(`<tr><td align="center">`)

	// Inner card
	sb.WriteString(`<table width="640" cellpadding="0" cellspacing="0" style="max-width:640px;width:100%;background-color:#ffffff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1);">`)

	// Header — dark background with logo
	sb.WriteString(`<tr><td style="background-color:#0f172a;padding:20px 32px;">`)
	sb.WriteString(`<table width="100%" cellpadding="0" cellspacing="0"><tr>`)

	// Logo
	sb.WriteString(`<td style="vertical-align:middle;">`)
	logoURL := branding.LogoLightURL
	if logoURL == "" && baseURL != "" {
		logoURL = baseURL + "/api/v1/settings/logos/light"
	}
	if logoURL != "" {
		// Use both width and height attributes, and provide alt text fallback
		fmt.Fprintf(&sb, `<img src="%s" alt="PatchMon" width="140" height="32" style="height:32px;width:auto;max-width:180px;display:block;border:0;" />`, esc(logoURL))
	} else {
		sb.WriteString(`<span style="color:#ffffff;font-size:20px;font-weight:700;letter-spacing:-0.3px;">PatchMon</span>`)
	}
	sb.WriteString(`</td>`)

	// Report date
	sb.WriteString(`<td style="text-align:right;vertical-align:middle;">`)
	now := time.Now().UTC()
	fmt.Fprintf(&sb, `<span style="color:#94a3b8;font-size:12px;">%s</span>`, now.Format("January 2, 2006"))
	sb.WriteString(`</td>`)

	sb.WriteString(`</tr></table>`)
	sb.WriteString(`</td></tr>`)

	// Report title bar
	sb.WriteString(`<tr><td style="padding:20px 32px 8px;">`)
	fmt.Fprintf(&sb, `<h1 style="margin:0;color:#0f172a;font-size:20px;font-weight:700;">%s</h1>`, esc(reportName))
	sb.WriteString(`<p style="margin:4px 0 0;color:#64748b;font-size:12px;">Generated `)
	sb.WriteString(now.Format("Mon, Jan 2 2006 at 15:04 UTC"))
	sb.WriteString(`</p>`)
	sb.WriteString(`</td></tr>`)

	// Divider
	sb.WriteString(`<tr><td style="padding:0 32px;"><hr style="border:none;border-top:1px solid #e2e8f0;margin:0;"/></td></tr>`)

	// Body content
	sb.WriteString(`<tr><td style="padding:16px 32px 28px;">`)
	sb.WriteString(bodyContent)
	sb.WriteString(`</td></tr>`)

	// CTA button
	if baseURL != "" {
		sb.WriteString(`<tr><td style="padding:0 32px 28px;text-align:center;">`)
		fmt.Fprintf(&sb, `<a href="%s" style="display:inline-block;background-color:#2563eb;color:#ffffff;padding:10px 28px;border-radius:6px;text-decoration:none;font-weight:600;font-size:13px;">Open PatchMon Dashboard</a>`, esc(baseURL))
		sb.WriteString(`</td></tr>`)
	}

	// Footer
	sb.WriteString(`<tr><td style="background-color:#f8fafc;padding:16px 32px;border-top:1px solid #e2e8f0;">`)
	sb.WriteString(`<table width="100%" cellpadding="0" cellspacing="0"><tr>`)
	sb.WriteString(`<td style="color:#94a3b8;font-size:11px;">Sent by PatchMon</td>`)
	if baseURL != "" {
		sb.WriteString(`<td style="text-align:right;">`)
		fmt.Fprintf(&sb, `<a href="%s/reporting" style="color:#94a3b8;font-size:11px;text-decoration:none;">Manage notifications</a>`, esc(baseURL))
		sb.WriteString(`</td>`)
	}
	sb.WriteString(`</tr></table>`)
	sb.WriteString(`</td></tr>`)

	sb.WriteString(`</table>`)           // inner card
	sb.WriteString(`</td></tr></table>`) // outer container
	sb.WriteString(`</body></html>`)
	return sb.String()
}

// --- HTML helpers ---

func sectionHeader(title string) string {
	return fmt.Sprintf(`<h2 style="color:#0f172a;font-size:16px;font-weight:600;margin:24px 0 12px;padding-bottom:8px;border-bottom:2px solid #e2e8f0;">%s</h2>`, TemplateEscape(title))
}

func writeKPICard(sb *strings.Builder, value, label, color string) {
	fmt.Fprintf(sb, `<td width="25%%" style="padding:4px;">
		<table width="100%%" cellpadding="0" cellspacing="0" style="background-color:#f8fafc;border-radius:6px;padding:12px 16px;">
		<tr><td style="font-size:22px;font-weight:700;color:%s;line-height:1.2;">%s</td></tr>
		<tr><td style="font-size:11px;color:#64748b;padding-top:2px;text-transform:uppercase;letter-spacing:0.03em;">%s</td></tr>
		</table></td>`, color, TemplateEscape(value), TemplateEscape(label))
}

func tableOpen(headers []string) string {
	var sb strings.Builder
	sb.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;margin-bottom:16px;">`)
	sb.WriteString(`<tr>`)
	for _, h := range headers {
		fmt.Fprintf(&sb, `<th style="text-align:left;padding:8px 10px;background-color:#f1f5f9;color:#475569;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.04em;border-bottom:2px solid #e2e8f0;">%s</th>`, TemplateEscape(h))
	}
	sb.WriteString(`</tr>`)
	return sb.String()
}

func tableRow(cells []string, isAlt bool) string {
	bg := "#ffffff"
	if isAlt {
		bg = "#f8fafc"
	}
	var sb strings.Builder
	sb.WriteString(`<tr>`)
	for _, c := range cells {
		fmt.Fprintf(&sb, `<td style="padding:8px 10px;font-size:13px;color:#334155;border-bottom:1px solid #f1f5f9;background-color:%s;">%s</td>`, bg, c)
	}
	sb.WriteString(`</tr>`)
	return sb.String()
}

func statusBadgeHTML(status string) string {
	color := "#64748b"
	bg := "#f1f5f9"
	switch strings.ToLower(status) {
	case "completed", "active", "compliant":
		color = "#16a34a"
		bg = "#f0fdf4"
	case "failed", "critical":
		color = "#dc2626"
		bg = "#fef2f2"
	case "running", "warning":
		color = "#d97706"
		bg = "#fffbeb"
	case "queued", "pending", "pending_validation", "pending_approval":
		color = "#6366f1"
		bg = "#eef2ff"
	case "inactive", "offline":
		color = "#94a3b8"
		bg = "#f1f5f9"
	}
	return fmt.Sprintf(`<span style="display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600;color:%s;background-color:%s;">%s</span>`,
		color, bg, TemplateEscape(strings.ToUpper(status)))
}

func severityBadgeHTML(severity string) string {
	color := "#64748b"
	bg := "#f1f5f9"
	switch strings.ToLower(severity) {
	case "critical":
		color = "#dc2626"
		bg = "#fef2f2"
	case "error":
		color = "#dc2626"
		bg = "#fef2f2"
	case "warning":
		color = "#d97706"
		bg = "#fffbeb"
	case "informational", "info":
		color = "#2563eb"
		bg = "#eff6ff"
	}
	return fmt.Sprintf(`<span style="display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600;color:%s;background-color:%s;">%s</span>`,
		color, bg, TemplateEscape(strings.ToUpper(severity)))
}

func complianceColor(score float64) string {
	if score >= 80 {
		return "#16a34a"
	}
	if score >= 50 {
		return "#d97706"
	}
	return "#dc2626"
}

func criticalColor(count int) string {
	if count == 0 {
		return "#16a34a"
	}
	return "#dc2626"
}
