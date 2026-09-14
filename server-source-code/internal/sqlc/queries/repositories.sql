-- name: GetRepositoryByID :one
SELECT * FROM repositories WHERE id = $1;

-- name: ListRepositories :many
WITH filtered_repositories AS (
    SELECT r.*
    FROM repositories r
    WHERE (sqlc.narg('host_id')::text IS NULL OR EXISTS (SELECT 1 FROM host_repositories hr WHERE hr.repository_id = r.id AND hr.host_id = sqlc.narg('host_id')))
    AND (sqlc.narg('search')::text IS NULL OR r.name ILIKE '%' || sqlc.narg('search') || '%' OR r.url ILIKE '%' || sqlc.narg('search') || '%' OR r.distribution ILIKE '%' || sqlc.narg('search') || '%' OR COALESCE(r.description, '') ILIKE '%' || sqlc.narg('search') || '%')
    AND (sqlc.narg('status')::text IS NULL OR (sqlc.narg('status') = 'active' AND r.is_active = true) OR (sqlc.narg('status') = 'inactive' AND r.is_active = false))
    AND (sqlc.narg('type')::text IS NULL OR (sqlc.narg('type') = 'secure' AND r.is_secure = true) OR (sqlc.narg('type') = 'insecure' AND r.is_secure = false))
),
repo_counts AS (
    SELECT hr.repository_id, COUNT(*)::int AS host_count
    FROM host_repositories hr
    JOIN filtered_repositories fr ON fr.id = hr.repository_id
    GROUP BY hr.repository_id
),
enriched_repositories AS (
    SELECT fr.*, COALESCE(rc.host_count, 0)::int AS host_count
    FROM filtered_repositories fr
    LEFT JOIN repo_counts rc ON rc.repository_id = fr.id
)
SELECT id, name, url, distribution, components, repo_type, is_active, is_secure, priority, description, created_at, updated_at
FROM enriched_repositories
ORDER BY
    CASE WHEN sqlc.arg('sort_key')::text = 'name'         AND sqlc.arg('sort_dir')::text = 'asc'  THEN name END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'name'         AND sqlc.arg('sort_dir')::text = 'desc' THEN name END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'url'          AND sqlc.arg('sort_dir')::text = 'asc'  THEN url END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'url'          AND sqlc.arg('sort_dir')::text = 'desc' THEN url END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'distribution' AND sqlc.arg('sort_dir')::text = 'asc'  THEN distribution END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'distribution' AND sqlc.arg('sort_dir')::text = 'desc' THEN distribution END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'security'     AND sqlc.arg('sort_dir')::text = 'asc'  THEN is_secure END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'security'     AND sqlc.arg('sort_dir')::text = 'desc' THEN is_secure END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'status'       AND sqlc.arg('sort_dir')::text = 'asc'  THEN is_active END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'status'       AND sqlc.arg('sort_dir')::text = 'desc' THEN is_active END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'hostCount'    AND sqlc.arg('sort_dir')::text = 'asc'  THEN host_count END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'hostCount'    AND sqlc.arg('sort_dir')::text = 'desc' THEN host_count END DESC,
    name ASC,
    url ASC,
    id ASC
LIMIT sqlc.arg('row_limit')::int
OFFSET sqlc.arg('row_offset')::int;

-- name: CountRepositoriesForList :one
SELECT COUNT(*)::int
FROM repositories r
WHERE (sqlc.narg('host_id')::text IS NULL OR EXISTS (SELECT 1 FROM host_repositories hr WHERE hr.repository_id = r.id AND hr.host_id = sqlc.narg('host_id')))
AND (sqlc.narg('search')::text IS NULL OR r.name ILIKE '%' || sqlc.narg('search') || '%' OR r.url ILIKE '%' || sqlc.narg('search') || '%' OR r.distribution ILIKE '%' || sqlc.narg('search') || '%' OR COALESCE(r.description, '') ILIKE '%' || sqlc.narg('search') || '%')
AND (sqlc.narg('status')::text IS NULL OR (sqlc.narg('status') = 'active' AND r.is_active = true) OR (sqlc.narg('status') = 'inactive' AND r.is_active = false))
AND (sqlc.narg('type')::text IS NULL OR (sqlc.narg('type') = 'secure' AND r.is_secure = true) OR (sqlc.narg('type') = 'insecure' AND r.is_secure = false));

