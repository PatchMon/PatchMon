package notifications

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

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

// BuildScheduledReport produces HTML and CSV for a scheduled report.
func BuildScheduledReport(ctx context.Context, d *database.DB, reportName string, defJSON []byte) (subject string, html string, csvOut string, err error) {
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

	var htmlB strings.Builder
	var csvB strings.Builder
	cw := csv.NewWriter(&csvB)

	htmlB.WriteString("<html><body><h1>")
	htmlB.WriteString(TemplateEscape(reportName))
	htmlB.WriteString("</h1>")
	_ = cw.Write([]string{"section", "metric", "value"})

	complianceStore := store.NewComplianceStore(d)
	patchStore := store.NewPatchRunsStore(d)

	for _, sec := range def.Sections {
		switch sec {
		case "executive_summary":
			dash, e := complianceStore.GetDashboard(ctx)
			if e == nil {
				htmlB.WriteString("<h2>Executive summary</h2><ul>")
				_, _ = fmt.Fprintf(&htmlB, "<li>Hosts (compliance view): %d</li>", dash.Summary.TotalHosts)
				_, _ = fmt.Fprintf(&htmlB, "<li>Average compliance score: %.1f</li>", dash.Summary.AverageScore)
				htmlB.WriteString("</ul>")
				_ = cw.Write([]string{"executive_summary", "total_hosts", fmt.Sprintf("%d", dash.Summary.TotalHosts)})
				_ = cw.Write([]string{"executive_summary", "average_score", fmt.Sprintf("%.1f", dash.Summary.AverageScore)})
			}
			total, byStatus, _, _, e := patchStore.GetDashboard(ctx)
			if e == nil {
				htmlB.WriteString("<h3>Patching</h3><ul>")
				_, _ = fmt.Fprintf(&htmlB, "<li>Total runs: %d</li>", total)
				for k, v := range byStatus {
					_, _ = fmt.Fprintf(&htmlB, "<li>Status %s: %d</li>", TemplateEscape(k), v)
					_ = cw.Write([]string{"patching_status", k, fmt.Sprintf("%d", v)})
				}
				htmlB.WriteString("</ul>")
			}
		case "compliance_summary":
			dash, e := complianceStore.GetDashboard(ctx)
			if e == nil {
				htmlB.WriteString("<h2>Compliance</h2><ul>")
				_, _ = fmt.Fprintf(&htmlB, "<li>Failed rules (total): %d</li>", dash.Summary.TotalFailedRules)
				_, _ = fmt.Fprintf(&htmlB, "<li>Passed rules (total): %d</li>", dash.Summary.TotalPassedRules)
				_, _ = fmt.Fprintf(&htmlB, "<li>Critical hosts: %d</li>", dash.Summary.HostsCritical)
				htmlB.WriteString("</ul>")
				_ = cw.Write([]string{"compliance", "total_failed_rules", fmt.Sprintf("%d", dash.Summary.TotalFailedRules)})
				_ = cw.Write([]string{"compliance", "hosts_critical", fmt.Sprintf("%d", dash.Summary.HostsCritical)})
			}
		case "recent_patch_runs":
			_, _, recent, _, e := patchStore.GetDashboard(ctx)
			if e == nil {
				htmlB.WriteString("<h2>Recent patch runs</h2><table border=\"1\" cellpadding=\"4\"><tr><th>Host</th><th>Status</th><th>Type</th></tr>")
				n := 0
				for _, r := range recent {
					if n >= top {
						break
					}
					host := ""
					if r.HostHostname != nil {
						host = *r.HostHostname
					}
					_, _ = fmt.Fprintf(&htmlB, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>",
						TemplateEscape(host), TemplateEscape(r.Status), TemplateEscape(r.PatchType))
					_ = cw.Write([]string{"recent_patch_run", r.ID, r.Status + "|" + r.PatchType + "|" + host})
					n++
				}
				htmlB.WriteString("</table>")
			}
		case "hosts_offline":
			hosts, e := d.Queries.ListHosts(ctx)
			if e == nil {
				htmlB.WriteString("<h2>Hosts</h2><table border=\"1\" cellpadding=\"4\"><tr><th>Name</th><th>Status</th><th>Last update</th></tr>")
				n := 0
				for _, h := range hosts {
					if n >= top {
						break
					}
					st := h.Status
					name := h.ApiID
					if h.FriendlyName != "" {
						name = h.FriendlyName
					}
					lu := ""
					if h.LastUpdate.Valid {
						lu = h.LastUpdate.Time.String()
					}
					_, _ = fmt.Fprintf(&htmlB, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>",
						TemplateEscape(name), TemplateEscape(st), TemplateEscape(lu))
					_ = cw.Write([]string{"host", h.ID, st + "|" + lu})
					n++
				}
				htmlB.WriteString("</table>")
			}
		default:
			_, _ = fmt.Fprintf(&htmlB, "<p><em>Unknown section %s</em></p>", TemplateEscape(sec))
		}
	}

	htmlB.WriteString("</body></html>")
	cw.Flush()
	subject = fmt.Sprintf("PatchMon scheduled report: %s", reportName)
	return subject, htmlB.String(), csvB.String(), nil
}
