DELETE FROM alert_config WHERE alert_type = 'sso_fallback_login';

ALTER TABLE settings
    DROP COLUMN IF EXISTS oidc_local_auth_fallback;
