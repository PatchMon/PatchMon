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
        COALESCE((
            SELECT COUNT(*)::int
            FROM (
                SELECT hp.host_id
                FROM host_packages hp
                WHERE hp.needs_update = true
                GROUP BY hp.host_id
            ) hosts_with_updates
        ), 0)::int AS hosts_needing_updates,
        COALESCE((
            SELECT COUNT(*)::int
            FROM (
                SELECT hp.package_id
                FROM host_packages hp
                WHERE hp.needs_update = true
                GROUP BY hp.package_id
            ) outdated_packages
        ), 0)::int AS total_outdated_packages,
        COALESCE((
            SELECT COUNT(*)::int
            FROM (
                SELECT hp.package_id
                FROM host_packages hp
                WHERE hp.needs_update = true AND hp.is_security_update = true
                GROUP BY hp.package_id
            ) security_packages
        ), 0)::int AS security_updates
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
-- Paginated host list for the Hosts UI. Filtering and ordering happen
-- over the full matching set, then LIMIT/OFFSET selects the current page.
-- That keeps header sorting correct across all hosts, not just the page
-- already loaded in the browser.
WITH base_hosts AS (
    SELECT h.id, h.machine_id, h.friendly_name, h.hostname, h.ip, h.os_type, h.os_version,
        h.status, h.agent_version, h.auto_update, h.notes, h.api_id,
        h.needs_reboot, h.reboot_reason, h.system_uptime, h.docker_enabled, h.compliance_enabled, h.compliance_on_demand_only,
        h.last_update,
        h.compliance_scanner_status->'scanner_info'->>'ssg_version' as ssg_version,
        COALESCE((
            SELECT MIN(hg.name)
            FROM host_group_memberships hgm
            JOIN host_groups hg ON hg.id = hgm.host_group_id
            WHERE hgm.host_id = h.id
        ), '')::text AS first_group_name
    FROM hosts h
    WHERE (sqlc.narg('search')::text IS NULL OR h.friendly_name ILIKE '%' || sqlc.narg('search') || '%' OR h.hostname ILIKE '%' || sqlc.narg('search') || '%' OR h.ip ILIKE '%' || sqlc.narg('search') || '%' OR h.os_type ILIKE '%' || sqlc.narg('search') || '%' OR h.notes ILIKE '%' || sqlc.narg('search') || '%')
    AND (
        sqlc.narg('group')::text IS NULL
        OR (sqlc.narg('group') = 'ungrouped' AND NOT EXISTS (SELECT 1 FROM host_group_memberships hgm WHERE hgm.host_id = h.id))
        OR (sqlc.narg('group') != 'ungrouped' AND EXISTS (SELECT 1 FROM host_group_memberships hgm WHERE hgm.host_id = h.id AND hgm.host_group_id = sqlc.narg('group')))
    )
    AND (sqlc.narg('status')::text IS NULL OR h.status = sqlc.narg('status'))
    AND (sqlc.narg('os')::text IS NULL OR h.os_type ILIKE sqlc.narg('os'))
    AND (sqlc.narg('os_version')::text IS NULL OR h.os_version ILIKE sqlc.narg('os_version'))
    AND (
        sqlc.narg('filter')::text IS DISTINCT FROM 'selected'
        OR h.id = ANY(sqlc.arg('selected_ids')::text[])
    )
),
hp_counts AS (
    SELECT hp.host_id,
           COUNT(*) FILTER (WHERE hp.needs_update)::int                                AS updates_count,
           COUNT(*) FILTER (WHERE hp.needs_update AND hp.is_security_update)::int       AS security_count,
           COUNT(*)::int                                                               AS total_count
    FROM host_packages hp
    JOIN base_hosts bh ON bh.id = hp.host_id
    GROUP BY hp.host_id
),
enriched_hosts AS (
    SELECT bh.*,
           COALESCE(hp.updates_count, 0)::int AS updates_count,
           COALESCE(hp.security_count, 0)::int AS security_updates_count,
           COALESCE(hp.total_count, 0)::int AS total_packages_count,
           (bh.status = 'active' AND bh.last_update < sqlc.arg('stale_threshold')::timestamp) AS is_stale,
           CASE
               WHEN bh.status = 'active' AND bh.last_update < sqlc.arg('stale_threshold')::timestamp THEN 'inactive'
               ELSE bh.status
           END AS effective_status,
           ((CASE WHEN bh.docker_enabled THEN 1 ELSE 0 END) + (CASE WHEN bh.compliance_enabled THEN 1 ELSE 0 END))::int AS integrations_count
    FROM base_hosts bh
    LEFT JOIN hp_counts hp ON hp.host_id = bh.id
),
filtered_hosts AS (
    SELECT *
    FROM enriched_hosts
    WHERE (
        sqlc.narg('filter')::text IS NULL
        OR (sqlc.narg('filter') = 'needsUpdates' AND updates_count > 0)
        OR (sqlc.narg('filter') = 'inactive' AND effective_status = 'inactive')
        OR (sqlc.narg('filter') = 'upToDate' AND is_stale = false AND updates_count = 0)
        OR (sqlc.narg('filter') = 'stale' AND is_stale = true)
        OR (sqlc.narg('filter') = 'selected')
    )
    AND (sqlc.arg('reboot_only')::boolean = false OR needs_reboot = true)
    AND (sqlc.arg('hide_stale')::boolean = false OR is_stale = false)
)
SELECT id, machine_id, friendly_name, hostname, ip, os_type, os_version,
    status, agent_version, auto_update, notes, api_id,
    needs_reboot, reboot_reason, system_uptime, docker_enabled, compliance_enabled, compliance_on_demand_only,
    last_update, ssg_version, updates_count, security_updates_count, total_packages_count
