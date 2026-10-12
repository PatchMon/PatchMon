package handler

import (
	"encoding/json"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func intPtr(n int) *int { return &n }

func storedAlertConfig() *models.AlertConfig {
	user, rule := "u1", "round_robin"
	return &models.AlertConfig{
		AlertType:             "host_down",
		IsEnabled:             true,
		DefaultSeverity:       "warning",
		AutoAssignEnabled:     true,
		AutoAssignUserID:      &user,
		AutoAssignRule:        &rule,
		RetentionDays:         intPtr(30),
		AutoResolveAfterDays:  intPtr(7),
		CleanupResolvedOnly:   true,
		NotificationEnabled:   true,
		EscalationEnabled:     true,
		EscalationAfterHours:  intPtr(4),
		AlertDelaySeconds:     intPtr(300),
		CheckIntervalMinutes:  intPtr(5),
		ReminderIntervalHours: intPtr(24),
		Metadata:              models.JSON(`{"threshold":30}`),
		Category:              "host",
	}
}

func decodeBody(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestApplyAlertConfigUpdate_OmittedFieldsKeepStoredValues(t *testing.T) {
	t.Parallel()
	cfg := storedAlertConfig()
	applyAlertConfigUpdate(cfg, decodeBody(t, `{"is_enabled": false}`))

	if cfg.IsEnabled {
		t.Error("is_enabled not applied")
	}
	checks := map[string]*int{
		"retention_days":          cfg.RetentionDays,
		"auto_resolve_after_days": cfg.AutoResolveAfterDays,
		"escalation_after_hours":  cfg.EscalationAfterHours,
		"alert_delay_seconds":     cfg.AlertDelaySeconds,
		"check_interval_minutes":  cfg.CheckIntervalMinutes,
		"reminder_interval_hours": cfg.ReminderIntervalHours,
	}
	for name, v := range checks {
		if v == nil {
			t.Errorf("%s cleared although it was absent from the body", name)
		}
	}
	if cfg.AutoAssignUserID == nil || cfg.AutoAssignRule == nil {
		t.Error("auto-assign fields cleared although absent from the body")
	}
	if string(cfg.Metadata) != `{"threshold":30}` {
		t.Errorf("metadata changed: %s", cfg.Metadata)
	}
}

func TestApplyAlertConfigUpdate_ExplicitValuesApplied(t *testing.T) {
	t.Parallel()
	cfg := storedAlertConfig()
	applyAlertConfigUpdate(cfg, decodeBody(t, `{
		"alert_delay_seconds": 0,
		"escalation_after_hours": null,
		"retention_days": 90,
		"auto_resolve_after_days": null,
		"check_interval_minutes": null,
		"reminder_interval_hours": 6,
		"cleanup_resolved_only": false,
		"auto_assign_user_id": null,
		"auto_assign_rule": ""
	}`))

	if cfg.AlertDelaySeconds == nil || *cfg.AlertDelaySeconds != 0 {
		t.Errorf("explicit 0 delay not applied: %v", cfg.AlertDelaySeconds)
	}
	if cfg.EscalationAfterHours != nil || cfg.AutoResolveAfterDays != nil || cfg.CheckIntervalMinutes != nil {
		t.Error("explicit null did not clear the field")
	}
	if cfg.ReminderIntervalHours == nil || *cfg.ReminderIntervalHours != 6 {
		t.Errorf("reminder_interval_hours not applied: %v", cfg.ReminderIntervalHours)
	}
	if cfg.RetentionDays == nil || *cfg.RetentionDays != 90 {
		t.Errorf("retention_days not applied: %v", cfg.RetentionDays)
	}
	if cfg.CleanupResolvedOnly {
		t.Error("explicit false not applied")
	}
	if cfg.AutoAssignUserID != nil {
		t.Error("explicit null auto_assign_user_id did not clear it")
	}
	if cfg.AutoAssignRule != nil {
		t.Error("empty auto_assign_rule did not clear it")
	}
}
