package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/PatchMon/PatchMon/server-source-code/internal/clientip"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

// rateLimitScript increments a bucket and guarantees the key carries an expiry.
//
// INCR and EXPIRE used to be separate calls, with EXPIRE issued only when the
// counter came back as 1 and its error discarded. One lost EXPIRE therefore
// stranded the key with no TTL for good: the counter kept climbing, the
// count == 1 branch never came round again, and every later request from that
// IP was answered 429 until somebody deleted the key by hand. Re-arming
// whenever the TTL is missing makes that state self-healing, and returning the
// remaining TTL from the same call saves the follow-up PTTL the 429 path made.
// A PTTL below zero covers both cases that need arming: INCR has just created
// the key, or the key outlived its expiry. Anything else is a bucket already
// counting down, left alone so the window stays fixed rather than sliding.
var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
	ttl = tonumber(ARGV[1])
end
return {count, ttl}
`)

// rateLimitHit registers one request against key and reports the running count
// for the current window along with the time left in it. A non-positive TTL is
// reported as zero so callers fall back to the configured window.
func rateLimitHit(ctx context.Context, client redis.Scripter, key string, windowMs int) (int64, time.Duration, error) {
	res, err := rateLimitScript.Run(ctx, client, []string{key}, windowMs).Int64Slice()
	if err != nil {
		return 0, 0, err
	}
	if len(res) != 2 {
		return 0, 0, fmt.Errorf("rate limit script returned %d values, want 2", len(res))
	}
	if res[1] <= 0 {
		return res[0], 0, nil
	}
	return res[0], time.Duration(res[1]) * time.Millisecond, nil
}

// RateLimitType identifies which rate limit config to use.
type RateLimitType int

const (
	RateLimitGeneral RateLimitType = iota
	RateLimitAuth
	RateLimitAgent
	RateLimitPassword
)

// rateLimitSecurityCritical returns true for rate limit types that protect
// authentication endpoints. When Redis is unavailable these must fail-closed
// (deny the request) to prevent brute-force attacks.
func rateLimitSecurityCritical(typ RateLimitType) bool {
	return typ == RateLimitAuth || typ == RateLimitPassword
}

func rateLimitUnavailable(w http.ResponseWriter, r *http.Request, typ RateLimitType) {
	if rateLimitSecurityCritical(typ) {
		slog.Warn("rate limiter unavailable, blocking security-critical request", "path", r.URL.Path, "type", typ)
		w.Header().Set("Retry-After", "30")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"Service temporarily unavailable. Please try again shortly."}`))
		return
	}
	// Non-security rate limits degrade gracefully: allow through but log.
	slog.Warn("rate limiter unavailable, allowing request (non-critical)", "path", r.URL.Path, "type", typ)
}

// RateLimit returns middleware that limits requests per client by type.
func RateLimit(rdb *hostctx.RedisResolver, cfgResolver *hostctx.ConfigResolver, typ RateLimitType) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resolved := cfgResolver.Resolve(r.Context())
			if rdb == nil || resolved == nil {
				rateLimitUnavailable(w, r, typ)
				if rateLimitSecurityCritical(typ) {
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			client := rdb.RDB(r.Context())
			if client == nil {
				rateLimitUnavailable(w, r, typ)
				if rateLimitSecurityCritical(typ) {
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			var windowMs, max int
			var keyPrefix string
			switch typ {
			case RateLimitGeneral:
				windowMs = resolved.RateLimitWindowMs
				max = resolved.RateLimitMax
				keyPrefix = "ratelimit:general:"
			case RateLimitAuth:
				windowMs = resolved.AuthRateLimitWindowMs
				max = resolved.AuthRateLimitMax
				keyPrefix = "ratelimit:auth:"
			case RateLimitAgent:
				windowMs = resolved.AgentRateLimitWindowMs
				max = resolved.AgentRateLimitMax
				keyPrefix = "ratelimit:agent:"
			case RateLimitPassword:
				windowMs = resolved.PasswordRateLimitWindowMs
				max = resolved.PasswordRateLimitMax
				keyPrefix = "ratelimit:password:"
			default:
				next.ServeHTTP(w, r)
				return
			}
			if windowMs <= 0 || max <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			clientIP := rateLimitClientIP(r)
			key := hostctx.TenantKey(r.Context(), keyPrefix+clientIP)
			ctx := r.Context()
			count, ttl, err := rateLimitHit(ctx, client, key, windowMs)
			if err != nil {
				rateLimitUnavailable(w, r, typ)
				if rateLimitSecurityCritical(typ) {
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if count > int64(max) {
				remainingSec := int(ttl.Seconds())
				if remainingSec <= 0 {
					remainingSec = windowMs / 1000
				}
				w.Header().Set("Retry-After", strconv.Itoa(remainingSec))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"message":"Too many requests. Try again later."}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitAgentByAPIID returns middleware for agent routes that uses API ID as key.
func RateLimitAgentByAPIID(rdb *hostctx.RedisResolver, cfgResolver *hostctx.ConfigResolver, getAPIID func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resolved := cfgResolver.Resolve(r.Context())
			if rdb == nil || resolved == nil {
				slog.Warn("agent rate limiter unavailable, allowing request", "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}
			client := rdb.RDB(r.Context())
			if client == nil {
				slog.Warn("agent rate limiter unavailable, allowing request", "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}
			apiID := getAPIID(r)
			if apiID == "" {
				apiID = rateLimitClientIP(r)
			}
			key := hostctx.TenantKey(r.Context(), "ratelimit:agent:"+apiID)
			windowMs := resolved.AgentRateLimitWindowMs
			max := resolved.AgentRateLimitMax
			if windowMs <= 0 || max <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			count, ttl, err := rateLimitHit(ctx, client, key, windowMs)
			if err != nil {
				slog.Warn("agent rate limiter redis error, allowing request", "error", err, "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}
			if count > int64(max) {
				remainingSec := int(ttl.Seconds())
				if remainingSec <= 0 {
					remainingSec = windowMs / 1000
				}
				w.Header().Set("Retry-After", strconv.Itoa(remainingSec))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"message":"Too many requests. Try again later."}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitClientIP returns the client IP to key the rate-limit bucket on.
//
// It deliberately does NOT read X-Forwarded-For itself. The RealIP middleware
// has already resolved it into RemoteAddr; parsing the header here would take
// the client-supplied leftmost entry and let callers rotate buckets at will.
func rateLimitClientIP(r *http.Request) string {
	if ip := clientip.FromRequest(r); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
