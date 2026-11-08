-- name: LockAdminSet :exec
SELECT pg_advisory_xact_lock(7262616101);

-- name: SetupCompleted :one
SELECT EXISTS (SELECT 1 FROM install_state) OR EXISTS (SELECT 1 FROM users) AS completed;

-- name: MarkSetupCompleted :execrows
INSERT INTO install_state (id, setup_completed_at) VALUES (1, CURRENT_TIMESTAMP)
ON CONFLICT (id) DO NOTHING;
