-- Dashboard stats
-- name: CountDockerHosts :one
SELECT COUNT(DISTINCT host_id)::int FROM docker_containers;

-- name: GetDockerDashboardStats :one
SELECT
    (SELECT COUNT(*)::int FROM docker_containers),
    (SELECT COUNT(*)::int FROM docker_containers WHERE status = 'running'),
    (SELECT COUNT(*)::int FROM docker_images di WHERE (EXISTS (SELECT 1 FROM docker_containers dc0 WHERE dc0.image_id = di.id) OR EXISTS (SELECT 1 FROM docker_host_images dhi0 WHERE dhi0.image_id = di.id))),
    (SELECT COUNT(DISTINCT image_id)::int FROM docker_image_checks WHERE status = 'update_available');

-- name: GetDockerImageCheckStats :one
SELECT
    (SELECT COUNT(*)::int FROM docker_containers WHERE image_stale) AS containers_needing_recreate,
    (SELECT MAX(checked_at) FROM docker_image_checks)::timestamp(3) AS last_image_check_at;

-- name: GetContainersByStatus :many
SELECT status, COUNT(*)::int as count FROM docker_containers GROUP BY status;

-- name: GetImagesBySource :many
SELECT source, COUNT(*)::int as count FROM docker_images di
WHERE (EXISTS (SELECT 1 FROM docker_containers dc0 WHERE dc0.image_id = di.id) OR EXISTS (SELECT 1 FROM docker_host_images dhi0 WHERE dhi0.image_id = di.id))
GROUP BY source;

-- Containers
-- name: GetContainerByID :one
SELECT * FROM docker_containers WHERE id = $1;

-- name: ListContainers :many
SELECT * FROM docker_containers
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
AND (sqlc.narg('host_id')::text IS NULL OR host_id = sqlc.narg('host_id'))
AND (sqlc.narg('image_id')::text IS NULL OR image_id = sqlc.narg('image_id'))
AND (sqlc.narg('search')::text IS NULL OR name ILIKE '%' || sqlc.narg('search') || '%' OR image_name ILIKE '%' || sqlc.narg('search') || '%')
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountContainers :one
SELECT COUNT(*)::int FROM docker_containers
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
AND (sqlc.narg('host_id')::text IS NULL OR host_id = sqlc.narg('host_id'))
AND (sqlc.narg('image_id')::text IS NULL OR image_id = sqlc.narg('image_id'))
AND (sqlc.narg('search')::text IS NULL OR name ILIKE '%' || sqlc.narg('search') || '%' OR image_name ILIKE '%' || sqlc.narg('search') || '%');

-- name: DeleteContainer :exec
DELETE FROM docker_containers WHERE id = $1;

-- name: ListOrphanedContainers :many
SELECT id, host_id, container_id, name, image_name FROM docker_containers dc
WHERE NOT EXISTS (SELECT 1 FROM hosts h WHERE h.id = dc.host_id);

-- name: ListOrphanedImages :many
SELECT id, repository, tag, image_id FROM docker_images di
WHERE NOT EXISTS (SELECT 1 FROM docker_containers dc WHERE dc.image_id = di.id)
AND NOT EXISTS (SELECT 1 FROM docker_host_images dhi WHERE dhi.image_id = di.id);

-- name: DeleteContainersByIDs :exec
DELETE FROM docker_containers WHERE id = ANY($1::text[]);

-- name: DeleteImagesByIDs :exec
DELETE FROM docker_images WHERE id = ANY($1::text[]);

-- name: DeleteStaleContainers :exec
DELETE FROM docker_containers
WHERE host_id = $1 AND NOT (container_id = ANY(sqlc.arg(container_ids)::text[]));

-- name: GetContainersByImageID :many
SELECT * FROM docker_containers WHERE image_id = $1 AND id != $2 LIMIT 10;

-- name: GetContainersByHostID :many
SELECT * FROM docker_containers WHERE host_id = $1 ORDER BY name ASC;

-- name: GetDockerHostsByIDs :many
SELECT id, friendly_name, hostname, ip, os_type, os_version FROM hosts WHERE id = ANY($1::text[]);

-- name: GetDockerHostsMinimalByIDs :many
SELECT id, friendly_name, hostname, ip FROM hosts WHERE id = ANY($1::text[]);

