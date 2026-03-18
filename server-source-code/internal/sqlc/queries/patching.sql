-- patch_runs
-- name: CreatePatchRun :exec
INSERT INTO patch_runs (id, host_id, job_id, patch_type, package_name, package_names, status, shell_output, triggered_by_user_id, dry_run, scheduled_at, policy_id, policy_name, policy_snapshot, validation_run_id, approved_by_user_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW(), NOW());

-- name: UpdatePatchRunValidated :exec
UPDATE patch_runs SET status = 'validated', shell_output = shell_output || $2, packages_affected = $3, completed_at = NOW(), updated_at = NOW() WHERE id = $1;

-- name: MarkValidationApproved :exec
UPDATE patch_runs SET status = 'approved', approved_by_user_id = $2, updated_at = NOW() WHERE id = $1 AND status IN ('validated', 'pending_validation');

-- name: SetPatchRunPolicySnapshot :exec
UPDATE patch_runs SET policy_id = $2, policy_name = $3, policy_snapshot = $4, updated_at = NOW() WHERE id = $1;

-- name: UpdatePatchRunScheduledAt :exec
UPDATE patch_runs SET scheduled_at = $2, updated_at = NOW() WHERE id = $1;

-- name: UpdatePatchRunPackagesAffected :exec
UPDATE patch_runs SET packages_affected = $2, updated_at = NOW() WHERE id = $1;

-- name: DeletePatchRun :exec
DELETE FROM patch_runs WHERE id = $1;

-- name: GetPatchRunByID :one
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username, au.username AS approved_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
LEFT JOIN users au ON pr.approved_by_user_id = au.id
WHERE pr.id = $1;

-- name: GetPatchRunByIDSimple :one
SELECT * FROM patch_runs WHERE id = $1;

-- name: UpdatePatchRunStarted :exec
-- Clear dry-run output fields so real-run output starts fresh.
UPDATE patch_runs SET status = 'running', started_at = NOW(), completed_at = NULL,
    shell_output = '', packages_affected = NULL, error_message = NULL, updated_at = NOW() WHERE id = $1;

-- name: ClearScheduledAt :exec
UPDATE patch_runs SET scheduled_at = NULL, updated_at = NOW() WHERE id = $1;

-- name: UpdatePatchRunProgress :exec
UPDATE patch_runs SET shell_output = shell_output || $2, updated_at = NOW() WHERE id = $1;

-- name: UpdatePatchRunCompleted :exec
UPDATE patch_runs SET status = 'completed', shell_output = shell_output || $2, completed_at = NOW(), updated_at = NOW() WHERE id = $1;

-- name: UpdatePatchRunFailed :exec
UPDATE patch_runs SET status = 'failed', shell_output = shell_output || $2, error_message = $3, completed_at = NOW(), updated_at = NOW() WHERE id = $1;

-- name: UpdatePatchRunStatus :exec
UPDATE patch_runs SET status = $2, updated_at = NOW() WHERE id = $1;

-- name: ListPatchRuns :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (sqlc.arg('host_id')::text = '' OR pr.host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR pr.status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR pr.patch_type = sqlc.arg('patch_type'))
ORDER BY pr.created_at DESC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- name: ListPatchRunsOrderByCreatedAtAsc :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (sqlc.arg('host_id')::text = '' OR pr.host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR pr.status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR pr.patch_type = sqlc.arg('patch_type'))
ORDER BY pr.created_at ASC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- name: ListPatchRunsOrderByStartedAt :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (sqlc.arg('host_id')::text = '' OR pr.host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR pr.status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR pr.patch_type = sqlc.arg('patch_type'))
ORDER BY COALESCE(pr.started_at, pr.created_at) DESC NULLS LAST, pr.created_at DESC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- name: ListPatchRunsOrderByStartedAtAsc :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (sqlc.arg('host_id')::text = '' OR pr.host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR pr.status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR pr.patch_type = sqlc.arg('patch_type'))
ORDER BY COALESCE(pr.started_at, pr.created_at) ASC NULLS LAST, pr.created_at DESC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- name: ListPatchRunsOrderByCompletedAt :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (sqlc.arg('host_id')::text = '' OR pr.host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR pr.status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR pr.patch_type = sqlc.arg('patch_type'))
ORDER BY COALESCE(pr.completed_at, pr.created_at) DESC NULLS LAST, pr.created_at DESC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- name: ListPatchRunsOrderByCompletedAtAsc :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (sqlc.arg('host_id')::text = '' OR pr.host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR pr.status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR pr.patch_type = sqlc.arg('patch_type'))
ORDER BY COALESCE(pr.completed_at, pr.created_at) ASC NULLS LAST, pr.created_at DESC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- name: ListPatchRunsOrderByStatus :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (sqlc.arg('host_id')::text = '' OR pr.host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR pr.status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR pr.patch_type = sqlc.arg('patch_type'))
ORDER BY pr.status ASC, pr.created_at DESC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- name: ListPatchRunsOrderByStatusDesc :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (sqlc.arg('host_id')::text = '' OR pr.host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR pr.status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR pr.patch_type = sqlc.arg('patch_type'))
ORDER BY pr.status DESC, pr.created_at DESC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- name: CountPatchRuns :one
SELECT COUNT(*) FROM patch_runs
WHERE (sqlc.arg('host_id')::text = '' OR host_id = sqlc.arg('host_id'))
  AND (sqlc.arg('status')::text = '' OR status = sqlc.arg('status'))
  AND (sqlc.arg('patch_type')::text = '' OR patch_type = sqlc.arg('patch_type'));

