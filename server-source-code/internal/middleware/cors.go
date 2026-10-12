package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// OriginResolver returns the allowed origin for a request in multi-context mode.
// When ok is false the configured CORS_ORIGIN list applies.
type OriginResolver func(r *http.Request) (origin string, ok bool)

const (
	corsAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowHeaders = "Content-Type, Authorization, X-API-ID, X-API-KEY, X-Device-ID"
)

type originSet struct {
	origins  []string
	hosts    map[string]bool
	wildcard bool
}

func newOriginSet(origins []string) originSet {
	return originSet{origins: origins, hosts: buildAllowedHosts(origins), wildcard: hasWildcard(origins)}
}

// CORS sets the CORS response headers and rejects requests whose host
// (X-Forwarded-Host, else Host) is outside CORS_ORIGIN.
func CORS(origin string, dynamicResolver OriginResolver) func(http.Handler) http.Handler {
	static := newOriginSet(ParseOrigins(origin))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			set := static
			if dynamicResolver != nil {
				if dynOrigin, ok := dynamicResolver(r); ok && dynOrigin != "" {
					set = newOriginSet(ParseOrigins(dynOrigin))
				}
			}

			reqOrigin := r.Header.Get("Origin")
			allowOrigin := ""
			for _, o := range set.origins {
				if o == "*" {
					allowOrigin = "*"
					break
				}
				if o == reqOrigin {
					allowOrigin = reqOrigin
					break
				}
			}

			if fwd := forwardedHost(r); fwd != "" {
				if !set.allowsForwarded(fwd) {
					reflectOrigin := reqOrigin
					if reflectOrigin == "" {
						reflectOrigin = EffectiveOrigin(r, fwd)
					}
					rejectHost(w, reflectOrigin, "CORS_ORIGIN mismatch. Access this app via the URL configured in CORS_ORIGIN (.env or Database settings).", "cors_mismatch")
					return
				}
			} else if !set.allowsDirect(r.Host) {
				rejectHost(w, "", "Host not allowed. Access this app via the URL configured in CORS_ORIGIN (.env or Database settings).", "host_mismatch")
				return
			}

			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			}
			w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
			// Browsers reject wildcard + credentials, so only a specific origin gets credentials.
			if allowOrigin != "" && allowOrigin != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Vary", "Origin")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), corsOriginSetKey{}, set)))
		})
	}
}

type corsOriginSetKey struct{}

func (s originSet) allowsForwarded(hostPort string) bool {
	return s.wildcard || hostAllowed(s.hosts, hostPort)
}

func (s originSet) allowsDirect(hostPort string) bool {
	return s.wildcard || hostPort == "" || isLoopback(hostPort) || hostAllowed(s.hosts, hostPort) || s.hosts[hostnameOf(strings.ToLower(hostPort))]
}

// CORSAllowsHost reports whether the CORS middleware would accept hostPort for a
// request arriving like r; it is false when the middleware did not run.
func CORSAllowsHost(r *http.Request, hostPort string) bool {
	set, ok := r.Context().Value(corsOriginSetKey{}).(originSet)
	if !ok {
		return false
	}
	if forwardedHost(r) != "" {
		return set.allowsForwarded(hostPort)
	}
	return set.allowsDirect(hostPort)
}

func rejectHost(w http.ResponseWriter, reflectOrigin, msg, code string) {
	if reflectOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", reflectOrigin)
		w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
		w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

// Rightmost entry: an appending proxy puts its own value after whatever the
// client sent, as with X-Forwarded-For, so anything further left is untrusted.
func forwardedHost(r *http.Request) string {
	parts := strings.Split(r.Header.Get("X-Forwarded-Host"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if p := strings.TrimSpace(parts[i]); p != "" {
			return p
		}
	}
	return ""
}

func hostAllowed(hosts map[string]bool, hostPort string) bool {
	h := strings.ToLower(strings.TrimSpace(hostPort))
	if h == "" {
		return false
	}
	if hosts[h] {
		return true
	}
	name, port := splitHostPort(h)
	return (port == "80" || port == "443") && hosts[name]
}

func hostnameOf(hostPort string) string {
	name, _ := splitHostPort(hostPort)
	return name
}

func splitHostPort(hostPort string) (string, string) {
	port := ""
	if i := strings.LastIndex(hostPort, ":"); i != -1 && !strings.Contains(hostPort[i:], "]") {
		port = hostPort[i+1:]
		hostPort = hostPort[:i]
	}
	return strings.TrimSuffix(strings.TrimPrefix(hostPort, "["), "]"), port
}

// ParseOrigins splits a CORS_ORIGIN value into trimmed, lower-cased origins with
// any path and default port removed; "*" and entries that are not URLs are kept as is.
func ParseOrigins(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p != "*" {
			if u, err := url.Parse(p); err == nil && u.Scheme != "" && u.Host != "" {
				p = normalisedOrigin(u)
			}
		}
		out = append(out, p)
	}
	return out
}

func normalisedOrigin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	if port := u.Port(); (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		host = strings.ToLower(u.Hostname())
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
	}
	return scheme + "://" + host
}

// ValidateOrigins reports the first CORS_ORIGIN entry that is neither "*" nor
// an http(s) origin without path, query, fragment or credentials.
func ValidateOrigins(value string) error {
	for _, p := range strings.Split(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" || p == "*" {
			continue
		}
		u, err := url.Parse(p)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("CORS_ORIGIN entry %q is not an origin; use scheme://host[:port], for example https://patchmon.example.com", p)
		}
	}
	return nil
}

func hasWildcard(origins []string) bool {
	return slices.Contains(origins, "*")
}

// EffectiveOrigin derives an origin URL from the request and host.
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

// buildAllowedHosts maps each origin's host[:port] to true. Origins on a
// default port also allow the bare hostname, which is what nginx $host sends.
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
		m[strings.ToLower(u.Host)] = true
		if port := u.Port(); port == "" || port == "80" || port == "443" {
			m[strings.ToLower(u.Hostname())] = true
		}
	}
	return m
}

func isLoopback(hostPort string) bool {
	h := hostnameOf(strings.ToLower(hostPort))
	return h == "localhost" || h == "::1" || strings.HasPrefix(h, "127.")
}
