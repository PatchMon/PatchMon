-- name: ListAlertHistoryByAlertID :many
SELECT
    ah.id,
    ah.alert_id,
    ah.user_id,
    ah.action,
    ah.metadata,
    ah.created_at,
    u.id AS user_id_val,
    u.username,
    u.email,
    u.first_name,
    u.last_name
FROM alert_history ah
LEFT JOIN users u ON ah.user_id = u.id
WHERE ah.alert_id = $1
ORDER BY ah.created_at DESC;

-- name: GetLatestAlertHistoryForAlerts :many
SELECT DISTINCT ON (ah.alert_id)
    ah.alert_id,
    ah.id,
    ah.user_id,
    ah.action,
    ah.metadata,
    ah.created_at,
    u.id AS user_id_val,
    u.username,
    u.email,
    u.first_name,
    u.last_name
FROM alert_history ah
LEFT JOIN users u ON ah.user_id = u.id
WHERE ah.alert_id = ANY($1::text[])
ORDER BY ah.alert_id, ah.created_at DESC;

-- name: InsertAlertHistory :one
INSERT INTO alert_history (id, alert_id, user_id, action, metadata, created_at)
VALUES ($1, $2, $3, $4, COALESCE($5, '{}'::jsonb), NOW())
RETURNING *;
