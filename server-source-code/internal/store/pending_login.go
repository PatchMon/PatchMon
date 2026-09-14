package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/redis/go-redis/v9"
)

// ErrInvalidPendingLogin is returned when a TFA ticket is missing, expired, or
// has already been consumed.
var ErrInvalidPendingLogin = errors.New("invalid or expired login ticket")

const (
	pendingLoginPrefix = "auth:pending_tfa:"
	pendingLoginTTL    = 5 * time.Minute
)

// PendingLoginStore holds short-lived, single-use tickets proving the first
// authentication factor has been satisfied.
type PendingLoginStore struct {
	rdb *hostctx.RedisResolver
}

// NewPendingLoginStore creates a new pending-login ticket store.
func NewPendingLoginStore(rdb *hostctx.RedisResolver) *PendingLoginStore {
	return &PendingLoginStore{rdb: rdb}
}

// The user is pinned at issue time so the second factor cannot be verified
// against a different account.
type pendingLoginData struct {
	UserID    string `json:"userId"`
	CreatedAt int64  `json:"createdAt"`
}

func (s *PendingLoginStore) Create(ctx context.Context, userID string) (string, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", errors.New("pending login: redis not available")
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(b)

	raw, err := json.Marshal(pendingLoginData{
		UserID:    userID,
		CreatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return "", err
	}

	key := hostctx.TenantKey(ctx, pendingLoginPrefix+ticket)
	if err := rdb.Set(ctx, key, raw, pendingLoginTTL).Err(); err != nil {
		return "", err
	}
	return ticket, nil
}

// GETDEL, so one ticket drives exactly one verification attempt whatever the
// outcome; a captured ticket cannot be replayed against TOTP codes.
func (s *PendingLoginStore) Consume(ctx context.Context, ticket string) (string, error) {
	if ticket == "" {
		return "", ErrInvalidPendingLogin
	}
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", ErrInvalidPendingLogin
	}

	key := hostctx.TenantKey(ctx, pendingLoginPrefix+ticket)
	raw, err := rdb.GetDel(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrInvalidPendingLogin
		}
		return "", err
	}

	var data pendingLoginData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", ErrInvalidPendingLogin
	}
	if data.UserID == "" {
		return "", ErrInvalidPendingLogin
	}
	return data.UserID, nil
}
