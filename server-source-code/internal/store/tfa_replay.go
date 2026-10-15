package store

import (
	"context"
	"strconv"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

// TfaReplayTTL outlives the ±2 step window in which a TOTP code stays valid.
const TfaReplayTTL = 3 * time.Minute

// TfaReplayStore records TOTP steps already used, so a code works once.
type TfaReplayStore struct {
	rdb *hostctx.RedisResolver
}

// NewTfaReplayStore creates a TOTP replay store.
func NewTfaReplayStore(rdb *hostctx.RedisResolver) *TfaReplayStore {
	return &TfaReplayStore{rdb: rdb}
}

// MarkUsed claims the step for the user. It returns false when the step was already used.
// Without Redis it returns true: every route that verifies TOTP already fails closed then.
func (s *TfaReplayStore) MarkUsed(ctx context.Context, userID string, step int64) (bool, error) {
	if s == nil || s.rdb == nil {
		return true, nil
	}
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return true, nil
	}
	key := hostctx.TenantKey(ctx, "tfa:used:"+userID+":"+strconv.FormatInt(step, 10))
	return rdb.SetNX(ctx, key, "1", TfaReplayTTL).Result()
}
