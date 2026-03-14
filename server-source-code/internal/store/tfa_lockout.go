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
	maxAttempts     int
	lockoutDuration time.Duration
}

// NewTfaLockoutStore creates a TFA lockout store.
func NewTfaLockoutStore(rdb *hostctx.RedisResolver, maxAttempts int, lockoutDurationMinutes int) *TfaLockoutStore {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if lockoutDurationMinutes <= 0 {
		lockoutDurationMinutes = 30
	}
	return &TfaLockoutStore{
		rdb:             rdb,
		maxAttempts:     maxAttempts,
		lockoutDuration: time.Duration(lockoutDurationMinutes) * time.Minute,
	}
}

// IsTFALocked returns whether the user is locked and remaining seconds.
func (s *TfaLockoutStore) IsTFALocked(ctx context.Context, userID string) (locked bool, remainingSec int) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return false, 0
	}
	key := TfaLockoutPrefix + userID
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
	key := TfaFailedPrefix + userID
	attempts64, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, false
	}
	attempts = int(attempts64)
	if attempts == 1 {
		_ = rdb.Expire(ctx, key, s.lockoutDuration)
	}
	if attempts >= s.maxAttempts {
		lockKey := TfaLockoutPrefix + userID
		_ = rdb.Set(ctx, lockKey, strconv.FormatInt(time.Now().UnixMilli(), 10), s.lockoutDuration).Err()
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
	key := TfaFailedPrefix + userID
	_ = rdb.Del(ctx, key).Err()
}
