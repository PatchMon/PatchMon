package middleware

import (
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

// CORS sets the CORS response headers and rejects browser requests whose host
// (X-Forwarded-Host, else Host) is outside CORS_ORIGIN; agents and API clients are exempt.
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

			if !set.wildcard && isBrowserRequest(r) {
				if fwd := forwardedHosts(r); len(fwd) > 0 {
					if !slices.ContainsFunc(fwd, func(h string) bool { return hostAllowed(set.hosts, h) }) {
						reflectOrigin := reqOrigin
						if reflectOrigin == "" {
							reflectOrigin = EffectiveOrigin(r, fwd[0])
						}
						rejectHost(w, reflectOrigin, "CORS_ORIGIN mismatch. Access this app via the URL configured in CORS_ORIGIN (.env or Database settings).", "cors_mismatch")
						return
					}
				} else if host := r.Host; host != "" && !isLoopback(host) && !hostAllowed(set.hosts, host) && !set.hosts[hostnameOf(strings.ToLower(host))] {
					rejectHost(w, "", "Host not allowed. Access this app via the URL configured in CORS_ORIGIN (.env or Database settings).", "host_mismatch")
					return
				}
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

			next.ServeHTTP(w, r)
		})
	}
}

func rejectHost(w http.ResponseWriter, reflectOrigin, msg, code string) {
	if reflectOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", reflectOrigin)
		w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
		w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

// Sec-Fetch-Mode is only sent to HTTPS and localhost, so plain-HTTP browsers are
// recognised by Origin, Referer or an HTML navigation; agents and scripts send none.
func isBrowserRequest(r *http.Request) bool {
	h := r.Header
	return h.Get("Sec-Fetch-Mode") != "" || h.Get("Origin") != "" || h.Get("Referer") != "" ||
		strings.Contains(h.Get("Accept"), "text/html")
}

func forwardedHosts(r *http.Request) []string {
	raw := r.Header.Get("X-Forwarded-Host")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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
