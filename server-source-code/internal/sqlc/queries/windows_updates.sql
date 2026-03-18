-- Windows Update (WUA) specific queries
-- These operate on host_packages rows where category='Windows Update' and wua_guid IS NOT NULL

-- name: GetHostWindowsUpdates :many
-- Returns all Windows Update entries for a host, most recently checked first.
SELECT hp.id, hp.host_id, hp.package_id, hp.current_version, hp.available_version,
    hp.needs_update, hp.is_security_update,
    hp.wua_guid, hp.wua_kb, hp.wua_severity, hp.wua_categories,
    hp.wua_description, hp.wua_support_url, hp.wua_revision_number,
    hp.wua_date_installed, hp.wua_install_result, hp.last_checked,
    p.name AS pkg_name, p.description AS pkg_description
FROM host_packages hp
JOIN packages p ON p.id = hp.package_id
WHERE hp.host_id = $1
  AND hp.wua_guid IS NOT NULL
ORDER BY hp.needs_update DESC, hp.is_security_update DESC, p.name ASC;

-- name: UpdateHostPackageWUAInstallResult :exec
-- Records the outcome of a Windows Update installation for a specific host+GUID combination.
UPDATE host_packages SET
    wua_install_result = $3,
    wua_date_installed = CASE WHEN $3 = 'success' THEN NOW() ELSE wua_date_installed END,
    needs_update       = CASE WHEN $3 = 'success' THEN false ELSE needs_update END,
    current_version    = CASE WHEN $3 = 'success' THEN COALESCE(available_version, current_version) ELSE current_version END,
    last_checked       = NOW()
WHERE host_id = $1 AND wua_guid = $2;

-- name: DeleteHostPackageByWUAGUID :exec
-- Removes a host_package row that was reported as superseded by the agent.
DELETE FROM host_packages WHERE host_id = $1 AND wua_guid = $2;

-- name: DeleteHostPackagesByWUAGUIDs :exec
-- Bulk-removes superseded Windows Update entries for a host.
DELETE FROM host_packages WHERE host_id = $1 AND wua_guid = ANY($2::text[]);

-- name: GetPendingWindowsUpdateGUIDs :many
-- Returns WUA GUIDs that are still pending (needs_update=true, not yet installed) for a host.
-- Used by the server to tell the agent which updates to install.
SELECT wua_guid FROM host_packages
WHERE host_id = $1
  AND wua_guid IS NOT NULL
  AND needs_update = true
  AND (wua_install_result IS NULL OR wua_install_result = 'failed')
ORDER BY is_security_update DESC, last_checked ASC;

-- name: CountWindowsUpdatesByHostID :one
-- Counts pending Windows Updates for a host (for dashboard/stats).
SELECT
    COUNT(*) FILTER (WHERE needs_update = true)::int                          AS pending_count,
    COUNT(*) FILTER (WHERE needs_update = true AND is_security_update = true)::int AS security_count,
    COUNT(*) FILTER (WHERE needs_update = false)::int                         AS installed_count
FROM host_packages
WHERE host_id = $1 AND wua_guid IS NOT NULL;
