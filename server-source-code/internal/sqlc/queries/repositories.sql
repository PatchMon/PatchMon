-- name: GetRepositoryByID :one
SELECT * FROM repositories WHERE id = $1;

-- name: ListRepositories :many
SELECT * FROM repositories r
WHERE (sqlc.narg('host_id')::text IS NULL OR EXISTS (SELECT 1 FROM host_repositories hr WHERE hr.repository_id = r.id AND hr.host_id = sqlc.narg('host_id')))
AND (sqlc.narg('search')::text IS NULL OR r.name ILIKE '%' || sqlc.narg('search') || '%' OR r.url ILIKE '%' || sqlc.narg('search') || '%' OR r.distribution ILIKE '%' || sqlc.narg('search') || '%' OR COALESCE(r.description, '') ILIKE '%' || sqlc.narg('search') || '%')
AND (sqlc.narg('status')::text IS NULL OR (sqlc.narg('status') = 'active' AND r.is_active = true) OR (sqlc.narg('status') = 'inactive' AND r.is_active = false))
AND (sqlc.narg('type')::text IS NULL OR (sqlc.narg('type') = 'secure' AND r.is_secure = true) OR (sqlc.narg('type') = 'insecure' AND r.is_secure = false))
ORDER BY r.name ASC, r.url ASC;

-- name: GetHostCountsForRepos :many
SELECT hr.repository_id, h.id, h.friendly_name, h.status, hr.is_enabled, hr.last_checked
FROM host_repositories hr
JOIN hosts h ON h.id = hr.host_id
WHERE hr.repository_id = ANY($1::text[])
ORDER BY hr.repository_id, h.friendly_name;

-- name: GetHostRepositoriesByHostID :many
SELECT hr.id, hr.host_id, hr.repository_id, hr.is_enabled, hr.last_checked,
    r.id as repo_id, r.name as repo_name, r.url as repo_url,
    r.distribution as repo_distribution, r.components as repo_components,
    r.repo_type as repo_repo_type, r.is_active as repo_is_active,
    r.is_secure as repo_is_secure, r.priority as repo_priority,
    r.description as repo_description, r.created_at as repo_created_at,
    r.updated_at as repo_updated_at,
    h.id as host_id2, h.friendly_name as host_friendly_name
FROM host_repositories hr
JOIN repositories r ON r.id = hr.repository_id
JOIN hosts h ON h.id = hr.host_id
WHERE hr.host_id = $1
ORDER BY r.name ASC;

-- name: GetHostRepositoriesForRepo :many
SELECT hr.id, hr.host_id, hr.repository_id, hr.is_enabled, hr.last_checked,
    h.id as host_id2, h.friendly_name as host_friendly_name, h.hostname as host_hostname,
    h.ip as host_ip, h.os_type as host_os_type, h.os_version as host_os_version,
    h.status as host_status, h.last_update as host_last_update, h.needs_reboot as host_needs_reboot
FROM host_repositories hr
JOIN hosts h ON h.id = hr.host_id
WHERE hr.repository_id = $1
ORDER BY h.friendly_name ASC;

-- name: UpdateRepository :exec
UPDATE repositories SET
    name = $1,
    description = $2,
    is_active = $3,
    priority = $4,
    updated_at = NOW()
WHERE id = $5;

-- name: ToggleHostRepository :exec
UPDATE host_repositories SET is_enabled = $1, last_checked = NOW() WHERE host_id = $2 AND repository_id = $3;

-- name: GetHostRepositoryCountByHostIDs :many
SELECT host_id, COUNT(*)::int AS cnt
FROM host_repositories
WHERE host_id = ANY($1::text[])
GROUP BY host_id;

-- name: CountRepositories :one
SELECT COUNT(*)::int FROM repositories;

-- name: CountActiveRepositories :one
SELECT COUNT(*)::int FROM repositories WHERE is_active = true;

-- name: CountSecureRepositories :one
SELECT COUNT(*)::int FROM repositories WHERE is_secure = true;

-- name: CountEnabledHostRepositories :one
SELECT COUNT(*)::int FROM host_repositories WHERE is_enabled = true;

-- name: GetRepositoryForDelete :one
SELECT r.id, r.name, r.url, COUNT(hr.id)::int as count
FROM repositories r
LEFT JOIN host_repositories hr ON hr.repository_id = r.id
WHERE r.id = $1
GROUP BY r.id, r.name, r.url;

-- name: DeleteRepository :exec
DELETE FROM repositories WHERE id = $1;

-- name: ListOrphanedRepositories :many
SELECT id, name, url FROM repositories r
WHERE NOT EXISTS (SELECT 1 FROM host_repositories hr WHERE hr.repository_id = r.id);

-- name: DeleteRepositoriesByIDs :exec
DELETE FROM repositories WHERE id = ANY($1::text[]);

-- name: UpsertRepository :one
-- Replaces the previous GetRepositoryByURLDistComponents + InsertRepository
-- SELECT-then-INSERT pattern. Migration 000040 added a UNIQUE constraint on
-- (url, distribution, components), making this a true upsert and closing
-- the TOCTOU race where two concurrent host reports could both see "no row"
-- and both INSERT.
--
-- DO UPDATE touches non-key columns only, so the row lock taken is FOR NO
-- KEY UPDATE — safe vs concurrent FK FOR KEY SHARE locks held by
-- host_packages inserts pointing at this row.
--
-- Returns the canonical id whether the row was newly inserted, updated, or
-- (in the no-op case where every column matches) updated to identical
-- values. There is no skip-no-op WHERE here because reports rarely repeat
-- identical repository metadata across runs and the row count per host is
-- small (typically 1-10) — the WAL/lock cost of unconditional UPDATE is
-- negligible compared with packages, and avoiding the WHERE keeps RETURNING
-- always-populated for a simpler caller contract.
INSERT INTO repositories (id, name, url, distribution, components, repo_type, is_active, is_secure, priority, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
ON CONFLICT (url, distribution, components) DO UPDATE SET
    name        = EXCLUDED.name,
    repo_type   = EXCLUDED.repo_type,
    is_active   = EXCLUDED.is_active,
    is_secure   = EXCLUDED.is_secure,
    priority    = EXCLUDED.priority,
    description = COALESCE(EXCLUDED.description, repositories.description),
    updated_at  = NOW()
RETURNING id;

-- name: DeleteHostRepositoriesByHostID :exec
DELETE FROM host_repositories WHERE host_id = $1;

-- name: InsertHostRepository :exec
INSERT INTO host_repositories (id, host_id, repository_id, is_enabled, last_checked)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (host_id, repository_id) DO UPDATE SET is_enabled = EXCLUDED.is_enabled, last_checked = NOW();
