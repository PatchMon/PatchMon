package store

import (
	"context"
	"encoding/json"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/redis/go-redis/v9"
)

const (
	oidcSessionPrefix = "oidc:session:"
	oidcIDTokenPrefix = "oidc:id_token:"
	oidcIDTokenTTL    = 7 * 24 * time.Hour
)

// OidcSessionData holds PKCE and state data for the OIDC flow (stored in Redis).
type OidcSessionData struct {
	CodeVerifier string `json:"codeVerifier"`
	Nonce        string `json:"nonce"`
	State        string `json:"state"`
	CreatedAt    int64  `json:"createdAt"`
}

// OidcSessionStore stores OIDC flow state and ID tokens in Redis.
type OidcSessionStore struct {
	rdb *hostctx.RedisResolver
}

// NewOidcSessionStore creates a new OIDC session store.
func NewOidcSessionStore(rdb *hostctx.RedisResolver) *OidcSessionStore {
	return &OidcSessionStore{rdb: rdb}
}

// Store saves OIDC session data for the given state.
func (s *OidcSessionStore) Store(ctx context.Context, state string, data *OidcSessionData, ttl time.Duration) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return redis.Nil
	}
	key := oidcSessionPrefix + state
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, key, b, ttl).Err()
}

// GetAndDelete retrieves and deletes OIDC session data for the given state.
func (s *OidcSessionStore) GetAndDelete(ctx context.Context, state string) (*OidcSessionData, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return nil, redis.Nil
	}
	key := oidcSessionPrefix + state
	b, err := rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	_ = rdb.Del(ctx, key)
	var data OidcSessionData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// StoreIDToken saves the ID token for RP-initiated logout.
func (s *OidcSessionStore) StoreIDToken(ctx context.Context, userID, idToken string) error {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return redis.Nil
	}
	key := oidcIDTokenPrefix + userID
	return rdb.Set(ctx, key, idToken, oidcIDTokenTTL).Err()
}

// GetAndDeleteIDToken retrieves and deletes the stored ID token for the user.
func (s *OidcSessionStore) GetAndDeleteIDToken(ctx context.Context, userID string) (string, error) {
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", redis.Nil
	}
	key := oidcIDTokenPrefix + userID
	token, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}
	_ = rdb.Del(ctx, key)
	return token, nil
}
