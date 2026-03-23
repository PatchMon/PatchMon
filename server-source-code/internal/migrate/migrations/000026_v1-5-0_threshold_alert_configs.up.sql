-- Seed alert_config rows for update threshold alert types.
INSERT INTO alert_config (id, alert_type, is_enabled, default_severity, auto_assign_enabled, notification_enabled, metadata, updated_at)
VALUES
    -- Threshold exceeded alerts (disabled by default — admin opts in)
    (gen_random_uuid()::TEXT, 'host_security_updates_exceeded', false, 'warning', false, true, '{"threshold": 1}', NOW()),
    (gen_random_uuid()::TEXT, 'host_pending_updates_exceeded', false, 'warning', false, true, '{"threshold": 10}', NOW()),
    -- Resolved counterparts (enabled so notifications fire when threshold clears)
    (gen_random_uuid()::TEXT, 'host_security_updates_resolved', true, 'informational', false, true, NULL, NOW()),
    (gen_random_uuid()::TEXT, 'host_pending_updates_resolved', true, 'informational', false, true, NULL, NOW())
ON CONFLICT (alert_type) DO NOTHING;
