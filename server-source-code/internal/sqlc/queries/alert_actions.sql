-- name: ListAlertActions :many
SELECT * FROM alert_actions ORDER BY display_name ASC;

-- name: GetAlertActionByName :one
SELECT * FROM alert_actions WHERE name = $1;
