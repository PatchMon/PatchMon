-- name: CreateTrustedDevice :exec
INSERT INTO user_trusted_devices (
    id, user_id, token_hash, device_id, user_agent, ip_address, label,
    created_at, last_used_at, expires_at, is_revoked
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, false);

-- name: FindValidTrustedDevice :one
SELECT id, user_id, token_hash, device_id, user_agent, ip_address, label,
    created_at, last_used_at, expires_at, is_revoked
FROM user_trusted_devices
WHERE user_id = $1 AND token_hash = $2
  AND is_revoked = false AND expires_at > NOW()
LIMIT 1;

-- name: TouchTrustedDeviceLastUsed :exec
UPDATE user_trusted_devices
SET last_used_at = $2
WHERE id = $1;

-- name: RevokeTrustedDeviceByID :exec
UPDATE user_trusted_devices
SET is_revoked = true
WHERE id = $1 AND user_id = $2;

-- name: RevokeAllTrustedDevicesForUser :exec
UPDATE user_trusted_devices
SET is_revoked = true
WHERE user_id = $1 AND is_revoked = false;

-- name: ListTrustedDevicesForUser :many
SELECT id, user_id, token_hash, device_id, user_agent, ip_address, label,
    created_at, last_used_at, expires_at, is_revoked
FROM user_trusted_devices
WHERE user_id = $1 AND is_revoked = false AND expires_at > NOW()
ORDER BY last_used_at DESC;

-- name: DeleteExpiredTrustedDevices :exec
DELETE FROM user_trusted_devices
WHERE expires_at < NOW() OR is_revoked = true;
