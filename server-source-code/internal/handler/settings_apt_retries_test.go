package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestApplySettingsUpdate_AptDisableRetries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		start bool
		req   map[string]interface{}
		want  bool
	}{
		{"absent leaves true", true, map[string]interface{}{"update_interval": float64(30)}, true},
		{"absent leaves false", false, map[string]interface{}{"update_interval": float64(30)}, false},
		{"snake_case false", true, map[string]interface{}{"apt_disable_retries": false}, false},
		{"camelCase true", false, map[string]interface{}{"aptDisableRetries": true}, true},
		{"non-bool ignored", true, map[string]interface{}{"apt_disable_retries": "false"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &models.Settings{AptDisableRetries: tt.start}
			if err := applySettingsUpdate(s, tt.req, nil); err != nil {
				t.Fatal(err)
			}
			if s.AptDisableRetries != tt.want {
				t.Errorf("AptDisableRetries = %v, want %v", s.AptDisableRetries, tt.want)
			}
		})
	}
}

func TestSettingsToResponse_AptDisableRetries(t *testing.T) {
	t.Parallel()

	for _, want := range []bool{true, false} {
		res := settingsToResponse(&models.Settings{AptDisableRetries: want}, nil)
		got, ok := res["apt_disable_retries"].(bool)
		if !ok || got != want {
			t.Errorf("apt_disable_retries = %v (ok=%v), want %v", res["apt_disable_retries"], ok, want)
		}
	}
}
