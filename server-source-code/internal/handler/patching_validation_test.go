package handler

import "testing"

func TestValidatePolicyInput(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	int32Ptr := func(i int32) *int32 { return &i }

	cases := []struct {
		name          string
		policyName    string
		delayType     string
		delayMinutes  *int32
		fixedTimeUtc  *string
		wantErrSubstr string
	}{
		{
			name:       "valid immediate",
			policyName: "Immediate",
			delayType:  "immediate",
		},
		{
			name:         "valid delayed",
			policyName:   "Delayed",
			delayType:    "delayed",
			delayMinutes: int32Ptr(30),
		},
		{
			name:         "valid fixed_time HH:MM",
			policyName:   "Fixed",
			delayType:    "fixed_time",
			fixedTimeUtc: strPtr("03:00"),
		},
		{
			name:         "valid fixed_time HH:MM:SS",
			policyName:   "Fixed",
			delayType:    "fixed_time",
			fixedTimeUtc: strPtr("03:00:30"),
		},
		{
			name:          "missing name",
			policyName:    "",
			delayType:     "immediate",
			wantErrSubstr: "name",
		},
		{
			name:          "invalid delay type",
			policyName:    "X",
			delayType:     "asap",
			wantErrSubstr: "patch_delay_type",
		},
		{
			name:          "delayed without minutes",
			policyName:    "X",
			delayType:     "delayed",
			wantErrSubstr: "delay_minutes",
		},
		{
			name:          "delayed negative minutes",
			policyName:    "X",
			delayType:     "delayed",
			delayMinutes:  int32Ptr(-5),
			wantErrSubstr: "delay_minutes",
		},
		{
			name:          "fixed_time missing time",
			policyName:    "X",
			delayType:     "fixed_time",
			wantErrSubstr: "fixed_time_utc",
		},
		{
			name:          "fixed_time empty string",
			policyName:    "X",
			delayType:     "fixed_time",
			fixedTimeUtc:  strPtr(""),
			wantErrSubstr: "fixed_time_utc",
		},
		{
			name:          "fixed_time with newline injection",
			policyName:    "X",
			delayType:     "fixed_time",
			fixedTimeUtc:  strPtr("03:00\nlevel=critical"),
			wantErrSubstr: "fixed_time_utc must be HH:MM",
		},
		{
			name:          "fixed_time with ANSI escape",
			policyName:    "X",
			delayType:     "fixed_time",
			fixedTimeUtc:  strPtr("\x1b[31m03:00"),
			wantErrSubstr: "fixed_time_utc must be HH:MM",
		},
		{
			name:          "fixed_time 24:00 rejected",
			policyName:    "X",
			delayType:     "fixed_time",
			fixedTimeUtc:  strPtr("24:00"),
			wantErrSubstr: "fixed_time_utc must be HH:MM",
		},
		{
			name:          "fixed_time leap second 60 rejected",
			policyName:    "X",
			delayType:     "fixed_time",
			fixedTimeUtc:  strPtr("23:59:60"),
			wantErrSubstr: "fixed_time_utc must be HH:MM",
		},
		{
			name:          "fixed_time negative hour rejected",
			policyName:    "X",
			delayType:     "fixed_time",
			fixedTimeUtc:  strPtr("-1:30"),
			wantErrSubstr: "fixed_time_utc must be HH:MM",
		},
		{
			name:          "fixed_time totally bogus",
			policyName:    "X",
			delayType:     "fixed_time",
			fixedTimeUtc:  strPtr("rm -rf /"),
			wantErrSubstr: "fixed_time_utc must be HH:MM",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validatePolicyInput(c.policyName, c.delayType, c.delayMinutes, c.fixedTimeUtc)
			if c.wantErrSubstr == "" {
				if got != "" {
					t.Errorf("validatePolicyInput: want no error, got %q", got)
				}
				return
			}
			if got == "" {
				t.Errorf("validatePolicyInput: want error containing %q, got no error", c.wantErrSubstr)
				return
			}
			if !contains(got, c.wantErrSubstr) {
				t.Errorf("validatePolicyInput: want error containing %q, got %q", c.wantErrSubstr, got)
			}
		})
	}
}

func contains(s, substr string) bool {
	if substr == "" {
		return true
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
