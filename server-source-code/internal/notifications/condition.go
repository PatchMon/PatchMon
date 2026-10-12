package notifications

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	checkDueSlack     = 30 * time.Second
	checkKeyGrace     = 7 * 24 * time.Hour
	conditionStoreTTL = 30 * 24 * time.Hour
)

func checkRunKey(name string) string {
	return "alerts:lastrun:" + name
}

func conditionKey(eventType string) string {
	return "alerts:notified:" + eventType
}

var claimCheckScript = redis.NewScript(`
local last = redis.call('GET', KEYS[1])
if last and tonumber(ARGV[1]) - tonumber(last) < tonumber(ARGV[2]) then return 0 end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
return 1
`)

// ClaimCheckDue reports whether the named periodic check is due for this context and, if so, records the run.
func (e *Emitter) ClaimCheckDue(ctx context.Context, tenantHost, name string, every time.Duration, now time.Time) bool {
	if e == nil {
		return true
	}
	if e.rdb == nil {
		return false
	}
	minGap := every - checkDueSlack
	if minGap < 0 {
		minGap = 0
	}
	key := tenantRedisKey(tenantHost, checkRunKey(name))
	n, err := claimCheckScript.Run(ctx, e.rdb, []string{key}, now.UnixMilli(), minGap.Milliseconds(), (every + checkKeyGrace).Milliseconds()).Int()
	if err != nil {
		if e.log != nil {
			e.log.Warn("alerts: check due lookup failed", "check", sanitizeLogValue(name), "error", sanitizeLogValue(err.Error()))
		}
		return false
	}
	return n == 1
}

// ReleaseCheck forgets the last run of a periodic check so the next tick runs it again.
func (e *Emitter) ReleaseCheck(ctx context.Context, tenantHost, name string) {
	if e == nil || e.rdb == nil {
		return
	}
	_ = e.rdb.Del(ctx, tenantRedisKey(tenantHost, checkRunKey(name))).Err()
}

var syncConditionScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local remind = tonumber(ARGV[3])
local first = 5 + tonumber(ARGV[4])
local held = {}
for i = 5, first - 1 do held[ARGV[i]] = true end
local current = {}
for i = first, #ARGV do current[ARGV[i]] = true end
local cleared = {}
for _, id in ipairs(redis.call('HKEYS', KEYS[1])) do
	if not current[id] then
		redis.call('HDEL', KEYS[1], id)
		table.insert(cleared, id)
	end
end
local added = {}
local reminded = {}
for i = first, #ARGV do
	local id = ARGV[i]
	if redis.call('HSETNX', KEYS[1], id, ARGV[1]) == 1 then
		table.insert(added, id)
	elseif remind > 0 and not held[id] then
		local last = tonumber(redis.call('HGET', KEYS[1], id))
		if last and now - last >= remind then
			redis.call('HSET', KEYS[1], id, ARGV[1])
			table.insert(reminded, id)
		end
	end
end
if #ARGV >= first then redis.call('PEXPIRE', KEYS[1], ARGV[2]) end
return {added, cleared, reminded}
`)

type ConditionSync struct {
	Entered  []string
	Left     []string
	Reminded []string
}

// SyncConditions records who is in the condition and reports entries, exits, and reminders due after remindEvery.
// Refs in hold are never reported as reminders and keep their last-notified time.
func (e *Emitter) SyncConditions(ctx context.Context, tenantHost, eventType string, active []string, remindEvery time.Duration, hold []string, now time.Time) (ConditionSync, error) {
	if e == nil || e.rdb == nil {
		return ConditionSync{Entered: active}, nil
	}
	remindMs := int64(0)
	if remindEvery > 0 {
		remindMs = max((remindEvery - checkDueSlack).Milliseconds(), 1)
	}
	key := tenantRedisKey(tenantHost, conditionKey(eventType))
	args := make([]interface{}, 0, len(active)+len(hold)+4)
	args = append(args, strconv.FormatInt(now.UnixMilli(), 10), conditionStoreTTL.Milliseconds(), remindMs, len(hold))
	for _, id := range hold {
		args = append(args, id)
	}
	for _, id := range active {
		args = append(args, id)
	}
	res, err := syncConditionScript.Run(ctx, e.rdb, []string{key}, args...).Slice()
	if err != nil {
		return ConditionSync{}, err
	}
	return ConditionSync{Entered: toStrings(res, 0), Left: toStrings(res, 1), Reminded: toStrings(res, 2)}, nil
}

// ForgetConditions removes refs from the condition for eventType, so they count as newly entered next time.
func (e *Emitter) ForgetConditions(ctx context.Context, tenantHost, eventType string, refIDs []string) {
	if e == nil || e.rdb == nil || len(refIDs) == 0 {
		return
	}
	_ = e.rdb.HDel(ctx, tenantRedisKey(tenantHost, conditionKey(eventType)), refIDs...).Err()
}

// RewindConditions marks refs as never notified, so they are reported as reminders due on the next sync.
func (e *Emitter) RewindConditions(ctx context.Context, tenantHost, eventType string, refIDs []string) {
	if e == nil || e.rdb == nil || len(refIDs) == 0 {
		return
	}
	key := tenantRedisKey(tenantHost, conditionKey(eventType))
	pipe := e.rdb.Pipeline()
	for _, id := range refIDs {
		pipe.HSet(ctx, key, id, "0")
	}
	_, _ = pipe.Exec(ctx)
}

// ClearConditions forgets every reference recorded for eventType, so the next occurrence notifies again.
func (e *Emitter) ClearConditions(ctx context.Context, tenantHost, eventType string) {
	if e == nil || e.rdb == nil {
		return
	}
	_ = e.rdb.Del(ctx, tenantRedisKey(tenantHost, conditionKey(eventType))).Err()
}

func toStrings(res []interface{}, i int) []string {
	if i >= len(res) {
		return nil
	}
	items, _ := res[i].([]interface{})
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