FROM filtered_hosts
ORDER BY
    CASE WHEN sqlc.arg('sort_key')::text = 'friendly_name'      AND sqlc.arg('sort_dir')::text = 'asc'  THEN friendly_name END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'friendly_name'      AND sqlc.arg('sort_dir')::text = 'desc' THEN friendly_name END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'hostname'           AND sqlc.arg('sort_dir')::text = 'asc'  THEN hostname END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'hostname'           AND sqlc.arg('sort_dir')::text = 'desc' THEN hostname END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'ip'                 AND sqlc.arg('sort_dir')::text = 'asc'  THEN ip END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'ip'                 AND sqlc.arg('sort_dir')::text = 'desc' THEN ip END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'group'              AND sqlc.arg('sort_dir')::text = 'asc'  THEN first_group_name END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'group'              AND sqlc.arg('sort_dir')::text = 'desc' THEN first_group_name END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'os_type'            AND sqlc.arg('sort_dir')::text = 'asc'  THEN os_type END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'os_type'            AND sqlc.arg('sort_dir')::text = 'desc' THEN os_type END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'os_version'         AND sqlc.arg('sort_dir')::text = 'asc'  THEN os_version END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'os_version'         AND sqlc.arg('sort_dir')::text = 'desc' THEN os_version END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'agent_version'      AND sqlc.arg('sort_dir')::text = 'asc'  THEN agent_version END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'agent_version'      AND sqlc.arg('sort_dir')::text = 'desc' THEN agent_version END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'status'             AND sqlc.arg('sort_dir')::text = 'asc'  THEN effective_status END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'status'             AND sqlc.arg('sort_dir')::text = 'desc' THEN effective_status END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'updates'            AND sqlc.arg('sort_dir')::text = 'asc'  THEN updates_count END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'updates'            AND sqlc.arg('sort_dir')::text = 'desc' THEN updates_count END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'security_updates'   AND sqlc.arg('sort_dir')::text = 'asc'  THEN security_updates_count END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'security_updates'   AND sqlc.arg('sort_dir')::text = 'desc' THEN security_updates_count END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'needs_reboot'       AND sqlc.arg('sort_dir')::text = 'asc'  THEN needs_reboot END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'needs_reboot'       AND sqlc.arg('sort_dir')::text = 'desc' THEN needs_reboot END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'uptime'             AND sqlc.arg('sort_dir')::text = 'asc'  THEN system_uptime END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'uptime'             AND sqlc.arg('sort_dir')::text = 'desc' THEN system_uptime END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'last_update'        AND sqlc.arg('sort_dir')::text = 'asc'  THEN last_update END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'last_update'        AND sqlc.arg('sort_dir')::text = 'desc' THEN last_update END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'ssg_version'        AND sqlc.arg('sort_dir')::text = 'asc'  THEN ssg_version END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'ssg_version'        AND sqlc.arg('sort_dir')::text = 'desc' THEN ssg_version END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'notes'              AND sqlc.arg('sort_dir')::text = 'asc'  THEN notes END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'notes'              AND sqlc.arg('sort_dir')::text = 'desc' THEN notes END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'integrations'       AND sqlc.arg('sort_dir')::text = 'asc'  THEN integrations_count END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'integrations'       AND sqlc.arg('sort_dir')::text = 'desc' THEN integrations_count END DESC,
    -- Default sort (also the tiebreaker for every key above): newest activity first.
    last_update DESC NULLS LAST,
    id ASC -- stable ordering across paginated requests
