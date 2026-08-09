-- Discord registration controls and guild restriction.
--
-- discord_allow_registration gates whether new users may self-register via
-- Discord OAuth; it is independent of the global signup_enabled flag (both
-- must be true for Discord registration). Defaults false so existing
-- installations keep their current behaviour: Discord login/link still works
-- for established users, but no new accounts are auto-created.
--
-- discord_required_guild_id is an optional Discord server (guild) ID. When
-- set, only members of that guild may complete a Discord login or link, and
-- the OAuth flow requests the `guilds` scope to verify membership. Nullable
-- so leaving it blank disables the restriction.
--
-- Idempotent: safe to rerun (ADD COLUMN IF NOT EXISTS).

ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS discord_allow_registration BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS discord_required_guild_id TEXT;