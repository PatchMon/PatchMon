package handler

import (
	"encoding/json"
	"testing"
)

func TestValidateScopes(t *testing.T) {
	tests := []struct {
		name  string
		json  string
		valid bool
	}{
		{"existing host read", `{"host":["get"]}`, true},
		{"existing host delete", `{"host":["get","delete"]}`, true},
		{"legacy host selections", `{"host":["get","put","patch","update","delete"]}`, true},
		{"requested automation scopes", `{"host":["get","delete"],"host_group":["get","create","update"],"enrollment_token":["get","create","update"]}`, true},
		{"explicit deletion scopes", `{"host_group":["delete"],"enrollment_token":["delete"]}`, true},
		{"independent action", `{"enrollment_token":["update"]}`, true},
		{"empty scope grants", `{}`, true},
		{"empty actions", `{"host":[]}`, true},
		{"not object", `[]`, false},
		{"null", `null`, false},
		{"malformed", `{`, false},
		{"unknown resource", `{"user":["get"]}`, false},
		{"wildcard resource", `{"*":["get"]}`, false},
		{"wildcard action", `{"host":["*"]}`, false},
		{"implicit admin", `{"host_group":["admin"]}`, false},
		{"unknown host action", `{"host":["create"]}`, false},
		{"actions string", `{"host":"get"}`, false},
		{"actions object", `{"host":{"get":true}}`, false},
		{"actions null", `{"host":null}`, false},
		{"non-string action", `{"host":[true]}`, false},
		{"null action", `{"host":[null]}`, false},
		{"whitespace action", `{"host":[" get "]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScopes(json.RawMessage(tt.json))
			if (err == nil) != tt.valid {
				t.Fatalf("validateScopes(%s) = %v, want valid=%v", tt.json, err, tt.valid)
			}
		})
	}
}
