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
	"strconv"
	"strings"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// ScheduledReportsDispatchHandler enqueues run jobs for due scheduled reports.
type ScheduledReportsDispatchHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	qc        *asynq.Client
	log       *slog.Logger
}

// NewScheduledReportsDispatchHandler creates the handler.
func NewScheduledReportsDispatchHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, qc *asynq.Client, log *slog.Logger) *ScheduledReportsDispatchHandler {
	return &ScheduledReportsDispatchHandler{defaultDB: defaultDB, poolCache: poolCache, qc: qc, log: log}
}

func (h *ScheduledReportsDispatchHandler) resolveDB(ctx context.Context, payload []byte) *database.DB {
	db := h.defaultDB
	if len(payload) == 0 || h.poolCache == nil {
		return db
	}
	var p AutomationPayload
	if err := json.Unmarshal(payload, &p); err == nil && strings.TrimSpace(p.Host) != "" {
		if resolved, err := h.poolCache.GetOrCreate(ctx, p.Host); err == nil && resolved != nil {
			db = resolved
		}
	}
	return db
}

func (h *ScheduledReportsDispatchHandler) processDB(ctx context.Context, d *database.DB, tenantHost string) {
	now := time.Now()
	rows, err := d.Queries.ListScheduledReportsDue(ctx, pgtype.Timestamp{Time: now, Valid: true})
	if err != nil {
		if h.log != nil {
			h.log.Error("scheduled_reports_dispatch: list due", "error", err)
		}
		return
	}
	for _, r := range rows {
		// Use the same enqueue path as the event-driven chain so TaskIDs
		// are consistent and duplicate runs are prevented.
		if err := EnqueueScheduledReportAt(h.qc, r.ID, tenantHost, now); err != nil {
			if h.log != nil {
				h.log.Debug("scheduled_reports_dispatch: enqueue skipped", "report_id", r.ID, "error", err)
			}
		}
	}
}

// ProcessTask implements asynq.Handler.
func (h *ScheduledReportsDispatchHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	payload := t.Payload()
	if len(payload) > 0 {
		d := h.resolveDB(ctx, payload)
		h.processDB(ctx, d, tenantHostFromPayload(payload))
		return nil
	}
	if h.poolCache == nil {
		h.processDB(ctx, h.defaultDB, "")
		return nil
	}
	h.processDB(ctx, h.defaultDB, "")
	hosts := h.poolCache.ListHosts()
	for _, host := range hosts {
		d, err := h.poolCache.GetOrCreate(ctx, host)
		if err != nil || d == nil {
			continue
		}
		h.processDB(ctx, d, host)
	}
	return nil
}

// ScheduledReportRunPayload is the payload for scheduled_report_run.
type ScheduledReportRunPayload struct {
	ReportID string `json:"report_id"`
	Host     string `json:"host,omitempty"`
}

// NewScheduledReportRunTask enqueues report generation and delivery.
// The TaskID includes a minute-bucket of the target run time so that each
// scheduled execution is unique, self-enqueue doesn't collide with the
// currently-active task, and "Run Now" can always enqueue.
func NewScheduledReportRunTask(p ScheduledReportRunPayload, runAt time.Time) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	taskID := fmt.Sprintf("scheduled-report-run-%s-%d", p.ReportID, runAt.Unix()/60)
	return asynq.NewTask(TypeScheduledReportRun, b,
		asynq.Queue(QueueScheduledReports),
		asynq.MaxRetry(3),
		asynq.TaskID(taskID),
	), nil
}

// EnqueueScheduledReportAt enqueues a scheduled report run to fire at a specific time.
// Duplicate tasks for the same report+time bucket are silently ignored.
func EnqueueScheduledReportAt(qc *asynq.Client, reportID, host string, runAt time.Time) error {
	if qc == nil {
		return nil
	}
	task, err := NewScheduledReportRunTask(ScheduledReportRunPayload{ReportID: reportID, Host: host}, runAt)
	if err != nil {
		return err
	}
	_, err = qc.Enqueue(task, asynq.ProcessAt(runAt))
	if err == asynq.ErrDuplicateTask || err == asynq.ErrTaskIDConflict {
		return nil
	}
	return err
}

