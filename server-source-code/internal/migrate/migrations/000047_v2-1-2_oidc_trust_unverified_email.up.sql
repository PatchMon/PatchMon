ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS oidc_trust_unverified_email BOOLEAN NOT NULL DEFAULT false;
