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
SELECT p.id, p.name, p.description, p.category, p.latest_version, p.created_at
FROM packages p
WHERE (sqlc.narg('search')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('search') || '%' OR p.description ILIKE '%' || sqlc.narg('search') || '%')
AND (sqlc.narg('category')::text IS NULL OR p.category = sqlc.narg('category'))
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
        AND (sqlc.narg('is_security_update')::text IS NULL OR (sqlc.narg('is_security_update') = 'true' AND hp.needs_update = true AND hp.is_security_update = true))
        AND (sqlc.narg('repository_id')::text IS NULL OR hp.source_repository_id = sqlc.narg('repository_id'))
    )
)
ORDER BY p.name ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountPackages :one
SELECT COUNT(*)::int FROM packages p
WHERE (sqlc.narg('search')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('search') || '%' OR p.description ILIKE '%' || sqlc.narg('search') || '%')
AND (sqlc.narg('category')::text IS NULL OR p.category = sqlc.narg('category'))
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
        AND (sqlc.narg('is_security_update')::text IS NULL OR (sqlc.narg('is_security_update') = 'true' AND hp.needs_update = true AND hp.is_security_update = true))
        AND (sqlc.narg('repository_id')::text IS NULL OR hp.source_repository_id = sqlc.narg('repository_id'))
    )
);

-- name: GetHostPackageStatsByPackageIDs :many
SELECT package_id, COUNT(*)::int as cnt FROM host_packages
WHERE package_id = ANY($1::text[])
AND (sqlc.narg('host_id')::text IS NULL OR host_id = sqlc.narg('host_id'))
GROUP BY package_id;

-- name: GetUpdatesCountByPackageIDs :many
SELECT package_id, COUNT(*)::int as cnt FROM host_packages
WHERE package_id = ANY($1::text[]) AND needs_update = true
AND (sqlc.narg('host_id')::text IS NULL OR host_id = sqlc.narg('host_id'))
GROUP BY package_id;

-- name: GetSecurityCountByPackageIDs :many
SELECT package_id, COUNT(*)::int as cnt FROM host_packages
WHERE package_id = ANY($1::text[]) AND needs_update = true AND is_security_update = true
AND (sqlc.narg('host_id')::text IS NULL OR host_id = sqlc.narg('host_id'))
GROUP BY package_id;

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
SELECT hp.package_id, h.id as host_id, h.friendly_name, h.os_type,
    hp.current_version, hp.available_version, hp.needs_update, hp.is_security_update
FROM host_packages hp
JOIN hosts h ON h.id = hp.host_id
WHERE hp.package_id = ANY($1::text[])
AND (sqlc.narg('host_id')::text IS NULL OR hp.host_id = sqlc.narg('host_id'))
ORDER BY hp.needs_update DESC, h.friendly_name;

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
