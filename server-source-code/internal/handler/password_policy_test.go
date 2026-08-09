package handler

import (
	"encoding/json"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
)

// TestResolvePasswordPolicy_UsesResolvedConfig guards the login-settings payload
// against drifting from what ValidatePasswordPolicy enforces.
func TestResolvePasswordPolicy_UsesResolvedConfig(t *testing.T) {
	t.Parallel()

	resolved := &config.ResolvedConfig{
		PasswordMinLength:        12,
		PasswordRequireUppercase: false,
		PasswordRequireLowercase: true,
		PasswordRequireNumber:    false,
		PasswordRequireSpecial:   false,
	}
	got := resolvePasswordPolicy(resolved)
	want := passwordPolicy{
		MinLength:        12,
		RequireUppercase: false,
		RequireLowercase: true,
		RequireNumber:    false,
		RequireSpecial:   false,
	}
	if got != want {
		t.Fatalf("resolvePasswordPolicy() = %+v, want %+v", got, want)
	}

	// A password the emitted policy accepts must also pass server-side validation.
	if err := ValidatePasswordPolicy(resolved, "abcdefghijkl"); err != nil {
		t.Errorf("password allowed by the emitted policy must validate, got %v", err)
	}
}

// TestResolvePasswordPolicy_DefaultsWithoutConfig covers the no-settings-row branch.
func TestResolvePasswordPolicy_DefaultsWithoutConfig(t *testing.T) {
	t.Parallel()

	want := passwordPolicy{
		MinLength:        8,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumber:    true,
		RequireSpecial:   true,
	}
	if got := resolvePasswordPolicy(nil); got != want {
		t.Fatalf("resolvePasswordPolicy(nil) = %+v, want %+v", got, want)
	}
}

// TestPasswordPolicyJSONKeys pins the field names the first-time setup wizard reads.
func TestPasswordPolicyJSONKeys(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(resolvePasswordPolicy(nil))
	if err != nil {
		t.Fatalf("marshal password policy: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal password policy: %v", err)
	}
	if _, ok := got["min_length"].(float64); !ok {
		t.Errorf("min_length must be a number, payload is %s", b)
	}
	for _, key := range []string{"require_uppercase", "require_lowercase", "require_number", "require_special"} {
		if _, ok := got[key].(bool); !ok {
			t.Errorf("%s must be a boolean, payload is %s", key, b)
		}
	}
}
