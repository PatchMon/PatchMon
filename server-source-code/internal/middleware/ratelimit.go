package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
)

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
func RateLimit(rdb *hostctx.RedisResolver, resolved *config.ResolvedConfig, typ RateLimitType) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			count, err := client.Incr(ctx, key).Result()
			if err != nil {
				rateLimitUnavailable(w, r, typ)
				if rateLimitSecurityCritical(typ) {
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				_ = client.Expire(ctx, key, time.Duration(windowMs)*time.Millisecond).Err()
			}
			if count > int64(max) {
				ttl, _ := client.TTL(ctx, key).Result()
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
func RateLimitAgentByAPIID(rdb *hostctx.RedisResolver, resolved *config.ResolvedConfig, getAPIID func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			count, err := client.Incr(ctx, key).Result()
			if err != nil {
				slog.Warn("agent rate limiter redis error, allowing request", "error", err, "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				_ = client.Expire(ctx, key, time.Duration(windowMs)*time.Millisecond).Err()
			}
			if count > int64(max) {
				ttl, _ := client.TTL(ctx, key).Result()
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

func rateLimitClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	if host != "" {
		return host
	}
	return r.RemoteAddr
}
