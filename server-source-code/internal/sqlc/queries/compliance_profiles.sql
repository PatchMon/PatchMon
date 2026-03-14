-- name: ListComplianceProfiles :many
SELECT id, name, type, os_family, version, description, created_at, updated_at
FROM compliance_profiles
ORDER BY name ASC;

-- name: GetComplianceProfileByName :one
SELECT id, name, type, os_family, version, description, created_at, updated_at
FROM compliance_profiles
WHERE name = $1;

-- name: GetComplianceProfileByID :one
SELECT id, name, type, os_family, version, description, created_at, updated_at
FROM compliance_profiles
WHERE id = $1;

-- name: GetFirstComplianceProfileByType :one
SELECT id, name, type, os_family, version, description, created_at, updated_at
FROM compliance_profiles
WHERE type = $1
ORDER BY name ASC
LIMIT 1;

-- name: CreateComplianceProfile :one
INSERT INTO compliance_profiles (id, name, type, os_family, version, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
RETURNING id, name, type, os_family, version, description, created_at, updated_at;