-- name: GetHostCountsForRepos :many
SELECT hr.repository_id, h.id, h.friendly_name, h.status, hr.is_enabled, hr.last_checked
FROM host_repositories hr
JOIN hosts h ON h.id = hr.host_id
WHERE hr.repository_id = ANY($1::text[])
ORDER BY hr.repository_id, h.friendly_name;

-- name: GetRepoCountsForRepos :many
SELECT hr.repository_id,
    COUNT(*)::int AS host_count,
    COUNT(*) FILTER (WHERE hr.is_enabled)::int AS enabled_host_count,
    COUNT(*) FILTER (WHERE h.status = 'active')::int AS active_host_count
FROM host_repositories hr
JOIN hosts h ON h.id = hr.host_id
WHERE hr.repository_id = ANY($1::text[])
GROUP BY hr.repository_id;

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
-- Repository rows are shared across the whole fleet: every host on a distro
-- reports the same (url, distribution, components). Running this statement on
-- every report takes a FOR NO KEY UPDATE lock on ONE hot row and holds it
-- until the report transaction commits — which is after that transaction's
-- full host_packages delete and bulk insert. Contention scales with fleet
-- concurrency, not with the handful of repository rows per host, so at 64
-- concurrent reports they serialise behind the hot row until statement_timeout.
--
-- The skip-no-op WHERE below does NOT prevent that lock. PostgreSQL's
-- ON CONFLICT arbiter locks the conflicting tuple BEFORE it evaluates the
-- DO UPDATE WHERE, so a skipped no-op still holds the row lock to end of
-- transaction (verified: holder reports INSERT 0 0 while a concurrent
-- FOR NO KEY UPDATE on the same row blocks). The WHERE earns its keep by
-- avoiding the heap write, dead tuple and WAL when two reports race on the
-- same genuine change — not by avoiding locks.
--
-- Avoiding the lock is therefore the CALLER's job: upsertRepositoryResolvingID
-- reads the row first (a plain SELECT takes no lock) and only reaches this
-- statement when the row is absent or a column it would set actually differs.
-- Do not "simplify" the caller back to calling this unconditionally.
--
-- Consequence of the WHERE: RETURNING is NOT always populated. A skipped
-- no-op returns zero rows and this is a :one query, so the caller gets
-- pgx.ErrNoRows and MUST resolve the id via GetRepositoryByURLDistComponents.
-- Do NOT "fix" this with a same-statement fallback SELECT — both halves would
-- read at the same statement snapshot and miss rows a concurrent transaction
-- committed after the statement began.
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
WHERE
       repositories.name        IS DISTINCT FROM EXCLUDED.name
    OR repositories.repo_type   IS DISTINCT FROM EXCLUDED.repo_type
    OR repositories.is_active   IS DISTINCT FROM EXCLUDED.is_active
    OR repositories.is_secure   IS DISTINCT FROM EXCLUDED.is_secure
    OR repositories.priority    IS DISTINCT FROM EXCLUDED.priority
    OR repositories.description IS DISTINCT FROM COALESCE(EXCLUDED.description, repositories.description)
RETURNING id;

-- name: GetRepositoryByURLDistComponents :one
-- Lock-free read on the (url, distribution, components) unique key. Serves
-- two purposes for the report path, both described on UpsertRepository:
--   1. The pre-check that lets an unchanged report skip the upsert entirely,
--      so it never takes the hot-row lock. Hence the projection is exactly
--      the columns UpsertRepository's DO UPDATE would set.
--   2. Resolving the id when that upsert skipped its no-op DO UPDATE and
--      returned nothing. As a separate statement it reads at a fresh
--      snapshot and sees rows committed by concurrent reports.
--
-- Use (2) REQUIRES READ COMMITTED: only there does a new statement take a new
-- snapshot. At REPEATABLE READ or SERIALIZABLE it would reuse the transaction
-- snapshot and resolve nothing. The pool sets no TxOptions, so the server
-- default applies — do not raise it without revisiting this.
SELECT id, name, repo_type, is_active, is_secure, priority, description
FROM repositories
WHERE url = $1 AND distribution = $2 AND components = $3;

-- name: DeleteHostRepositoriesByHostID :exec
DELETE FROM host_repositories WHERE host_id = $1;

-- name: InsertHostRepository :exec
INSERT INTO host_repositories (id, host_id, repository_id, is_enabled, last_checked)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (host_id, repository_id) DO UPDATE SET is_enabled = EXCLUDED.is_enabled, last_checked = NOW();
