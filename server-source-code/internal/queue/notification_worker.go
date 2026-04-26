package queue

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// NotificationDeliverHandler sends webhook or email notifications.
type NotificationDeliverHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	enc       *util.Encryption
	rdb       *redis.Client
	log       *slog.Logger
}

// NewNotificationDeliverHandler creates the handler.
func NewNotificationDeliverHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, enc *util.Encryption, rdb *redis.Client, log *slog.Logger) *NotificationDeliverHandler {
	return &NotificationDeliverHandler{defaultDB: defaultDB, poolCache: poolCache, enc: enc, rdb: rdb, log: log}
}

func (h *NotificationDeliverHandler) resolveDB(ctx context.Context, payload []byte) *database.DB {
	db := h.defaultDB
	if len(payload) == 0 || h.poolCache == nil {
		return db
	}
	var p notifications.NotificationDeliverPayload
	if err := json.Unmarshal(payload, &p); err == nil && strings.TrimSpace(p.Host) != "" {
		if resolved, err := h.poolCache.GetOrCreate(ctx, p.Host); err == nil && resolved != nil {
			db = resolved
		}
	}
	return db
}

// ProcessTask implements asynq.Handler.
func (h *NotificationDeliverHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p notifications.NotificationDeliverPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	// If this was a delayed notification, check if it's been cancelled by a counterpart event.
	if p.Delayed && p.CancelKey != "" && notifications.IsDelayedCancelled(h.rdb, p.Host, p.CancelKey) {
		if h.log != nil {
			h.log.Debug("notification_deliver: cancelled by counterpart event", "event_type", p.EventType, "cancel_key", p.CancelKey)
		}
		return nil
	}
	d := h.resolveDB(ctx, t.Payload())
	dest, err := d.Queries.GetNotificationDestinationByID(ctx, p.DestinationID)
	if err != nil || !dest.Enabled {
		return nil
	}
	plain, err := h.decryptConfig(dest.ConfigEncrypted)
	if err != nil {
		h.logDelivery(ctx, d, p, "failed", fmt.Errorf("decrypt config: %w", err), "")
		return nil
	}
	switch strings.ToLower(dest.ChannelType) {
	case "webhook":
		err = h.sendWebhook(ctx, plain, p)
	case "email":
		err = h.sendEmail(ctx, plain, p)
	case "ntfy":
		err = h.sendNtfy(ctx, plain, p)
	case "internal":
		err = h.sendInternal(ctx, d, p)
	default:
		err = fmt.Errorf("unknown channel_type %q", dest.ChannelType)
	}
	if err != nil {
		if h.log != nil {
			h.log.Error("notification_deliver", "destination_id", p.DestinationID, "error", err)
		}
		h.logDelivery(ctx, d, p, "failed", err, err.Error())
		return err
	}
	h.logDelivery(ctx, d, p, "sent", nil, "")
	return nil
}

func (h *NotificationDeliverHandler) decryptConfig(encStr string) (string, error) {
	if encStr == "" {
		return "{}", nil
	}
	if h.enc != nil && util.IsEncrypted(encStr) {
		return h.enc.Decrypt(encStr)
	}
	return encStr, nil
}

type webhookConfig struct {
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers"`
	SigningSecret string            `json:"signing_secret"`
}

func isDiscordWebhookURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "discord.com", "discordapp.com", "www.discord.com":
		return strings.Contains(u.Path, "/api/webhooks/")
	default:
		return false
	}
}

func isSlackIncomingWebhookURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if strings.ToLower(u.Hostname()) != "hooks.slack.com" {
		return false
	}
	return strings.HasPrefix(u.Path, "/services/")
}

func truncateUTF8(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes-1]) + "…"
}

func discordColorForSeverity(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 15548997
	case "high":
		return 15105570
	case "medium":
		return 16776960
	case "low", "informational", "info":
		return 5793266
	default:
		return 3447003
	}
}

