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
	if e == nil || e.rdb == nil {
		return true
	}
	minGap := every - checkDueSlack
	if minGap < 0 {
		minGap = 0
	}
	key := tenantRedisKey(tenantHost, checkRunKey(name))
	n, err := claimCheckScript.Run(ctx, e.rdb, []string{key}, now.UnixMilli(), minGap.Milliseconds(), (every + checkKeyGrace).Milliseconds()).Int()
	if err != nil {
		if e.log != nil {
			e.log.Debug("alerts: check due lookup failed", "check", sanitizeLogValue(name), "error", sanitizeLogValue(err.Error()))
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
local current = {}
for i = 3, #ARGV do current[ARGV[i]] = true end
local cleared = {}
for _, id in ipairs(redis.call('HKEYS', KEYS[1])) do
	if not current[id] then
		redis.call('HDEL', KEYS[1], id)
		table.insert(cleared, id)
	end
end
local added = {}
for i = 3, #ARGV do
	if redis.call('HSETNX', KEYS[1], ARGV[i], ARGV[1]) == 1 then table.insert(added, ARGV[i]) end
end
if #ARGV > 2 then redis.call('PEXPIRE', KEYS[1], ARGV[2]) end
return {added, cleared}
`)

// SyncConditions records the references currently in the condition for eventType.
// It returns the references that newly entered it and those that left it since the last sync.
func (e *Emitter) SyncConditions(ctx context.Context, tenantHost, eventType string, active []string, now time.Time) (entered, left []string, err error) {
	if e == nil || e.rdb == nil {
		return active, nil, nil
	}
	key := tenantRedisKey(tenantHost, conditionKey(eventType))
	args := make([]interface{}, 0, len(active)+2)
	args = append(args, strconv.FormatInt(now.UnixMilli(), 10), conditionStoreTTL.Milliseconds())
	for _, id := range active {
		args = append(args, id)
	}
	res, err := syncConditionScript.Run(ctx, e.rdb, []string{key}, args...).Slice()
	if err != nil {
		return nil, nil, err
	}
	return toStrings(res, 0), toStrings(res, 1), nil
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
