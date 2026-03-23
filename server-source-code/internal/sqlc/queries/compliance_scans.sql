-- name: ListStalledComplianceScans :many
SELECT id, host_id, profile_id, started_at, status FROM compliance_scans
WHERE (status = 'running' OR (completed_at IS NULL AND status != 'failed'))
AND started_at < $1;

-- name: UpdateStalledComplianceScans :exec
UPDATE compliance_scans
SET status = 'failed', completed_at = NOW(), error_message = $2
WHERE (status = 'running' OR (completed_at IS NULL AND status != 'failed'))
AND started_at < $1;

-- name: ListComplianceScansByHost :many
SELECT cs.id, cs.host_id, cs.profile_id, cs.started_at, cs.completed_at, cs.status,
       cs.total_rules, cs.passed, cs.failed, cs.warnings, cs.skipped, cs.not_applicable,
       cs.score, cs.error_message, cs.created_at, cs.updated_at,
       cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE cs.host_id = $1
ORDER BY cs.completed_at DESC NULLS LAST
LIMIT $2 OFFSET $3;

-- name: CountComplianceScansByHost :one
SELECT COUNT(*) FROM compliance_scans WHERE host_id = $1;

-- name: GetLatestComplianceScanByHost :one
SELECT cs.id, cs.host_id, cs.profile_id, cs.started_at, cs.completed_at, cs.status,
       cs.total_rules, cs.passed, cs.failed, cs.warnings, cs.skipped, cs.not_applicable,
       cs.score, cs.error_message, cs.raw_output, cs.created_at, cs.updated_at,
       cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE cs.host_id = $1 AND cs.status = 'completed'
ORDER BY cs.completed_at DESC
LIMIT 1;

-- name: GetLatestCompletedScans :many
SELECT cs.id, cs.host_id, cs.profile_id, cs.completed_at, cs.passed, cs.failed,
       cs.warnings, cs.skipped, cs.not_applicable, cs.score, cs.total_rules,
       cp.name as profile_name, cp.type as profile_type
FROM (
    SELECT DISTINCT ON (host_id, profile_id)
        id, host_id, profile_id, completed_at, passed, failed, warnings,
        skipped, not_applicable, score, total_rules
    FROM compliance_scans
    WHERE status = 'completed'
    ORDER BY host_id, profile_id, completed_at DESC
) cs
JOIN compliance_profiles cp ON cp.id = cs.profile_id;

-- name: GetLatestCompletedScansByProfile :many
SELECT cs.id, cs.host_id, cs.profile_id, cs.completed_at, cs.passed, cs.failed,
       cs.warnings, cs.skipped, cs.not_applicable, cs.score, cs.total_rules,
       cp.name as profile_name, cp.type as profile_type
FROM (
    SELECT DISTINCT ON (host_id)
        id, host_id, profile_id, completed_at, passed, failed, warnings,
        skipped, not_applicable, score, total_rules
    FROM compliance_scans
    WHERE status = 'completed' AND profile_id = $1
    ORDER BY host_id, completed_at DESC
) cs
JOIN compliance_profiles cp ON cp.id = cs.profile_id;

-- name: ListComplianceScansHistory :many
SELECT cs.id, cs.host_id, cs.status, cs.started_at, cs.completed_at,
       cs.total_rules, cs.passed, cs.failed, cs.warnings, cs.skipped, cs.not_applicable,
       cs.score, cs.error_message,
       h.hostname, h.friendly_name,
       cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN hosts h ON h.id = cs.host_id
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE (sqlc.narg('status')::text IS NULL OR cs.status = sqlc.narg('status'))
  AND (sqlc.narg('host_id')::text IS NULL OR cs.host_id = sqlc.narg('host_id'))
  AND (sqlc.narg('profile_type')::text IS NULL OR cp.type = sqlc.narg('profile_type'))
ORDER BY cs.started_at DESC
LIMIT $1 OFFSET $2;

