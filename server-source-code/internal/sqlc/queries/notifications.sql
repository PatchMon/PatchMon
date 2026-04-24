-- name: ListNotificationDestinations :many
SELECT * FROM notification_destinations ORDER BY display_name;

-- name: GetNotificationDestinationByID :one
SELECT * FROM notification_destinations WHERE id = $1;

-- name: CreateNotificationDestination :one
INSERT INTO notification_destinations (id, channel_type, display_name, config_encrypted, enabled, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
RETURNING *;

-- name: UpdateNotificationDestination :one
UPDATE notification_destinations
SET display_name = $2, config_encrypted = $3, enabled = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteNotificationDestination :exec
DELETE FROM notification_destinations WHERE id = $1;

-- name: ListNotificationRoutes :many
SELECT
    r.id, r.destination_id, r.event_types, r.min_severity, r.host_group_ids, r.host_ids, r.match_rules, r.enabled AS route_enabled,
    r.created_at, r.updated_at,
    d.channel_type, d.display_name AS destination_display_name
FROM notification_routes r
JOIN notification_destinations d ON d.id = r.destination_id
ORDER BY d.display_name;

-- name: ListNotificationRoutesForEvent :many
SELECT
    r.id, r.destination_id, r.event_types, r.min_severity, r.host_group_ids, r.host_ids, r.match_rules, r.enabled AS route_enabled,
    r.created_at, r.updated_at,
    d.channel_type, d.display_name AS destination_display_name, d.config_encrypted, d.enabled AS destination_enabled
FROM notification_routes r
JOIN notification_destinations d ON d.id = r.destination_id
WHERE r.enabled = true AND d.enabled = true
  AND (r.event_types @> to_jsonb($1::text) OR r.event_types @> '["*"]'::jsonb)
ORDER BY r.id;

-- name: GetNotificationRouteByID :one
SELECT * FROM notification_routes WHERE id = $1;

-- name: CreateNotificationRoute :one
INSERT INTO notification_routes (id, destination_id, event_types, min_severity, host_group_ids, host_ids, match_rules, enabled, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
RETURNING *;

-- name: UpdateNotificationRoute :one
UPDATE notification_routes
SET destination_id = $2, event_types = $3, min_severity = $4, host_group_ids = $5, host_ids = $6, match_rules = $7, enabled = $8, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteNotificationRoute :exec
DELETE FROM notification_routes WHERE id = $1;

-- name: InsertNotificationDeliveryLog :one
INSERT INTO notification_delivery_log (
    id, event_fingerprint, reference_type, reference_id, destination_id, event_type, status, error_message, attempt_count, provider_message_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
RETURNING *;

-- name: ListNotificationDeliveryLog :many
SELECT * FROM notification_delivery_log ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: ListScheduledReports :many
SELECT * FROM scheduled_reports ORDER BY name;

-- name: GetScheduledReportByID :one
SELECT * FROM scheduled_reports WHERE id = $1;

-- name: ListScheduledReportsDue :many
SELECT * FROM scheduled_reports
WHERE enabled = true AND (next_run_at IS NULL OR next_run_at <= $1)
ORDER BY next_run_at NULLS FIRST;

-- name: CreateScheduledReport :one
INSERT INTO scheduled_reports (id, name, cron_expr, enabled, definition, destination_ids, timezone, next_run_at, last_run_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
RETURNING *;

-- name: UpdateScheduledReport :one
UPDATE scheduled_reports
SET name = $2, cron_expr = $3, enabled = $4, definition = $5, destination_ids = $6, timezone = $7, next_run_at = $8, last_run_at = $9, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateScheduledReportRunTimes :exec
UPDATE scheduled_reports SET last_run_at = $2, next_run_at = $3, updated_at = NOW() WHERE id = $1;

-- name: DeleteScheduledReport :exec
DELETE FROM scheduled_reports WHERE id = $1;

-- name: InsertScheduledReportRun :one
INSERT INTO scheduled_report_runs (id, scheduled_report_id, run_at, status, error_message, summary_hash, created_at)
VALUES ($1, $2, NOW(), $3, $4, $5, NOW())
RETURNING *;
