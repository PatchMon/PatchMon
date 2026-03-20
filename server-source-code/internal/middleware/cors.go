package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

// OriginResolver returns the allowed origin for a request when in multi-host mode.
// When ok is true, the returned origin is used; when false, the static CORS_ORIGIN is used.
type OriginResolver func(r *http.Request) (origin string, ok bool)

// CORS returns a middleware that sets CORS headers and enforces origin restrictions.
//
// When dynamicResolver is non-nil and returns (origin, true), that origin is used for the request
// (e.g. per-host: scheme + "://" + X-Forwarded-Host). When it returns ("", false), the static
// origin string is used (single-host or fallback).
//
// In addition to standard CORS header validation, it checks X-Forwarded-Host
// (set by the nginx reverse proxy for browser requests) against the allowed
// origins. This blocks access from disallowed hostnames even when the browser
// treats the request as same-origin (frontend proxy -> backend).
//
// Internal requests (health checks, agent connections) that bypass nginx
// won't have X-Forwarded-Host and are not affected.
func CORS(origin string, dynamicResolver OriginResolver) func(http.Handler) http.Handler {
	origins := parseOrigins(origin)
	allowedHosts := buildAllowedHosts(origins)
	wildcard := hasWildcard(origins)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var effectiveOrigins []string
			var effectiveAllowedHosts map[string]bool
			effectiveWildcard := wildcard

			if dynamicResolver != nil {
				if dynOrigin, ok := dynamicResolver(r); ok && dynOrigin != "" {
					effectiveOrigins = []string{dynOrigin}
					effectiveAllowedHosts = buildAllowedHosts(effectiveOrigins)
					effectiveWildcard = false
				}
			}
			if len(effectiveOrigins) == 0 {
				effectiveOrigins = origins
				effectiveAllowedHosts = allowedHosts
			}

			reqOrigin := r.Header.Get("Origin")

			allowOrigin := ""
			for _, o := range effectiveOrigins {
				if o == "*" {
					allowOrigin = "*"
					break
				}
				if o == reqOrigin {
					allowOrigin = reqOrigin
					break
				}
			}

			// Enforce allowed hosts via X-Forwarded-Host on all requests when present.
			// Blocks access from disallowed URLs before the login page loads.
			// Internal requests (health checks, agents) have no X-Forwarded-Host and pass through.
			if !effectiveWildcard {
				if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
					if !effectiveAllowedHosts[strings.ToLower(fwdHost)] {
						// Set CORS headers so the browser allows the client to read this 403.
						reflectOrigin := reqOrigin
						if reflectOrigin == "" {
							reflectOrigin = EffectiveOrigin(r, fwdHost)
						}
						if reflectOrigin != "" {
							w.Header().Set("Access-Control-Allow-Origin", reflectOrigin)
						}
						w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
						w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-ID, X-API-KEY, X-Device-ID")
						w.Header().Set("Access-Control-Allow-Credentials", "true")
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"error":"CORS_ORIGIN mismatch. Access this app via the URL configured in CORS_ORIGIN (.env or Database settings).","code":"cors_mismatch"}`))
						return
					}
				}
			}

			if allowOrigin == "*" && reqOrigin != "" {
				// Browsers reject credentials with a wildcard origin.
				// Reflect the specific request origin so credentialed requests work.
				allowOrigin = reqOrigin
			}
			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-ID, X-API-KEY, X-Device-ID")
			// Only send Allow-Credentials when a specific origin is set; the wildcard + credentials
			// combination is rejected by browsers per the CORS spec.
			if allowOrigin != "" && allowOrigin != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Vary", "Origin")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func parseOrigins(origin string) []string {
	parts := strings.Split(origin, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func hasWildcard(origins []string) bool {
	for _, o := range origins {
		if o == "*" {
			return true
		}
	}
	return false
}

// EffectiveOrigin derives an origin URL from the request and host.
// Used for CORS headers and for dynamic per-host origin resolution.
func EffectiveOrigin(r *http.Request, host string) string {
	if host == "" {
		return ""
	}
	scheme := "https"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.ToLower(proto)
	} else if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + host
}

// buildAllowedHosts extracts host[:port] from each origin URL.
// E.g. "http://patchmon-local:3000" -> "patchmon-local:3000".
// Also adds hostname without port when using default ports (80, 443) so nginx $host works.
// Nginx $host omits the port for default ports (e.g. "patchmon-local.local" for HTTPS).
func buildAllowedHosts(origins []string) map[string]bool {
	m := make(map[string]bool)
	for _, o := range origins {
		if o == "*" {
			continue
		}
		u, err := url.Parse(o)
		if err != nil || u.Host == "" {
			continue
		}
		host := strings.ToLower(u.Host)
		m[host] = true
		hostname := strings.ToLower(u.Hostname())
		port := u.Port()
		// Nginx $host omits port for default ports. Add hostname for http:80 and https:443.
		if port == "" || port == "80" || port == "443" {
			m[hostname] = true
		}
	}
	return m
}
