package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
)

// RequireApiScope returns a middleware that checks the API token has the required scope
// (resource and action). Use after ApiAuth. Returns 403 with doc-parity messages if scope is missing.
func RequireApiScope(resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := GetApiToken(r.Context())
			if token == nil {
				apiError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}

			// Only validate scopes for API type tokens (metadata already checked in ApiAuth)
			var metadata struct {
				IntegrationType string `json:"integration_type"`
			}
			if len(token.Metadata) > 0 {
				_ = json.Unmarshal(token.Metadata, &metadata)
			}
			if metadata.IntegrationType != "api" {
				next.ServeHTTP(w, r)
				return
			}

			if len(token.Scopes) == 0 {
				apiErrorWithMessage(w, "This API key does not have the required permissions")
				return
			}

			var scopes map[string][]string
			if err := json.Unmarshal(token.Scopes, &scopes); err != nil {
				apiErrorWithMessage(w, "Invalid API key permissions configuration")
				return
			}

			actions, ok := scopes[resource]
			if !ok {
				apiErrorWithMessage(w, "This API key does not have access to "+resource)
				return
			}

			if !sliceContains(actions, action) {
				apiErrorWithMessage(w, "This API key does not have permission to "+action+" "+resource)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func apiErrorWithMessage(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "Access denied",
		"message": message,
	})
}

func sliceContains(s []string, x string) bool {
	for _, v := range s {
		if strings.TrimSpace(v) == x {
			return true
		}
	}
	return false
}
