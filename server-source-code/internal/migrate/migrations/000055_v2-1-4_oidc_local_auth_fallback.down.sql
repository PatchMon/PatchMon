DELETE FROM alert_config WHERE alert_type IN ('sso_fallback_login', 'sso_fallback_refused');

ALTER TABLE settings
    DROP COLUMN IF EXISTS oidc_local_auth_fallback;