// nocMetaStr safely extracts a string from notification metadata.
func nocMetaStr(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// nocMetaStrSlice extracts a []string from metadata (stored as []interface{} after JSON round-trip).
func nocMetaStrSlice(m map[string]interface{}, key string) []string {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// severityEmoji returns a short icon for NOC-visible severity.
func severityEmoji(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return "🔴"
	case "error", "high":
		return "🟠"
	case "warning", "warn", "medium":
		return "🟡"
	default:
		return "🟢"
	}
}

// buildNOCFields creates structured Discord embed fields based on event type.
func buildNOCFields(p notifications.NotificationDeliverPayload) []map[string]interface{} {
	m := p.Metadata
	fields := []map[string]interface{}{}

	addField := func(name, value string, inline bool) {
		if value != "" {
			fields = append(fields, map[string]interface{}{
				"name": name, "value": truncateUTF8(value, 1024), "inline": inline,
			})
		}
	}

	switch {
	case strings.HasPrefix(p.EventType, "patch_run_"):
		addField("Host", nocMetaStr(m, "host_name"), true)
		addField("Status", severityEmoji(p.Severity)+" "+strings.ToUpper(nocMetaStr(m, "stage")), true)
		addField("Type", nocMetaStr(m, "patch_type"), true)

		packages := nocMetaStrSlice(m, "packages")
		if len(packages) > 0 {
			pkgDisplay := strings.Join(packages, ", ")
			if len(packages) > 5 {
				pkgDisplay = strings.Join(packages[:5], ", ") + fmt.Sprintf(" … +%d more", len(packages)-5)
			}
			addField("Packages", pkgDisplay, false)
		}
		addField("Policy", nocMetaStr(m, "policy_name"), true)
		if nocMetaStr(m, "dry_run") == "true" {
			addField("Mode", "🧪 Dry Run", true)
		}
		if errMsg := nocMetaStr(m, "error_message"); errMsg != "" {
			if len(errMsg) > 500 {
				errMsg = errMsg[:500] + "…"
			}
			addField("Error", "```\n"+errMsg+"\n```", false)
		}

	case p.EventType == "compliance_scan_completed":
		addField("Host", nocMetaStr(m, "host_name"), true)
		addField("Scans", nocMetaStr(m, "scans_count"), true)
		totalRules := nocMetaStr(m, "total_rules")
		if totalRules != "" && totalRules != "0" {
			passedCount := nocMetaStr(m, "passed_count")
			failedCount := nocMetaStr(m, "failed_count")
			addField("Results", fmt.Sprintf("✅ %s passed  ❌ %s failed  (of %s total)", passedCount, failedCount, totalRules), false)
		}
		// Show per-profile summaries
		if profiles, ok := m["profile_summaries"].([]interface{}); ok {
			for _, raw := range profiles {
				if ps, ok := raw.(map[string]interface{}); ok {
					profileName := nocMetaStr(ps, "profile")
					score := nocMetaStr(ps, "score")
					line := profileName
					if score != "" {
						line += " - Score: " + score
					}
					passed := nocMetaStr(ps, "passed")
					failed := nocMetaStr(ps, "failed")
					if passed != "" {
						line += fmt.Sprintf(" - ✅ %s passed, ❌ %s failed", passed, failed)
					}
					addField("Profile", line, false)
				}
			}
		} else if profiles, ok := m["profile_summaries"].([]map[string]interface{}); ok {
			for _, ps := range profiles {
				profileName := nocMetaStr(ps, "profile")
				score := nocMetaStr(ps, "score")
				line := profileName
				if score != "" {
					line += " - Score: " + score
				}
				passed := nocMetaStr(ps, "passed")
				failed := nocMetaStr(ps, "failed")
				if passed != "" {
					line += fmt.Sprintf(" - ✅ %s passed, ❌ %s failed", passed, failed)
				}
				addField("Profile", line, false)
			}
		}

	case p.EventType == "host_down":
		addField("Host", nocMetaStr(m, "host_name"), true)
		addField("Severity", severityEmoji(p.Severity)+" "+strings.ToUpper(p.Severity), true)
		if lastUpdate := nocMetaStr(m, "last_update"); lastUpdate != "" {
			addField("Last Seen", lastUpdate, true)
		}
		if threshold := nocMetaStr(m, "threshold_minutes"); threshold != "" && threshold != "0" {
			addField("Threshold", threshold+" minutes", true)
		}
		if reason := nocMetaStr(m, "disconnect_reason"); reason != "" {
			addField("Disconnect", reason, true)
		}

	case p.EventType == "host_recovered":
		addField("Host", nocMetaStr(m, "host_name"), true)
		addField("Status", "🟢 RECOVERED", true)

	case p.EventType == "server_update" || p.EventType == "agent_update":
		addField("Current Version", nocMetaStr(m, "current_version"), true)
		addField("Available Version", "⬆️ "+nocMetaStr(m, "latest_version"), true)

	default:
		// Generic: show severity
		addField("Severity", severityEmoji(p.Severity)+" "+strings.ToUpper(p.Severity), true)
	}

	return fields
}

func discordWebhookBody(p notifications.NotificationDeliverPayload) ([]byte, error) {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = "PatchMon"
	}
	title = truncateUTF8(title, 256)

	desc := strings.TrimSpace(p.Message)
	if desc == "" {
		desc = "_No message_"
	}
	desc = truncateUTF8(desc, 3800)

	fields := buildNOCFields(p)

	// Add clickable link to PatchMon if available
	if link := nocMetaStr(p.Metadata, "app_link"); link != "" {
		fields = append(fields, map[string]interface{}{
			"name": "🔗 View in PatchMon", "value": "[Open](" + link + ")", "inline": false,
		})
	}

	embed := map[string]interface{}{
		"title":       title,
		"description": desc,
		"color":       discordColorForSeverity(p.Severity),
		"fields":      fields,
		"footer":      map[string]interface{}{"text": "PatchMon"},
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}
	body := map[string]interface{}{
		"username": "PatchMon",
		"embeds":   []interface{}{embed},
	}
	return json.Marshal(body)
}