-- name: ListActivePatchRuns :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE pr.status IN ('queued', 'running', 'pending_validation', 'validated') AND (pr.dry_run = false OR pr.dry_run IS NULL OR pr.status IN ('pending_validation', 'validated'))
ORDER BY pr.created_at ASC;

-- name: CountPatchRunsTotal :one
SELECT COUNT(*) FROM patch_runs WHERE (dry_run = false OR dry_run IS NULL);

-- name: ListPatchRunsByStatus :many
SELECT status, COUNT(*)::int AS count FROM patch_runs WHERE (dry_run = false OR dry_run IS NULL) OR status IN ('pending_validation', 'validated') GROUP BY status;

-- name: ListRecentPatchRuns :many
SELECT pr.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname, u.username AS triggered_by_username
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
LEFT JOIN users u ON pr.triggered_by_user_id = u.id
WHERE (pr.dry_run = false OR pr.dry_run IS NULL)
ORDER BY pr.created_at DESC
LIMIT $1;

-- name: ListPatchRunsByPackage :many
SELECT pr.id, pr.host_id, pr.completed_at, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname
FROM patch_runs pr
LEFT JOIN hosts h ON pr.host_id = h.id
WHERE pr.status = 'completed'
  AND (pr.dry_run = false OR pr.dry_run IS NULL)
  AND (
    pr.package_name = sqlc.arg('package_name')
    OR (pr.package_names IS NOT NULL AND pr.package_names @> jsonb_build_array(sqlc.arg('package_name')::text))
    OR (pr.packages_affected IS NOT NULL AND pr.packages_affected @> jsonb_build_array(sqlc.arg('package_name')::text))
  )
ORDER BY pr.completed_at DESC NULLS LAST, pr.created_at DESC
LIMIT sqlc.arg('limit_arg') OFFSET sqlc.arg('offset_arg');

-- patch_policies
-- name: ListPatchPolicies :many
SELECT * FROM patch_policies ORDER BY name ASC;

-- name: GetPatchPolicyByID :one
SELECT * FROM patch_policies WHERE id = $1;

-- name: CreatePatchPolicy :exec
INSERT INTO patch_policies (id, name, description, patch_delay_type, delay_minutes, fixed_time_utc, timezone, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW());

-- name: UpdatePatchPolicy :exec
UPDATE patch_policies
SET name = $2, description = $3, patch_delay_type = $4, delay_minutes = $5, fixed_time_utc = $6, timezone = $7, updated_at = NOW()
WHERE id = $1;

-- name: DeletePatchPolicy :exec
DELETE FROM patch_policies WHERE id = $1;

-- patch_policy_assignments
-- name: GetDirectPatchPolicyAssignment :one
SELECT pp.* FROM patch_policy_assignments ppa
JOIN patch_policies pp ON ppa.patch_policy_id = pp.id
WHERE ppa.target_type = 'host' AND ppa.target_id = $1
ORDER BY ppa.created_at ASC
LIMIT 1;

-- name: GetHostGroupMemberships :many
SELECT host_group_id FROM host_group_memberships WHERE host_id = $1;

-- name: GetPatchPolicyByGroupAssignment :one
SELECT pp.* FROM patch_policy_assignments ppa
JOIN patch_policies pp ON ppa.patch_policy_id = pp.id
WHERE ppa.target_type = 'host_group' AND ppa.target_id = $1
ORDER BY ppa.created_at ASC
LIMIT 1;

-- name: ExistsPatchPolicyExclusion :one
SELECT EXISTS(SELECT 1 FROM patch_policy_exclusions WHERE patch_policy_id = $1 AND host_id = $2);

-- name: ListPatchPolicyAssignments :many
SELECT * FROM patch_policy_assignments WHERE patch_policy_id = $1 AND target_type = $2 AND target_id = $3;

-- name: ListPatchPolicyAssignmentsByPolicy :many
SELECT * FROM patch_policy_assignments WHERE patch_policy_id = $1 ORDER BY created_at ASC;

-- name: CreatePatchPolicyAssignment :exec
INSERT INTO patch_policy_assignments (id, patch_policy_id, target_type, target_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, NOW(), NOW());

-- name: DeletePatchPolicyAssignment :exec
DELETE FROM patch_policy_assignments WHERE id = $1;

-- name: GetPatchPolicyAssignmentByID :one
SELECT * FROM patch_policy_assignments WHERE id = $1;

-- patch_policy_exclusions
-- name: ListPatchPolicyExclusions :many
SELECT ppe.*, h.friendly_name AS host_friendly_name, h.hostname AS host_hostname
FROM patch_policy_exclusions ppe
LEFT JOIN hosts h ON ppe.host_id = h.id
WHERE ppe.patch_policy_id = $1;

-- name: CreatePatchPolicyExclusion :exec
INSERT INTO patch_policy_exclusions (id, patch_policy_id, host_id, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW());

-- name: DeletePatchPolicyExclusion :exec
DELETE FROM patch_policy_exclusions WHERE patch_policy_id = $1 AND host_id = $2;