-- Images
-- name: GetImageByID :one
SELECT * FROM docker_images WHERE id = $1;

-- name: ListImages :many
SELECT * FROM docker_images
WHERE (sqlc.narg('source')::text IS NULL OR source = sqlc.narg('source'))
AND (sqlc.narg('search')::text IS NULL OR repository ILIKE '%' || sqlc.narg('search') || '%' OR tag ILIKE '%' || sqlc.narg('search') || '%')
AND (EXISTS (SELECT 1 FROM docker_containers dc0 WHERE dc0.image_id = docker_images.id) OR EXISTS (SELECT 1 FROM docker_host_images dhi0 WHERE dhi0.image_id = docker_images.id))
AND (sqlc.narg('update_status')::text IS NULL
    OR (sqlc.narg('update_status') = 'recreate_needed' AND EXISTS (
        SELECT 1 FROM docker_containers dc WHERE dc.image_id = docker_images.id AND dc.image_stale))
    OR (sqlc.narg('update_status') <> 'recreate_needed' AND COALESCE((
        SELECT CASE MIN(CASE c.status
                WHEN 'update_available' THEN 1 WHEN 'unknown' THEN 2 WHEN 'up_to_date' THEN 3
                WHEN 'local_build' THEN 4 WHEN 'pinned' THEN 5 WHEN 'skipped' THEN 6 END)
            WHEN 1 THEN 'update_available' WHEN 2 THEN 'unknown' WHEN 3 THEN 'up_to_date'
            WHEN 4 THEN 'local_build' WHEN 5 THEN 'pinned' WHEN 6 THEN 'skipped' END
        FROM docker_image_checks c WHERE c.image_id = docker_images.id), 'not_checked') = sqlc.narg('update_status')))
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountImages :one
SELECT COUNT(*)::int FROM docker_images
WHERE (sqlc.narg('source')::text IS NULL OR source = sqlc.narg('source'))
AND (sqlc.narg('search')::text IS NULL OR repository ILIKE '%' || sqlc.narg('search') || '%' OR tag ILIKE '%' || sqlc.narg('search') || '%')
AND (EXISTS (SELECT 1 FROM docker_containers dc0 WHERE dc0.image_id = docker_images.id) OR EXISTS (SELECT 1 FROM docker_host_images dhi0 WHERE dhi0.image_id = docker_images.id))
AND (sqlc.narg('update_status')::text IS NULL
    OR (sqlc.narg('update_status') = 'recreate_needed' AND EXISTS (
        SELECT 1 FROM docker_containers dc WHERE dc.image_id = docker_images.id AND dc.image_stale))
    OR (sqlc.narg('update_status') <> 'recreate_needed' AND COALESCE((
        SELECT CASE MIN(CASE c.status
                WHEN 'update_available' THEN 1 WHEN 'unknown' THEN 2 WHEN 'up_to_date' THEN 3
                WHEN 'local_build' THEN 4 WHEN 'pinned' THEN 5 WHEN 'skipped' THEN 6 END)
            WHEN 1 THEN 'update_available' WHEN 2 THEN 'unknown' WHEN 3 THEN 'up_to_date'
            WHEN 4 THEN 'local_build' WHEN 5 THEN 'pinned' WHEN 6 THEN 'skipped' END
        FROM docker_image_checks c WHERE c.image_id = docker_images.id), 'not_checked') = sqlc.narg('update_status')));

-- name: GetContainerCountsByImageIDs :many
SELECT image_id, COUNT(*)::int as cnt FROM docker_containers WHERE image_id = ANY($1::text[]) GROUP BY image_id;

-- name: GetStaleContainerCountsByImageIDs :many
SELECT image_id, COUNT(*)::int as cnt FROM docker_containers WHERE image_id = ANY($1::text[]) AND image_stale GROUP BY image_id;

-- name: GetContainersByImageIDAll :many
SELECT * FROM docker_containers WHERE image_id = $1 LIMIT 100;

-- name: CountContainersByImageID :one
SELECT COUNT(*)::int FROM docker_containers WHERE image_id = $1;

-- name: DeleteImageByID :exec
DELETE FROM docker_images WHERE id = $1;

-- name: GetImagesByIDs :many
SELECT * FROM docker_images WHERE id = ANY($1::text[]);

