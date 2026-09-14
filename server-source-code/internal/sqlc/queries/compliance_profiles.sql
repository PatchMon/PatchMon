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

-- name: UpsertComplianceProfile :one
-- DO UPDATE rather than DO NOTHING so the row is always returned on conflict.
INSERT INTO compliance_profiles (id, name, type, os_family, version, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
-- Deliberately does not touch type: an existing profile keeps the type it was
-- created with. Overwriting it flips which scanner toggle gates it in SubmitScan.
ON CONFLICT (name) DO UPDATE SET
    updated_at = NOW()
RETURNING id, name, type, os_family, version, description, created_at, updated_at;

-- name: CreateComplianceProfile :one
INSERT INTO compliance_profiles (id, name, type, os_family, version, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
RETURNING id, name, type, os_family, version, description, created_at, updated_at;
