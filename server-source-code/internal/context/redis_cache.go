package context

import (
	stdctx "context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisEntry struct {
	client    *redis.Client
	expiresAt time.Time
}

// RedisCache maintains a cache of host → *redis.Client with lazy initialization.
// If the registry Entry has no Redis credentials, GetOrCreate returns the default client.
type RedisCache struct {
	registry   *Registry
	defaultRDB *redis.Client
	ttl        time.Duration
	mu         sync.RWMutex
	entries    map[string]*redisEntry
	log        *slog.Logger
}

// NewRedisCache creates a new Redis client cache.
func NewRedisCache(registry *Registry, defaultRDB *redis.Client, ttlMin int, log *slog.Logger) *RedisCache {
	ttl := time.Duration(ttlMin) * time.Minute
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &RedisCache{
		registry:   registry,
		defaultRDB: defaultRDB,
		ttl:        ttl,
		entries:    make(map[string]*redisEntry),
		log:        log,
	}
}

// GetOrCreate returns a *redis.Client for the given host.
// If the registry Entry has no RedisHost, returns the default client.
// Creates and caches a new client on first access.
func (c *RedisCache) GetOrCreate(ctx stdctx.Context, host string) (*redis.Client, error) {
	key := strings.ToLower(strings.TrimSpace(host))
	if key == "" {
		return c.defaultRDB, nil
	}

	entry := c.registry.GetByHost(host)
	if entry == nil || entry.RedisHost == nil {
		return c.defaultRDB, nil
	}

	c.mu.RLock()
	ent, ok := c.entries[key]
	c.mu.RUnlock()

	if ok && ent != nil && (c.ttl <= 0 || time.Now().Before(ent.expiresAt)) {
		return ent.client, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock.
	if ent, ok := c.entries[key]; ok && ent != nil && (c.ttl <= 0 || time.Now().Before(ent.expiresAt)) {
		return ent.client, nil
	}

	// Close stale entry if present.
	if old, ok := c.entries[key]; ok && old != nil {
		_ = old.client.Close()
		delete(c.entries, key)
	}

	port := 6379
	if entry.RedisPort != nil && *entry.RedisPort > 0 {
		port = *entry.RedisPort
	}
	db := 0
	if entry.RedisDB != nil {
		db = *entry.RedisDB
	}

	opts := &redis.Options{
		Addr: fmt.Sprintf("%s:%d", *entry.RedisHost, port),
		DB:   db,
	}
	// When host has no per-host credentials (DB-index-only isolation, no ACL),
	// use the default Redis credentials from env so we can connect to the same
	// Redis instance with the host's DB index.
	if entry.RedisUsername != nil {
		opts.Username = *entry.RedisUsername
	} else if u := os.Getenv("REDIS_USER"); u != "" {
		opts.Username = u
	}
	if entry.RedisPassword != nil {
		opts.Password = *entry.RedisPassword
	} else if p := os.Getenv("REDIS_PASSWORD"); p != "" {
		opts.Password = p
	}

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		log := c.log
		if log == nil {
			log = slog.Default()
		}
		log.Warn("host redis ping failed, falling back to default redis",
			"host", key, "redis_addr", opts.Addr, "err", err)
		return c.defaultRDB, nil
	}

	expiresAt := time.Time{}
	if c.ttl > 0 {
		expiresAt = time.Now().Add(c.ttl)
	}
	c.entries[key] = &redisEntry{client: client, expiresAt: expiresAt}

	log := c.log
	if log == nil {
		log = slog.Default()
	}
	log.Debug("redis client created for host", "host", key, "db", db)
	return client, nil
}

// Default returns the default (system) Redis client used as fallback.
func (c *RedisCache) Default() *redis.Client {
	return c.defaultRDB
}

// Evict closes and removes the cached Redis client for the given host.
func (c *RedisCache) Evict(host string) {
	key := strings.ToLower(strings.TrimSpace(host))
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ent, ok := c.entries[key]; ok {
		if ent != nil && ent.client != nil {
			_ = ent.client.Close()
		}
		delete(c.entries, key)
	}
}