// ScheduledReportRunHandler builds and sends a scheduled report.
type ScheduledReportRunHandler struct {
	defaultDB *database.DB
	poolCache *hostctx.PoolCache
	qc        *asynq.Client
	enc       *util.Encryption
	timezone  string
	log       *slog.Logger
}

// NewScheduledReportRunHandler creates the handler.
func NewScheduledReportRunHandler(defaultDB *database.DB, poolCache *hostctx.PoolCache, qc *asynq.Client, enc *util.Encryption, timezone string, log *slog.Logger) *ScheduledReportRunHandler {
	if timezone == "" {
		timezone = "UTC"
	}
	return &ScheduledReportRunHandler{defaultDB: defaultDB, poolCache: poolCache, qc: qc, enc: enc, timezone: timezone, log: log}
}

func (h *ScheduledReportRunHandler) resolveDB(ctx context.Context, payload []byte) *database.DB {
	db := h.defaultDB
	if len(payload) == 0 || h.poolCache == nil {
		return db
	}
	var p ScheduledReportRunPayload
	if err := json.Unmarshal(payload, &p); err == nil && strings.TrimSpace(p.Host) != "" {
		if resolved, err := h.poolCache.GetOrCreate(ctx, p.Host); err == nil && resolved != nil {
			db = resolved
		}
	}
	return db
}

// ProcessTask implements asynq.Handler.
func (h *ScheduledReportRunHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p ScheduledReportRunPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	d := h.resolveDB(ctx, t.Payload())
	rep, err := d.Queries.GetScheduledReportByID(ctx, p.ReportID)
	if err != nil {
		return nil
	}
	if !rep.Enabled {
		return nil
	}

	// Build branding from settings for email template.
	branding := notifications.ReportBranding{}
	if settings, sErr := d.Queries.GetFirstSettings(ctx); sErr == nil {
		baseURL := strings.TrimRight(settings.ServerUrl, "/")
		branding.ServerURL = baseURL
		if settings.LogoLight != nil && *settings.LogoLight != "" {
			branding.LogoLightURL = baseURL + *settings.LogoLight
		}
		if settings.LogoDark != nil && *settings.LogoDark != "" {
			branding.LogoDarkURL = baseURL + *settings.LogoDark
		}
	}

	subject, htmlBody, csvBody, err := notifications.BuildScheduledReport(ctx, d, rep.Name, rep.Definition, branding)
	if err != nil {
		h.insertRun(ctx, d, p.ReportID, "failed", err.Error(), "")
		return err
	}
	sum := sha256.Sum256([]byte(htmlBody + csvBody))
	sumHex := hex.EncodeToString(sum[:])

	var destIDs []string
	if len(rep.DestinationIds) > 0 {
		_ = json.Unmarshal(rep.DestinationIds, &destIDs)
	}
	if len(destIDs) == 0 {
		h.insertRun(ctx, d, p.ReportID, "failed", "no destinations configured", sumHex)
		return fmt.Errorf("no destinations")
	}

	for _, did := range destIDs {
		if strings.TrimSpace(did) == "" {
			continue
		}
		dest, err := d.Queries.GetNotificationDestinationByID(ctx, did)
		if err != nil || !dest.Enabled {
			continue
		}
		plain, err := decryptNotifConfig(h.enc, dest.ConfigEncrypted)
		if err != nil {
			continue
		}
		switch strings.ToLower(dest.ChannelType) {
		case "webhook":
			err = sendScheduledWebhook(ctx, plain, subject, htmlBody, csvBody)
		case "email":
			err = sendScheduledEmail(plain, subject, htmlBody, csvBody)
		case "ntfy":
			err = sendScheduledNtfy(ctx, plain, subject, htmlBody, csvBody)
		default:
			err = fmt.Errorf("unknown channel %q", dest.ChannelType)
		}
		if err != nil && h.log != nil {
			h.log.Error("scheduled_report: send failed", "destination_id", did, "error", err)
		}
	}

	now := time.Now()
	next, nerr := notifications.NextCronRun(rep.CronExpr, h.timezone, now)
	if nerr != nil {
		next = now.Add(24 * time.Hour)
	}
	_ = d.Queries.UpdateScheduledReportRunTimes(ctx, db.UpdateScheduledReportRunTimesParams{
		ID:        rep.ID,
		LastRunAt: pgtype.Timestamp{Time: now, Valid: true},
		NextRunAt: pgtype.Timestamp{Time: next, Valid: true},
	})
	h.insertRun(ctx, d, p.ReportID, "completed", "", sumHex)

	// Self-enqueue the next run at the computed time (event-driven chain).
	if err := EnqueueScheduledReportAt(h.qc, p.ReportID, p.Host, next); err != nil && h.log != nil {
		h.log.Error("scheduled_report: failed to enqueue next run", "report_id", p.ReportID, "next", next, "error", err)
	}
	return nil
}

