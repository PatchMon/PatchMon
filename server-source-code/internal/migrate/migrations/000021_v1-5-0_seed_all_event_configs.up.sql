-- Seed alert_config rows for all event types so users can control lifecycle for every event.
-- Uses ON CONFLICT to preserve existing customizations.

INSERT INTO alert_config (id, alert_type, is_enabled, default_severity, auto_assign_enabled, notification_enabled, cleanup_resolved_only, created_at, updated_at)
VALUES
    (gen_random_uuid()::TEXT, 'host_recovered', true, 'informational', false, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'patch_run_completed', true, 'informational', false, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'patch_run_failed', true, 'error', false, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'compliance_scan_completed', true, 'informational', false, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (alert_type) DO NOTHING;
