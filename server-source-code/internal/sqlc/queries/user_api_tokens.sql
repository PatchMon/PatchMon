-- name: ListUserApiTokens :many
SELECT id, name, created_at, expires_at, last_used_at
FROM user_api_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: GetUserApiTokenByHash :one
SELECT t.id, t.user_id, t.name, t.token_hash, t.created_at, t.expires_at, t.last_used_at,
       u.id AS u_id, u.username AS u_username, u.email AS u_email,
       u.role AS u_role, u.is_active AS u_is_active
FROM user_api_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1;

-- name: CreateUserApiToken :one
INSERT INTO user_api_tokens (id, user_id, name, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, name, created_at, expires_at, last_used_at;

-- name: DeleteUserApiToken :exec
DELETE FROM user_api_tokens WHERE id = $1;

-- name: UpdateUserApiTokenLastUsed :exec
UPDATE user_api_tokens SET last_used_at = NOW() WHERE id = $1;
