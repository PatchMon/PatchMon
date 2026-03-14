package context

import (
	stdctx "context"

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
