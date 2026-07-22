-- name: ListSSHHostAccounts :many
SELECT * FROM ssh_host_accounts WHERE host_id = $1 ORDER BY linux_username;

-- name: GetSSHHostAccount :one
SELECT * FROM ssh_host_accounts WHERE host_id = $1 AND linux_username = $2;

-- name: UpsertSSHHostAccount :one
INSERT INTO ssh_host_accounts (id, host_id, linux_username, allow_sudo, created_at, updated_at)
VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (host_id, linux_username) DO UPDATE SET
    allow_sudo = EXCLUDED.allow_sudo,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: DeleteSSHHostAccount :exec
DELETE FROM ssh_host_accounts WHERE host_id = $1 AND linux_username = $2;

-- name: CreateSSHSession :one
INSERT INTO ssh_sessions (
    id, host_id, user_id, linux_username, transport, status, client_ip,
    user_agent, started_at, recorded
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP, $9)
RETURNING *;

-- name: GetSSHSession :one
SELECT * FROM ssh_sessions WHERE id = $1;

-- name: UpdateSSHSessionStatus :one
UPDATE ssh_sessions SET
    status = $2,
    ended_at = CASE WHEN $2 IN ('completed', 'failed', 'disconnected') THEN CURRENT_TIMESTAMP ELSE ended_at END,
    duration_ms = CASE WHEN $2 IN ('completed', 'failed', 'disconnected')
        THEN (EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - started_at)) * 1000)::BIGINT
        ELSE duration_ms END,
    failure_reason = $3,
    event_count = $4,
    recording_bytes = $5
WHERE id = $1
RETURNING *;

-- name: MarkSSHRecordingDeleted :exec
UPDATE ssh_sessions
SET recording_deleted_at = CURRENT_TIMESTAMP, recording_bytes = 0
WHERE id = $1;

-- name: ListSSHSessionRecordings :many
SELECT s.*, h.friendly_name AS host_name, u.username AS patchmon_username
FROM ssh_sessions s
JOIN hosts h ON h.id = s.host_id
JOIN users u ON u.id = s.user_id
WHERE s.recorded = true
  AND (sqlc.narg('host_id')::text IS NULL OR s.host_id = sqlc.narg('host_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR s.user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('linux_username')::text IS NULL OR s.linux_username = sqlc.narg('linux_username'))
  AND (sqlc.narg('status')::text IS NULL OR s.status = sqlc.narg('status'))
  AND (sqlc.narg('started_after')::timestamp IS NULL OR s.started_at >= sqlc.narg('started_after'))
  AND (sqlc.narg('started_before')::timestamp IS NULL OR s.started_at <= sqlc.narg('started_before'))
ORDER BY s.started_at DESC
LIMIT sqlc.arg('limit_count') OFFSET sqlc.arg('offset_count');

-- name: ListExpiredSSHRecordings :many
SELECT * FROM ssh_sessions
WHERE recorded = true
  AND recording_deleted_at IS NULL
  AND started_at < $1
ORDER BY started_at
LIMIT $2;

-- name: CreateSSHRecordingAccessAudit :one
INSERT INTO ssh_recording_access_audit (id, session_id, user_id, action, client_ip)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CountActiveSSHSessionForUser :one
SELECT COUNT(*) FROM ssh_sessions
WHERE user_id = $1 AND status IN ('opening', 'active');

-- name: CountActiveSSHSessionForHost :one
SELECT COUNT(*) FROM ssh_sessions
WHERE host_id = $1 AND status IN ('opening', 'active');
