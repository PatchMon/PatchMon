ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS oidc_local_auth_fallback BOOLEAN NOT NULL DEFAULT false;

INSERT INTO alert_config (id, alert_type, is_enabled, default_severity, auto_assign_enabled, notification_enabled, category, updated_at)
VALUES
    (gen_random_uuid()::TEXT, 'sso_fallback_login', true, 'warning', false, true, 'security', NOW()),
    (gen_random_uuid()::TEXT, 'sso_fallback_refused', true, 'warning', false, true, 'security', NOW())
ON CONFLICT (alert_type) DO NOTHING;
