package context

import (
	stdctx "context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/migrate"
)

type poolEntry struct {
	db        *database.DB
	expiresAt time.Time
}

// PoolCache maintains a cache of host → *database.DB with lazy initialization.
// On first access: runs migrations, creates pool, caches.
type PoolCache struct {
	registry *Registry
	cfg      *config.Config
	ttl      time.Duration
	mu       sync.RWMutex
	entries  map[string]*poolEntry
	log      *slog.Logger
}

// NewPoolCache creates a new pool cache.
func NewPoolCache(registry *Registry, cfg *config.Config, ttlMin int, log *slog.Logger) *PoolCache {
	ttl := time.Duration(ttlMin) * time.Minute
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &PoolCache{
		registry: registry,
		cfg:      cfg,
		ttl:      ttl,
		entries:  make(map[string]*poolEntry),
		log:      log,
	}
}

// GetOrCreate returns the database for the given host, creating it on first access.
// Runs migrations before creating the pool. Returns nil if the host is not found or suspended.
func (c *PoolCache) GetOrCreate(ctx stdctx.Context, host string) (*database.DB, error) {
	key := strings.ToLower(strings.TrimSpace(host))
	if key == "" {
		return nil, nil
	}

	entry := c.registry.GetByHost(host)
	if entry == nil {
		return nil, nil
	}
	if entry.DatabaseURL == "" {
		return nil, nil
	}

	c.mu.RLock()
	ent, ok := c.entries[key]
	c.mu.RUnlock()

	if ok && ent != nil && (c.ttl <= 0 || time.Now().Before(ent.expiresAt)) {
		return ent.db, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock
	if ent, ok := c.entries[key]; ok && ent != nil && (c.ttl <= 0 || time.Now().Before(ent.expiresAt)) {
		return ent.db, nil
	}

	// Evict stale entry if present
	if old, ok := c.entries[key]; ok && old != nil {
		old.db.Close()
		delete(c.entries, key)
	}

	maxConns := c.cfg.HostPoolMaxConns
	if maxConns <= 0 {
		maxConns = 5
	}
	if entry.MaxConnections != nil && *entry.MaxConnections > 0 {
		maxConns = *entry.MaxConnections
	}
	minConns := c.cfg.HostPoolMinConns
	if minConns < 0 {
		minConns = 1
	}
	if entry.MinConnections != nil && *entry.MinConnections >= 0 {
		minConns = *entry.MinConnections
	}

	log := c.log
	if log == nil {
		log = slog.Default()
	}
	if err := migrate.Run(entry.DatabaseURL, log); err != nil {
		return nil, err
	}

	db, err := database.NewFromURL(ctx, entry.DatabaseURL, maxConns, minConns, c.cfg)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Time{}
	if c.ttl > 0 {
		expiresAt = time.Now().Add(c.ttl)
	}
	c.entries[key] = &poolEntry{db: db, expiresAt: expiresAt}
	return db, nil
}

// ListHosts returns all hosts from the registry. Returns nil if single-host.
func (c *PoolCache) ListHosts() []string {
	if c == nil || c.registry == nil {
		return nil
	}
	return c.registry.ListHosts()
}

// Evict removes the host's pool from the cache and closes it.
func (c *PoolCache) Evict(host string) {
	key := strings.ToLower(strings.TrimSpace(host))
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ent, ok := c.entries[key]; ok {
		if ent != nil && ent.db != nil {
			ent.db.Close()
		}
		delete(c.entries, key)
	}
}
