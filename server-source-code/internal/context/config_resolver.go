package context

import (
	stdctx "context"
	"sync"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

// configCacheTTL bounds how long a context's resolved settings are reused.
// Short enough that a settings change takes effect promptly, long enough that
// hot paths (rate limiting, lockout) do not read the database per request.
const configCacheTTL = 30 * time.Second

type configCacheEntry struct {
	cfg *config.ResolvedConfig
	exp time.Time
}

// ConfigResolver resolves ResolvedConfig for the context a request belongs to.
//
// config.ResolveConfig is called once at startup against whichever database
// DATABASE_URL points at. In a multi-context deployment that is one context out
// of many, so a value resolved that way cannot be used to enforce anything for
// the others. Anything a context can set for itself must be resolved through
// here, per request. Deployment-wide values (proxy trust, HSTS, DB timeouts)
// stay on the startup config by design.
type ConfigResolver struct {
	cfg         *config.Config
	startup     *config.ResolvedConfig
	getSettings func(stdctx.Context) (*models.Settings, error)

	mu    sync.RWMutex
	cache map[string]configCacheEntry
}

// NewConfigResolver builds a resolver. getSettings must read through a
// context-aware store so it returns the calling context's own settings row.
func NewConfigResolver(cfg *config.Config, startup *config.ResolvedConfig, getSettings func(stdctx.Context) (*models.Settings, error)) *ConfigResolver {
	return &ConfigResolver{
		cfg:         cfg,
		startup:     startup,
		getSettings: getSettings,
		cache:       map[string]configCacheEntry{},
	}
}

// Resolve returns the effective config for ctx's context, falling back to the
// startup config if the resolver is unset or the settings read fails.
func (r *ConfigResolver) Resolve(ctx stdctx.Context) *config.ResolvedConfig {
	if r == nil {
		return nil
	}
	if r.getSettings == nil {
		return r.startup
	}
	key := TenantHostKey(ctx)

	r.mu.RLock()
	e, ok := r.cache[key]
	r.mu.RUnlock()
	if ok && e.cfg != nil && time.Now().Before(e.exp) {
		return e.cfg
	}

	s, err := r.getSettings(ctx)
	if err != nil || s == nil {
		return r.startup
	}
	resolved := config.ResolveConfig(ctx, r.cfg, s)
	if resolved == nil {
		return r.startup
	}

	r.mu.Lock()
	r.cache[key] = configCacheEntry{cfg: resolved, exp: time.Now().Add(configCacheTTL)}
	r.mu.Unlock()
	return resolved
}

// Invalidate drops a context's cached config so the next Resolve re-reads it.
// Called after a settings write so the change is not delayed by the TTL.
func (r *ConfigResolver) Invalidate(ctx stdctx.Context) {
	if r == nil {
		return
	}
	key := TenantHostKey(ctx)
	r.mu.Lock()
	delete(r.cache, key)
	r.mu.Unlock()
}
