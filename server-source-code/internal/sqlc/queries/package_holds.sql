-- name: ListEffectivePackageHoldsForHost :many
SELECT DISTINCT h.package_name
FROM host_group_package_holds h
JOIN host_group_memberships m ON m.host_group_id = h.host_group_id
WHERE m.host_id = $1
ORDER BY h.package_name;

-- name: ListHostGroupPackageHolds :many
SELECT package_name FROM host_group_package_holds
WHERE host_group_id = $1
ORDER BY package_name;

-- name: ListAllHostGroupPackageHolds :many
SELECT host_group_id, package_name
FROM host_group_package_holds
ORDER BY host_group_id, package_name;

-- name: DeleteHostGroupPackageHolds :exec
DELETE FROM host_group_package_holds WHERE host_group_id = $1;

-- name: InsertHostGroupPackageHold :exec
INSERT INTO host_group_package_holds (id, host_group_id, package_name, created_at)
VALUES ($1, $2, $3, NOW());

-- name: ListSecurityUpdatePackageNamesByHost :many
SELECT p.name
FROM host_packages hp
JOIN packages p ON p.id = hp.package_id
WHERE hp.host_id = $1
  AND hp.needs_update = true
  AND hp.is_security_update = true
ORDER BY p.name;

-- name: InsertPatchRunExclude :exec
INSERT INTO patch_run_excludes (patch_run_id, package_name) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListPatchRunExcludes :many
SELECT package_name FROM patch_run_excludes
WHERE patch_run_id = $1
ORDER BY package_name;
