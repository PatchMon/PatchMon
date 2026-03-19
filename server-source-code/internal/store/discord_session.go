package store

import (
	"context"
	"encoding/json"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/redis/go-redis/v9"
)

const (
	discordSessionPrefix = "discord:session:"
	discordSessionTTL    = 600 * time.Second // 10 minutes
)

// DiscordSessionData holds PKCE and state data for the Discord OAuth flow (stored in Redis).
type DiscordSessionData struct {
	CodeVerifier string `json:"codeVerifier"`
	State        string `json:"state"`
	Mode         string `json:"mode"` // "login" or "link"
	UserID       string `json:"userId,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
}

// DiscordSessionStore stores Discord OAuth flow state in Redis.
type DiscordSessionStore struct {
	rdb *hostctx.RedisResolver
}

// NewDiscordSessionStore creates a new Discord session store.
func NewDiscordSessionStore(rdb *hostctx.RedisResolver) *DiscordSessionStore {
	return &DiscordSessionStore{rdb: rdb}
}

// Store saves Discord session data for the given state.
func (s *DiscordSessionStore) Store(ctx context.Context, state string, data *DiscordSessionData, ttl time.Duration) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return redis.Nil
	}
	if ttl <= 0 {
		ttl = discordSessionTTL
	}
	key := hostctx.TenantKey(ctx, discordSessionPrefix+state)
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, key, b, ttl).Err()
}

// GetAndDelete retrieves and deletes Discord session data for the given state.
func (s *DiscordSessionStore) GetAndDelete(ctx context.Context, state string) (*DiscordSessionData, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil, redis.Nil
	}
	key := hostctx.TenantKey(ctx, discordSessionPrefix+state)
	b, err := rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	_ = rdb.Del(ctx, key)
	var data DiscordSessionData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
