package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const dedupTTL = 3 * time.Minute
const ratePerMinute = 60

// Sentinel errors for direct enqueue (e.g. test from API).
var (
	ErrDestinationNotFound   = errors.New("destination not found")
	ErrDestinationDisabled   = errors.New("destination disabled")
	ErrNotificationsDisabled = errors.New("notifications unavailable")
	ErrRateLimited           = errors.New("rate limited")
)

// Emitter enqueues outbound notification deliveries (webhook / email).
type Emitter struct {
	qc  *asynq.Client
	rdb *redis.Client
	log *slog.Logger
}

// NewEmitter returns a notification emitter. Any nil dependency disables emitting.
func NewEmitter(qc *asynq.Client, rdb *redis.Client, log *slog.Logger) *Emitter {
	return &Emitter{qc: qc, rdb: rdb, log: log}
}

// Event is a normalised notification payload.
type Event struct {
	Type          string
	Severity      string
	Title         string
	Message       string
	ReferenceType string
	ReferenceID   string
	Metadata      map[string]interface{}
}

// EmitForAlert dispatches alert notifications via matching routes.
func (e *Emitter) EmitForAlert(ctx context.Context, d *database.DB, tenantHost string, alert *models.Alert, metadata map[string]interface{}, cfg *store.AlertConfigWithUser) {
	if e == nil || e.qc == nil || d == nil || alert == nil {
		return
	}
	meta := metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	ev := Event{
		Type:          alert.Type,
		Severity:      alert.Severity,
		Title:         alert.Title,
		Message:       alert.Message,
		ReferenceType: "alert",
		ReferenceID:   alert.ID,
		Metadata:      meta,
	}
	e.emit(ctx, d, tenantHost, ev)
}

// EmitEvent dispatches a non-alert domain event (patch, compliance, host_recovered, etc.).
func (e *Emitter) EmitEvent(ctx context.Context, d *database.DB, tenantHost string, ev Event) {
	if e == nil || e.qc == nil || d == nil {
		return
	}
	e.emit(ctx, d, tenantHost, ev)
}

func (e *Emitter) emit(ctx context.Context, d *database.DB, tenantHost string, ev Event) {
	routes, err := d.Queries.ListNotificationRoutesForEvent(ctx, ev.Type)
	if err != nil {
		if e.log != nil {
			e.log.Debug("notifications: list routes failed", "error", err)
		}
		return
	}
	if len(routes) == 0 {
		return
	}

	// Inject app_link into metadata so formatters can render clickable links.
	ev.Metadata = e.injectAppLink(ctx, d, ev)

	evSev := SeverityRank(ev.Severity)
	for _, row := range routes {
		if !row.DestinationEnabled {
			continue
		}
		if SeverityRank(row.MinSeverity) > evSev {
			continue
		}
		if row.HostGroupID != nil && *row.HostGroupID != "" {
			hostID := metadataString(ev.Metadata, "host_id")
			if hostID == "" {
				continue
			}
			ok, err := d.Queries.HostInHostGroup(ctx, db.HostInHostGroupParams{
				HostID:      hostID,
				HostGroupID: *row.HostGroupID,
			})
			if err != nil || !ok {
				continue
			}
		}
		if !matchRulesOK(row.MatchRules, ev.Metadata, ev.Type) {
			continue
		}
		fp := fingerprint(ev, row.DestinationID)
		if e.rdb != nil && !e.tryDedup(tenantHost, fp) {
			continue
		}
		if e.rdb != nil && !e.allowRate(tenantHost, row.DestinationID) {
			if e.log != nil {
				e.log.Warn("notifications: rate limited", "destination_id", row.DestinationID)
			}
			continue
		}
		task, err := NewNotificationDeliverTask(NotificationDeliverPayload{
			Host:             tenantHost,
			DestinationID:    row.DestinationID,
			RouteID:          row.ID,
			ChannelType:      row.ChannelType,
			EventType:        ev.Type,
			Severity:         ev.Severity,
			Title:            ev.Title,
			Message:          ev.Message,
			ReferenceType:    ev.ReferenceType,
			ReferenceID:      ev.ReferenceID,
			EventFingerprint: fp,
			Metadata:         ev.Metadata,
		})
		if err != nil {
			if e.log != nil {
				e.log.Error("notifications: build task failed", "error", err)
			}
			continue
		}
		if _, err := e.qc.Enqueue(task, asynq.Queue(QueueNotifications), asynq.MaxRetry(5)); err != nil && e.log != nil {
			e.log.Error("notifications: enqueue failed", "error", err)
		}
	}
}

