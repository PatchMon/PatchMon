ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS oidc_local_auth_fallback BOOLEAN NOT NULL DEFAULT false;
