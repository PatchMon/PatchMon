-- Reverse 000046: drop the Discord registration controls and guild columns.
-- Drop the nullable column first, then the NOT NULL boolean, each guarded by
-- IF EXISTS so the rollback is idempotent against a partial state.

ALTER TABLE settings
    DROP COLUMN IF EXISTS discord_required_guild_id,
    DROP COLUMN IF EXISTS discord_allow_registration;