-- Hosts (include hosts with containers, volumes, or networks)
-- name: GetDistinctDockerHostIDs :many
SELECT DISTINCT host_id FROM (
    SELECT host_id FROM docker_containers
    UNION
    SELECT host_id FROM docker_volumes
    UNION
    SELECT host_id FROM docker_networks
) AS docker_hosts;

-- name: ListDockerHostsPaginated :many
SELECT id, friendly_name, hostname, ip FROM hosts
WHERE id IN (
    SELECT DISTINCT host_id FROM docker_containers
    UNION
    SELECT DISTINCT host_id FROM docker_volumes
    UNION
    SELECT DISTINCT host_id FROM docker_networks
)
ORDER BY friendly_name ASC
LIMIT $1 OFFSET $2;

-- name: GetHostDockerStats :one
SELECT
    COUNT(*)::int,
    COUNT(*) FILTER (WHERE status = 'running')::int,
    (SELECT COUNT(*)::int FROM (
        SELECT dhi.image_id FROM docker_host_images dhi WHERE dhi.host_id = $1
        UNION
        SELECT dc2.image_id FROM docker_containers dc2 WHERE dc2.host_id = $1 AND dc2.image_id IS NOT NULL
    ) hi)
FROM docker_containers
WHERE host_id = $1;

-- name: GetImagesByHostID :many
SELECT * FROM docker_images
WHERE id IN (
    SELECT dhi.image_id FROM docker_host_images dhi WHERE dhi.host_id = $1
    UNION
    SELECT dc.image_id FROM docker_containers dc WHERE dc.host_id = $1 AND dc.image_id IS NOT NULL
)
ORDER BY repository ASC, tag ASC;

-- name: GetHostIDsForImage :many
SELECT dhi.host_id FROM docker_host_images dhi WHERE dhi.image_id = $1
UNION
SELECT dc.host_id FROM docker_containers dc WHERE dc.image_id = $1;

-- name: ListDockerHostAPIIDs :many
SELECT id, api_id FROM hosts
WHERE id IN (
    SELECT host_id FROM docker_containers
    UNION
    SELECT host_id FROM docker_host_images
    UNION
    SELECT host_id FROM docker_volumes
    UNION
    SELECT host_id FROM docker_networks
)
ORDER BY id;

-- Volumes
-- name: GetVolumeByID :one
SELECT * FROM docker_volumes WHERE id = $1;

-- name: ListVolumes :many
SELECT * FROM docker_volumes
WHERE (sqlc.narg('driver')::text IS NULL OR driver = sqlc.narg('driver'))
AND (sqlc.narg('search')::text IS NULL OR name ILIKE '%' || sqlc.narg('search') || '%')
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountVolumes :one
SELECT COUNT(*)::int FROM docker_volumes
WHERE (sqlc.narg('driver')::text IS NULL OR driver = sqlc.narg('driver'))
AND (sqlc.narg('search')::text IS NULL OR name ILIKE '%' || sqlc.narg('search') || '%');

-- name: DeleteVolume :exec
DELETE FROM docker_volumes WHERE id = $1;

-- name: DeleteStaleVolumes :exec
DELETE FROM docker_volumes
WHERE host_id = $1 AND NOT (volume_id = ANY(sqlc.arg(volume_ids)::text[]));

-- name: GetVolumesByHostID :many
SELECT * FROM docker_volumes WHERE host_id = $1 ORDER BY name ASC;

-- name: CountVolumesByHostID :one
SELECT COUNT(*)::int FROM docker_volumes WHERE host_id = $1;

-- Networks
-- name: GetNetworkByID :one
SELECT * FROM docker_networks WHERE id = $1;

-- name: ListNetworks :many
SELECT * FROM docker_networks
WHERE (sqlc.narg('driver')::text IS NULL OR driver = sqlc.narg('driver'))
AND (sqlc.narg('search')::text IS NULL OR name ILIKE '%' || sqlc.narg('search') || '%')
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountNetworks :one
SELECT COUNT(*)::int FROM docker_networks
WHERE (sqlc.narg('driver')::text IS NULL OR driver = sqlc.narg('driver'))
AND (sqlc.narg('search')::text IS NULL OR name ILIKE '%' || sqlc.narg('search') || '%');

-- name: DeleteNetwork :exec
DELETE FROM docker_networks WHERE id = $1;

