-- name: ListAlerts :many
SELECT
    a.id,
    a.type,
    a.severity,
    a.title,
    a.message,
    a.metadata,
    a.is_active,
    a.assigned_to_user_id,
    a.resolved_at,
    a.resolved_by_user_id,
    a.created_at,
    a.updated_at,
    u.id AS assigned_user_id,
    u.username AS assigned_username,
    u.email AS assigned_email,
    u.first_name AS assigned_first_name,
    u.last_name AS assigned_last_name
FROM alerts a
LEFT JOIN users u ON a.assigned_to_user_id = u.id
ORDER BY a.created_at DESC;

-- name: ListAlertsAssignedTo :many
SELECT
    a.id,
    a.type,
    a.severity,
    a.title,
    a.message,
    a.metadata,
    a.is_active,
    a.assigned_to_user_id,
    a.resolved_at,
    a.resolved_by_user_id,
    a.created_at,
    a.updated_at,
    u.id AS assigned_user_id,
    u.username AS assigned_username,
    u.email AS assigned_email,
    u.first_name AS assigned_first_name,
    u.last_name AS assigned_last_name
FROM alerts a
LEFT JOIN users u ON a.assigned_to_user_id = u.id
WHERE a.assigned_to_user_id = $1
ORDER BY a.created_at DESC;

-- name: GetAlertByID :one
SELECT
    a.id,
    a.type,
    a.severity,
    a.title,
    a.message,
    a.metadata,
    a.is_active,
    a.assigned_to_user_id,
    a.resolved_at,
    a.resolved_by_user_id,
    a.created_at,
    a.updated_at,
    u.id AS assigned_user_id,
    u.username AS assigned_username,
    u.email AS assigned_email,
    u.first_name AS assigned_first_name,
    u.last_name AS assigned_last_name
FROM alerts a
LEFT JOIN users u ON a.assigned_to_user_id = u.id
WHERE a.id = $1;

-- name: CreateAlert :one
INSERT INTO alerts (id, type, severity, title, message, metadata, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, COALESCE($6::jsonb, '{}'::jsonb), COALESCE($7, true), NOW(), NOW())
RETURNING *;

-- name: UpdateAlertResolved :exec
UPDATE alerts
SET is_active = $2, resolved_at = $3, resolved_by_user_id = $4, updated_at = NOW()
WHERE id = $1;

-- name: UpdateAlertUnresolve :exec
UPDATE alerts
SET is_active = true, resolved_at = NULL, resolved_by_user_id = NULL, updated_at = NOW()
WHERE id = $1;

-- name: UpdateAlertAssignment :exec
UPDATE alerts
SET assigned_to_user_id = $2, is_active = true, resolved_at = NULL, resolved_by_user_id = NULL, updated_at = NOW()
WHERE id = $1;

-- name: UpdateAlertUnassign :exec
UPDATE alerts
SET assigned_to_user_id = NULL, updated_at = NOW()
WHERE id = $1;

-- name: UpdateAlert :exec
UPDATE alerts
SET updated_at = NOW()
WHERE id = $1;

-- name: DeleteAlert :exec
DELETE FROM alerts WHERE id = $1;

-- name: DeleteAlertsByIDs :exec
DELETE FROM alerts WHERE id = ANY($1::text[]);

-- name: GetAlertStatsBySeverity :many
SELECT severity, COUNT(*)::int AS count
FROM alerts
WHERE is_active = true AND resolved_at IS NULL
GROUP BY severity;

-- name: GetAlertsForCleanup :many
SELECT id, type, created_at
FROM alerts
WHERE type = $1
  AND created_at < $2
  AND is_active = false
  AND (NOT $3 OR resolved_at IS NOT NULL);

-- name: ListActiveAlertsByType :many
SELECT id, type, metadata FROM alerts WHERE type = $1 AND is_active = true;

-- name: GetAlertsToAutoResolve :many
SELECT id, type, created_at FROM alerts
WHERE type = $1 AND is_active = true AND created_at < $2;
