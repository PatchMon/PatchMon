-- name: GetSystemStatsForInsert :one
SELECT
    (SELECT COUNT(DISTINCT p.id)::int FROM packages p
        INNER JOIN host_packages hp ON hp.package_id = p.id AND hp.needs_update = true),
    (SELECT COUNT(DISTINCT p.id)::int FROM packages p
        INNER JOIN host_packages hp ON hp.package_id = p.id AND hp.needs_update = true AND hp.is_security_update = true),
    (SELECT COUNT(DISTINCT p.id)::int FROM packages p
        WHERE EXISTS (SELECT 1 FROM host_packages hp WHERE hp.package_id = p.id)),
    (SELECT COUNT(*)::int FROM hosts WHERE status = 'active'),
    (SELECT COUNT(DISTINCT h.id)::int FROM hosts h
        INNER JOIN host_packages hp ON hp.host_id = h.id AND hp.needs_update = true);

-- name: InsertSystemStatistics :exec
INSERT INTO system_statistics (id, unique_packages_count, unique_security_count, total_packages, total_hosts, hosts_needing_updates, timestamp)
VALUES ($1, $2, $3, $4, $5, $6, NOW());

-- name: GetLatestSystemStatistics :one
SELECT total_packages, unique_packages_count, unique_security_count, timestamp
FROM system_statistics
ORDER BY timestamp DESC
LIMIT 1;

-- name: ListSystemStatisticsByDateRange :many
SELECT id, unique_packages_count, unique_security_count, total_packages, total_hosts, hosts_needing_updates, timestamp
FROM system_statistics
WHERE timestamp >= $1 AND timestamp <= $2
ORDER BY timestamp ASC;

-- name: GetSystemStatisticsDaily :many
SELECT DATE(timestamp)::text as ts,
    MAX(unique_packages_count)::int as packages_count,
    MAX(unique_security_count)::int as security_count,
    MAX(total_packages)::int as total_packages
FROM system_statistics
WHERE timestamp >= $1 AND timestamp <= $2
  AND total_packages >= 0
  AND unique_packages_count >= 0
  AND unique_security_count >= 0
  AND unique_security_count <= unique_packages_count
GROUP BY DATE(timestamp)
ORDER BY ts;