-- name: DeleteStaleNetworks :exec
DELETE FROM docker_networks
WHERE host_id = $1 AND NOT (network_id = ANY(sqlc.arg(network_ids)::text[]));

-- name: GetNetworksByHostID :many
SELECT * FROM docker_networks WHERE host_id = $1 ORDER BY name ASC;

-- name: CountNetworksByHostID :one
SELECT COUNT(*)::int FROM docker_networks WHERE host_id = $1;

-- name: ResolveDockerImageIDs :many
SELECT di.id, di.repository, di.tag, di.image_id FROM docker_images di
JOIN docker_host_images dhi ON dhi.image_id = di.id AND dhi.host_id = sqlc.arg(host_id)
JOIN (
    SELECT unnest(sqlc.arg(repositories)::text[]) AS repository,
           unnest(sqlc.arg(tags)::text[]) AS tag,
           unnest(sqlc.arg(image_ids)::text[]) AS image_id
) k ON di.repository = k.repository AND di.tag = k.tag AND di.image_id = k.image_id;

-- name: UpsertDockerImage :one
INSERT INTO docker_images (id, repository, tag, image_id, digest, size_bytes, source, created_at, last_checked, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(repository), sqlc.arg(tag), sqlc.arg(image_id), sqlc.narg(digest), sqlc.narg(size_bytes), sqlc.arg(source),
        COALESCE(sqlc.narg(created_at)::timestamp(3), sqlc.arg(last_checked)::timestamp(3)), sqlc.arg(last_checked), sqlc.arg(updated_at))
ON CONFLICT (repository, tag, image_id) DO UPDATE SET
  digest = COALESCE(EXCLUDED.digest, docker_images.digest),
  size_bytes = COALESCE(EXCLUDED.size_bytes, docker_images.size_bytes),
  source = EXCLUDED.source,
  created_at = COALESCE(sqlc.narg(created_at)::timestamp(3), docker_images.created_at),
  last_checked = EXCLUDED.last_checked,
  updated_at = EXCLUDED.updated_at
RETURNING id;

-- name: InsertDockerImageFromContainer :one
INSERT INTO docker_images (id, repository, tag, image_id, source, created_at, last_checked, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6, $6)
ON CONFLICT (repository, tag, image_id) DO UPDATE SET
  last_checked = EXCLUDED.last_checked
RETURNING id;

-- name: UpsertDockerHostImage :exec
INSERT INTO docker_host_images (host_id, image_id, last_seen)
VALUES ($1, $2, $3)
ON CONFLICT (host_id, image_id) DO UPDATE SET last_seen = EXCLUDED.last_seen;

-- name: DeleteStaleDockerHostImages :exec
DELETE FROM docker_host_images
WHERE host_id = $1 AND NOT (image_id = ANY(sqlc.arg(image_ids)::text[]));

-- name: ListDockerImageChecksByHost :many
SELECT image_id, status, first_seen_update_at FROM docker_image_checks WHERE host_id = $1;

