ALTER TABLE users DROP COLUMN IF EXISTS previous_password_hash;
ALTER TABLE users DROP COLUMN IF EXISTS password_change_required;
