-- Dashboard stats
-- name: CountDockerHosts :one
SELECT COUNT(DISTINCT host_id)::int FROM docker_containers;

-- name: GetDockerDashboardStats :one
SELECT
    (SELECT COUNT(*)::int FROM docker_containers),
    (SELECT COUNT(*)::int FROM docker_containers WHERE status = 'running'),
    (SELECT COUNT(*)::int FROM docker_images),
    (SELECT COUNT(*)::int FROM docker_image_updates);

-- name: GetContainersByStatus :many
SELECT status, COUNT(*)::int as count FROM docker_containers GROUP BY status;

-- name: GetImagesBySource :many
SELECT source, COUNT(*)::int as count FROM docker_images GROUP BY source;

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
WHERE NOT EXISTS (SELECT 1 FROM docker_containers dc WHERE dc.image_id = di.id);

-- name: DeleteContainersByIDs :exec
DELETE FROM docker_containers WHERE id = ANY($1::text[]);

-- name: DeleteImagesByIDs :exec
DELETE FROM docker_images WHERE id = ANY($1::text[]);

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
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountImages :one
SELECT COUNT(*)::int FROM docker_images
WHERE (sqlc.narg('source')::text IS NULL OR source = sqlc.narg('source'))
AND (sqlc.narg('search')::text IS NULL OR repository ILIKE '%' || sqlc.narg('search') || '%' OR tag ILIKE '%' || sqlc.narg('search') || '%');

-- name: GetContainerCountsByImageIDs :many
SELECT image_id, COUNT(*)::int as cnt FROM docker_containers WHERE image_id = ANY($1::text[]) GROUP BY image_id;

-- name: GetUpdateCountsByImageIDs :many
SELECT image_id, COUNT(*)::int as cnt FROM docker_image_updates WHERE image_id = ANY($1::text[]) GROUP BY image_id;

-- name: GetContainersByImageIDAll :many
SELECT * FROM docker_containers WHERE image_id = $1 LIMIT 100;

-- name: CountContainersByImageID :one
SELECT COUNT(*)::int FROM docker_containers WHERE image_id = $1;

-- name: DeleteImageUpdatesByImageID :exec
DELETE FROM docker_image_updates WHERE image_id = $1;

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
    COUNT(DISTINCT image_id) FILTER (WHERE image_id IS NOT NULL)::int
FROM docker_containers
WHERE host_id = $1;

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

-- name: GetNetworksByHostID :many
SELECT * FROM docker_networks WHERE host_id = $1 ORDER BY name ASC;

-- name: CountNetworksByHostID :one
SELECT COUNT(*)::int FROM docker_networks WHERE host_id = $1;

-- name: GetImageUpdatesByImageID :many
SELECT * FROM docker_image_updates WHERE image_id = $1 ORDER BY created_at DESC;

-- name: GetImageIDByRepositoryTagImageID :one
SELECT id FROM docker_images WHERE repository = $1 AND tag = $2 AND image_id = $3;

-- name: UpsertDockerImage :one
INSERT INTO docker_images (id, repository, tag, image_id, digest, size_bytes, source, created_at, last_checked, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (repository, tag, image_id) DO UPDATE SET
  digest = COALESCE(EXCLUDED.digest, docker_images.digest),
  size_bytes = COALESCE(EXCLUDED.size_bytes, docker_images.size_bytes),
  last_checked = EXCLUDED.last_checked,
  updated_at = EXCLUDED.updated_at
RETURNING id;

-- name: UpsertDockerContainer :exec
INSERT INTO docker_containers (id, host_id, container_id, name, image_id, image_name, image_tag, status, state, ports, labels, created_at, started_at, updated_at, last_checked)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (host_id, container_id) DO UPDATE SET
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

-- name: UpsertDockerImageUpdate :exec
INSERT INTO docker_image_updates (id, image_id, current_tag, available_tag, is_security_update, severity, changelog_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (image_id, available_tag) DO UPDATE SET
  severity = EXCLUDED.severity,
  changelog_url = EXCLUDED.changelog_url,
  updated_at = EXCLUDED.updated_at;
