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

-- name: ListPackages :many
WITH filtered_packages AS (
    SELECT p.id, p.name, p.description, p.category, p.latest_version, p.created_at
    FROM packages p
    WHERE (sqlc.narg('search')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('search') || '%' OR p.description ILIKE '%' || sqlc.narg('search') || '%')
    AND (sqlc.narg('category')::text IS NULL OR p.category = sqlc.narg('category'))
    AND (
        sqlc.narg('is_security_update')::text IS DISTINCT FROM 'false'
        OR NOT EXISTS (
            SELECT 1 FROM host_packages hp_security
            WHERE hp_security.package_id = p.id
            AND hp_security.needs_update = true
            AND hp_security.is_security_update = true
        )
    )
    AND (
        sqlc.narg('host_id')::text IS NULL
        AND sqlc.narg('needs_update')::text IS NULL
        AND sqlc.narg('is_security_update')::text IS NULL
        AND sqlc.narg('repository_id')::text IS NULL
        OR EXISTS (
            SELECT 1 FROM host_packages hp
            WHERE hp.package_id = p.id
            AND (sqlc.narg('host_id')::text IS NULL OR hp.host_id = sqlc.narg('host_id'))
            AND (sqlc.narg('needs_update')::text IS NULL OR (sqlc.narg('needs_update') = 'true' AND hp.needs_update = true))
            AND (
                sqlc.narg('is_security_update')::text IS NULL
                OR (sqlc.narg('is_security_update') = 'true' AND hp.needs_update = true AND hp.is_security_update = true)
                OR (sqlc.narg('is_security_update') = 'false' AND hp.needs_update = true AND hp.is_security_update = false)
            )
            AND (sqlc.narg('repository_id')::text IS NULL OR hp.source_repository_id = sqlc.narg('repository_id'))
        )
    )
),
-- Per-package counts come from mv_package_stats (a materialised view of
-- per-package install / update / security counters refreshed every couple
-- of minutes by the asynq scheduler — see TypePackageStatsRefresh).
--
-- Why a matview rather than a fresh aggregate per request:
--   * Global GROUP BY over the full host_packages table (~1.3 M rows at
--     1k-host scale) needs ~140 MB work_mem to avoid disk spill and
--     still takes ~10 s for the aggregation.
--   * LEFT JOIN LATERAL with a per-package COUNT lookup is fast per
--     call but with ~2.3 M `packages` rows the outer driver costs
--     ~30 s before LIMIT can fire.
--   * mv_package_stats stores the counters keyed by package_id and is
--     joined here as a single indexed hash join. Sub-millisecond lookup
--     for the small page we LIMIT to. Trade-off: counters are stale by
--     up to the refresh interval (2 min) — acceptable on an admin page.
enriched_packages AS (
    SELECT fp.id,
           fp.name,
           fp.description,
           fp.category,
           fp.latest_version,
           fp.created_at,
           COALESCE(s.total_installs, 0)::int AS total_installs,
           COALESCE(s.updates_needed, 0)::int AS updates_needed,
           COALESCE(s.security_updates, 0)::int AS security_updates,
           CASE
               WHEN COALESCE(s.security_updates, 0) > 0 THEN 0
               WHEN COALESCE(s.updates_needed, 0) > 0 THEN 1
               ELSE 2
           END AS status_rank
    FROM filtered_packages fp
    LEFT JOIN mv_package_stats s ON s.package_id = fp.id
)
-- Return the per-package counters from mv_package_stats alongside the
-- core fields so the store can render the page response without firing
-- additional aggregate round-trips. These are global counts (i.e.
-- "this package is installed on N hosts across the fleet"), not
-- host-filtered — that matches the existing UX where the per-row
-- "Installed On" badge always shows the package's full footprint even
-- when a host filter is active in the table above.
SELECT id, name, description, category, latest_version, created_at,
       total_installs, updates_needed, security_updates
FROM enriched_packages
ORDER BY
    CASE WHEN sqlc.arg('sort_key')::text = 'name'          AND sqlc.arg('sort_dir')::text = 'asc'  THEN name END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'name'          AND sqlc.arg('sort_dir')::text = 'desc' THEN name END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'latestVersion' AND sqlc.arg('sort_dir')::text = 'asc'  THEN latest_version END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'latestVersion' AND sqlc.arg('sort_dir')::text = 'desc' THEN latest_version END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'packageHosts'  AND sqlc.arg('sort_dir')::text = 'asc'  THEN total_installs END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'packageHosts'  AND sqlc.arg('sort_dir')::text = 'desc' THEN total_installs END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'status'        AND sqlc.arg('sort_dir')::text = 'asc'  THEN status_rank END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'status'        AND sqlc.arg('sort_dir')::text = 'desc' THEN status_rank END DESC,
    name ASC,
    id ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountPackages :one
SELECT COUNT(*)::int FROM packages p
WHERE (sqlc.narg('search')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('search') || '%' OR p.description ILIKE '%' || sqlc.narg('search') || '%')
AND (sqlc.narg('category')::text IS NULL OR p.category = sqlc.narg('category'))
AND (
    sqlc.narg('is_security_update')::text IS DISTINCT FROM 'false'
    OR NOT EXISTS (
        SELECT 1 FROM host_packages hp_security
        WHERE hp_security.package_id = p.id
        AND hp_security.needs_update = true
        AND hp_security.is_security_update = true
    )
)
AND (
    sqlc.narg('host_id')::text IS NULL
    AND sqlc.narg('needs_update')::text IS NULL
    AND sqlc.narg('is_security_update')::text IS NULL
    AND sqlc.narg('repository_id')::text IS NULL
    OR EXISTS (
        SELECT 1 FROM host_packages hp
        WHERE hp.package_id = p.id
        AND (sqlc.narg('host_id')::text IS NULL OR hp.host_id = sqlc.narg('host_id'))
        AND (sqlc.narg('needs_update')::text IS NULL OR (sqlc.narg('needs_update') = 'true' AND hp.needs_update = true))
        AND (
            sqlc.narg('is_security_update')::text IS NULL
            OR (sqlc.narg('is_security_update') = 'true' AND hp.needs_update = true AND hp.is_security_update = true)
            OR (sqlc.narg('is_security_update') = 'false' AND hp.needs_update = true AND hp.is_security_update = false)
        )
        AND (sqlc.narg('repository_id')::text IS NULL OR hp.source_repository_id = sqlc.narg('repository_id'))
    )
);

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
