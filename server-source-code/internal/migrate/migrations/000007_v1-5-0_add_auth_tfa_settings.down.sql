ALTER TABLE settings DROP COLUMN IF EXISTS jwt_expires_in;
ALTER TABLE settings DROP COLUMN IF EXISTS max_tfa_attempts;
ALTER TABLE settings DROP COLUMN IF EXISTS tfa_lockout_duration_minutes;
ALTER TABLE settings DROP COLUMN IF EXISTS tfa_remember_me_expires_in;