// slackTextMaxRunes stays under Slack incoming-webhook message size limits with headroom.
const slackTextMaxRunes = 12000

// buildSlackNOCFields creates human-readable Slack mrkdwn lines per event type.
func buildSlackNOCFields(p notifications.NotificationDeliverPayload) string {
	m := p.Metadata
	var sb strings.Builder

	addLine := func(label, value string) {
		if value != "" {
			sb.WriteString("*")
			sb.WriteString(label)
			sb.WriteString(":* ")
			sb.WriteString(value)
			sb.WriteString("\n")
		}
	}

	switch {
	case strings.HasPrefix(p.EventType, "patch_run_"):
		addLine("Host", nocMetaStr(m, "host_name"))
		addLine("Status", severityEmoji(p.Severity)+" "+strings.ToUpper(nocMetaStr(m, "stage")))
		addLine("Type", nocMetaStr(m, "patch_type"))
		packages := nocMetaStrSlice(m, "packages")
		if len(packages) > 0 {
			pkgDisplay := strings.Join(packages, ", ")
			if len(packages) > 5 {
				pkgDisplay = strings.Join(packages[:5], ", ") + fmt.Sprintf(" … +%d more", len(packages)-5)
			}
			addLine("Packages", pkgDisplay)
		}
		addLine("Policy", nocMetaStr(m, "policy_name"))
		if nocMetaStr(m, "dry_run") == "true" {
			addLine("Mode", "🧪 Dry Run")
		}
		if errMsg := nocMetaStr(m, "error_message"); errMsg != "" {
			if len(errMsg) > 500 {
				errMsg = errMsg[:500] + "…"
			}
			sb.WriteString("*Error:*\n```\n")
			sb.WriteString(errMsg)
			sb.WriteString("\n```\n")
		}

	case p.EventType == "compliance_scan_completed":
		addLine("Host", nocMetaStr(m, "host_name"))
		addLine("Scans", nocMetaStr(m, "scans_count"))
		totalRules := nocMetaStr(m, "total_rules")
		if totalRules != "" && totalRules != "0" {
			addLine("Results", fmt.Sprintf("✅ %s passed  ❌ %s failed  (of %s total)",
				nocMetaStr(m, "passed_count"), nocMetaStr(m, "failed_count"), totalRules))
		}
		if profiles, ok := m["profile_summaries"].([]interface{}); ok {
			for _, raw := range profiles {
				if ps, ok := raw.(map[string]interface{}); ok {
					line := nocMetaStr(ps, "profile")
					if score := nocMetaStr(ps, "score"); score != "" {
						line += " - Score: " + score
					}
					if passed := nocMetaStr(ps, "passed"); passed != "" {
						line += fmt.Sprintf(" - ✅ %s / ❌ %s", passed, nocMetaStr(ps, "failed"))
					}
					addLine("Profile", line)
				}
			}
		} else if profiles, ok := m["profile_summaries"].([]map[string]interface{}); ok {
			for _, ps := range profiles {
				line := nocMetaStr(ps, "profile")
				if score := nocMetaStr(ps, "score"); score != "" {
					line += " - Score: " + score
				}
				if passed := nocMetaStr(ps, "passed"); passed != "" {
					line += fmt.Sprintf(" - ✅ %s / ❌ %s", passed, nocMetaStr(ps, "failed"))
				}
				addLine("Profile", line)
			}
		}

	case p.EventType == "host_down":
		addLine("Host", nocMetaStr(m, "host_name"))
		addLine("Severity", severityEmoji(p.Severity)+" "+strings.ToUpper(p.Severity))
		addLine("Last Seen", nocMetaStr(m, "last_update"))
		if t := nocMetaStr(m, "threshold_minutes"); t != "" && t != "0" {
			addLine("Threshold", t+" minutes")
		}
		addLine("Disconnect", nocMetaStr(m, "disconnect_reason"))

	case p.EventType == "host_recovered":
		addLine("Host", nocMetaStr(m, "host_name"))
		addLine("Status", "🟢 RECOVERED")

	case p.EventType == "server_update" || p.EventType == "agent_update":
		addLine("Current Version", nocMetaStr(m, "current_version"))
		addLine("Available Version", "⬆️ "+nocMetaStr(m, "latest_version"))

	default:
		addLine("Severity", severityEmoji(p.Severity)+" "+strings.ToUpper(p.Severity))
	}

	return sb.String()
}