LIMIT  sqlc.arg('row_limit')::int
OFFSET sqlc.arg('row_offset')::int;

-- name: GetHostsWithPageCounts :many
-- Fast path for host-column sorting: order/page hosts first, then aggregate
-- package counts only for the visible page. Package-count sorting uses
-- GetHostsWithCounts so the sort still applies across the full filtered set.
WITH base_hosts AS (
    SELECT h.id, h.machine_id, h.friendly_name, h.hostname, h.ip, h.os_type, h.os_version,
        h.status, h.agent_version, h.auto_update, h.notes, h.api_id,
        h.needs_reboot, h.reboot_reason, h.system_uptime, h.docker_enabled, h.compliance_enabled, h.compliance_on_demand_only,
        h.last_update,
        h.compliance_scanner_status->'scanner_info'->>'ssg_version' as ssg_version,
        COALESCE((
            SELECT MIN(hg.name)
            FROM host_group_memberships hgm
            JOIN host_groups hg ON hg.id = hgm.host_group_id
            WHERE hgm.host_id = h.id
        ), '')::text AS first_group_name,
        (h.status = 'active' AND h.last_update < sqlc.arg('stale_threshold')::timestamp) AS is_stale,
        CASE
            WHEN h.status = 'active' AND h.last_update < sqlc.arg('stale_threshold')::timestamp THEN 'inactive'
            ELSE h.status
        END AS effective_status,
        ((CASE WHEN h.docker_enabled THEN 1 ELSE 0 END) + (CASE WHEN h.compliance_enabled THEN 1 ELSE 0 END))::int AS integrations_count
    FROM hosts h
    WHERE (sqlc.narg('search')::text IS NULL OR h.friendly_name ILIKE '%' || sqlc.narg('search') || '%' OR h.hostname ILIKE '%' || sqlc.narg('search') || '%' OR h.ip ILIKE '%' || sqlc.narg('search') || '%' OR h.os_type ILIKE '%' || sqlc.narg('search') || '%' OR h.notes ILIKE '%' || sqlc.narg('search') || '%')
    AND (
        sqlc.narg('group')::text IS NULL
        OR (sqlc.narg('group') = 'ungrouped' AND NOT EXISTS (SELECT 1 FROM host_group_memberships hgm WHERE hgm.host_id = h.id))
        OR (sqlc.narg('group') != 'ungrouped' AND EXISTS (SELECT 1 FROM host_group_memberships hgm WHERE hgm.host_id = h.id AND hgm.host_group_id = sqlc.narg('group')))
    )
    AND (sqlc.narg('status')::text IS NULL OR h.status = sqlc.narg('status'))
    AND (sqlc.narg('os')::text IS NULL OR h.os_type ILIKE sqlc.narg('os'))
    AND (sqlc.narg('os_version')::text IS NULL OR h.os_version ILIKE sqlc.narg('os_version'))
    AND (
        sqlc.narg('filter')::text IS DISTINCT FROM 'selected'
        OR h.id = ANY(sqlc.arg('selected_ids')::text[])
    )
),
filtered_hosts AS (
    SELECT *
    FROM base_hosts bh
    WHERE (
        sqlc.narg('filter')::text IS NULL
        OR (sqlc.narg('filter') = 'needsUpdates' AND EXISTS (
            SELECT 1 FROM host_packages hp WHERE hp.host_id = bh.id AND hp.needs_update
        ))
        OR (sqlc.narg('filter') = 'inactive' AND bh.effective_status = 'inactive')
        OR (sqlc.narg('filter') = 'upToDate' AND bh.is_stale = false AND NOT EXISTS (
            SELECT 1 FROM host_packages hp WHERE hp.host_id = bh.id AND hp.needs_update
        ))
        OR (sqlc.narg('filter') = 'stale' AND bh.is_stale = true)
        OR (sqlc.narg('filter') = 'selected')
    )
    AND (sqlc.arg('reboot_only')::boolean = false OR bh.needs_reboot = true)
    AND (sqlc.arg('hide_stale')::boolean = false OR bh.is_stale = false)
),
ordered_page AS (
    SELECT *
    FROM filtered_hosts
    ORDER BY
        CASE WHEN sqlc.arg('sort_key')::text = 'friendly_name'    AND sqlc.arg('sort_dir')::text = 'asc'  THEN friendly_name END ASC,
        CASE WHEN sqlc.arg('sort_key')::text = 'friendly_name'    AND sqlc.arg('sort_dir')::text = 'desc' THEN friendly_name END DESC,
        CASE WHEN sqlc.arg('sort_key')::text = 'hostname'         AND sqlc.arg('sort_dir')::text = 'asc'  THEN hostname END ASC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'hostname'         AND sqlc.arg('sort_dir')::text = 'desc' THEN hostname END DESC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'ip'               AND sqlc.arg('sort_dir')::text = 'asc'  THEN ip END ASC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'ip'               AND sqlc.arg('sort_dir')::text = 'desc' THEN ip END DESC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'group'            AND sqlc.arg('sort_dir')::text = 'asc'  THEN first_group_name END ASC,
        CASE WHEN sqlc.arg('sort_key')::text = 'group'            AND sqlc.arg('sort_dir')::text = 'desc' THEN first_group_name END DESC,
        CASE WHEN sqlc.arg('sort_key')::text = 'os_type'          AND sqlc.arg('sort_dir')::text = 'asc'  THEN os_type END ASC,
        CASE WHEN sqlc.arg('sort_key')::text = 'os_type'          AND sqlc.arg('sort_dir')::text = 'desc' THEN os_type END DESC,
        CASE WHEN sqlc.arg('sort_key')::text = 'os_version'       AND sqlc.arg('sort_dir')::text = 'asc'  THEN os_version END ASC,
        CASE WHEN sqlc.arg('sort_key')::text = 'os_version'       AND sqlc.arg('sort_dir')::text = 'desc' THEN os_version END DESC,
        CASE WHEN sqlc.arg('sort_key')::text = 'agent_version'    AND sqlc.arg('sort_dir')::text = 'asc'  THEN agent_version END ASC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'agent_version'    AND sqlc.arg('sort_dir')::text = 'desc' THEN agent_version END DESC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'status'           AND sqlc.arg('sort_dir')::text = 'asc'  THEN effective_status END ASC,
        CASE WHEN sqlc.arg('sort_key')::text = 'status'           AND sqlc.arg('sort_dir')::text = 'desc' THEN effective_status END DESC,
        CASE WHEN sqlc.arg('sort_key')::text = 'needs_reboot'     AND sqlc.arg('sort_dir')::text = 'asc'  THEN needs_reboot END ASC,
        CASE WHEN sqlc.arg('sort_key')::text = 'needs_reboot'     AND sqlc.arg('sort_dir')::text = 'desc' THEN needs_reboot END DESC,
        CASE WHEN sqlc.arg('sort_key')::text = 'uptime'           AND sqlc.arg('sort_dir')::text = 'asc'  THEN system_uptime END ASC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'uptime'           AND sqlc.arg('sort_dir')::text = 'desc' THEN system_uptime END DESC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'last_update'      AND sqlc.arg('sort_dir')::text = 'asc'  THEN last_update END ASC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'last_update'      AND sqlc.arg('sort_dir')::text = 'desc' THEN last_update END DESC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'ssg_version'      AND sqlc.arg('sort_dir')::text = 'asc'  THEN ssg_version END ASC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'ssg_version'      AND sqlc.arg('sort_dir')::text = 'desc' THEN ssg_version END DESC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'notes'            AND sqlc.arg('sort_dir')::text = 'asc'  THEN notes END ASC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'notes'            AND sqlc.arg('sort_dir')::text = 'desc' THEN notes END DESC NULLS LAST,
        CASE WHEN sqlc.arg('sort_key')::text = 'integrations'     AND sqlc.arg('sort_dir')::text = 'asc'  THEN integrations_count END ASC,
        CASE WHEN sqlc.arg('sort_key')::text = 'integrations'     AND sqlc.arg('sort_dir')::text = 'desc' THEN integrations_count END DESC,
        last_update DESC NULLS LAST,
        id ASC
    LIMIT  sqlc.arg('row_limit')::int
    OFFSET sqlc.arg('row_offset')::int
),
page_counts AS (
    SELECT hp.host_id,
           COUNT(*) FILTER (WHERE hp.needs_update)::int                          AS updates_count,
           COUNT(*) FILTER (WHERE hp.needs_update AND hp.is_security_update)::int AS security_count,
           COUNT(*)::int                                                         AS total_count
    FROM host_packages hp
    JOIN ordered_page op ON op.id = hp.host_id
    GROUP BY hp.host_id
)
SELECT op.id, op.machine_id, op.friendly_name, op.hostname, op.ip, op.os_type, op.os_version,
    op.status, op.agent_version, op.auto_update, op.notes, op.api_id,
    op.needs_reboot, op.reboot_reason, op.system_uptime, op.docker_enabled, op.compliance_enabled, op.compliance_on_demand_only,
    op.last_update, op.ssg_version,
    COALESCE(pc.updates_count, 0)::int AS updates_count,
    COALESCE(pc.security_count, 0)::int AS security_updates_count,
    COALESCE(pc.total_count, 0)::int AS total_packages_count
