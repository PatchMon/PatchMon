-- name: ListJobHistoryByApiID :many
SELECT id, job_id, queue_name, job_name, host_id, api_id, status, attempt_number, error_message, output, created_at, updated_at, completed_at
FROM job_history
WHERE api_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: InsertJobHistory :exec
INSERT INTO job_history (id, job_id, queue_name, job_name, host_id, api_id, status, attempt_number, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW());

-- name: UpdateJobHistoryCompleted :exec
UPDATE job_history
SET status = 'completed', completed_at = NOW(), updated_at = NOW()
WHERE job_id = $1;

-- name: UpdateJobHistoryFailed :exec
UPDATE job_history
SET status = 'failed', error_message = $2, completed_at = NOW(), updated_at = NOW()
WHERE job_id = $1;

-- name: UpdateJobHistoryDelayed :exec
UPDATE job_history
SET status = 'delayed', updated_at = NOW()
WHERE job_id = $1;
