package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const ApiTokenKey contextKey = "api_token"

// apiError writes a JSON error response for scoped API (doc parity).
func apiError(w http.ResponseWriter, code int, errorMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": errorMsg})
}

// ApiAuth returns a middleware that validates Basic Auth with an API scoped credential
// (auto_enrollment_tokens where metadata.integration_type == "api"), updates last_used_at,
// and attaches the token to the request context.
// Pass a logger for debug-level API auth logging.
func ApiAuth(tokens *store.AutoEnrollmentStore, log *slog.Logger) func(http.Handler) http.Handler {
	return ApiAuthForIntegration(tokens, "api", log)
}

// ApiAuthForIntegration returns a middleware that validates Basic Auth with an API token
// (auto_enrollment_tokens where metadata.integration_type == integrationType), updates last_used_at,
// and attaches the token to the request context.
// Use for integration-specific endpoints (e.g. "gethomepage").
func ApiAuthForIntegration(tokens *store.AutoEnrollmentStore, integrationType string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" || !strings.HasPrefix(auth, "Basic ") {
				if log != nil {
					log.Debug("api_auth failed: missing or invalid authorization header", "path", r.URL.Path)
				}
				apiError(w, http.StatusUnauthorized, "Missing or invalid authorization header")
				return
			}
			encoded := strings.TrimSpace(strings.TrimPrefix(auth, "Basic "))
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				if log != nil {
					log.Debug("api_auth failed: base64 decode error", "path", r.URL.Path, "error", err)
				}
				apiError(w, http.StatusUnauthorized, "Missing or invalid authorization header")
				return
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				if log != nil {
					log.Debug("api_auth failed: invalid credentials format", "path", r.URL.Path)
				}
				apiError(w, http.StatusUnauthorized, "Invalid credentials format")
				return
			}
			apiKey, apiSecret := parts[0], parts[1]

			if log != nil {
				log.Debug("api_auth validating", "path", r.URL.Path, "api_key", apiKey)
			}

			ctx := r.Context()
			token, err := tokens.GetByKey(ctx, apiKey)
			if err != nil {
				if log != nil {
					log.Debug("api_auth failed: token not found", "path", r.URL.Path, "api_key", apiKey, "error", err)
				}
				apiError(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

			if !token.IsActive {
				if log != nil {
					log.Debug("api_auth failed: token disabled", "path", r.URL.Path, "token_id", token.ID)
				}
				apiError(w, http.StatusUnauthorized, "API key is disabled")
				return
			}

			if token.ExpiresAt.Valid && token.ExpiresAt.Time.Before(time.Now()) {
				if log != nil {
					log.Debug("api_auth failed: token expired", "path", r.URL.Path, "token_id", token.ID)
				}
				apiError(w, http.StatusUnauthorized, "API key has expired")
				return
			}

			var metadata struct {
				IntegrationType string `json:"integration_type"`
			}
			if len(token.Metadata) > 0 {
				_ = json.Unmarshal(token.Metadata, &metadata)
			}
			if metadata.IntegrationType != integrationType {
				if log != nil {
					log.Debug("api_auth failed: wrong integration type", "path", r.URL.Path, "type", metadata.IntegrationType)
				}
				apiError(w, http.StatusUnauthorized, "Invalid API key type")
				return
			}

			if err := bcrypt.CompareHashAndPassword([]byte(token.TokenSecret), []byte(apiSecret)); err != nil {
				if log != nil {
					log.Debug("api_auth failed: secret mismatch", "path", r.URL.Path, "token_id", token.ID, "error", err)
				}
				apiError(w, http.StatusUnauthorized, "Invalid API secret")
				return
			}

			if len(token.AllowedIpRanges) > 0 {
				clientIP := clientIPFromRequest(r)
				if !ipAllowed(clientIP, token.AllowedIpRanges) {
					if log != nil {
						log.Debug("api_auth failed: IP not allowed", "path", r.URL.Path, "client_ip", clientIP)
					}
					apiError(w, http.StatusForbidden, "IP address not allowed")
					return
				}
			}

			_ = tokens.UpdateLastUsedAt(ctx, token.ID)

			if log != nil {
				log.Debug("api_auth success", "path", r.URL.Path, "token_id", token.ID)
			}

			ctx = context.WithValue(ctx, ApiTokenKey, &token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetApiToken returns the API token from the request context, or nil.
// Must be called from a handler after ApiAuth middleware.
func GetApiToken(ctx context.Context) *db.AutoEnrollmentToken {
	v := ctx.Value(ApiTokenKey)
	if v == nil {
		return nil
	}
	t, _ := v.(*db.AutoEnrollmentToken)
	return t
}

// clientIPFromRequest returns the client IP, respecting X-Forwarded-For when present.
func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			xff = strings.TrimSpace(xff[:i])
		} else {
			xff = strings.TrimSpace(xff)
		}
		if xff != "" {
			if host, _, err := net.SplitHostPort(xff); err == nil {
				return host
			}
			return xff
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "" {
		return host
	}
	return r.RemoteAddr
}

// ipAllowed returns true if clientIP matches any of the allowed ranges (exact IP or CIDR).
func ipAllowed(clientIP string, allowedRanges []string) bool {
	if clientIP == "" {
		return false
	}
	client := net.ParseIP(clientIP)
	if client == nil {
		return false
	}
	clientV4 := client.To4()
	if clientV4 == nil {
		clientV4 = client
	}
	for _, rng := range allowedRanges {
		rng = strings.TrimSpace(rng)
		if rng == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(rng); err == nil {
			if network.Contains(client) {
				return true
			}
			continue
		}
		if allowed := net.ParseIP(rng); allowed != nil {
			allowedV4 := allowed.To4()
			if allowedV4 == nil {
				allowedV4 = allowed
			}
			if clientV4 != nil && allowedV4 != nil && clientV4.Equal(allowedV4) {
				return true
			}
			if client.Equal(allowed) {
				return true
			}
		}
	}
	return false
}