func (h *ScheduledReportRunHandler) insertRun(ctx context.Context, d *database.DB, reportID, status, errMsg, hash string) {
	var em *string
	if errMsg != "" {
		em = &errMsg
	}
	var sh *string
	if hash != "" {
		sh = &hash
	}
	_, err := d.Queries.InsertScheduledReportRun(ctx, db.InsertScheduledReportRunParams{
		ID:                uuid.New().String(),
		ScheduledReportID: reportID,
		Status:            status,
		ErrorMessage:      em,
		SummaryHash:       sh,
	})
	if err != nil && h.log != nil {
		h.log.Debug("scheduled_report: insert run failed", "error", err)
	}
}

func decryptNotifConfig(enc *util.Encryption, s string) (string, error) {
	if s == "" {
		return "{}", nil
	}
	if enc != nil && util.IsEncrypted(s) {
		return enc.Decrypt(s)
	}
	return s, nil
}

type scheduledWebhookConfig struct {
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers"`
	SigningSecret string            `json:"signing_secret"`
}

func sendScheduledWebhook(ctx context.Context, plain, subject, html, csv string) error {
	var cfg scheduledWebhookConfig
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
		b, err = discordScheduledReportWebhookBody(subject, html, csv)
	case isSlackIncomingWebhookURL(cfg.URL):
		b, err = slackScheduledReportWebhookBody(subject, html, csv)
	default:
		body := map[string]interface{}{
			"kind":    "scheduled_report",
			"subject": subject,
			"html":    html,
			"csv":     csv,
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
	client := &http.Client{Timeout: 45 * time.Second}
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

type scheduledEmailConfig struct {
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	UseTLS   bool   `json:"use_tls"`
}

func sendScheduledEmail(plain, subject, html, csv string) error {
	var cfg scheduledEmailConfig
	if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
		return err
	}
	if cfg.SMTPHost == "" || cfg.From == "" || cfg.To == "" {
		return fmt.Errorf("email smtp_host, from, to required")
	}
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s",
		cfg.From, cfg.To, subject, html))
	addr := cfg.SMTPHost + ":" + strconv.Itoa(cfg.SMTPPort)
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)
	}
	tlsCfg := &tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}

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
		if ok, _ := client.Extension("STARTTLS"); ok {
			if tlsErr := client.StartTLS(tlsCfg); tlsErr != nil {
				_ = client.Close()
				return nil, nil, tlsErr
			}
			return client, plainConn, nil
		}
		if cfg.UseTLS {
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

func sendScheduledNtfy(ctx context.Context, plain, subject, html, csv string) error {
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

	title := strings.TrimSpace(subject)
	if title == "" {
		title = "PatchMon scheduled report"
	}

	// Build a plain-text excerpt from the HTML for ntfy
	message := stripScheduledReportHTML(html)
	if message == "" {
		message = "Scheduled report delivered"
	}
	message = truncateUTF8(message, 4000)

	body := map[string]interface{}{
		"topic":    cfg.Topic,
		"title":    truncateUTF8(title, 256),
		"message":  message,
		"priority": 3,
		"tags":     []string{"bar_chart", "clipboard"},
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