// EnqueueToDestination sends one delivery to a destination without a notification route (e.g. UI test).
func (e *Emitter) EnqueueToDestination(ctx context.Context, d *database.DB, tenantHost, destinationID string, ev Event) error {
	if e == nil || e.qc == nil || d == nil {
		return ErrNotificationsDisabled
	}
	if destinationID == "" {
		return ErrDestinationNotFound
	}
	dest, err := d.Queries.GetNotificationDestinationByID(ctx, destinationID)
	if err != nil {
		return ErrDestinationNotFound
	}
	if !dest.Enabled {
		return ErrDestinationDisabled
	}
	fp := fingerprint(ev, destinationID)
	if e.rdb != nil && !e.tryDedup(tenantHost, fp) {
		return nil
	}
	if e.rdb != nil && !e.allowRate(tenantHost, destinationID) {
		return ErrRateLimited
	}
	task, err := NewNotificationDeliverTask(NotificationDeliverPayload{
		Host:             tenantHost,
		DestinationID:    destinationID,
		RouteID:          "",
		ChannelType:      dest.ChannelType,
		EventType:        ev.Type,
		Severity:         ev.Severity,
		Title:            ev.Title,
		Message:          ev.Message,
		ReferenceType:    ev.ReferenceType,
		ReferenceID:      ev.ReferenceID,
		EventFingerprint: fp,
		Metadata:         ev.Metadata,
	})
	if err != nil {
		return err
	}
	_, err = e.qc.Enqueue(task, asynq.Queue(QueueNotifications), asynq.MaxRetry(5))
	return err
}

// injectAppLink clones the event metadata and adds an app_link field with a
// clickable URL pointing to the relevant PatchMon page.
func (e *Emitter) injectAppLink(ctx context.Context, d *database.DB, ev Event) map[string]interface{} {
	// Clone metadata so we don't mutate the caller's map.
	meta := make(map[string]interface{}, len(ev.Metadata)+1)
	for k, v := range ev.Metadata {
		meta[k] = v
	}

	baseURL := ""
	if settings, err := d.Queries.GetFirstSettings(ctx); err == nil {
		baseURL = strings.TrimRight(settings.ServerUrl, "/")
	}
	if baseURL == "" {
		return meta
	}

	link := ""
	switch ev.ReferenceType {
	case "patch_run":
		link = baseURL + "/patching/runs/" + ev.ReferenceID
	case "host":
		link = baseURL + "/hosts/" + ev.ReferenceID
	case "alert":
		// Link to the host page if host_id is available, otherwise dashboard.
		if hostID := metadataString(meta, "host_id"); hostID != "" {
			link = baseURL + "/hosts/" + hostID
		} else {
			link = baseURL + "/"
		}
	case "test":
		link = baseURL + "/settings/alert-channels"
	default:
		link = baseURL + "/"
	}
	if link != "" {
		meta["app_link"] = link
	}
	return meta
}

func metadataString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func matchRulesOK(rulesJSON []byte, meta map[string]interface{}, eventType string) bool {
	if len(rulesJSON) == 0 {
		return true
	}
	var rules map[string]interface{}
	if err := json.Unmarshal(rulesJSON, &rules); err != nil || rules == nil {
		return true
	}
	if minF, ok := rules["min_failed_rules"]; ok {
		n := intFromInterface(minF)
		if n > 0 {
			fc := intFromInterface(meta["failed_count"])
			if fc < n {
				return false
			}
		}
	}
	if minSev, ok := rules["min_patch_severity"].(string); ok && minSev != "" {
		// For patch events: optional filter when metadata includes "failed" bool
		if eventType == "patch_run_failed" || eventType == "patch_run_completed" {
			if meta != nil {
				if failed, ok := meta["failed"].(bool); ok && !failed && eventType == "patch_run_failed" {
					return false
				}
			}
		}
	}
	return true
}

func intFromInterface(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

func fingerprint(ev Event, destinationID string) string {
	bucket := time.Now().Unix() / 120
	raw := fmt.Sprintf("%s|%s|%s|%d", ev.Type, ev.ReferenceID, destinationID, bucket)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func tenantRedisKey(tenantHost, suffix string) string {
	if tenantHost != "" {
		return "t:" + tenantHost + ":" + suffix
	}
	return suffix
}

func (e *Emitter) tryDedup(tenantHost, fp string) bool {
	ctx := context.Background()
	key := tenantRedisKey(tenantHost, "notif:dedup:"+fp)
	err := e.rdb.SetArgs(ctx, key, "1", redis.SetArgs{Mode: "NX", TTL: dedupTTL}).Err()
	if err == nil {
		return true
	}
	if errors.Is(err, redis.Nil) {
		return false
	}
	return true
}

func (e *Emitter) allowRate(tenantHost, destinationID string) bool {
	ctx := context.Background()
	min := time.Now().Unix() / 60
	key := tenantRedisKey(tenantHost, fmt.Sprintf("notif:rl:%s:%d", destinationID, min))
	n, err := e.rdb.Incr(ctx, key).Result()
	if err != nil {
		return true
	}
	if n == 1 {
		_ = e.rdb.Expire(ctx, key, 2*time.Minute).Err()
	}
	return n <= ratePerMinute
}