FROM ordered_page op
LEFT JOIN page_counts pc ON pc.host_id = op.id
ORDER BY
    CASE WHEN sqlc.arg('sort_key')::text = 'friendly_name'    AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.friendly_name END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'friendly_name'    AND sqlc.arg('sort_dir')::text = 'desc' THEN op.friendly_name END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'hostname'         AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.hostname END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'hostname'         AND sqlc.arg('sort_dir')::text = 'desc' THEN op.hostname END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'ip'               AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.ip END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'ip'               AND sqlc.arg('sort_dir')::text = 'desc' THEN op.ip END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'group'            AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.first_group_name END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'group'            AND sqlc.arg('sort_dir')::text = 'desc' THEN op.first_group_name END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'os_type'          AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.os_type END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'os_type'          AND sqlc.arg('sort_dir')::text = 'desc' THEN op.os_type END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'os_version'       AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.os_version END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'os_version'       AND sqlc.arg('sort_dir')::text = 'desc' THEN op.os_version END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'agent_version'    AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.agent_version END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'agent_version'    AND sqlc.arg('sort_dir')::text = 'desc' THEN op.agent_version END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'status'           AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.effective_status END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'status'           AND sqlc.arg('sort_dir')::text = 'desc' THEN op.effective_status END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'needs_reboot'     AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.needs_reboot END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'needs_reboot'     AND sqlc.arg('sort_dir')::text = 'desc' THEN op.needs_reboot END DESC,
    CASE WHEN sqlc.arg('sort_key')::text = 'uptime'           AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.system_uptime END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'uptime'           AND sqlc.arg('sort_dir')::text = 'desc' THEN op.system_uptime END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'last_update'      AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.last_update END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'last_update'      AND sqlc.arg('sort_dir')::text = 'desc' THEN op.last_update END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'ssg_version'      AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.ssg_version END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'ssg_version'      AND sqlc.arg('sort_dir')::text = 'desc' THEN op.ssg_version END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'notes'            AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.notes END ASC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'notes'            AND sqlc.arg('sort_dir')::text = 'desc' THEN op.notes END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_key')::text = 'integrations'     AND sqlc.arg('sort_dir')::text = 'asc'  THEN op.integrations_count END ASC,
    CASE WHEN sqlc.arg('sort_key')::text = 'integrations'     AND sqlc.arg('sort_dir')::text = 'desc' THEN op.integrations_count END DESC,
    op.last_update DESC NULLS LAST,
    op.id ASC;

