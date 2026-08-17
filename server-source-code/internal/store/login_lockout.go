package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
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
	cfgResolver     *hostctx.ConfigResolver
	maxAttempts     int
	lockoutDuration time.Duration
}

// NewLoginLockoutStore creates a login lockout store. cfgResolver supplies the
// calling context's thresholds; the passed values are the fallback.
func NewLoginLockoutStore(rdb *hostctx.RedisResolver, cfgResolver *hostctx.ConfigResolver, maxAttempts int, lockoutDurationMinutes int) *LoginLockoutStore {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if lockoutDurationMinutes <= 0 {
		lockoutDurationMinutes = 15
	}
	return &LoginLockoutStore{
		rdb:             rdb,
		cfgResolver:     cfgResolver,
		maxAttempts:     maxAttempts,
		lockoutDuration: time.Duration(lockoutDurationMinutes) * time.Minute,
	}
}

func (s *LoginLockoutStore) limits(ctx context.Context) (int, time.Duration) {
	if rc := s.cfgResolver.Resolve(ctx); rc != nil {
		dur := time.Duration(rc.LockoutDurationMin) * time.Minute
		if rc.MaxLoginAttempts > 0 && dur > 0 {
			return rc.MaxLoginAttempts, dur
		}
	}
	return s.maxAttempts, s.lockoutDuration
}

// Identifier returns the lockout key identifier (IP + hashed username).
//
// The username is hashed rather than embedded, because a failed attempt for a
// username that does not exist still creates a Redis key: interpolating the raw
// value would let an unauthenticated caller store a key as large as the request
// body limit allows. It is lowercased and trimmed first because the user lookup
// matches with LOWER(), so admin/Admin/ADMIN are one account and must share one
// counter rather than getting a fresh allowance each.
func (s *LoginLockoutStore) Identifier(clientIP, username string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username))))
	return clientIP + "|" + hex.EncodeToString(sum[:16])
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
	maxAttempts, lockoutDuration := s.limits(ctx)
	attempts = int(attempts64)
	if attempts == 1 {
		_ = rdb.Expire(ctx, key, lockoutDuration)
	}
	if attempts >= maxAttempts {
		lockKey := hostctx.TenantKey(ctx, LoginLockoutPrefix+identifier)
		_ = rdb.Set(ctx, lockKey, strconv.FormatInt(time.Now().UnixMilli(), 10), lockoutDuration).Err()
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