func slackIncomingWebhookBody(p notifications.NotificationDeliverPayload) ([]byte, error) {
	var sb strings.Builder
	sev := strings.TrimSpace(p.Severity)
	if sev == "" {
		sev = "notice"
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = "PatchMon"
	}

	// Header line with severity icon
	sb.WriteString(severityEmoji(sev))
	sb.WriteString(" *")
	sb.WriteString(title)
	sb.WriteString("*\n")

	// Description
	if msg := strings.TrimSpace(p.Message); msg != "" {
		sb.WriteString("\n")
		sb.WriteString(msg)
		sb.WriteString("\n")
	}

	// Structured fields
	sb.WriteString("\n")
	sb.WriteString(buildSlackNOCFields(p))

	// Clickable link
	if link := nocMetaStr(p.Metadata, "app_link"); link != "" {
		sb.WriteString("\n<")
		sb.WriteString(link)
		sb.WriteString("|🔗 View in PatchMon>\n")
	}

	text := truncateUTF8(sb.String(), slackTextMaxRunes)
	out := map[string]interface{}{
		"text":       text,
		"username":   "PatchMon",
		"icon_emoji": ":bell:",
	}
	return json.Marshal(out)
}

// stripScheduledReportHTML removes tags (including script blocks) for a short Discord-friendly excerpt.
var stripScheduledReportHTMLRE = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>|<[^>]+>`)

func stripScheduledReportHTML(s string) string {
	s = stripScheduledReportHTMLRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func discordScheduledReportWebhookBody(subject, html, csv string) ([]byte, error) {
	title := strings.TrimSpace(subject)
	if title == "" {
		title = "PatchMon scheduled report"
	}
	title = truncateUTF8(title, 256)

	desc := stripScheduledReportHTML(html)
	if desc == "" {
		desc = "_No HTML body in this delivery_"
	}
	desc = truncateUTF8(desc, 3800)

	fields := []map[string]interface{}{}
	if strings.TrimSpace(csv) != "" {
		const maxField = 1024
		prefix, suffix := "```csv\n", "\n```"
		availRunes := maxField - utf8.RuneCountInString(prefix) - utf8.RuneCountInString(suffix)
		if availRunes < 1 {
			availRunes = 1
		}
		cv := truncateUTF8(strings.TrimSpace(csv), availRunes)
		fields = append(fields, map[string]interface{}{
			"name": "CSV preview", "value": prefix + cv + suffix, "inline": false,
		})
	}

	embed := map[string]interface{}{
		"title":       title,
		"description": desc,
		"color":       5814783,
		"footer":      map[string]interface{}{"text": "PatchMon · scheduled report"},
	}
	if len(fields) > 0 {
		embed["fields"] = fields
	}
	body := map[string]interface{}{
		"username": "PatchMon",
		"embeds":   []interface{}{embed},
	}
	return json.Marshal(body)
}

func slackScheduledReportWebhookBody(subject, html, csv string) ([]byte, error) {
	var sb strings.Builder
	sb.WriteString("*PatchMon · scheduled report*\n*")
	subj := strings.TrimSpace(subject)
	if subj == "" {
		subj = "(no subject)"
	}
	sb.WriteString(subj)
	sb.WriteString("*\n\n")
	body := stripScheduledReportHTML(html)
	if body == "" {
		body = "_No HTML body in this delivery_"
	}
	body = truncateUTF8(body, 9000)
	sb.WriteString(body)
	if strings.TrimSpace(csv) != "" {
		sb.WriteString("\n\n*CSV preview*\n```\n")
		cv := truncateUTF8(strings.TrimSpace(csv), 2500)
		sb.WriteString(cv)
		sb.WriteString("\n```\n")
	}
	text := truncateUTF8(sb.String(), slackTextMaxRunes)
	out := map[string]interface{}{
		"text":       text,
		"username":   "PatchMon",
		"icon_emoji": ":bar_chart:",
	}
	return json.Marshal(out)
}

type emailConfig struct {
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	UseTLS   bool   `json:"use_tls"`
}