-- name: CountHostsForList :one
-- Total count matching the same filter set as GetHostsWithCounts. Used by
-- the paginated UI so it can render "Showing X-Y of Z" and a correct page
-- count.
WITH base_hosts AS (
    SELECT h.id, h.status, h.last_update, h.needs_reboot
    FROM hosts h
    WHERE (sqlc.narg('search')::text IS NULL OR h.friendly_name ILIKE '%' || sqlc.narg('search') || '%' OR h.hostname ILIKE '%' || sqlc.narg('search') || '%' OR h.ip ILIKE '%' || sqlc.narg('search') || '%' OR h.os_type ILIKE '%' || sqlc.narg('search') || '%' OR h.notes ILIKE '%' || sqlc.narg('search') || '%')
    AND (
        sqlc.narg('group')::text IS NULL
        OR (sqlc.narg('group') = 'ungrouped' AND NOT EXISTS (SELECT 1 FROM host_group_memberships hgm WHERE hgm.host_id = h.id))
        OR (sqlc.narg('group') != 'ungrouped' AND EXISTS (SELECT 1 FROM host_group_memberships hgm WHERE hgm.host_id = h.id AND hgm.host_group_id = sqlc.narg('group')))
    )
    AND (sqlc.narg('status')::text IS NULL OR h.status = sqlc.narg('status'))
    AND (sqlc.narg('os')::text IS NULL OR h.os_type ILIKE sqlc.narg('os'))
    AND (sqlc.narg('os_version')::text IS NULL OR h.os_version ILIKE sqlc.narg('os_version'))
    AND (
        sqlc.narg('filter')::text IS DISTINCT FROM 'selected'
        OR h.id = ANY(sqlc.arg('selected_ids')::text[])
    )
),
filtered_hosts AS (
    SELECT bh.id,
           (bh.status = 'active' AND bh.last_update < sqlc.arg('stale_threshold')::timestamp) AS is_stale,
           CASE
               WHEN bh.status = 'active' AND bh.last_update < sqlc.arg('stale_threshold')::timestamp THEN 'inactive'
               ELSE bh.status
           END AS effective_status,
           bh.needs_reboot
    FROM base_hosts bh
)
SELECT COUNT(*)::int
FROM filtered_hosts
WHERE (
    sqlc.narg('filter')::text IS NULL
    OR (sqlc.narg('filter') = 'needsUpdates' AND EXISTS (
        SELECT 1 FROM host_packages hp WHERE hp.host_id = filtered_hosts.id AND hp.needs_update
    ))
    OR (sqlc.narg('filter') = 'inactive' AND effective_status = 'inactive')
    OR (sqlc.narg('filter') = 'upToDate' AND is_stale = false AND NOT EXISTS (
        SELECT 1 FROM host_packages hp WHERE hp.host_id = filtered_hosts.id AND hp.needs_update
    ))
    OR (sqlc.narg('filter') = 'stale' AND is_stale = true)
    OR (sqlc.narg('filter') = 'selected')
)
AND (sqlc.arg('reboot_only')::boolean = false OR needs_reboot = true)
AND (sqlc.arg('hide_stale')::boolean = false OR is_stale = false);

