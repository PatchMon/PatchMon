package context

import (
	stdctx "context"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides Redis client access for multi-host mode.
// All tenants share the default Redis instance; isolation is handled by
// key-prefixing (TenantKey) in the stores rather than per-tenant DB indices.
type RedisCache struct {
	defaultRDB *redis.Client
	log        *slog.Logger
}

// NewRedisCache creates a new Redis cache. All tenants share the default client.
func NewRedisCache(registry *Registry, defaultRDB *redis.Client, ttlMin int, log *slog.Logger) *RedisCache {
	return &RedisCache{
		defaultRDB: defaultRDB,
		log:        log,
	}
}

// GetOrCreate returns the shared Redis client. Tenant isolation is handled by
// key-prefixing (TenantKey) in stores and middleware, not by separate clients.
func (c *RedisCache) GetOrCreate(ctx stdctx.Context, host string) (*redis.Client, error) {
	return c.defaultRDB, nil
}

// Default returns the default (system) Redis client.
func (c *RedisCache) Default() *redis.Client {
	return c.defaultRDB
}

// Evict is a no-op since all tenants share the default client.
func (c *RedisCache) Evict(host string) {}