func (h *NotificationDeliverHandler) sendWebhook(ctx context.Context, plain string, p notifications.NotificationDeliverPayload) error {
	var cfg webhookConfig
	if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
		return err
	}
	if cfg.URL == "" {
		return fmt.Errorf("webhook url missing")
	}
	var b []byte
	var err error
	switch {
	case isDiscordWebhookURL(cfg.URL):
		b, err = discordWebhookBody(p)
	case isSlackIncomingWebhookURL(cfg.URL):
		b, err = slackIncomingWebhookBody(p)
	default:
		body := map[string]interface{}{
			"event_type": p.EventType,
			"severity":   p.Severity,
			"title":      p.Title,
			"message":    p.Message,
			"reference": map[string]string{
				"type": p.ReferenceType,
				"id":   p.ReferenceID,
			},
			"metadata": p.Metadata,
		}
		if link := nocMetaStr(p.Metadata, "app_link"); link != "" {
			body["app_link"] = link
		}
		b, err = json.Marshal(body)
	}
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	if cfg.SigningSecret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.SigningSecret))
		mac.Write(b)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-PatchMon-Signature", "sha256="+sig)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

// buildEmailHTML constructs a clean, NOC-readable HTML email body.
func buildEmailHTML(p notifications.NotificationDeliverPayload) string {
	esc := notifications.TemplateEscape
	m := p.Metadata

	var sb strings.Builder
	sb.WriteString(`<html><body style="font-family:sans-serif;color:#333;max-width:600px;margin:0 auto;">`)
	fmt.Fprintf(&sb, `<h2 style="margin-bottom:4px;">%s %s</h2>`, severityEmoji(p.Severity), esc(p.Title))

	// Message body (preserve newlines)
	if msg := strings.TrimSpace(p.Message); msg != "" {
		for _, line := range strings.Split(msg, "\n") {
			fmt.Fprintf(&sb, "<p style=\"margin:4px 0;\">%s</p>", esc(line))
		}
	}

	// Structured details table
	sb.WriteString(`<table style="border-collapse:collapse;margin:16px 0;width:100%;">`)
	addRow := func(label, value string) {
		if value != "" {
			fmt.Fprintf(&sb, `<tr><td style="padding:6px 12px 6px 0;font-weight:bold;vertical-align:top;white-space:nowrap;">%s</td><td style="padding:6px 0;">%s</td></tr>`,
				esc(label), esc(value))
		}
	}

	switch {
	case strings.HasPrefix(p.EventType, "patch_run_"):
		addRow("Host", nocMetaStr(m, "host_name"))
		addRow("Patch Type", nocMetaStr(m, "patch_type"))
		packages := nocMetaStrSlice(m, "packages")
		if len(packages) > 0 {
			pkgDisplay := strings.Join(packages, ", ")
			if len(packages) > 10 {
				pkgDisplay = strings.Join(packages[:10], ", ") + fmt.Sprintf(" … +%d more", len(packages)-10)
			}
			addRow("Packages", pkgDisplay)
		}
		addRow("Policy", nocMetaStr(m, "policy_name"))
		if nocMetaStr(m, "dry_run") == "true" {
			addRow("Mode", "Dry Run")
		}
		addRow("Error", nocMetaStr(m, "error_message"))

	case p.EventType == "compliance_scan_completed":
		addRow("Host", nocMetaStr(m, "host_name"))
		addRow("Scans", nocMetaStr(m, "scans_count"))
		addRow("Passed", nocMetaStr(m, "passed_count"))
		addRow("Failed", nocMetaStr(m, "failed_count"))
		addRow("Total Rules", nocMetaStr(m, "total_rules"))

	case p.EventType == "host_down":
		addRow("Host", nocMetaStr(m, "host_name"))
		addRow("Last Seen", nocMetaStr(m, "last_update"))
		addRow("Threshold", nocMetaStr(m, "threshold_minutes")+" minutes")

	case p.EventType == "host_recovered":
		addRow("Host", nocMetaStr(m, "host_name"))

	case p.EventType == "server_update" || p.EventType == "agent_update":
		addRow("Current Version", nocMetaStr(m, "current_version"))
		addRow("Available Version", nocMetaStr(m, "latest_version"))
	}

	sb.WriteString("</table>")

	// Clickable link
	if link := nocMetaStr(m, "app_link"); link != "" {
		fmt.Fprintf(&sb, `<p><a href="%s" style="display:inline-block;background:#2563eb;color:#fff;padding:10px 20px;border-radius:6px;text-decoration:none;font-weight:bold;">View in PatchMon</a></p>`, esc(link))
	}

	sb.WriteString(`<hr style="border:none;border-top:1px solid #ddd;margin:20px 0;"/>`)
	sb.WriteString(`<p style="color:#999;font-size:12px;">Sent by PatchMon</p>`)
	sb.WriteString("</body></html>")
	return sb.String()
}

