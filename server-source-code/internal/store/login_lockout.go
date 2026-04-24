package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

const (
	LoginLockoutPrefix = "login:lockout:"
	LoginFailedPrefix  = "login:failed:"
)

// LoginLockoutStore manages login attempt tracking and lockout in Redis.
// Uses identifier = IP + username to avoid cross-user lockout while still
// limiting by IP for distributed attacks.
type LoginLockoutStore struct {
	rdb             *hostctx.RedisResolver
	maxAttempts     int
	lockoutDuration time.Duration
}

// NewLoginLockoutStore creates a login lockout store.
func NewLoginLockoutStore(rdb *hostctx.RedisResolver, maxAttempts int, lockoutDurationMinutes int) *LoginLockoutStore {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if lockoutDurationMinutes <= 0 {
		lockoutDurationMinutes = 15
	}
	return &LoginLockoutStore{
		rdb:             rdb,
		maxAttempts:     maxAttempts,
		lockoutDuration: time.Duration(lockoutDurationMinutes) * time.Minute,
	}
}

// Identifier returns the lockout key identifier (IP + username).
func (s *LoginLockoutStore) Identifier(clientIP, username string) string {
	return fmt.Sprintf("%s|%s", clientIP, username)
}

// IsLocked returns whether the identifier is locked and remaining seconds.
func (s *LoginLockoutStore) IsLocked(ctx context.Context, identifier string) (locked bool, remainingSec int) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return false, 0
	}
	key := hostctx.TenantKey(ctx, LoginLockoutPrefix+identifier)
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		return false, 0
	}
	return true, int(ttl.Seconds())
}

// RecordFailedAttempt increments failed attempts. Returns attempts count and whether locked.
func (s *LoginLockoutStore) RecordFailedAttempt(ctx context.Context, identifier string) (attempts int, locked bool) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return 0, false
	}
	key := hostctx.TenantKey(ctx, LoginFailedPrefix+identifier)
	attempts64, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, false
	}
	attempts = int(attempts64)
	if attempts == 1 {
		_ = rdb.Expire(ctx, key, s.lockoutDuration)
	}
	if attempts >= s.maxAttempts {
		lockKey := hostctx.TenantKey(ctx, LoginLockoutPrefix+identifier)
		_ = rdb.Set(ctx, lockKey, strconv.FormatInt(time.Now().UnixMilli(), 10), s.lockoutDuration).Err()
		_ = rdb.Del(ctx, key).Err()
		return attempts, true
	}
	return attempts, false
}

// ClearFailedAttempts removes failed attempt counter (call on success).
func (s *LoginLockoutStore) ClearFailedAttempts(ctx context.Context, identifier string) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return
	}
	key := hostctx.TenantKey(ctx, LoginFailedPrefix+identifier)
	_ = rdb.Del(ctx, key).Err()
}
