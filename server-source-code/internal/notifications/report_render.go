package notifications

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
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
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.HostsCompliant), "Compliant Hosts", "#059669")
				body.WriteString(`</tr></table>`)

				_ = cw.Write([]string{"executive_summary", "total_hosts", fmt.Sprintf("%d", dash.Summary.TotalHosts)})
				_ = cw.Write([]string{"executive_summary", "average_score", fmt.Sprintf("%.1f", dash.Summary.AverageScore)})
			}
			total, byStatus, _, _, e := patchStore.GetDashboard(ctx)
			if e == nil {
				body.WriteString(`<h3 style="color:#1e293b;font-size:16px;margin:16px 0 8px;">Patching Overview</h3>`)
				body.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="margin-bottom:16px;"><tr>`)
				writeKPICard(&body, fmt.Sprintf("%d", total), "Total Runs", "#6366f1")
				completed := byStatus["completed"]
				failed := byStatus["failed"]
				running := byStatus["running"]
				writeKPICard(&body, fmt.Sprintf("%d", completed), "Completed", "#059669")
				writeKPICard(&body, fmt.Sprintf("%d", failed), "Failed", criticalColor(failed))
				writeKPICard(&body, fmt.Sprintf("%d", running), "Running", "#f59e0b")
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
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.TotalPassedRules), "Passed Rules", "#059669")
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.TotalFailedRules), "Failed Rules", criticalColor(dash.Summary.TotalFailedRules))
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.HostsCritical), "Critical Hosts", criticalColor(dash.Summary.HostsCritical))
				writeKPICard(&body, fmt.Sprintf("%d", dash.Summary.Unscanned), "Unscanned", "#94a3b8")
				body.WriteString(`</tr></table>`)

				// Worst hosts table
				if len(dash.WorstHosts) > 0 {
					body.WriteString(`<h3 style="color:#1e293b;font-size:14px;margin:16px 0 8px;">Lowest Scoring Hosts</h3>`)
					body.WriteString(tableOpen([]string{"Host", "Score", "Profile"}))
					limit := top
					if limit > len(dash.WorstHosts) {
						limit = len(dash.WorstHosts)
					}
					for _, wh := range dash.WorstHosts[:limit] {
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
						body.WriteString(tableRow([]string{hostLink, esc(score), esc(profile)}, false))
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
				body.WriteString(tableOpen([]string{"Host", "Status", "Type", "Package"}))
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

					statusBadge := statusBadgeHTML(r.Status)
					hostLink := esc(host)
					if baseURL != "" {
						hostLink = fmt.Sprintf(`<a href="%s/patching/runs/%s" style="color:#2563eb;text-decoration:none;">%s</a>`, esc(baseURL), esc(r.ID), esc(host))
					}
					body.WriteString(tableRow([]string{hostLink, statusBadge, esc(r.PatchType), esc(pkg)}, false))
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
					body.WriteString(tableRow([]string{hostLink, statusBadge, esc(lu)}, false))
					_ = cw.Write([]string{"host", h.ID, h.Status + "|" + lu})
					n++
				}
				body.WriteString(`</table>`)
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
	sb.WriteString(`<table width="640" cellpadding="0" cellspacing="0" style="max-width:640px;width:100%;background-color:#ffffff;border-radius:12px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1);">`)

	// Header with logo
	sb.WriteString(`<tr><td style="background: linear-gradient(135deg, #1e293b 0%, #334155 100%); padding:24px 32px;">`)
	sb.WriteString(`<table width="100%" cellpadding="0" cellspacing="0"><tr>`)

	// Logo
	sb.WriteString(`<td style="vertical-align:middle;">`)
	logoURL := branding.LogoLightURL
	if logoURL == "" && baseURL != "" {
		// Fall back to the light logo endpoint - if no custom logo is set the endpoint will 404 gracefully
		logoURL = baseURL + "/api/v1/settings/logos/light"
	}
	if logoURL != "" {
		fmt.Fprintf(&sb, `<img src="%s" alt="PatchMon" height="36" style="height:36px;max-width:200px;display:block;" />`, esc(logoURL))
	} else {
		sb.WriteString(`<span style="color:#ffffff;font-size:22px;font-weight:700;letter-spacing:-0.5px;">PatchMon</span>`)
	}
	sb.WriteString(`</td>`)

	// Report date
	sb.WriteString(`<td style="text-align:right;vertical-align:middle;">`)
	fmt.Fprintf(&sb, `<span style="color:#94a3b8;font-size:13px;">%s</span>`, time.Now().UTC().Format("January 2, 2006"))
	sb.WriteString(`</td>`)

	sb.WriteString(`</tr></table>`)
	sb.WriteString(`</td></tr>`)

	// Report title bar
	sb.WriteString(`<tr><td style="padding:20px 32px 12px;">`)
	fmt.Fprintf(&sb, `<h1 style="margin:0;color:#0f172a;font-size:22px;font-weight:700;">%s</h1>`, esc(reportName))
	sb.WriteString(`<p style="margin:4px 0 0;color:#64748b;font-size:13px;">Scheduled report generated at `)
	sb.WriteString(time.Now().UTC().Format("15:04 UTC"))
	sb.WriteString(`</p>`)
	sb.WriteString(`</td></tr>`)

	// Divider
	sb.WriteString(`<tr><td style="padding:0 32px;"><hr style="border:none;border-top:1px solid #e2e8f0;margin:0;"/></td></tr>`)

	// Body content
	sb.WriteString(`<tr><td style="padding:20px 32px 32px;">`)
	sb.WriteString(bodyContent)
	sb.WriteString(`</td></tr>`)

	// CTA button
	if baseURL != "" {
		sb.WriteString(`<tr><td style="padding:0 32px 32px;text-align:center;">`)
		fmt.Fprintf(&sb, `<a href="%s" style="display:inline-block;background-color:#2563eb;color:#ffffff;padding:12px 32px;border-radius:8px;text-decoration:none;font-weight:600;font-size:14px;">Open PatchMon Dashboard</a>`, esc(baseURL))
		sb.WriteString(`</td></tr>`)
	}

	// Footer
	sb.WriteString(`<tr><td style="background-color:#f8fafc;padding:20px 32px;border-top:1px solid #e2e8f0;">`)
	sb.WriteString(`<table width="100%" cellpadding="0" cellspacing="0"><tr>`)
	sb.WriteString(`<td style="color:#94a3b8;font-size:12px;">Sent by PatchMon</td>`)
	if baseURL != "" {
		sb.WriteString(`<td style="text-align:right;">`)
		fmt.Fprintf(&sb, `<a href="%s/settings/alert-channels" style="color:#94a3b8;font-size:12px;text-decoration:none;">Manage notifications</a>`, esc(baseURL))
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
	return fmt.Sprintf(`<h2 style="color:#0f172a;font-size:18px;font-weight:600;margin:24px 0 12px;padding-bottom:8px;border-bottom:2px solid #e2e8f0;">%s</h2>`, TemplateEscape(title))
}

func writeKPICard(sb *strings.Builder, value, label, color string) {
	fmt.Fprintf(sb, `<td width="25%%" style="padding:4px;">
		<table width="100%%" cellpadding="0" cellspacing="0" style="background-color:#f8fafc;border-radius:8px;border-left:4px solid %s;padding:12px 16px;">
		<tr><td style="font-size:24px;font-weight:700;color:#0f172a;line-height:1.2;">%s</td></tr>
		<tr><td style="font-size:12px;color:#64748b;padding-top:2px;">%s</td></tr>
		</table></td>`, color, TemplateEscape(value), TemplateEscape(label))
}

func tableOpen(headers []string) string {
	var sb strings.Builder
	sb.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;margin-bottom:16px;">`)
	sb.WriteString(`<tr>`)
	for _, h := range headers {
		fmt.Fprintf(&sb, `<th style="text-align:left;padding:10px 12px;background-color:#f1f5f9;color:#475569;font-size:12px;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;border-bottom:2px solid #e2e8f0;">%s</th>`, TemplateEscape(h))
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
		fmt.Fprintf(&sb, `<td style="padding:10px 12px;font-size:14px;color:#334155;border-bottom:1px solid #f1f5f9;background-color:%s;">%s</td>`, bg, c)
	}
	sb.WriteString(`</tr>`)
	return sb.String()
}

func statusBadgeHTML(status string) string {
	color := "#64748b"
	bg := "#f1f5f9"
	switch strings.ToLower(status) {
	case "completed", "active", "compliant":
		color = "#059669"
		bg = "#ecfdf5"
	case "failed", "critical":
		color = "#dc2626"
		bg = "#fef2f2"
	case "running", "warning":
		color = "#d97706"
		bg = "#fffbeb"
	case "queued", "pending", "pending_validation":
		color = "#6366f1"
		bg = "#eef2ff"
	}
	return fmt.Sprintf(`<span style="display:inline-block;padding:2px 10px;border-radius:12px;font-size:12px;font-weight:600;color:%s;background-color:%s;">%s</span>`,
		color, bg, TemplateEscape(strings.ToUpper(status)))
}

func complianceColor(score float64) string {
	if score >= 80 {
		return "#059669"
	}
	if score >= 50 {
		return "#d97706"
	}
	return "#dc2626"
}

func criticalColor(count int) string {
	if count == 0 {
		return "#059669"
	}
	return "#dc2626"
}
