package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/redis/go-redis/v9"
)

const (
	bootstrapPrefix = "bootstrap:"
	bootstrapTTL    = 300 * time.Second // 5 minutes
)

// BootstrapStore stores one-time bootstrap tokens in Redis.
type BootstrapStore struct {
	rdb *hostctx.RedisResolver
	enc *util.Encryption
}

// NewBootstrapStore creates a new bootstrap token store.
func NewBootstrapStore(rdb *hostctx.RedisResolver, enc *util.Encryption) *BootstrapStore {
	return &BootstrapStore{rdb: rdb, enc: enc}
}

// GenerateToken creates a bootstrap token and stores encrypted credentials in Redis.
// Returns the token (hex string) for injection into the install script.
func (s *BootstrapStore) GenerateToken(ctx context.Context, apiID, apiKey string) (string, error) {
	if s.enc == nil {
		return "", fmt.Errorf("bootstrap: encryption not configured")
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)
	key := bootstrapPrefix + token

	payload := map[string]interface{}{
		"apiId":     apiID,
		"apiKey":    apiKey,
		"createdAt": 0,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encrypted, err := s.enc.Encrypt(string(b))
	if err != nil {
		return "", err
	}
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", fmt.Errorf("bootstrap: redis not available")
	}
	if err := rdb.Set(ctx, key, encrypted, bootstrapTTL).Err(); err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeToken retrieves and deletes the bootstrap token (one-time use).
// Returns apiID, apiKey and true if valid; empty strings and false otherwise.
func (s *BootstrapStore) ConsumeToken(ctx context.Context, token string) (apiID, apiKey string, ok bool) {
	if token == "" {
		return "", "", false
	}
	rdb := s.rdb.RDB(ctx)
	if rdb == nil {
		return "", "", false
	}
	key := bootstrapPrefix + token
	encrypted, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", "", false
	}
	if err != nil {
		return "", "", false
	}
	_ = rdb.Del(ctx, key) // delete immediately (one-time use)

	decrypted, err := s.enc.Decrypt(encrypted)
	if err != nil {
		return "", "", false
	}
	var payload struct {
		ApiID  string `json:"apiId"`
		ApiKey string `json:"apiKey"`
	}
	if err := json.Unmarshal([]byte(decrypted), &payload); err != nil {
		return "", "", false
	}
	if payload.ApiID == "" || payload.ApiKey == "" {
		return "", "", false
	}
	return payload.ApiID, payload.ApiKey, true
}
