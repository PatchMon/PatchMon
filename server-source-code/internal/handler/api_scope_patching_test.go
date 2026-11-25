package handler

import (
	"encoding/json"
	"testing"
)

func TestValidatePatchingScope(t *testing.T) {
	for _, tc := range []struct {
		scopes string
		valid  bool
	}{
		{`{"patching":["trigger"]}`, true},
		{`{"host":["get"],"patching":["trigger"]}`, true},
		{`{"patching":[]}`, true},
		{`{"patching":["approve"]}`, false},
		{`{"patching":["delete"]}`, false},
		{`{"patching":["*"]}`, false},
	} {
		if err := validateScopes(json.RawMessage(tc.scopes)); (err == nil) != tc.valid {
			t.Errorf("validateScopes(%s) = %v, want valid=%v", tc.scopes, err, tc.valid)
		}
	}
}
