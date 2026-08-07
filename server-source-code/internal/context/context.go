package context

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/redis/go-redis/v9"
)

type contextKey string

const dbContextKey contextKey = "request_db"
const redisContextKey contextKey = "request_rdb"
const entryContextKey contextKey = "host_entry"

// WithDB injects the request's database into the context.
func WithDB(ctx stdctx.Context, db *database.DB) stdctx.Context {
	return stdctx.WithValue(ctx, dbContextKey, db)
}

// DBFromContext returns the request's database from the context, or nil if not set.
func DBFromContext(ctx stdctx.Context) *database.DB {
	v := ctx.Value(dbContextKey)
	if v == nil {
		return nil
	}
	db, _ := v.(*database.DB)
	return db
}

// DBResolver resolves the database for a request: DB from context, or default.
// Pass to stores so they use the correct DB per request.
type DBResolver struct {
	Default *database.DB
}

// DB returns the request's database from context, or the default if not set.
func (r *DBResolver) DB(ctx stdctx.Context) *database.DB {
	if r == nil {
		return nil
	}
	if db := DBFromContext(ctx); db != nil {
		return db
	}
	return r.Default
}

// WithRedis injects the request's Redis client into the context.
func WithRedis(ctx stdctx.Context, rdb *redis.Client) stdctx.Context {
	return stdctx.WithValue(ctx, redisContextKey, rdb)
}

// RedisFromContext returns the request's Redis client from the context, or nil if not set.
func RedisFromContext(ctx stdctx.Context) *redis.Client {
	v := ctx.Value(redisContextKey)
	if v == nil {
		return nil
	}
	rdb, _ := v.(*redis.Client)
	return rdb
}

// RedisResolver resolves the Redis client for a request: client from context, or default.
// Pass to stores so they use the correct Redis client per request.
type RedisResolver struct {
	Default *redis.Client
}

// RDB returns the request's Redis client from context, or the default if not set.
func (r *RedisResolver) RDB(ctx stdctx.Context) *redis.Client {
	if r == nil {
		return nil
	}
	if rdb := RedisFromContext(ctx); rdb != nil {
		return rdb
	}
	return r.Default
}

// WithEntry injects the host registry entry into the context.
func WithEntry(ctx stdctx.Context, e *Entry) stdctx.Context {
	return stdctx.WithValue(ctx, entryContextKey, e)
}

// EntryFromContext returns the host registry entry from the context, or nil if not set.
func EntryFromContext(ctx stdctx.Context) *Entry {
	v := ctx.Value(entryContextKey)
	if v == nil {
		return nil
	}
	e, _ := v.(*Entry)
	return e
}

// TenantHostKey returns the canonical per-context identifier (the domain/host) for jobs and Redis.
// Empty when not in multi-host context.
func TenantHostKey(ctx stdctx.Context) string {
	e := EntryFromContext(ctx)
	if e == nil {
		return ""
	}
	return e.Host
}

// multiContext records whether this process serves more than one context. Set
// once at startup when a registry is configured, before any request is served.
var multiContext atomic.Bool

// EnableMultiContextChecks marks the process as serving multiple contexts, which
// turns on the unprefixed-key warning in TenantKey.
func EnableMultiContextChecks() { multiContext.Store(true) }

// TenantKey prefixes a Redis key with the context domain for multi-host isolation.
// In single-context mode (no entry in context), returns the key unchanged.
// Example: TenantKey(ctx, "ssh:ticket:abc") -> "t:ctx1.patchmon.cloud:ssh:ticket:abc"
//
// Redis is shared by every context, so this prefix is the ONLY isolation
// boundary there. The no-entry fallback exists for single-context installs, but
// in a multi-context process it means an unprefixed key in a shared keyspace,
// and it is silent: the usual cause is a background worker, whose context never
// carries an entry because only the HTTP middleware sets one. Workers must
// derive the host from the job payload (see workerTenantKey in internal/queue)
// or inject it with WithEntry. The warning below exists because this failure
// mode is otherwise invisible and only manifests in the cloud.
func TenantKey(ctx stdctx.Context, key string) string {
	if e := EntryFromContext(ctx); e != nil && e.Host != "" {
		return "t:" + e.Host + ":" + key
	}
	if multiContext.Load() {
		caller := "unknown"
		if _, file, line, ok := runtime.Caller(1); ok {
			caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
		slog.Warn("unprefixed context key in a multi-context process; this lands in the shared keyspace",
			"key_prefix", keyPrefixForLog(key), "caller", caller)
	}
	return key
}

// keyPrefixForLog returns the namespace portion of a Redis key, so the warning
// identifies the call site without logging identifiers from the key itself.
func keyPrefixForLog(key string) string {
	if i := strings.Index(key, ":"); i > 0 {
		return key[:i]
	}
	return key
}

// HasModule checks whether the context entry includes the given module.
// Returns true if: no entry in context (single-context mode), or entry.Modules is nil (all allowed),
// or entry.Modules is "*" (wildcard = all modules), or the module is present in the comma-separated
// Modules list.
func HasModule(ctx stdctx.Context, module string) bool {
	entry := EntryFromContext(ctx)
	if entry == nil {
		return true // single-context mode - no restrictions
	}
	if entry.Modules == nil {
		return true // nil = all modules allowed
	}
	modules := strings.TrimSpace(*entry.Modules)
	if modules == "" || modules == "*" {
		return true // wildcard or empty string = all modules allowed
	}
	for _, m := range strings.Split(modules, ",") {
		if strings.TrimSpace(m) == module {
			return true
		}
	}
	return false
}

// RequireModule returns middleware that checks if the context's package includes
// the given module. Returns 403 if the module is not enabled.
// In single-context mode (no entry in context), the request is always allowed.
func RequireModule(module string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !HasModule(r.Context(), module) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "module not available in your plan",
					"code":  "module_not_available",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
