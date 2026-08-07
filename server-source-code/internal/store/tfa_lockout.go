package store

import (
	"context"
	"strconv"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

const (
	TfaLockoutPrefix = "tfa:lockout:"
	TfaFailedPrefix  = "tfa:failed:"
)

// TfaLockoutStore manages TFA attempt tracking and lockout in Redis.
// Uses same keys as Node: tfa:lockout:{userID}, tfa:failed:{userID}.
type TfaLockoutStore struct {
	rdb             *hostctx.RedisResolver
	cfgResolver     *hostctx.ConfigResolver
	maxAttempts     int
	lockoutDuration time.Duration
}

// NewTfaLockoutStore creates a TFA lockout store. cfgResolver supplies the
// calling context's own thresholds; the passed values are the fallback used when
// it is unset or a settings read fails.
func NewTfaLockoutStore(rdb *hostctx.RedisResolver, cfgResolver *hostctx.ConfigResolver, maxAttempts int, lockoutDurationMinutes int) *TfaLockoutStore {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if lockoutDurationMinutes <= 0 {
		lockoutDurationMinutes = 30
	}
	return &TfaLockoutStore{
		rdb:             rdb,
		cfgResolver:     cfgResolver,
		maxAttempts:     maxAttempts,
		lockoutDuration: time.Duration(lockoutDurationMinutes) * time.Minute,
	}
}

// limits returns the thresholds for ctx's context.
func (s *TfaLockoutStore) limits(ctx context.Context) (int, time.Duration) {
	if rc := s.cfgResolver.Resolve(ctx); rc != nil {
		dur := time.Duration(rc.TfaLockoutDurationMin) * time.Minute
		if rc.MaxTfaAttempts > 0 && dur > 0 {
			return rc.MaxTfaAttempts, dur
		}
	}
	return s.maxAttempts, s.lockoutDuration
}

// IsTFALocked returns whether the user is locked and remaining seconds.
func (s *TfaLockoutStore) IsTFALocked(ctx context.Context, userID string) (locked bool, remainingSec int) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return false, 0
	}
	key := hostctx.TenantKey(ctx, TfaLockoutPrefix+userID)
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		return false, 0
	}
	return true, int(ttl.Seconds())
}

// RecordFailedAttempt increments failed attempts. Returns attempts count and whether locked.
func (s *TfaLockoutStore) RecordFailedAttempt(ctx context.Context, userID string) (attempts int, locked bool) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return 0, false
	}
	key := hostctx.TenantKey(ctx, TfaFailedPrefix+userID)
	attempts64, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, false
	}
	maxAttempts, lockoutDuration := s.limits(ctx)
	attempts = int(attempts64)
	if attempts == 1 {
		_ = rdb.Expire(ctx, key, lockoutDuration)
	}
	if attempts >= maxAttempts {
		lockKey := hostctx.TenantKey(ctx, TfaLockoutPrefix+userID)
		_ = rdb.Set(ctx, lockKey, strconv.FormatInt(time.Now().UnixMilli(), 10), lockoutDuration).Err()
		_ = rdb.Del(ctx, key).Err()
		return attempts, true
	}
	return attempts, false
}

// ClearFailedAttempts removes failed attempt counter (call on success).
func (s *TfaLockoutStore) ClearFailedAttempts(ctx context.Context, userID string) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return
	}
	key := hostctx.TenantKey(ctx, TfaFailedPrefix+userID)
	_ = rdb.Del(ctx, key).Err()
}
