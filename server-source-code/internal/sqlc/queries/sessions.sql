-- name: CreateSession :exec
INSERT INTO user_sessions (
    id, user_id, refresh_token, access_token_hash, ip_address, user_agent,
    device_fingerprint, device_id, last_activity, expires_at, created_at, is_revoked,
    tfa_remember_me, tfa_bypass_until, login_count, last_login_ip
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, false, $12, $13, 1, $14);

-- name: FindSessionWithTfaBypass :one
SELECT id, user_id, refresh_token, access_token_hash, ip_address, user_agent,
    device_fingerprint, last_activity, expires_at, created_at, is_revoked,
    tfa_remember_me, tfa_bypass_until, login_count, last_login_ip
FROM user_sessions
WHERE user_id = $1 AND device_fingerprint = $2 AND tfa_remember_me = true
  AND tfa_bypass_until > NOW() AND is_revoked = false AND expires_at > NOW()
ORDER BY last_activity DESC
LIMIT 1;

-- name: FindSessionByUserAndDevice :one
SELECT id, user_id, refresh_token, access_token_hash, ip_address, user_agent,
    device_fingerprint, device_id, last_activity, expires_at, created_at, is_revoked,
    tfa_remember_me, tfa_bypass_until, login_count, last_login_ip
FROM user_sessions
WHERE user_id = $1 AND device_fingerprint = $2 AND is_revoked = false AND expires_at > NOW()
ORDER BY last_activity DESC
LIMIT 1;

-- name: FindSessionByUserAndDeviceID :one
SELECT id, user_id, refresh_token, access_token_hash, ip_address, user_agent,
    device_fingerprint, device_id, last_activity, expires_at, created_at, is_revoked,
    tfa_remember_me, tfa_bypass_until, login_count, last_login_ip
FROM user_sessions
WHERE user_id = $1 AND device_id = $2 AND is_revoked = false AND expires_at > NOW()
ORDER BY last_activity DESC
LIMIT 1;

-- name: UpdateSessionOnLogin :exec
UPDATE user_sessions SET
    refresh_token = $2,
    last_activity = $3,
    expires_at = $4,
    ip_address = $5,
    user_agent = $6,
    last_login_ip = $7,
    tfa_remember_me = $8,
    tfa_bypass_until = $9,
    device_id = COALESCE($10, device_id),
    login_count = login_count + 1
WHERE id = $1 AND user_id = $11;

-- name: GetSessionByRefreshToken :one
SELECT id, user_id, refresh_token, access_token_hash, ip_address, user_agent,
    device_fingerprint, device_id, last_activity, expires_at, created_at, is_revoked,
    tfa_remember_me, tfa_bypass_until, login_count, last_login_ip
FROM user_sessions WHERE refresh_token = $1 AND is_revoked = false AND expires_at > NOW();

-- name: GetSessionByID :one
SELECT id, user_id, refresh_token, access_token_hash, ip_address, user_agent,
    device_fingerprint, device_id, last_activity, expires_at, created_at, is_revoked,
    tfa_remember_me, tfa_bypass_until, login_count, last_login_ip
FROM user_sessions WHERE id = $1 AND user_id = $2 AND is_revoked = false AND expires_at > NOW();

-- name: ListSessionsByUserID :many
SELECT id, user_id, refresh_token, access_token_hash, ip_address, user_agent,
    device_fingerprint, device_id, last_activity, expires_at, created_at, is_revoked,
    tfa_remember_me, tfa_bypass_until, login_count, last_login_ip
FROM user_sessions WHERE user_id = $1 AND is_revoked = false AND expires_at > NOW() ORDER BY last_activity DESC;

-- name: RevokeSessionByID :exec
UPDATE user_sessions SET is_revoked = true WHERE id = $1 AND user_id = $2;

-- name: RevokeAllSessionsForUserExcept :exec
UPDATE user_sessions SET is_revoked = true WHERE user_id = $1 AND id != $2;

-- name: RevokeAllSessionsForUser :exec
UPDATE user_sessions SET is_revoked = true WHERE user_id = $1;

-- name: UpdateSessionActivity :exec
UPDATE user_sessions SET last_activity = NOW() WHERE id = $1;

-- name: CountTfaRememberSessionsByUser :one
SELECT COUNT(*) FROM user_sessions
WHERE user_id = $1 AND tfa_remember_me = true AND is_revoked = false AND expires_at > NOW();

-- name: DeleteOldestTfaRememberSession :exec
DELETE FROM user_sessions us
WHERE us.id = (
    SELECT s.id FROM user_sessions s
    WHERE s.user_id = $1 AND s.tfa_remember_me = true AND s.is_revoked = false AND s.expires_at > NOW()
    ORDER BY s.last_activity ASC
    LIMIT 1
);

-- name: DeleteExpiredSessions :exec
DELETE FROM user_sessions WHERE expires_at < NOW() OR is_revoked = true;
