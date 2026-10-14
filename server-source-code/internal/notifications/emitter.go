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
	"github.com/jackc/pgx/v5"
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
	ObservedAt    time.Time
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

// EmitEventChecked is EmitEvent, returning false when a matching route was dropped by
// the rate limit or a lookup or enqueue failure, so the caller can try again later.
func (e *Emitter) EmitEventChecked(ctx context.Context, d *database.DB, tenantHost string, ev Event) bool {
	if e == nil || e.qc == nil || d == nil {
		return true
	}
	return e.emit(ctx, d, tenantHost, ev)
}

func (e *Emitter) emit(ctx context.Context, d *database.DB, tenantHost string, ev Event) bool {
	observedAt := ev.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if target, ok := cancelPairs[ev.Type]; ok {
		if refID := eventRefID(ev); refID != "" {
			e.CancelPending(ctx, tenantHost, target, map[string]time.Time{refID: observedAt})
		}
	}

	routes, err := d.Queries.ListNotificationRoutesForEvent(ctx, ev.Type)
	if err != nil {
		if e.log != nil {
			e.log.Debug("notifications: list routes failed", "error", err)
		}
		return false
	}
	if len(routes) == 0 {
		return true
	}

	// Check alert_config: if the event type is explicitly disabled, skip all notifications.
	var alertDelay time.Duration
	cfg, err := d.Queries.GetAlertConfigByType(ctx, ev.Type)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && e.log != nil {
		e.log.Debug("notifications: alert config lookup failed, sending without delay", "event_type", sanitizeLogValue(ev.Type), "error", sanitizeLogValue(err.Error()))
	}
	if err == nil {
		if !cfg.IsEnabled {
			if e.log != nil {
				e.log.Debug("notifications: event type disabled in alert lifecycle", "event_type", ev.Type)
			}
			return true
		}
		if cfg.AlertDelaySeconds != nil && *cfg.AlertDelaySeconds > 0 {
			alertDelay = time.Duration(*cfg.AlertDelaySeconds) * time.Second
		}
	}
	// If no config row exists for this event type, allow it through (backwards compat).

	// Inject app_link into metadata so formatters can render clickable links.
	ev.Metadata = e.injectAppLink(ctx, d, ev)

	var cancelKey, recoveryGen string
	var emittedAt int64
	if alertDelay > 0 {
		cancelKey = delayedCancelKey(ev)
		emittedAt = observedAt.UnixNano()
		if cancelKey != "" {
			recoveryGen = e.recoveryGeneration(ctx, tenantHost, cancelKey)
			if !e.claimPending(ctx, tenantHost, ev.Type, eventRefID(ev), emittedAt, alertDelay+pendingGrace) {
				if e.log != nil {
					e.log.Debug("notifications: delayed event skipped, recovered or already pending", "event_type", sanitizeLogValue(ev.Type), "ref", sanitizeLogValue(eventRefID(ev)))
				}
				return true
			}
		}
	}

	delivered := true
	evSev := SeverityRank(ev.Severity)
	for _, row := range routes {
		if !row.DestinationEnabled {
			continue
		}
		if SeverityRank(row.MinSeverity) > evSev {
			continue
		}
		// Host group filtering: if route specifies host_group_ids, the event's host must be in at least one.
		if hgIDs := parseJSONStringArray(row.HostGroupIds); len(hgIDs) > 0 {
			hostID := metadataString(ev.Metadata, "host_id")
			if hostID == "" {
				continue
			}
			inAny := false
			for _, gid := range hgIDs {
				ok, err := d.Queries.HostInHostGroup(ctx, db.HostInHostGroupParams{
					HostID:      hostID,
					HostGroupID: gid,
				})
				if err == nil && ok {
					inAny = true
					break
				}
			}
			if !inAny {
				continue
			}
		}
		// Individual host filtering: if route specifies host_ids, the event's host must be in the list.
		if hIDs := parseJSONStringArray(row.HostIds); len(hIDs) > 0 {
			hostID := metadataString(ev.Metadata, "host_id")
			if hostID == "" {
				continue
			}
			found := false
			for _, hid := range hIDs {
				if hid == hostID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if !matchRulesOK(row.MatchRules, ev.Metadata, ev.Type) {
			continue
		}
		fp := fingerprint(ev, row.DestinationID, recoveryGen)
		if e.rdb != nil && !e.tryDedup(tenantHost, fp) {
			continue
		}
		if e.rdb != nil && !e.allowRate(tenantHost, row.DestinationID) {
			if e.log != nil {
				e.log.Warn("notifications: rate limited", "destination_id", row.DestinationID)
			}
			delivered = false
			e.releaseDedup(tenantHost, fp)
			continue
		}
		payload := NotificationDeliverPayload{
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
		}
		if alertDelay > 0 {
			payload.Delayed = true
			payload.CancelKey = cancelKey
			payload.EmittedAt = emittedAt
		}
		task, err := NewNotificationDeliverTask(payload)
		if err != nil {
			if e.log != nil {
				e.log.Error("notifications: build task failed", "error", err)
			}
			delivered = false
			e.releaseDedup(tenantHost, fp)
			continue
		}
		enqueueOpts := []asynq.Option{asynq.Queue(QueueNotifications), asynq.MaxRetry(5)}
		if alertDelay > 0 {
			enqueueOpts = append(enqueueOpts, asynq.ProcessIn(alertDelay))
		}
		if _, err := e.qc.Enqueue(task, enqueueOpts...); err != nil {
			if e.log != nil {
				e.log.Error("notifications: enqueue failed", "error", err)
			}
			delivered = false
			e.releaseDedup(tenantHost, fp)
		}
	}
	return delivered
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
	fp := fingerprint(ev, destinationID, "")
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
	case "user":
		link = baseURL + "/settings/users"
	case "test":
		link = baseURL + "/reporting"
	default:
		link = baseURL + "/"
	}
	if link != "" {
		meta["app_link"] = link
	}
	return meta
}

func parseJSONStringArray(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil
	}
	return arr
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

func fingerprint(ev Event, destinationID, recoveryGen string) string {
	bucket := time.Now().Unix() / 120
	raw := fmt.Sprintf("%s|%s|%s|%d", ev.Type, ev.ReferenceID, destinationID, bucket)
	if recoveryGen != "" {
		raw += "|" + recoveryGen
	}
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

func (e *Emitter) releaseDedup(tenantHost, fp string) {
	if e.rdb == nil {
		return
	}
	_ = e.rdb.Del(context.Background(), tenantRedisKey(tenantHost, "notif:dedup:"+fp)).Err()
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

// cancelPairs maps an event type to the delayed event type it should cancel.
// When host_recovered fires, any pending delayed host_down notification for the same reference should be suppressed.
var cancelPairs = map[string]string{
	"host_recovered":                 "host_down",
	"container_started":              "container_stopped",
	"host_security_updates_resolved": "host_security_updates_exceeded",
	"host_pending_updates_resolved":  "host_pending_updates_exceeded",
}

const pendingGrace = 24 * time.Hour

func eventRefID(ev Event) string {
	if ev.ReferenceID != "" {
		return ev.ReferenceID
	}
	return metadataString(ev.Metadata, "host_id")
}

func cancelKeyFor(eventType, refID string) string {
	return fmt.Sprintf("notif:cancel:%s:%s", eventType, refID)
}

func pendingKeyFor(eventType, refID string) string {
	return fmt.Sprintf("notif:pending:%s:%s", eventType, refID)
}

// delayedCancelKey returns the Redis key whose value, a recovery timestamp, suppresses this delayed notification.
func delayedCancelKey(ev Event) string {
	refID := eventRefID(ev)
	if refID == "" {
		return ""
	}
	return cancelKeyFor(ev.Type, refID)
}

const luaNewer = `
local function newer(a, b)
	if #a ~= #b then return #a > #b end
	return a > b
end
`

var claimPendingScript = redis.NewScript(luaNewer + `
local rec = redis.call('GET', KEYS[2])
if rec and newer(rec, ARGV[1]) then return 0 end
if ARGV[3] == '1' then
	local p = redis.call('GET', KEYS[1])
	if p and not (rec and newer(rec, p)) then
		if redis.call('PTTL', KEYS[1]) < tonumber(ARGV[2]) then redis.call('PEXPIRE', KEYS[1], ARGV[2]) end
		return 0
	end
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return 1
`)

var recordRecoveryScript = redis.NewScript(luaNewer + `
local pending = redis.call('GET', KEYS[1])
if not pending and ARGV[3] ~= '1' then return 0 end
if pending and not newer(ARGV[1], pending) then return 0 end
local cur = redis.call('GET', KEYS[2])
if cur and not newer(ARGV[1], cur) then return 0 end
local ttl = tonumber(ARGV[4])
if pending then ttl = redis.call('PTTL', KEYS[1]) + tonumber(ARGV[2]) end
local cttl = redis.call('PTTL', KEYS[2])
if cttl > ttl then ttl = cttl end
redis.call('SET', KEYS[2], ARGV[1], 'PX', ttl)
if pending then redis.call('DEL', KEYS[1]) end
return 1
`)

const recoveryRaceTTL = 10 * time.Minute

// onePerOutage lists delayed event types whose recovery is recorded without an alert row, so a pending marker always clears.
var onePerOutage = map[string]bool{"host_down": true}

// claimPending marks a delayed notification pending; false means the ref recovered after emittedAt or this outage is already pending.
func (e *Emitter) claimPending(ctx context.Context, tenantHost, eventType, refID string, emittedAt int64, ttl time.Duration) bool {
	if e == nil || e.rdb == nil || refID == "" {
		return true
	}
	keys := []string{
		tenantRedisKey(tenantHost, pendingKeyFor(eventType, refID)),
		tenantRedisKey(tenantHost, cancelKeyFor(eventType, refID)),
	}
	single := "0"
	if onePerOutage[eventType] {
		single = "1"
	}
	n, err := claimPendingScript.Run(ctx, e.rdb, keys, strconv.FormatInt(emittedAt, 10), ttl.Milliseconds(), single).Int()
	if err != nil {
		if e.log != nil {
			e.log.Debug("notifications: claim pending failed", "event_type", sanitizeLogValue(eventType), "error", sanitizeLogValue(err.Error()))
		}
		return true
	}
	return n == 1
}

func (e *Emitter) recoveryGeneration(ctx context.Context, tenantHost, cancelKey string) string {
	if e == nil || e.rdb == nil {
		return ""
	}
	val, err := e.rdb.Get(ctx, tenantRedisKey(tenantHost, cancelKey)).Result()
	if err != nil {
		return ""
	}
	return val
}

// CancelPending records a recovery time for each ref that has a pending delayed notification.
func (e *Emitter) CancelPending(ctx context.Context, tenantHost, eventType string, recoveredAt map[string]time.Time) {
	if e == nil || e.rdb == nil || len(recoveredAt) == 0 {
		return
	}
	refIDs := make([]string, 0, len(recoveredAt))
	for id := range recoveredAt {
		refIDs = append(refIDs, id)
	}
	pipe := e.rdb.Pipeline()
	exists := make([]*redis.IntCmd, len(refIDs))
	for i, id := range refIDs {
		exists[i] = pipe.Exists(ctx, tenantRedisKey(tenantHost, pendingKeyFor(eventType, id)))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		if e.log != nil {
			e.log.Debug("notifications: pending lookup failed", "event_type", sanitizeLogValue(eventType), "error", sanitizeLogValue(err.Error()))
		}
		return
	}
	for i, id := range refIDs {
		if exists[i].Val() > 0 {
			e.recordRecovery(ctx, tenantHost, eventType, id, recoveredAt[id], false)
		}
	}
}

// RecordRecovery records a recovery time even when nothing is pending yet, so a delayed
// notification for a down decision taken before at, but not yet enqueued, is never sent.
func (e *Emitter) RecordRecovery(ctx context.Context, tenantHost, eventType, refID string, at time.Time) {
	if e == nil || e.rdb == nil || refID == "" {
		return
	}
	e.recordRecovery(ctx, tenantHost, eventType, refID, at, true)
}

func (e *Emitter) recordRecovery(ctx context.Context, tenantHost, eventType, refID string, at time.Time, force bool) {
	if at.IsZero() {
		return
	}
	keys := []string{
		tenantRedisKey(tenantHost, pendingKeyFor(eventType, refID)),
		tenantRedisKey(tenantHost, cancelKeyFor(eventType, refID)),
	}
	forceArg := "0"
	if force {
		forceArg = "1"
	}
	err := recordRecoveryScript.Run(ctx, e.rdb, keys, strconv.FormatInt(at.UnixNano(), 10), pendingGrace.Milliseconds(), forceArg, recoveryRaceTTL.Milliseconds()).Err()
	if err != nil && !errors.Is(err, redis.Nil) && e.log != nil {
		e.log.Debug("notifications: record recovery failed", "event_type", sanitizeLogValue(eventType), "error", sanitizeLogValue(err.Error()))
	}
}

// IsDelayedCancelled reports whether the resource recovered after this delayed notification
// was emitted. A zero emittedAt comes from a payload queued before recovery timestamps existed.
func IsDelayedCancelled(rdb *redis.Client, tenantHost, cancelKey string, emittedAt int64) bool {
	if rdb == nil || cancelKey == "" {
		return false
	}
	val, err := rdb.Get(context.Background(), tenantRedisKey(tenantHost, cancelKey)).Result()
	if err != nil || val == "" {
		return false
	}
	if emittedAt == 0 {
		return true
	}
	recoveredAt, err := strconv.ParseInt(val, 10, 64)
	return err == nil && recoveredAt > emittedAt
}

func sanitizeLogValue(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(s)
}