func (h *NotificationDeliverHandler) sendEmail(ctx context.Context, plain string, p notifications.NotificationDeliverPayload) error {
	var cfg emailConfig
	if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
		return err
	}
	if cfg.SMTPHost == "" || cfg.From == "" || cfg.To == "" {
		return fmt.Errorf("email smtp_host, from, and to are required")
	}
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}
	subject := fmt.Sprintf("[%s] %s", strings.ToUpper(p.Severity), p.Title)
	// Sanitize subject to prevent SMTP header injection via \r\n in host names / alert titles.
	subject = strings.NewReplacer("\r", "", "\n", "").Replace(subject)
	html := buildEmailHTML(p)
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s",
		cfg.From, cfg.To, subject, html))
	addr := cfg.SMTPHost + ":" + strconv.Itoa(cfg.SMTPPort)
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)
	}
	tlsCfg := &tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}

	// Plain TCP first, then:
	// - use_tls=true and server offers STARTTLS: upgrade with STARTTLS (typical 587)
	// - use_tls=true and no STARTTLS: retry with implicit TLS (e.g. wrong host/port 465 on 25/587)
	// - use_tls=false: never call StartTLS even if the server advertises it (e.g. local relay)
	c, conn, err := func() (*smtp.Client, net.Conn, error) {
		plainConn, dialErr := net.DialTimeout("tcp", addr, 30*time.Second)
		if dialErr != nil {
			return nil, nil, dialErr
		}
		client, clientErr := smtp.NewClient(plainConn, cfg.SMTPHost)
		if clientErr != nil {
			_ = plainConn.Close()
			return nil, nil, clientErr
		}
		startTLS, _ := client.Extension("STARTTLS")
		if startTLS && cfg.UseTLS {
			if tlsErr := client.StartTLS(tlsCfg); tlsErr != nil {
				_ = client.Close()
				return nil, nil, tlsErr
			}
			return client, plainConn, nil
		}
		if cfg.UseTLS && !startTLS {
			_ = client.Close()
			tlsConn, tlsErr := tls.DialWithDialer(&net.Dialer{Timeout: 30 * time.Second}, "tcp", addr, tlsCfg)
			if tlsErr != nil {
				return nil, nil, tlsErr
			}
			client, clientErr = smtp.NewClient(tlsConn, cfg.SMTPHost)
			if clientErr != nil {
				_ = tlsConn.Close()
				return nil, nil, clientErr
			}
			return client, tlsConn, nil
		}
		return client, plainConn, nil
	}()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	defer func() { _ = c.Close() }()

	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := c.Mail(cfg.From); err != nil {
		return err
	}
	if err := c.Rcpt(cfg.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	return w.Close()
}

// ntfyConfig holds the configuration for an ntfy destination.
type ntfyConfig struct {
	ServerURL string `json:"server_url"` // e.g. "https://ntfy.sh"
	Topic     string `json:"topic"`
	Token     string `json:"token"`    // optional access token
	Username  string `json:"username"` // optional basic auth
	Password  string `json:"password"` // optional basic auth
	Priority  string `json:"priority"` // optional: min, low, default, high, urgent (or 1-5)
}

// ntfyPriorityForSeverity maps PatchMon severity to ntfy priority integers (1-5).
func ntfyPriorityForSeverity(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 5 // urgent
	case "error", "high":
		return 4 // high
	case "warning", "warn", "medium":
		return 3 // default
	default:
		return 2 // low
	}
}

// ntfyTagsForEvent returns ntfy emoji tags based on event type and severity.
func ntfyTagsForEvent(eventType, severity string) []string {
	tags := []string{}
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		tags = append(tags, "rotating_light")
	case "error", "high":
		tags = append(tags, "warning")
	case "warning", "warn", "medium":
		tags = append(tags, "large_orange_diamond")
	default:
		tags = append(tags, "white_check_mark")
	}
	switch {
	case strings.HasPrefix(eventType, "patch_run_"):
		tags = append(tags, "package")
	case eventType == "host_down":
		tags = append(tags, "skull")
	case eventType == "host_recovered":
		tags = append(tags, "tada")
	case eventType == "server_update" || eventType == "agent_update":
		tags = append(tags, "arrow_up")
	case eventType == "compliance_scan_completed":
		tags = append(tags, "shield")
	case eventType == "test":
		tags = append(tags, "test_tube")
	}
	return tags
}