-- name: UpsertDockerImageCheck :exec
INSERT INTO docker_image_checks (id, host_id, image_id, status, local_digest, remote_digest, registry, reason, checked_at, first_seen_update_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (host_id, image_id) DO UPDATE SET
  status = EXCLUDED.status,
  local_digest = EXCLUDED.local_digest,
  remote_digest = EXCLUDED.remote_digest,
  registry = EXCLUDED.registry,
  reason = EXCLUDED.reason,
  checked_at = EXCLUDED.checked_at,
  first_seen_update_at = EXCLUDED.first_seen_update_at,
  updated_at = EXCLUDED.updated_at;

-- name: DeleteStaleDockerImageChecks :exec
DELETE FROM docker_image_checks
WHERE host_id = $1 AND NOT (image_id = ANY(sqlc.arg(image_ids)::text[]));

-- name: GetImageCheckAggregatesByImageIDs :many
SELECT
    a.image_id,
    a.hosts_checked,
    a.hosts_update_available,
    a.hosts_unknown,
    a.hosts_up_to_date,
    a.hosts_local_build,
    a.hosts_pinned,
    a.hosts_skipped,
    l.checked_at,
    l.remote_digest,
    l.reason
FROM (
    SELECT
        image_id,
        COUNT(*)::int AS hosts_checked,
        COUNT(*) FILTER (WHERE status = 'update_available')::int AS hosts_update_available,
        COUNT(*) FILTER (WHERE status = 'unknown')::int AS hosts_unknown,
        COUNT(*) FILTER (WHERE status = 'up_to_date')::int AS hosts_up_to_date,
        COUNT(*) FILTER (WHERE status = 'local_build')::int AS hosts_local_build,
        COUNT(*) FILTER (WHERE status = 'pinned')::int AS hosts_pinned,
        COUNT(*) FILTER (WHERE status = 'skipped')::int AS hosts_skipped
    FROM docker_image_checks
    WHERE image_id = ANY(sqlc.arg(image_ids)::text[])
    GROUP BY image_id
) a
JOIN LATERAL (
    SELECT c.checked_at, c.remote_digest, c.reason
    FROM docker_image_checks c
    WHERE c.image_id = a.image_id
    ORDER BY c.checked_at DESC, c.host_id ASC
    LIMIT 1
) l ON true;

-- name: GetImageChecksByImageID :many
SELECT c.host_id, c.status, c.local_digest, c.remote_digest, c.reason, c.registry, c.checked_at,
       h.friendly_name, h.hostname
FROM docker_image_checks c
JOIN hosts h ON h.id = c.host_id
WHERE c.image_id = $1
ORDER BY h.friendly_name ASC;

-- name: GetImageChecksByHostID :many
SELECT image_id, status, local_digest, remote_digest, reason, registry, checked_at
FROM docker_image_checks
WHERE host_id = $1;

-- name: GetImageCheckStatusesByImageIDs :many
SELECT host_id, image_id, status FROM docker_image_checks WHERE image_id = ANY($1::text[]);

-- name: UpsertDockerContainer :exec
INSERT INTO docker_containers (id, host_id, container_id, name, image_id, image_name, image_tag, status, state, ports, labels, created_at, started_at, updated_at, last_checked, image_stale, restart_count, image_digest)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
ON CONFLICT (host_id, container_id) DO UPDATE SET
  image_stale = EXCLUDED.image_stale,
  image_digest = EXCLUDED.image_digest,
  restart_count = EXCLUDED.restart_count,
  name = EXCLUDED.name,
  image_id = EXCLUDED.image_id,
  image_name = EXCLUDED.image_name,
  image_tag = EXCLUDED.image_tag,
  status = EXCLUDED.status,
  state = EXCLUDED.state,
  ports = EXCLUDED.ports,
  labels = EXCLUDED.labels,
  started_at = EXCLUDED.started_at,
  updated_at = EXCLUDED.updated_at,
  last_checked = EXCLUDED.last_checked;

-- name: UpsertDockerVolume :exec
INSERT INTO docker_volumes (id, host_id, volume_id, name, driver, mountpoint, renderer, scope, labels, options, size_bytes, ref_count, created_at, updated_at, last_checked)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (host_id, volume_id) DO UPDATE SET
  name = EXCLUDED.name,
  driver = EXCLUDED.driver,
  mountpoint = EXCLUDED.mountpoint,
  renderer = EXCLUDED.renderer,
  scope = EXCLUDED.scope,
  labels = EXCLUDED.labels,
  options = EXCLUDED.options,
  size_bytes = EXCLUDED.size_bytes,
  ref_count = EXCLUDED.ref_count,
  updated_at = EXCLUDED.updated_at,
  last_checked = EXCLUDED.last_checked;

-- name: UpsertDockerNetwork :exec
INSERT INTO docker_networks (id, host_id, network_id, name, driver, scope, ipv6_enabled, internal, attachable, ingress, config_only, labels, ipam, container_count, created_at, updated_at, last_checked)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (host_id, network_id) DO UPDATE SET
  name = EXCLUDED.name,
  driver = EXCLUDED.driver,
  scope = EXCLUDED.scope,
  ipv6_enabled = EXCLUDED.ipv6_enabled,
  internal = EXCLUDED.internal,
  attachable = EXCLUDED.attachable,
  ingress = EXCLUDED.ingress,
  config_only = EXCLUDED.config_only,
  labels = EXCLUDED.labels,
  ipam = EXCLUDED.ipam,
  container_count = EXCLUDED.container_count,
  updated_at = EXCLUDED.updated_at,
  last_checked = EXCLUDED.last_checked;
