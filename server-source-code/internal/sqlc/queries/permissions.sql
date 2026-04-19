-- name: GetRolePermissions :one
SELECT * FROM role_permissions WHERE role = $1;

-- name: ListRoles :many
SELECT * FROM role_permissions ORDER BY role;

-- name: UpsertRolePermissions :one
INSERT INTO role_permissions (
    id, role, can_view_dashboard, can_view_hosts, can_manage_hosts,
    can_view_packages, can_manage_packages, can_view_users, can_manage_users,
    can_manage_superusers, can_view_reports, can_export_data, can_manage_settings,
    can_manage_notifications, can_view_notification_logs,
    can_manage_patching, can_manage_compliance, can_manage_docker,
    can_manage_alerts, can_manage_automation, can_use_remote_access,
    can_manage_billing,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20, $21, $22,
    CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
)
ON CONFLICT (role) DO UPDATE SET
    can_view_dashboard = EXCLUDED.can_view_dashboard,
    can_view_hosts = EXCLUDED.can_view_hosts,
    can_manage_hosts = EXCLUDED.can_manage_hosts,
    can_view_packages = EXCLUDED.can_view_packages,
    can_manage_packages = EXCLUDED.can_manage_packages,
    can_view_users = EXCLUDED.can_view_users,
    can_manage_users = EXCLUDED.can_manage_users,
    can_manage_superusers = EXCLUDED.can_manage_superusers,
    can_view_reports = EXCLUDED.can_view_reports,
    can_export_data = EXCLUDED.can_export_data,
    can_manage_settings = EXCLUDED.can_manage_settings,
    can_manage_notifications = EXCLUDED.can_manage_notifications,
    can_view_notification_logs = EXCLUDED.can_view_notification_logs,
    can_manage_patching = EXCLUDED.can_manage_patching,
    can_manage_compliance = EXCLUDED.can_manage_compliance,
    can_manage_docker = EXCLUDED.can_manage_docker,
    can_manage_alerts = EXCLUDED.can_manage_alerts,
    can_manage_automation = EXCLUDED.can_manage_automation,
    can_use_remote_access = EXCLUDED.can_use_remote_access,
    can_manage_billing = EXCLUDED.can_manage_billing,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: DeleteRolePermissions :exec
DELETE FROM role_permissions WHERE role = $1;

-- name: CountUsersByRole :one
SELECT COUNT(*) FROM users WHERE role = $1;