// buildNtfyMessage creates a plain-text message body for ntfy notifications.
func buildNtfyMessage(p notifications.NotificationDeliverPayload) string {
	m := p.Metadata
	var sb strings.Builder
	if msg := strings.TrimSpace(p.Message); msg != "" {
		sb.WriteString(msg)
		sb.WriteString("\n")
	}

	addLine := func(label, value string) {
		if value != "" {
			sb.WriteString(label)
			sb.WriteString(": ")
			sb.WriteString(value)
			sb.WriteString("\n")
		}
	}

	switch {
	case strings.HasPrefix(p.EventType, "patch_run_"):
		addLine("Host", nocMetaStr(m, "host_name"))
		addLine("Status", strings.ToUpper(nocMetaStr(m, "stage")))
		addLine("Type", nocMetaStr(m, "patch_type"))
		packages := nocMetaStrSlice(m, "packages")
		if len(packages) > 0 {
			pkgDisplay := strings.Join(packages, ", ")
			if len(packages) > 5 {
				pkgDisplay = strings.Join(packages[:5], ", ") + fmt.Sprintf(" +%d more", len(packages)-5)
			}
			addLine("Packages", pkgDisplay)
		}
		addLine("Policy", nocMetaStr(m, "policy_name"))
		if nocMetaStr(m, "dry_run") == "true" {
			addLine("Mode", "Dry Run")
		}
		if errMsg := nocMetaStr(m, "error_message"); errMsg != "" {
			if len(errMsg) > 300 {
				errMsg = errMsg[:300] + "..."
			}
			addLine("Error", errMsg)
		}
	case p.EventType == "compliance_scan_completed":
		addLine("Host", nocMetaStr(m, "host_name"))
		addLine("Scans", nocMetaStr(m, "scans_count"))
		totalRules := nocMetaStr(m, "total_rules")
		if totalRules != "" && totalRules != "0" {
			addLine("Results", fmt.Sprintf("%s passed, %s failed (of %s)",
				nocMetaStr(m, "passed_count"), nocMetaStr(m, "failed_count"), totalRules))
		}
	case p.EventType == "host_down":
		addLine("Host", nocMetaStr(m, "host_name"))
		addLine("Severity", strings.ToUpper(p.Severity))
		addLine("Last seen", nocMetaStr(m, "last_update"))
		if t := nocMetaStr(m, "threshold_minutes"); t != "" && t != "0" {
			addLine("Threshold", t+" minutes")
		}
		addLine("Disconnect", nocMetaStr(m, "disconnect_reason"))
	case p.EventType == "host_recovered":
		addLine("Host", nocMetaStr(m, "host_name"))
		addLine("Status", "RECOVERED")
	case p.EventType == "server_update" || p.EventType == "agent_update":
		addLine("Current version", nocMetaStr(m, "current_version"))
		addLine("Available version", nocMetaStr(m, "latest_version"))
	}
	return strings.TrimSpace(sb.String())
}

func (h *NotificationDeliverHandler) sendNtfy(ctx context.Context, plain string, p notifications.NotificationDeliverPayload) error {
	var cfg ntfyConfig
	if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
		return err
	}
	if cfg.Topic == "" {
		return fmt.Errorf("ntfy topic is required")
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = "https://ntfy.sh"
	}
	serverURL := strings.TrimRight(cfg.ServerURL, "/")

	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = "PatchMon"
	}

	priority := ntfyPriorityForSeverity(p.Severity)
	if cfg.Priority != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Priority)) {
		case "1", "min":
			priority = 1
		case "2", "low":
			priority = 2
		case "3", "default":
			priority = 3
		case "4", "high":
			priority = 4
		case "5", "urgent", "max":
			priority = 5
		}
	}

	message := buildNtfyMessage(p)
	if message == "" {
		message = strings.TrimSpace(p.Message)
		if message == "" {
			message = title
		}
	}

	tags := ntfyTagsForEvent(p.EventType, p.Severity)

	body := map[string]interface{}{
		"topic":    cfg.Topic,
		"title":    truncateUTF8(title, 256),
		"message":  truncateUTF8(message, 4000),
		"priority": priority,
		"tags":     tags,
	}

	if link := nocMetaStr(p.Metadata, "app_link"); link != "" {
		body["click"] = link
		body["actions"] = []map[string]interface{}{
			{"action": "view", "label": "View in PatchMon", "url": link},
		}
	}

	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	} else if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy status %d", resp.StatusCode)
	}
	return nil
}

