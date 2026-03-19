package context

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/redis/go-redis/v9"
)

const reloadSecretHeader = "X-Registry-Reload-Secret"

// secureCompare performs a constant-time comparison to prevent timing attacks on secrets.
func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// RegistryReloadHandler returns a handler for POST /api/v1/internal/reload-registry-map.
// Triggers an immediate refresh of the registry cache. Used by the provisioner
// after creating a host so new hosts are visible without waiting for the 5-min poll.
func RegistryReloadHandler(registry *Registry, reloadSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if reloadSecret != "" && !secureCompare(r.Header.Get(reloadSecretHeader), reloadSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if registry != nil {
			_ = registry.Reload(r.Context())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

// Middleware returns a middleware that resolves the host from X-Forwarded-Host,
// injects the host's database and Redis client into context, and returns 503 if the host is not found.
// When X-Forwarded-Host is absent (e.g. health checks, direct connections), defaultDB and defaultRDB are injected if non-nil.
func Middleware(registry *Registry, poolCache *PoolCache, redisCache *RedisCache, defaultDB *database.DB, defaultRDB *redis.Client, reloadSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
			if host == "" {
				ctx := r.Context()
				if defaultDB != nil {
					ctx = WithDB(ctx, defaultDB)
				}
				if defaultRDB != nil {
					ctx = WithRedis(ctx, defaultRDB)
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if registry == nil || poolCache == nil {
				next.ServeHTTP(w, r)
				return
			}

			db, err := poolCache.GetOrCreate(r.Context(), host)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "service temporarily unavailable",
					"code":  "db_error",
				})
				return
			}
			if db == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "host not found or suspended",
					"code":  "host_not_found",
				})
				return
			}

			ctx := WithDB(r.Context(), db)

			// Inject host registry entry so handlers can check package limits (MaxUsers, MaxHosts, Modules).
			if entry := registry.GetByHost(host); entry != nil {
				ctx = WithEntry(ctx, entry)
			}

			// Resolve per-host Redis; falls back to defaultRDB if no Redis credentials in registry.
			if redisCache != nil {
				rdb, rdbErr := redisCache.GetOrCreate(r.Context(), host)
				if rdbErr != nil {
					// Log but don't block - fall back to default Redis rather than returning 503.
					rdb = redisCache.Default()
				}
				if rdb != nil {
					ctx = WithRedis(ctx, rdb)
				}
			} else if defaultRDB != nil {
				ctx = WithRedis(ctx, defaultRDB)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ReloadHandler returns a handler for POST /internal/reload that evicts a host from the pool and Redis caches.
// Requires X-Registry-Reload-Secret header to match reloadSecret.
// Query param: host (the host to evict).
func ReloadHandler(poolCache *PoolCache, redisCache *RedisCache, reloadSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if reloadSecret != "" && !secureCompare(r.Header.Get(reloadSecretHeader), reloadSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		host := r.URL.Query().Get("host")
		if host == "" {
			http.Error(w, "host query param required", http.StatusBadRequest)
			return
		}
		if poolCache != nil {
			poolCache.Evict(host)
		}
		if redisCache != nil {
			redisCache.Evict(host)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}