-- name: CountComplianceScansHistory :one
SELECT COUNT(*)
FROM compliance_scans cs
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE (sqlc.narg('status')::text IS NULL OR cs.status = sqlc.narg('status'))
  AND (sqlc.narg('host_id')::text IS NULL OR cs.host_id = sqlc.narg('host_id'))
  AND (sqlc.narg('profile_type')::text IS NULL OR cp.type = sqlc.narg('profile_type'));

-- name: ListActiveComplianceScans :many
SELECT cs.id, cs.host_id, cs.profile_id, cs.started_at, cs.status,
       h.hostname, h.friendly_name, h.api_id,
       cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN hosts h ON h.id = cs.host_id
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE cs.status = 'running' OR (cs.completed_at IS NULL AND cs.status != 'failed')
ORDER BY cs.started_at DESC;

-- name: ListStalledComplianceScansWithDetails :many
SELECT cs.id, cs.host_id, cs.profile_id, cs.started_at, cs.status,
       h.hostname, h.friendly_name, h.api_id,
       cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN hosts h ON h.id = cs.host_id
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE (cs.status = 'running' OR (cs.completed_at IS NULL AND cs.status != 'failed'))
  AND cs.started_at < $1
ORDER BY cs.started_at ASC;

-- name: GetComplianceScanByID :one
SELECT cs.id, cs.host_id, cs.profile_id, cs.started_at, cs.completed_at, cs.status,
       cs.total_rules, cs.passed, cs.failed, cs.warnings, cs.skipped, cs.not_applicable,
       cs.score, cs.error_message, cs.raw_output, cs.created_at, cs.updated_at,
       cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE cs.id = $1;

-- name: CreateComplianceScan :one
INSERT INTO compliance_scans (
    id, host_id, profile_id, started_at, completed_at, status,
    total_rules, passed, failed, warnings, skipped, not_applicable,
    score, error_message, raw_output, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
RETURNING id, host_id, profile_id, started_at, completed_at, status,
          total_rules, passed, failed, warnings, skipped, not_applicable,
          score, error_message, raw_output, created_at, updated_at;

-- name: DeleteRunningComplianceScansByHost :exec
DELETE FROM compliance_scans
WHERE host_id = $1 AND status = 'running';

-- name: DeletePreviousCompletedScansByHostAndProfile :exec
DELETE FROM compliance_scans
WHERE host_id = $1 AND profile_id = $2 AND status = 'completed';

-- name: GetRecentComplianceScans :many
SELECT cs.id, cs.host_id, cs.profile_id, cs.started_at, cs.completed_at, cs.status,
       cs.total_rules, cs.passed, cs.failed, cs.warnings, cs.skipped, cs.not_applicable,
       cs.score, cs.error_message,
       h.hostname, h.friendly_name,
       cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN hosts h ON h.id = cs.host_id
JOIN compliance_profiles cp ON cp.id = cs.profile_id
ORDER BY cs.completed_at DESC NULLS LAST
LIMIT 10;

-- name: GetComplianceScansForTrends :many
SELECT cs.completed_at, cs.score, cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE cs.host_id = $1 AND cs.status = 'completed' AND cs.completed_at >= $2
ORDER BY cs.completed_at ASC;

-- name: GetLatestComplianceScanByHostAndType :one
SELECT cs.id, cs.host_id, cs.profile_id, cs.started_at, cs.completed_at, cs.status,
       cs.total_rules, cs.passed, cs.failed, cs.warnings, cs.skipped, cs.not_applicable,
       cs.score, cs.error_message, cs.raw_output, cs.created_at, cs.updated_at,
       cp.name as profile_name, cp.type as profile_type
FROM compliance_scans cs
JOIN compliance_profiles cp ON cp.id = cs.profile_id
WHERE cs.host_id = $1 AND cs.status = 'completed' AND cp.type = $2
ORDER BY cs.completed_at DESC
LIMIT 1;
