-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE LOWER(username) = LOWER($1);

-- name: GetUserByEmail :one
SELECT * FROM users WHERE LOWER(email) = LOWER($1);

-- name: GetUserByOidcSub :one
SELECT * FROM users WHERE oidc_sub = $1;

-- name: GetUserByDiscordID :one
SELECT * FROM users WHERE discord_id = $1;

-- name: GetUserByDiscordIDOrEmail :one
SELECT * FROM users
WHERE discord_id = $1 OR (LOWER(email) = LOWER($2) AND $2 != '')
ORDER BY CASE WHEN discord_id = $1 THEN 0 ELSE 1 END
LIMIT 1;

-- name: GetUserByOidcSubOrEmail :one
SELECT * FROM users
WHERE oidc_sub = $1 OR LOWER(email) = LOWER($2)
ORDER BY CASE WHEN oidc_sub = $1 THEN 0 ELSE 1 END
LIMIT 1;

-- name: GetUserByUsernameOrEmail :one
SELECT * FROM users
WHERE LOWER(username) = LOWER($1) OR LOWER(email) = LOWER($1)
ORDER BY CASE WHEN LOWER(username) = LOWER($1) THEN 0 ELSE 1 END
LIMIT 1;

-- name: ExistsByUsernameOrEmail :one
SELECT COUNT(*) > 0 FROM users
WHERE (LOWER(username) = LOWER(sqlc.arg('username')) OR LOWER(email) = LOWER(sqlc.arg('email')))
AND (sqlc.narg('exclude_user_id')::text IS NULL OR id != sqlc.narg('exclude_user_id'));

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CountSuperadmins :one
SELECT COUNT(*) FROM users WHERE role = 'superadmin' AND is_active = true;

-- name: CountActiveAdmins :one
SELECT COUNT(*) FROM users WHERE role = 'admin' AND is_active = true;

-- name: CountAdmins :one
SELECT COUNT(*) FROM users WHERE role IN ('admin', 'superadmin') AND is_active = true;

-- name: ListUsers :many
SELECT * FROM users ORDER BY username LIMIT $1 OFFSET $2;

-- name: ListActiveUsers :many
SELECT * FROM users WHERE is_active = true ORDER BY username;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2;

-- name: CreateUser :exec
INSERT INTO users (
    id, username, email, password_hash, role, is_active, created_at, updated_at,
    tfa_enabled, first_name, last_name, theme_preference, color_theme
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13
);

-- name: CreateOidcUser :exec
INSERT INTO users (
    id, username, email, password_hash, role, is_active, created_at, updated_at,
    tfa_enabled, first_name, last_name, theme_preference, color_theme,
    oidc_sub, oidc_provider, avatar_url
) VALUES (
    $1, $2, $3, NULL, $4, true, $5, $6,
    false, $7, $8, 'dark', 'cyber_blue',
    $9, $10, $11
);

-- name: UpdateUser :exec
UPDATE users SET
    username = $1, email = $2, role = $3, is_active = $4,
    updated_at = $5, first_name = $6, last_name = $7,
    theme_preference = $8, color_theme = $9
WHERE id = $10;

-- name: UpdateUserOidcLink :exec
UPDATE users SET
    oidc_sub = $1, oidc_provider = $2, avatar_url = $3, updated_at = NOW()
WHERE id = $4;

-- name: UpdateUserOidcProfile :exec
UPDATE users SET
    last_login = $1, avatar_url = COALESCE($2, avatar_url),
    first_name = COALESCE($3, first_name), last_name = COALESCE($4, last_name),
    role = $5, updated_at = NOW()
WHERE id = $6;

-- name: UpdateUserPreferences :exec
UPDATE users SET
    theme_preference = COALESCE(sqlc.narg('theme_preference')::text, theme_preference),
    color_theme = COALESCE(sqlc.narg('color_theme')::text, color_theme),
    ui_preferences = COALESCE(sqlc.narg('ui_preferences')::jsonb, ui_preferences),
    updated_at = NOW()
WHERE id = sqlc.arg('id');

-- name: UpdateTfaSecret :exec
UPDATE users SET tfa_secret = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateTfaEnabled :exec
UPDATE users SET tfa_enabled = $1, tfa_backup_codes = $2, updated_at = NOW() WHERE id = $3;

-- name: UpdateTfaBackupCodes :exec
UPDATE users SET tfa_backup_codes = $1, updated_at = NOW() WHERE id = $2;

-- name: DisableTfa :exec
UPDATE users SET tfa_enabled = false, tfa_secret = NULL, tfa_backup_codes = NULL, updated_at = NOW() WHERE id = $1;

-- name: CreateDiscordUser :exec
INSERT INTO users (
    id, username, email, password_hash, role, is_active, created_at, updated_at,
    tfa_enabled, first_name, last_name, theme_preference, color_theme,
    discord_id, discord_username, discord_avatar, discord_linked_at
) VALUES (
    $1, $2, $3, NULL, $4, true, $5, $6,
    false, $7, $8, 'dark', 'cyber_blue',
    $9, $10, $11, $12
);

-- name: UpdateUserDiscordLink :exec
UPDATE users SET
    discord_id = $1, discord_username = $2, discord_avatar = $3,
    discord_linked_at = $4, updated_at = NOW()
WHERE id = $5;

-- name: UpdateUserDiscordUnlink :exec
UPDATE users SET
    discord_id = NULL, discord_username = NULL, discord_avatar = NULL,
    discord_linked_at = NULL, updated_at = NOW()
WHERE id = $1;

-- name: UpdateUserDiscordProfile :exec
UPDATE users SET
    last_login = $1, discord_username = $2, discord_avatar = $3, updated_at = NOW()
WHERE id = $4;

-- name: SetNewsletterSubscribed :exec
UPDATE users SET newsletter_subscribed = true, newsletter_subscribed_at = NOW(), updated_at = NOW() WHERE id = $1;
