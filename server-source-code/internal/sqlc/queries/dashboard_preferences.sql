-- name: ListDashboardPreferencesByUserID :many
SELECT id, user_id, card_id, enabled, "order", col_span, created_at, updated_at
FROM dashboard_preferences
WHERE user_id = $1
ORDER BY "order" ASC;

-- name: DeleteDashboardPreferencesByUserID :exec
DELETE FROM dashboard_preferences WHERE user_id = $1;

-- name: InsertDashboardPreference :exec
INSERT INTO dashboard_preferences (id, user_id, card_id, enabled, "order", col_span, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetDashboardLayout :one
SELECT user_id, stats_columns, charts_columns, updated_at
FROM dashboard_layout
WHERE user_id = $1;

-- name: UpsertDashboardLayout :exec
INSERT INTO dashboard_layout (user_id, stats_columns, charts_columns, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE SET
    stats_columns = EXCLUDED.stats_columns,
    charts_columns = EXCLUDED.charts_columns,
    updated_at = EXCLUDED.updated_at;
