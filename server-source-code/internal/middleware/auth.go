package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

// Kept in sync with the handler package, which mints the tokens.
const tokenTypeAccess = "access"

const UserIDKey contextKey = "user_id"
const UserRoleKey contextKey = "user_role"
const SessionIDKey contextKey = "session_id"

// UserActivityHeader marks a request as the direct result of a person using the
// UI. Only these requests slide the inactivity window. The frontend polls several
// endpoints on a timer, so treating every authenticated request as activity meant
// an unattended browser tab kept its own session alive indefinitely and
// SESSION_INACTIVITY_TIMEOUT_MINUTES never fired.
const UserActivityHeader = "X-User-Activity"

// userActivity reports whether the caller says this request came from a real
// interaction. It is a hint about the caller's own session and nothing more: a
// client that always sent it would only be keeping alive a session it already
// holds the cookie for. The timeout itself is still checked on every request, so
// a client that never sends it simply times out.
func userActivity(r *http.Request) bool {
	return r.Header.Get(UserActivityHeader) == "1"
}

// Auth returns a middleware that validates JWT and sets user context.
// When sessionsStore and resolved are provided and sessionID is in the token,
// validates session inactivity timeout and updates last_activity.
func Auth(cfg *config.Config, log *slog.Logger) func(http.Handler) http.Handler {
	return AuthWithSessionCheck(cfg, nil, nil, log)
}

// AuthWithSessionCheck returns Auth middleware with session inactivity validation.
func AuthWithSessionCheck(cfg *config.Config, sessionsStore *store.SessionsStore, cfgResolver *hostctx.ConfigResolver, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, source := extractToken(r)
			if token == "" {
				if log != nil {
					log.Debug("auth failed: no token", "path", r.URL.Path, "method", r.Method)
				}
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if log != nil {
				log.Debug("auth validating token", "path", r.URL.Path, "source", source, "token_len", len(token))
			}

			claims := jwt.MapClaims{}
			t, err := jwt.ParseWithClaims(token, &claims, func(_ *jwt.Token) (interface{}, error) {
				return []byte(cfg.JWTSecret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !t.Valid {
				if log != nil {
					log.Debug("auth token invalid", "path", r.URL.Path, "error", err, "valid", t != nil && t.Valid)
				}
				http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
				return
			}

			// A missing typ is rejected too: pre-existing tokens are
			// indistinguishable from refresh tokens.
			if typ, _ := claims["typ"].(string); typ != tokenTypeAccess {
				if log != nil {
					log.Debug("auth rejected non-access token", "path", r.URL.Path, "typ", typ)
				}
				http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
				return
			}

			userID, _ := claims["sub"].(string)
			role, _ := claims["role"].(string)
			sessionID, _ := claims["sessionId"].(string)
			if userID == "" {
				if log != nil {
					log.Debug("auth token missing sub claim", "path", r.URL.Path)
				}
				http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
				return
			}

			// Deliberately not gated on the inactivity timeout: this lookup is
			// what makes revocation work, and must not be switchable off by
			// setting SESSION_INACTIVITY_TIMEOUT_MINUTES=0.
			if sessionID != "" && sessionsStore != nil {
				sess, err := sessionsStore.GetByID(r.Context(), sessionID, userID)
				if err != nil || sess == nil {
					http.Error(w, `{"error":"Session expired"}`, http.StatusUnauthorized)
					return
				}

				// The inactivity comparison is opt-in.
				resolved := cfgResolver.Resolve(r.Context())
				if resolved != nil && resolved.SessionInactivityTimeoutMin > 0 {
					inactive := time.Since(sess.LastActivity) > time.Duration(resolved.SessionInactivityTimeoutMin)*time.Minute
					if inactive {
						if err := sessionsStore.RevokeByID(r.Context(), sessionID, userID); err != nil {
							slog.Error("auth: failed to revoke inactive session", "session_id", sessionID, "error", err)
						}
						http.Error(w, `{"error":"Session expired due to inactivity"}`, http.StatusUnauthorized)
						return
					}
				}

				if userActivity(r) {
					if err := sessionsStore.UpdateActivity(r.Context(), sessionID); err != nil {
						slog.Error("auth: failed to update session activity", "session_id", sessionID, "error", err)
					}
				}
			}

			if log != nil {
				log.Debug("auth success", "path", r.URL.Path, "user_id", userID, "role", role)
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, UserIDKey, userID)
			ctx = context.WithValue(ctx, UserRoleKey, role)
			if sessionID != "" {
				ctx = context.WithValue(ctx, SessionIDKey, sessionID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth returns a middleware that parses JWT when present and sets user context.
// Does not return 401 when token is missing; continues to next handler.
func OptionalAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _ := extractToken(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			claims := jwt.MapClaims{}
			t, err := jwt.ParseWithClaims(token, &claims, func(_ *jwt.Token) (interface{}, error) {
				return []byte(cfg.JWTSecret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !t.Valid {
				next.ServeHTTP(w, r)
				return
			}
			// Same rule as Auth.
			if typ, _ := claims["typ"].(string); typ != tokenTypeAccess {
				next.ServeHTTP(w, r)
				return
			}
			userID, _ := claims["sub"].(string)
			role, _ := claims["role"].(string)
			sessionID, _ := claims["sessionId"].(string)
			if userID != "" {
				ctx := r.Context()
				ctx = context.WithValue(ctx, UserIDKey, userID)
				ctx = context.WithValue(ctx, UserRoleKey, role)
				if sessionID != "" {
					ctx = context.WithValue(ctx, SessionIDKey, sessionID)
				}
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractToken(r *http.Request) (token, source string) {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer "), "header"
		}
	}
	if c, err := r.Cookie("token"); err == nil {
		return c.Value, "cookie"
	}
	return "", ""
}
