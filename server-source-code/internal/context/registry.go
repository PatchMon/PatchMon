package context

import (
	stdctx "context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const statusActive = "active"

// Entry holds host metadata from the provisioner registry.
type Entry struct {
	ID             string
	Slug           string
	Host           string
	DatabaseURL    string
	RedisHost      *string
	RedisPort      *int
	RedisDB        *int
	RedisUsername  *string
	RedisPassword  *string
	BackendURL     string
	Status         string
	MaxConnections *int
	MinConnections *int
	MaxUsers       *int
	MaxHosts       *int
	Modules        *string // comma-separated allowed modules, nil = all
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Registry maintains an in-memory cache of host -> Entry, polling the provisioner DB.
type Registry struct {
	pool    *pgxpool.Pool
	mu      sync.RWMutex
	byHost  map[string]*Entry
	pollDur time.Duration
	stop    chan struct{}
	stopped chan struct{}
	log     *slog.Logger
}

// NewRegistry connects to the registry DB and starts a background poller.
func NewRegistry(ctx stdctx.Context, databaseURL string, pollInterval time.Duration, log *slog.Logger) (*Registry, error) {
	if databaseURL == "" {
		return nil, nil
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	r := &Registry{
		pool:    pool,
		byHost:  make(map[string]*Entry),
		pollDur: pollInterval,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		log:     log,
	}
	if err := r.refresh(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	go r.poll()
	return r, nil
}

// Close stops the poller and closes the pool.
func (r *Registry) Close() {
	if r == nil {
		return
	}
	close(r.stop)
	<-r.stopped
	r.pool.Close()
}

func (r *Registry) poll() {
	defer close(r.stopped)
	ticker := time.NewTicker(r.pollDur)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 30*time.Second)
			if err := r.refresh(ctx); err != nil && r.log != nil {
				r.log.Warn("registry refresh failed", "error", err)
			}
			cancel()
		}
	}
}

func (r *Registry) refresh(ctx stdctx.Context) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, slug, host, COALESCE(database_url, ''),
		       redis_host, redis_port, redis_db, redis_username, redis_password,
		       backend_url, status,
		       max_db_connections, min_db_connections, max_users, max_hosts, modules,
		       created_at, updated_at
		FROM tenants WHERE status = $1`,
		statusActive,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	newMap := make(map[string]*Entry)
	for rows.Next() {
		var t Entry
		var dbURL string
		err := rows.Scan(
			&t.ID, &t.Slug, &t.Host, &dbURL,
			&t.RedisHost, &t.RedisPort, &t.RedisDB, &t.RedisUsername, &t.RedisPassword,
			&t.BackendURL, &t.Status,
			&t.MaxConnections, &t.MinConnections, &t.MaxUsers, &t.MaxHosts, &t.Modules,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return err
		}
		t.DatabaseURL = dbURL
		hostKey := strings.ToLower(strings.TrimSpace(t.Host))
		if hostKey != "" {
			newMap[hostKey] = &t
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	r.byHost = newMap
	r.mu.Unlock()
	return nil
}

// GetByHost returns the entry for the given host (exact match, case-insensitive), or nil if not found.
func (r *Registry) GetByHost(host string) *Entry {
	if r == nil {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(host))
	if key == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byHost[key]
}

// ListHosts returns all active hosts. Used by automation jobs to process all hosts.
func (r *Registry) ListHosts() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	hosts := make([]string, 0, len(r.byHost))
	for _, e := range r.byHost {
		if e != nil && e.Host != "" {
			hosts = append(hosts, e.Host)
		}
	}
	return hosts
}

// Reload triggers an immediate refresh of the registry cache.
func (r *Registry) Reload(ctx stdctx.Context) error {
	if r == nil {
		return nil
	}
	return r.refresh(ctx)
}
