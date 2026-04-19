-- user_trusted_devices: dedicated table for MFA "remember this device" trust tokens.
-- Completely decoupled from user_sessions: session lifecycle (login/logout/expiry) is
-- independent from device-trust lifecycle (30d default, user-revocable per device).
-- The server stores SHA-256(raw_token); the raw token lives only in the HttpOnly cookie
-- patchmon_device_trust on the user's browser.

CREATE TABLE IF NOT EXISTS user_trusted_devices (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    device_id TEXT,
    user_agent TEXT,
    ip_address TEXT,
    label TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP(3) NOT NULL,
    is_revoked BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_user_trusted_devices_lookup
    ON user_trusted_devices (user_id, token_hash)
    WHERE is_revoked = false;

CREATE INDEX IF NOT EXISTS idx_user_trusted_devices_user
    ON user_trusted_devices (user_id)
    WHERE is_revoked = false;

CREATE INDEX IF NOT EXISTS idx_user_trusted_devices_expiry
    ON user_trusted_devices (expires_at);