-- name: GetHostCounts :one
-- Cheap host-only counts for the sidebar / navbar widgets. Replaces the
-- old pattern of fetching the entire host list and computing counts in
-- the browser. $1 = stale threshold, $2 = down threshold (timestamps).
SELECT
    COUNT(*)::int                                                                              AS total,
    COUNT(*) FILTER (WHERE status = 'active' AND last_update >= $1)::int                       AS up,
    COUNT(*) FILTER (WHERE status = 'active' AND last_update <  $1 AND last_update >= $2)::int AS stale,
    COUNT(*) FILTER (WHERE status = 'active' AND last_update <  $2)::int                       AS down,
    COUNT(*) FILTER (WHERE status = 'inactive')::int                                           AS inactive,
    COUNT(*) FILTER (WHERE needs_reboot = true)::int                                           AS needs_reboot,
    COUNT(*) FILTER (WHERE EXISTS (
        SELECT 1 FROM host_packages hp WHERE hp.host_id = hosts.id AND hp.needs_update = true
    ))::int                                                                                    AS needs_updates
FROM hosts;

-- name: GetNavigationStats :one
SELECT
    (SELECT COUNT(*)::int FROM hosts) AS total_hosts,
    (SELECT COUNT(*)::int FROM repositories) AS total_repos,
    COALESCE((
        SELECT unique_packages_count::int
        FROM system_statistics
        ORDER BY timestamp DESC
        LIMIT 1
    ), 0)::int AS total_outdated_packages,
    NOW()::timestamp AS last_updated;

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
SELECT COUNT(*)::int
FROM update_history
WHERE host_id = $1
  AND report_type IN ('full', 'partial');

