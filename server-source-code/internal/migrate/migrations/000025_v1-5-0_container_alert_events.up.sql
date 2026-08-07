-- Seed alert_config rows for new notification event types.
INSERT INTO alert_config (id, alert_type, is_enabled, default_severity, auto_assign_enabled, notification_enabled, updated_at)
VALUES
    -- Docker container events
    (gen_random_uuid()::TEXT, 'container_stopped', true, 'warning', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'container_started', true, 'informational', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'container_image_update_available', true, 'informational', false, true, NOW()),
    -- Patching workflow events
    (gen_random_uuid()::TEXT, 'patch_run_started', true, 'informational', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'patch_run_approved', true, 'informational', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'patch_run_cancelled', true, 'informational', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'patch_reboot_required', true, 'warning', false, true, NOW()),
    -- Fleet management events
    (gen_random_uuid()::TEXT, 'host_enrolled', true, 'informational', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'host_deleted', true, 'warning', false, true, NOW()),
    -- Compliance events
    (gen_random_uuid()::TEXT, 'compliance_scan_failed', true, 'error', false, true, NOW()),
    -- Remote access events
    (gen_random_uuid()::TEXT, 'ssh_session_started', true, 'informational', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'rdp_session_started', true, 'informational', false, true, NOW()),
    -- Auth/security events (user_login disabled by default to avoid noise)
    (gen_random_uuid()::TEXT, 'user_login', false, 'informational', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'user_login_failed', true, 'warning', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'account_locked', true, 'error', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'user_created', true, 'informational', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'user_role_changed', true, 'warning', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'user_tfa_disabled', true, 'warning', false, true, NOW())
ON CONFLICT (alert_type) DO NOTHING;
