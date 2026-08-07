-- name: ListAutoEnrollmentTokens :many
SELECT
    t.id, t.token_name, t.token_key, t.is_active,
    t.allowed_ip_ranges, t.max_hosts_per_day, t.hosts_created_today,
    t.last_used_at, t.expires_at, t.created_at, t.default_host_group_id,
    t.metadata, t.scopes,
    hg.id   AS hg_id,   hg.name AS hg_name,   hg.color AS hg_color,
    u.id    AS u_id,     u.username AS u_username,
    u.first_name AS u_first_name, u.last_name AS u_last_name
FROM auto_enrollment_tokens t
LEFT JOIN host_groups hg ON hg.id = t.default_host_group_id
LEFT JOIN users u ON u.id = t.created_by_user_id
ORDER BY t.created_at DESC;

-- name: GetAutoEnrollmentTokenByID :one
SELECT
    t.id, t.token_name, t.token_key, t.is_active,
    t.allowed_ip_ranges, t.max_hosts_per_day, t.hosts_created_today,
    t.last_used_at, t.expires_at, t.created_at, t.updated_at,
    t.default_host_group_id, t.metadata, t.scopes, t.created_by_user_id,
    hg.id   AS hg_id,   hg.name AS hg_name,   hg.color AS hg_color,
    u.id    AS u_id,     u.username AS u_username,
    u.first_name AS u_first_name, u.last_name AS u_last_name
FROM auto_enrollment_tokens t
LEFT JOIN host_groups hg ON hg.id = t.default_host_group_id
LEFT JOIN users u ON u.id = t.created_by_user_id
WHERE t.id = $1;

-- name: GetAutoEnrollmentTokenByKey :one
SELECT * FROM auto_enrollment_tokens WHERE token_key = $1;

-- name: CreateAutoEnrollmentToken :exec
INSERT INTO auto_enrollment_tokens (
    id, token_name, token_key, token_secret,
    created_by_user_id, is_active, allowed_ip_ranges,
    max_hosts_per_day, default_host_group_id,
    expires_at, metadata, scopes, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: UpdateAutoEnrollmentToken :exec
UPDATE auto_enrollment_tokens SET
    token_name = $1,
    is_active = $2,
    max_hosts_per_day = $3,
    allowed_ip_ranges = $4,
    expires_at = $5,
    default_host_group_id = $6,
    scopes = $7,
    updated_at = $8
WHERE id = $9;

-- name: DeleteAutoEnrollmentToken :exec
DELETE FROM auto_enrollment_tokens WHERE id = $1;

-- name: GetAutoEnrollmentTokenRaw :one
SELECT * FROM auto_enrollment_tokens WHERE id = $1;

-- name: UpdateAutoEnrollmentTokenLastUsedAt :exec
UPDATE auto_enrollment_tokens SET last_used_at = NOW() WHERE id = $1;

-- name: IncrementAutoEnrollmentHostsCreated :exec
UPDATE auto_enrollment_tokens SET
    hosts_created_today = CASE WHEN last_reset_date::date < CURRENT_DATE THEN 1 ELSE hosts_created_today + 1 END,
    last_reset_date = CASE WHEN last_reset_date::date < CURRENT_DATE THEN CURRENT_DATE ELSE last_reset_date END,
    last_used_at = NOW()
WHERE id = $1;