-- name: GetUpdateHistory :many
SELECT id, host_id, packages_count, security_count, total_packages,
    payload_size_kb, execution_time, timestamp, status, error_message
FROM update_history
WHERE host_id = $1
  AND report_type IN ('full', 'partial')
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
  AND report_type IN ('full', 'partial')
GROUP BY DATE(timestamp)
ORDER BY ts;

-- name: ListUpdateHistoryByDateRange :many
SELECT id, host_id, packages_count, security_count, total_packages,
    payload_size_kb, execution_time, timestamp, status, error_message
FROM update_history
WHERE host_id = $1 AND timestamp >= $2 AND timestamp <= $3 AND status = 'success'
  AND report_type IN ('full', 'partial')
ORDER BY timestamp ASC;

-- name: GetUpdateHistoryDaily :many
SELECT DATE(timestamp)::text as ts,
    MAX(packages_count)::int as packages_count,
    MAX(security_count)::int as security_count,
    MAX(COALESCE(total_packages, 0))::int as total_packages
FROM update_history
WHERE host_id = $1 AND timestamp >= $2 AND timestamp <= $3 AND status = 'success'
  AND report_type IN ('full', 'partial')
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
    (SELECT COUNT(*)::int FROM update_history WHERE timestamp >= sqlc.arg('since') AND status = 'success' AND report_type IN ('full', 'partial')) AS recent_updates_24h
FROM host_counts hc
CROSS JOIN hosts_needing_updates hnu
CROSS JOIN hosts_with_security hws
CROSS JOIN package_counts pc;

