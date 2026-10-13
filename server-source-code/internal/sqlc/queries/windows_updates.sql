-- Windows Update (WUA) specific queries
-- These operate on host_packages rows where category='Windows Update' and wua_guid IS NOT NULL

-- name: GetHostWindowsUpdates :many
-- Returns all Windows Update entries for a host, most recently checked first.
SELECT hp.id, hp.host_id, hp.package_id, hp.current_version, hp.available_version,
    hp.needs_update, hp.is_security_update,
    hp.wua_guid, hp.wua_kb, hp.wua_severity, hp.wua_categories,
    hp.wua_description, hp.wua_support_url, hp.wua_revision_number,
    hp.wua_date_installed, hp.last_checked,
    p.name AS pkg_name, p.description AS pkg_description
FROM host_packages hp
JOIN packages p ON p.id = hp.package_id
WHERE hp.host_id = $1
  AND hp.wua_guid IS NOT NULL
ORDER BY hp.needs_update DESC, hp.is_security_update DESC, p.name ASC;

-- name: CountWindowsUpdatesByHostID :one
-- Counts pending Windows Updates for a host (for dashboard/stats).
SELECT
    COUNT(*) FILTER (WHERE needs_update = true)::int                          AS pending_count,
    COUNT(*) FILTER (WHERE needs_update = true AND is_security_update = true)::int AS security_count,
    COUNT(*) FILTER (WHERE needs_update = false)::int                         AS installed_count
FROM host_packages
WHERE host_id = $1 AND wua_guid IS NOT NULL;
