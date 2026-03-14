-- name: GetDashboardStats :one
WITH host_counts AS (
    SELECT
        COUNT(*)::int AS total_hosts,
        COUNT(*) FILTER (WHERE status = 'active' AND last_update < $1)::int AS errored_hosts,
        COUNT(*) FILTER (WHERE status = 'active' AND last_update < $2)::int AS offline_hosts,
        COUNT(*) FILTER (WHERE needs_reboot = true)::int AS hosts_needing_reboot
    FROM hosts
),
hp_package_counts AS (
    SELECT
        COUNT(DISTINCT host_id)::int AS hosts_needing_updates,
        COUNT(DISTINCT package_id)::int AS total_outdated_packages,
        COUNT(DISTINCT package_id) FILTER (WHERE is_security_update)::int AS security_updates
    FROM host_packages
    WHERE needs_update = true
)
SELECT
    hc.total_hosts,
    hpc.hosts_needing_updates,
    hpc.total_outdated_packages,
    hc.errored_hosts,
    hpc.security_updates,
    hc.offline_hosts,
    hc.hosts_needing_reboot,
    (SELECT COUNT(*)::int FROM host_groups),
    (SELECT COUNT(*)::int FROM users),
    (SELECT COUNT(*)::int FROM repositories)
FROM host_counts hc
CROSS JOIN hp_package_counts hpc;

-- name: GetOSDistribution :many
SELECT os_type as name, COUNT(*)::int as count FROM hosts WHERE status = 'active' GROUP BY os_type;

-- name: GetOSDistributionByTypeAndVersion :many
SELECT os_type, os_version,
    (os_type || ' ' || os_version)::text as name,
    COUNT(*)::int as count
FROM hosts
WHERE status = 'active'
GROUP BY os_type, os_version
ORDER BY count DESC, os_type, os_version;

-- name: GetHostsWithCounts :many
SELECT h.id, h.machine_id, h.friendly_name, h.hostname, h.ip, h.os_type, h.os_version,
    h.status, h.agent_version, h.auto_update, h.notes, h.api_id,
    h.needs_reboot, h.system_uptime, h.docker_enabled, h.compliance_enabled, h.compliance_on_demand_only,
    h.last_update,
    COALESCE(uc.cnt, 0)::int as updates_count,
    COALESCE(sc.cnt, 0)::int as security_updates_count,
    COALESCE(tc.cnt, 0)::int as total_packages_count
FROM hosts h
LEFT JOIN (SELECT host_id, COUNT(*) as cnt FROM host_packages WHERE needs_update = true GROUP BY host_id) uc ON uc.host_id = h.id
LEFT JOIN (SELECT host_id, COUNT(*) as cnt FROM host_packages WHERE needs_update = true AND is_security_update = true GROUP BY host_id) sc ON sc.host_id = h.id
LEFT JOIN (SELECT host_id, COUNT(*) as cnt FROM host_packages GROUP BY host_id) tc ON tc.host_id = h.id
WHERE (sqlc.narg('search')::text IS NULL OR h.friendly_name ILIKE '%' || sqlc.narg('search') || '%' OR h.ip ILIKE '%' || sqlc.narg('search') || '%' OR h.os_type ILIKE '%' || sqlc.narg('search') || '%' OR h.notes ILIKE '%' || sqlc.narg('search') || '%')
AND (
    sqlc.narg('group')::text IS NULL
    OR (sqlc.narg('group') = 'ungrouped' AND NOT EXISTS (SELECT 1 FROM host_group_memberships hgm WHERE hgm.host_id = h.id))
    OR (sqlc.narg('group') != 'ungrouped' AND EXISTS (SELECT 1 FROM host_group_memberships hgm WHERE hgm.host_id = h.id AND hgm.host_group_id = sqlc.narg('group')))
)
AND (sqlc.narg('status')::text IS NULL OR h.status = sqlc.narg('status'))
AND (sqlc.narg('os')::text IS NULL OR h.os_type ILIKE sqlc.narg('os'))
AND (sqlc.narg('os_version')::text IS NULL OR h.os_version ILIKE sqlc.narg('os_version'))
ORDER BY h.last_update DESC NULLS LAST;

-- name: GetHostPackageStats :one
SELECT COUNT(*)::int, COUNT(*) FILTER (WHERE needs_update)::int, COUNT(*) FILTER (WHERE needs_update AND is_security_update)::int
FROM host_packages WHERE host_id = $1;

-- name: GetHostPackagesWithPackages :many
SELECT hp.id, hp.host_id, hp.package_id, hp.current_version, hp.available_version,
    hp.needs_update, hp.is_security_update, hp.last_checked,
    p.name as pkg_name
