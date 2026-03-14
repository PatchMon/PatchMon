-- name: ListAlertConfig :many
SELECT
    ac.*,
    u.id AS auto_assign_user_id_val,
    u.username AS auto_assign_username,
    u.email AS auto_assign_email,
    u.first_name AS auto_assign_first_name,
    u.last_name AS auto_assign_last_name
FROM alert_config ac
LEFT JOIN users u ON ac.auto_assign_user_id = u.id
ORDER BY ac.alert_type ASC;

-- name: GetAlertConfigByType :one
SELECT
    ac.*,
    u.id AS auto_assign_user_id_val,
    u.username AS auto_assign_username,
    u.email AS auto_assign_email,
    u.first_name AS auto_assign_first_name,
    u.last_name AS auto_assign_last_name
FROM alert_config ac
LEFT JOIN users u ON ac.auto_assign_user_id = u.id
WHERE ac.alert_type = $1;

-- name: UpsertAlertConfig :one
INSERT INTO alert_config (
    id, alert_type, is_enabled, default_severity,
    auto_assign_enabled, auto_assign_user_id, auto_assign_rule, auto_assign_conditions,
    retention_days, auto_resolve_after_days, cleanup_resolved_only,
    notification_enabled, escalation_enabled, escalation_after_hours, metadata,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, COALESCE($15, '{}'::jsonb),
    NOW(), NOW()
)
ON CONFLICT (alert_type) DO UPDATE SET
    is_enabled = EXCLUDED.is_enabled,
    default_severity = EXCLUDED.default_severity,
    auto_assign_enabled = EXCLUDED.auto_assign_enabled,
    auto_assign_user_id = EXCLUDED.auto_assign_user_id,
    auto_assign_rule = EXCLUDED.auto_assign_rule,
    auto_assign_conditions = EXCLUDED.auto_assign_conditions,
    retention_days = EXCLUDED.retention_days,
    auto_resolve_after_days = EXCLUDED.auto_resolve_after_days,
    cleanup_resolved_only = EXCLUDED.cleanup_resolved_only,
    notification_enabled = EXCLUDED.notification_enabled,
    escalation_enabled = EXCLUDED.escalation_enabled,
    escalation_after_hours = EXCLUDED.escalation_after_hours,
    metadata = EXCLUDED.metadata,
    updated_at = NOW()
RETURNING *;