// sendInternal creates an alert record in the alerts table (the built-in "Internal Alerts" destination).
func (h *NotificationDeliverHandler) sendInternal(ctx context.Context, d *database.DB, p notifications.NotificationDeliverPayload) error {
	if d == nil {
		return fmt.Errorf("no database available")
	}
	// Check if alerts are globally enabled.
	settings, err := d.Queries.GetFirstSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.AlertsEnabled {
		return nil // silently skip
	}
	// Dedup: skip if a matching active alert already exists for this event type.
	// This prevents duplicate records from periodic checks (host_down, agent_update, server_update).
	if h.hasActiveMatchingAlert(ctx, d, p) {
		return nil
	}
	metaJSON, _ := json.Marshal(p.Metadata)
	if metaJSON == nil {
		metaJSON = []byte("{}")
	}
	alertID := uuid.New().String()
	_, err = d.Queries.CreateAlert(ctx, db.CreateAlertParams{
		ID:       alertID,
		Type:     p.EventType,
		Severity: p.Severity,
		Title:    p.Title,
		Message:  p.Message,
		Column6:  metaJSON,
		Column7:  true, // is_active
	})
	if err != nil {
		return fmt.Errorf("create alert: %w", err)
	}
	// Record "created" in alert history.
	_, _ = d.Queries.InsertAlertHistory(ctx, db.InsertAlertHistoryParams{
		ID:      uuid.New().String(),
		AlertID: alertID,
		UserID:  nil,
		Action:  "created",
		Column5: []byte(`{"system_action":true}`),
	})
	// Auto-assign if configured for this alert type.
	if cfg, cfgErr := d.Queries.GetAlertConfigByType(ctx, p.EventType); cfgErr == nil && cfg.AutoAssignEnabled && cfg.AutoAssignUserID != nil && *cfg.AutoAssignUserID != "" {
		_ = d.Queries.UpdateAlertAssignment(ctx, db.UpdateAlertAssignmentParams{
			ID:               alertID,
			AssignedToUserID: cfg.AutoAssignUserID,
		})
		assignMeta, _ := json.Marshal(map[string]interface{}{"assigned_to": *cfg.AutoAssignUserID, "system_action": true})
		_, _ = d.Queries.InsertAlertHistory(ctx, db.InsertAlertHistoryParams{
			ID:      uuid.New().String(),
			AlertID: alertID,
			UserID:  nil,
			Action:  "assigned",
			Column5: assignMeta,
		})
	}
	return nil
}

// hasActiveMatchingAlert returns true if an active alert already exists that
// matches this event's type and key metadata, preventing duplicate records
// from periodic checks.
func (h *NotificationDeliverHandler) hasActiveMatchingAlert(ctx context.Context, d *database.DB, p notifications.NotificationDeliverPayload) bool {
	active, err := d.Queries.ListActiveAlertsByType(ctx, p.EventType)
	if err != nil || len(active) == 0 {
		return false
	}
	switch p.EventType {
	case "host_down":
		hostID := nocMetaStr(p.Metadata, "host_id")
		if hostID == "" {
			return false
		}
		for _, a := range active {
			var m map[string]interface{}
			if len(a.Metadata) > 0 {
				_ = json.Unmarshal(a.Metadata, &m)
			}
			if hid, _ := m["host_id"].(string); hid == hostID {
				return true
			}
		}
	case "agent_update", "server_update":
		latestVer := nocMetaStr(p.Metadata, "latest_version")
		if latestVer == "" {
			return false
		}
		for _, a := range active {
			var m map[string]interface{}
			if len(a.Metadata) > 0 {
				_ = json.Unmarshal(a.Metadata, &m)
			}
			if lv, _ := m["latest_version"].(string); lv == latestVer {
				return true
			}
		}
	case "host_security_updates_exceeded", "host_pending_updates_exceeded":
		hostID := nocMetaStr(p.Metadata, "host_id")
		if hostID == "" {
			return false
		}
		for _, a := range active {
			var m map[string]interface{}
			if len(a.Metadata) > 0 {
				_ = json.Unmarshal(a.Metadata, &m)
			}
			if hid, _ := m["host_id"].(string); hid == hostID {
				return true
			}
		}
	case "container_stopped", "container_started":
		containerID := nocMetaStr(p.Metadata, "container_id")
		if containerID == "" {
			return false
		}
		for _, a := range active {
			var m map[string]interface{}
			if len(a.Metadata) > 0 {
				_ = json.Unmarshal(a.Metadata, &m)
			}
			if cid, _ := m["container_id"].(string); cid == containerID {
				return true
			}
		}
	}
	return false
}

func (h *NotificationDeliverHandler) logDelivery(ctx context.Context, d *database.DB, p notifications.NotificationDeliverPayload, status string, logErr error, errMsg string) {
	if d == nil {
		return
	}
	em := errMsg
	if logErr != nil {
		em = logErr.Error()
	}
	var emPtr *string
	if em != "" {
		emPtr = &em
	}
	_, err := d.Queries.InsertNotificationDeliveryLog(ctx, db.InsertNotificationDeliveryLogParams{
		ID:                uuid.New().String(),
		EventFingerprint:  p.EventFingerprint,
		ReferenceType:     p.ReferenceType,
		ReferenceID:       p.ReferenceID,
		DestinationID:     p.DestinationID,
		EventType:         p.EventType,
		Status:            status,
		ErrorMessage:      emPtr,
		AttemptCount:      int32(1),
		ProviderMessageID: nil,
	})
	if err != nil && h.log != nil {
		h.log.Debug("notification delivery log insert failed", "error", err)
	}
}