FROM host_packages hp
JOIN packages p ON p.id = hp.package_id
WHERE hp.host_id = $1
ORDER BY hp.needs_update DESC;

-- name: GetHostPackagesForScopedApi :many
SELECT hp.id, hp.host_id, hp.package_id, hp.current_version, hp.available_version,
    hp.needs_update, hp.is_security_update, hp.last_checked,
    p.name as pkg_name, p.description as pkg_description, p.category as pkg_category
FROM host_packages hp
JOIN packages p ON p.id = hp.package_id
WHERE hp.host_id = $1
ORDER BY hp.is_security_update DESC, hp.needs_update DESC;

-- name: CountUpdateHistory :one
SELECT COUNT(*)::int FROM update_history WHERE host_id = $1;

-- name: GetUpdateHistory :many
SELECT id, host_id, packages_count, security_count, total_packages,
    payload_size_kb, execution_time, timestamp, status, error_message
FROM update_history
WHERE host_id = $1
ORDER BY timestamp DESC
LIMIT $2 OFFSET $3;

-- name: GetRecentUsers :many
SELECT id, username, email, first_name, last_name, role, last_login, created_at, avatar_url
FROM users WHERE last_login IS NOT NULL ORDER BY last_login DESC LIMIT $1;

-- name: GetRecentHosts :many
SELECT id, friendly_name, hostname, last_update, status
FROM hosts ORDER BY last_update DESC LIMIT $1;

-- name: GetUpdateTrends :many
SELECT DATE(timestamp) as ts, COUNT(*)::int as cnt,
    COALESCE(SUM(packages_count), 0)::int as pkg_sum,
    COALESCE(SUM(security_count), 0)::int as sec_sum
FROM update_history
WHERE timestamp >= $1
GROUP BY DATE(timestamp)
ORDER BY ts;

-- name: ListUpdateHistoryByDateRange :many
SELECT id, host_id, packages_count, security_count, total_packages,
    payload_size_kb, execution_time, timestamp, status, error_message
FROM update_history
WHERE host_id = $1 AND timestamp >= $2 AND timestamp <= $3 AND status = 'success'
ORDER BY timestamp ASC;

-- name: GetUpdateHistoryDaily :many
SELECT DATE(timestamp)::text as ts,
    MAX(packages_count)::int as packages_count,
    MAX(security_count)::int as security_count,
    MAX(COALESCE(total_packages, 0))::int as total_packages
FROM update_history
WHERE host_id = $1 AND timestamp >= $2 AND timestamp <= $3 AND status = 'success'
  AND COALESCE(total_packages, 0) >= 0
  AND packages_count >= 0
  AND security_count >= 0
  AND security_count <= packages_count
GROUP BY DATE(timestamp)
ORDER BY ts;

-- name: GetHostsForPackageTrends :many
SELECT id, friendly_name, hostname
FROM hosts
ORDER BY friendly_name ASC;

-- name: GetHomepageStats :one
WITH active_hosts AS (
    SELECT id FROM hosts WHERE status = 'active'
),
host_counts AS (
    SELECT COUNT(*)::int AS total_hosts FROM active_hosts
),
hosts_needing_updates AS (
    SELECT COUNT(DISTINCT hp.host_id)::int AS cnt
    FROM host_packages hp
    JOIN active_hosts ah ON ah.id = hp.host_id
    WHERE hp.needs_update = true
),
hosts_with_security AS (
    SELECT COUNT(DISTINCT hp.host_id)::int AS cnt
    FROM host_packages hp
    JOIN active_hosts ah ON ah.id = hp.host_id
    WHERE hp.needs_update = true AND hp.is_security_update = true
),
package_counts AS (
    SELECT
        COUNT(DISTINCT package_id)::int AS total_outdated,
        COUNT(DISTINCT package_id) FILTER (WHERE is_security_update)::int AS security_updates
    FROM host_packages
    WHERE needs_update = true
)
SELECT
    hc.total_hosts,
    hnu.cnt AS hosts_needing_updates,
    pc.total_outdated AS total_outdated_packages,
    pc.security_updates AS security_updates,
    hws.cnt AS hosts_with_security_updates,
    (SELECT COUNT(*)::int FROM repositories WHERE is_active = true) AS total_repos,
    (SELECT COUNT(*)::int FROM update_history WHERE timestamp >= sqlc.arg('since') AND status = 'success') AS recent_updates_24h
FROM host_counts hc
CROSS JOIN hosts_needing_updates hnu
CROSS JOIN hosts_with_security hws
CROSS JOIN package_counts pc;

