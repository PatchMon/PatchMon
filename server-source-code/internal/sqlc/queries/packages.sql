-- name: GetPackageByID :one
SELECT * FROM packages WHERE id = $1;

-- name: GetCategories :many
SELECT DISTINCT category FROM packages WHERE category IS NOT NULL AND category != '' ORDER BY category;

-- name: ListNeedingUpdates :many
SELECT p.id as pkg_id, p.name as pkg_name, p.description, p.category, p.latest_version,
    hp.current_version, hp.available_version, hp.is_security_update,
    h.id as host_id, h.friendly_name, h.os_type
FROM packages p
JOIN host_packages hp ON hp.package_id = p.id AND hp.needs_update = true
JOIN hosts h ON h.id = hp.host_id
ORDER BY p.name;

-- ListPackages and CountPackages are intentionally NOT defined here.
-- They live as raw SQL builders in internal/store/packages_list_sql.go.
--
-- Why hand-rolled rather than sqlc:
--   1. ORDER BY needs a CASE-WHEN-per-sort-key dance to stay parameterised,
--      which forces a full sort over the entire filtered CTE before LIMIT
--      can fire — defeats any index-ordered scan + LIMIT pushdown.
--      Building "ORDER BY <whitelisted column> <dir>" in Go lets the
--      planner drive output from the existing btree on packages(name)
--      (and similar) for typical queries, killing the parallel-sort path
--      that blows Docker's default /dev/shm.
--   2. The host_packages EXISTS / NOT EXISTS branches are only relevant
--      when the corresponding filter param is set; emitting them
--      conditionally in Go produces a much tighter predicate that the
--      planner can prune cheaply.
-- Per-package counters still come from mv_package_stats (refreshed every
-- ~2 min by an asynq scheduler — see TypePackageStatsRefresh) so we avoid
-- per-request aggregation over host_packages.

-- (Removed) GetHostPackageStatsByPackageIDs / GetUpdatesCountByPackageIDs /
-- GetSecurityCountByPackageIDs — superseded by mv_package_stats. The
-- per-package counters returned to the Packages list page now come from
-- ListPackages itself (which joins the matview), so the previous
-- per-id aggregate round-trips are no longer needed.

-- name: GetHostPackagesWithHostsByPackageID :many
SELECT hp.id, hp.host_id, hp.package_id, hp.current_version, hp.available_version,
    hp.needs_update, hp.is_security_update, hp.last_checked,
    hp.source_repository_id,
    r.name as source_repo_name, r.url as source_repo_url,
    h.friendly_name as host_friendly_name, h.hostname as host_hostname, h.ip as host_ip,
    h.os_type as host_os_type, h.os_version as host_os_version,
    h.last_update as host_last_update, h.needs_reboot as host_needs_reboot
FROM host_packages hp
JOIN hosts h ON h.id = hp.host_id
LEFT JOIN repositories r ON r.id = hp.source_repository_id
WHERE hp.package_id = $1
ORDER BY hp.needs_update DESC;

-- name: CountHostsForPackage :one
SELECT COUNT(*)::int FROM host_packages hp
JOIN hosts h ON h.id = hp.host_id
WHERE hp.package_id = $1
AND (sqlc.narg('search')::text IS NULL OR h.friendly_name ILIKE '%' || sqlc.narg('search') || '%' OR h.hostname ILIKE '%' || sqlc.narg('search') || '%' OR hp.current_version ILIKE '%' || sqlc.narg('search') || '%' OR hp.available_version ILIKE '%' || sqlc.narg('search') || '%')
AND (sqlc.narg('needs_update')::bool IS NULL OR hp.needs_update = sqlc.narg('needs_update'));

-- name: GetHostRefsForPackageIDs :many
WITH ranked_refs AS (
    SELECT hp.package_id, h.id as host_id, h.friendly_name, h.os_type,
        hp.current_version, hp.available_version, hp.needs_update, hp.is_security_update,
        row_number() OVER (
            PARTITION BY hp.package_id
            ORDER BY hp.needs_update DESC, h.friendly_name ASC, h.id ASC
        ) AS rn
    FROM host_packages hp
    JOIN hosts h ON h.id = hp.host_id
    WHERE hp.package_id = ANY($1::text[])
    AND (sqlc.narg('host_id')::text IS NULL OR hp.host_id = sqlc.narg('host_id'))
)
SELECT package_id, host_id, friendly_name, os_type, current_version, available_version, needs_update, is_security_update
FROM ranked_refs
WHERE rn <= 10
ORDER BY package_id, needs_update DESC, friendly_name ASC;

-- name: GetSourceReposByPackageIDs :many
SELECT DISTINCT hp.package_id, r.id as repo_id, r.name as repo_name, r.url as repo_url, r.repo_type
FROM host_packages hp
JOIN repositories r ON r.id = hp.source_repository_id
WHERE hp.package_id = ANY($1::text[])
AND (sqlc.narg('host_id')::text IS NULL OR hp.host_id = sqlc.narg('host_id'))
ORDER BY hp.package_id, r.name;

-- name: ListHostsForPackage :many
SELECT h.id, h.friendly_name, h.hostname, h.os_type, h.os_version, h.last_update, h.needs_reboot, h.reboot_reason,
    hp.current_version, hp.available_version, hp.needs_update, hp.is_security_update, hp.last_checked,
    hp.source_repository_id, r.name as source_repo_name
FROM host_packages hp
JOIN hosts h ON h.id = hp.host_id
LEFT JOIN repositories r ON r.id = hp.source_repository_id
WHERE hp.package_id = $1
AND (sqlc.narg('search')::text IS NULL OR h.friendly_name ILIKE '%' || sqlc.narg('search') || '%' OR h.hostname ILIKE '%' || sqlc.narg('search') || '%' OR hp.current_version ILIKE '%' || sqlc.narg('search') || '%' OR hp.available_version ILIKE '%' || sqlc.narg('search') || '%')
AND (sqlc.narg('needs_update')::bool IS NULL OR hp.needs_update = sqlc.narg('needs_update'))
ORDER BY hp.needs_update DESC, h.friendly_name ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetHostPackageStatsByHostIDs :many
SELECT host_id,
    COUNT(*)::int AS total,
    SUM(CASE WHEN needs_update THEN 1 ELSE 0 END)::int AS outdated,
    SUM(CASE WHEN needs_update AND is_security_update THEN 1 ELSE 0 END)::int AS security
FROM host_packages
WHERE host_id = ANY($1::text[])
GROUP BY host_id;

-- name: ListOrphanedPackages :many
SELECT id, name, description, category, latest_version FROM packages p
WHERE NOT EXISTS (SELECT 1 FROM host_packages hp WHERE hp.package_id = p.id);

-- name: DeletePackagesByIDs :exec
DELETE FROM packages WHERE id = ANY($1::text[]);

-- name: GetPendingUpdateCountsPerHost :many
SELECT
    hp.host_id,
    SUM(CASE WHEN hp.needs_update THEN 1 ELSE 0 END)::int AS pending_count,
    SUM(CASE WHEN hp.needs_update AND hp.is_security_update THEN 1 ELSE 0 END)::int AS security_count
FROM host_packages hp
JOIN hosts h ON h.id = hp.host_id AND h.status = 'active'
GROUP BY hp.host_id;
