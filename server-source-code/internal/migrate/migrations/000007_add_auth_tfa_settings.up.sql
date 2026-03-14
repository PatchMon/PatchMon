-- Add JWT and TFA settings columns for in-app configuration (env -> DB -> default)
ALTER TABLE settings ADD COLUMN IF NOT EXISTS jwt_expires_in TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS max_tfa_attempts INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS tfa_lockout_duration_minutes INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS tfa_remember_me_expires_in TEXT;
