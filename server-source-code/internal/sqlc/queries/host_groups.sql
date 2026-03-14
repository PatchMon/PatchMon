-- name: ListHostGroups :many
SELECT id, name, description, color, created_at, updated_at FROM host_groups ORDER BY name;

-- name: ListHostGroupsWithHostCount :many
SELECT hg.id, hg.name, hg.description, hg.color, hg.created_at, hg.updated_at,
       COUNT(hgm.host_id)::int AS host_count
FROM host_groups hg
LEFT JOIN host_group_memberships hgm ON hgm.host_group_id = hg.id
GROUP BY hg.id, hg.name, hg.description, hg.color, hg.created_at, hg.updated_at
ORDER BY hg.name;

-- name: GetHostGroupByID :one
SELECT * FROM host_groups WHERE id = $1;

-- name: CreateHostGroup :exec
INSERT INTO host_groups (id, name, description, color, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpdateHostGroup :exec
UPDATE host_groups SET name = $1, description = $2, color = $3, updated_at = $4 WHERE id = $5;

-- name: DeleteHostGroup :exec
DELETE FROM host_groups WHERE id = $1;

-- name: GetHostIDsByGroup :many
SELECT host_id FROM host_group_memberships WHERE host_group_id = $1;

-- name: GetHostIDsByGroupIDs :many
SELECT DISTINCT host_id FROM host_group_memberships WHERE host_group_id = ANY($1::text[]);

-- name: GetHostGroupsForHosts :many
SELECT hgm.host_id, hg.id, hg.name, hg.description, hg.color, hg.created_at, hg.updated_at
FROM host_group_memberships hgm
JOIN host_groups hg ON hg.id = hgm.host_group_id
WHERE hgm.host_id = ANY($1::text[])
ORDER BY hgm.host_id, hg.name;

-- name: DeleteHostGroupMemberships :exec
DELETE FROM host_group_memberships WHERE host_id = $1;

-- name: InsertHostGroupMembership :exec
INSERT INTO host_group_memberships (id, host_id, host_group_id) VALUES ($1, $2, $3);
